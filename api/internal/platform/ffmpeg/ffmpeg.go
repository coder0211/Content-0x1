// Package ffmpeg implements ports.MediaProber and ports.VideoRenderer by
// executing ffprobe/ffmpeg with argument vectors — never through a shell, and
// never with user text inside filter strings (SPEC-02 §5.6).
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/be/acs/api/internal/ports"
)

// Runner executes a binary; swapped out in tests.
type Runner func(ctx context.Context, bin string, args ...string) (stdout []byte, err error)

// ExecRunner runs bin via os/exec, capturing stderr into the error.
func ExecRunner(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if len(msg) > 2000 {
			msg = msg[len(msg)-2000:]
		}
		return nil, fmt.Errorf("%s: %w: %s", bin, err, strings.TrimSpace(msg))
	}
	return stdout.Bytes(), nil
}

// Prober implements ports.MediaProber with ffprobe.
type Prober struct {
	Bin string
	Run Runner
}

// NewProber returns a Prober for the ffprobe binary.
func NewProber(bin string) *Prober { return &Prober{Bin: bin, Run: ExecRunner} }

var _ ports.MediaProber = (*Prober)(nil)

// ProbeArgs builds the ffprobe argument vector for src.
func ProbeArgs(src string) []string {
	return []string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams", "--", src}
}

// Probe runs ffprobe on src (local path or internal signed URL).
func (p *Prober) Probe(ctx context.Context, src string) (ports.MediaInfo, error) {
	if strings.HasPrefix(src, "-") {
		return ports.MediaInfo{}, errors.New("ffprobe: invalid source")
	}
	out, err := p.Run(ctx, p.Bin, ProbeArgs(src)...)
	if err != nil {
		return ports.MediaInfo{}, err
	}
	return ParseProbe(out)
}

type probeJSON struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		Index        int    `json:"index"`
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		PixFmt       string `json:"pix_fmt"`
		AvgFrameRate string `json:"avg_frame_rate"`
		SampleRate   string `json:"sample_rate"`
		Channels     int    `json:"channels"`
	} `json:"streams"`
}

// ParseProbe decodes ffprobe JSON output.
func ParseProbe(out []byte) (ports.MediaInfo, error) {
	var pj probeJSON
	if err := json.Unmarshal(out, &pj); err != nil {
		return ports.MediaInfo{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	info := ports.MediaInfo{FormatName: pj.Format.FormatName}
	if d, err := strconv.ParseFloat(pj.Format.Duration, 64); err == nil {
		info.Duration = time.Duration(d * float64(time.Second))
	}
	info.SizeBytes, _ = strconv.ParseInt(pj.Format.Size, 10, 64)
	info.BitRate, _ = strconv.ParseInt(pj.Format.BitRate, 10, 64)
	for _, s := range pj.Streams {
		sr, _ := strconv.Atoi(s.SampleRate)
		info.Streams = append(info.Streams, ports.StreamInfo{
			Index: s.Index, Type: s.CodecType, Codec: s.CodecName,
			Width: s.Width, Height: s.Height, PixFmt: s.PixFmt,
			FPS: parseRate(s.AvgFrameRate), SampleRate: sr, Channels: s.Channels,
		})
	}
	return info, nil
}

func parseRate(r string) float64 {
	num, den, ok := strings.Cut(r, "/")
	if !ok {
		f, _ := strconv.ParseFloat(r, 64)
		return f
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 {
		return 0
	}
	return n / d
}

// Renderer implements ports.VideoRenderer (ACS-066): normalize, Ken Burns
// still clips, the final render (cross-fades, voice/music mix with ducking,
// burn-in or soft subtitles) and thumbnails.
type Renderer struct {
	Bin string
	Run Runner
}

// NewRenderer returns a Renderer for the ffmpeg binary.
func NewRenderer(bin string) *Renderer { return &Renderer{Bin: bin, Run: ExecRunner} }

var _ ports.VideoRenderer = (*Renderer)(nil)

func ms(d int) string { return strconv.FormatFloat(float64(d)/1000, 'f', 3, 64) }

// scalePad fits any input into the canonical frame without distortion.
func scalePad(f ports.OutputFormat) string {
	w, h := strconv.Itoa(f.Width), strconv.Itoa(f.Height)
	return "scale=" + w + ":" + h + ":force_original_aspect_ratio=decrease,pad=" + w + ":" + h +
		":(ow-iw)/2:(oh-ih)/2,setsar=1,fps=" + strconv.Itoa(f.FPS) + ",format=yuv420p"
}

var encodeArgs = []string{
	"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
	"-c:a", "aac", "-b:a", "160k", "-ar", "48000", "-ac", "2", "-movflags", "+faststart",
}

// NormalizeArgs builds the argument vector for Normalize. The output always
// has exactly one silent stereo AAC track (provider clip audio is dropped:
// the soundtrack is the voice-over + music mix of the final render), so
// every normalized clip has identical stream layout for concat/xfade. With
// DurationMs the clip is padded (last frame cloned) or trimmed to length.
func NormalizeArgs(s ports.NormalizeSpec) []string {
	vf := "[0:v]" + scalePad(s.Format)
	if s.DurationMs > 0 {
		vf += ",tpad=stop_mode=clone:stop_duration=" + ms(s.DurationMs)
	}
	args := []string{"-y", "-hide_banner", "-i", s.Input,
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-filter_complex", vf + "[v]",
		"-map", "[v]", "-map", "1:a", "-shortest"}
	if s.DurationMs > 0 {
		args = append(args, "-t", ms(s.DurationMs))
	}
	args = append(args, "-r", strconv.Itoa(s.Format.FPS))
	args = append(args, encodeArgs...)
	return append(args, s.Output)
}

// Normalize converts a clip to the canonical format.
func (r *Renderer) Normalize(ctx context.Context, s ports.NormalizeSpec) error {
	_, err := r.Run(ctx, r.Bin, NormalizeArgs(s)...)
	return err
}

// StillToClipArgs builds a slow zoom-in clip from a still image.
func StillToClipArgs(s ports.StillClipSpec) []string {
	frames := s.DurationMs * s.Format.FPS / 1000
	w, h := strconv.Itoa(s.Format.Width), strconv.Itoa(s.Format.Height)
	zoom := "scale=" + strconv.Itoa(s.Format.Width*2) + ":" + strconv.Itoa(s.Format.Height*2) +
		":force_original_aspect_ratio=increase,crop=" + strconv.Itoa(s.Format.Width*2) + ":" + strconv.Itoa(s.Format.Height*2) +
		",zoompan=z='min(zoom+0.0008,1.12)':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=" + strconv.Itoa(frames) +
		":s=" + w + "x" + h + ":fps=" + strconv.Itoa(s.Format.FPS) + ",format=yuv420p"
	args := []string{"-y", "-hide_banner", "-loop", "1", "-i", s.Image,
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-filter_complex", "[0:v]" + zoom + "[v]", "-map", "[v]", "-map", "1:a",
		"-t", ms(s.DurationMs)}
	args = append(args, encodeArgs...)
	return append(args, s.Output)
}

// StillToClip renders a Ken Burns clip from an image.
func (r *Renderer) StillToClip(ctx context.Context, s ports.StillClipSpec) error {
	_, err := r.Run(ctx, r.Bin, StillToClipArgs(s)...)
	return err
}

// filterMeta are characters with a meaning in filtergraph syntax; paths
// containing them are refused instead of escaped (SPEC-02 §5.6).
const filterMeta = "':,;[]\\=\n\r"

// RenderArgs builds the final render:
//   - clips joined by cross-fades (xfade + acrossfade) when Transition is
//     "fade" and ClipDurationsMs is set, else concatenated (hard cut);
//   - subtitles burned in via the subtitles filter referencing a FILE path
//     (user text never enters the filtergraph), or muxed as a mov_text track;
//   - voice-over (one track or timeline segments: atrim + adelay + amix) and
//     looped music, ducked under the voice with sidechaincompress;
//   - H.264/AAC 48 kHz, yuv420p, fixed fps, MP4 faststart.
func RenderArgs(s ports.RenderSpec) ([]string, error) {
	if len(s.Clips) == 0 {
		return nil, errors.New("render: no clips")
	}
	for _, p := range []string{s.Subtitles, s.FontsDir} {
		if p != "" && strings.ContainsAny(p, filterMeta) {
			return nil, errors.New("render: subtitle path contains filter metacharacters")
		}
	}
	n := len(s.Clips)
	fade := s.Transition == "fade" && n > 1 && len(s.ClipDurationsMs) == n && s.TransitionMs > 0
	if fade {
		for _, d := range s.ClipDurationsMs {
			if d <= s.TransitionMs {
				return nil, errors.New("render: clip shorter than transition")
			}
		}
	}
	args := []string{"-y", "-hide_banner"}
	input := 0
	addInput := func(extra ...string) int {
		args = append(args, extra...)
		input++
		return input - 1
	}
	for _, c := range s.Clips {
		addInput("-i", c)
	}
	var fc []string
	video, clipAudio := "[vcat]", "[acat]"
	switch {
	case fade:
		t := ms(s.TransitionMs)
		prevV, prevA := "[0:v]", "[0:a]"
		offset := 0
		for i := 1; i < n; i++ {
			offset += s.ClipDurationsMs[i-1] - s.TransitionMs
			v, a := fmt.Sprintf("[xv%d]", i), fmt.Sprintf("[xa%d]", i)
			fc = append(fc,
				fmt.Sprintf("%s[%d:v]xfade=transition=fade:duration=%s:offset=%s%s", prevV, i, t, ms(offset), v),
				fmt.Sprintf("%s[%d:a]acrossfade=d=%s%s", prevA, i, t, a))
			prevV, prevA = v, a
		}
		video, clipAudio = prevV, prevA
	default:
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "[%d:v][%d:a]", i, i)
		}
		fmt.Fprintf(&b, "concat=n=%d:v=1:a=1[vcat][acat]", n)
		fc = append(fc, b.String())
	}
	if s.Subtitles != "" && s.BurnIn {
		f := "subtitles=filename=" + s.Subtitles
		if s.FontsDir != "" {
			f += ":fontsdir=" + s.FontsDir
		}
		fc = append(fc, video+f+"[vsub]")
		video = "[vsub]"
	}
	fc = append(fc, fmt.Sprintf("%sformat=yuv420p[vout]", video))
	video = "[vout]"

	audio := []string{clipAudio}
	voice := ""
	voiceVol := 1.0
	if s.Voice != nil && s.Voice.Volume > 0 {
		voiceVol = s.Voice.Volume
	}
	switch {
	case len(s.VoiceSegments) > 0:
		var labels []string
		for i, seg := range s.VoiceSegments {
			idx := addInput("-i", seg.Path)
			chain := fmt.Sprintf("[%d:a]aformat=sample_rates=48000:channel_layouts=stereo", idx)
			if seg.MaxMs > 0 {
				chain += ",atrim=0:" + ms(seg.MaxMs)
			}
			chain += fmt.Sprintf(",asetpts=PTS-STARTPTS,adelay=%d|%d", max(seg.StartMs, 0), max(seg.StartMs, 0))
			l := fmt.Sprintf("[vs%d]", i)
			fc = append(fc, chain+l)
			labels = append(labels, l)
		}
		if len(labels) == 1 {
			fc = append(fc, fmt.Sprintf("%svolume=%.2f[vo0]", labels[0], voiceVol))
		} else {
			fc = append(fc, fmt.Sprintf("%samix=inputs=%d:duration=longest:normalize=0,volume=%.2f[vo0]", strings.Join(labels, ""), len(labels), voiceVol))
		}
		voice = "[vo0]"
	case s.Voice != nil:
		idx := addInput("-i", s.Voice.Path)
		fc = append(fc, fmt.Sprintf("[%d:a]aformat=sample_rates=48000:channel_layouts=stereo,volume=%.2f[vo0]", idx, voiceVol))
		voice = "[vo0]"
	}
	if s.Music != nil {
		idx := addInput("-stream_loop", "-1", "-i", s.Music.Path)
		fc = append(fc, fmt.Sprintf("[%d:a]aformat=sample_rates=48000:channel_layouts=stereo,volume=%.2f[mu0]", idx, s.Music.Volume))
		music := "[mu0]"
		if s.Music.Duck && voice != "" {
			fc = append(fc, voice+"asplit=2[vo1][vkey]", "[mu0][vkey]sidechaincompress=threshold=0.05:ratio=8:attack=20:release=300[mu1]")
			voice, music = "[vo1]", "[mu1]"
		}
		audio = append(audio, music)
	}
	if voice != "" {
		audio = append(audio, voice)
	}
	if len(audio) == 1 {
		fc = append(fc, audio[0]+"anull[aout]")
	} else {
		// duration=first: the clip track defines the length (music loops).
		fc = append(fc, fmt.Sprintf("%samix=inputs=%d:duration=first:normalize=0[aout]", strings.Join(audio, ""), len(audio)))
	}
	subIdx := -1
	if s.Subtitles != "" && !s.BurnIn {
		subIdx = addInput("-i", s.Subtitles)
	}
	args = append(args, "-filter_complex", strings.Join(fc, ";"), "-map", video, "-map", "[aout]")
	if subIdx >= 0 {
		args = append(args, "-map", fmt.Sprintf("%d:s", subIdx), "-c:s", "mov_text", "-metadata:s:s:0", "language=vie")
	}
	if s.Format.FPS > 0 {
		args = append(args, "-r", strconv.Itoa(s.Format.FPS))
	}
	args = append(args, encodeArgs...)
	return append(args, s.Output), nil
}

// Render produces the final MP4 and probes its duration.
func (r *Renderer) Render(ctx context.Context, s ports.RenderSpec) (ports.RenderResult, error) {
	args, err := RenderArgs(s)
	if err != nil {
		return ports.RenderResult{}, err
	}
	if _, err := r.Run(ctx, r.Bin, args...); err != nil {
		return ports.RenderResult{}, err
	}
	return ports.RenderResult{Output: s.Output}, nil
}

// ThumbnailArgs extracts one JPEG frame at `at`.
func ThumbnailArgs(input, output string, at time.Duration) []string {
	return []string{"-y", "-hide_banner", "-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64), "-i", input,
		"-frames:v", "1", "-q:v", "3", output}
}

// Thumbnail writes a JPEG frame of input at `at`.
func (r *Renderer) Thumbnail(ctx context.Context, input, output string, at time.Duration) error {
	_, err := r.Run(ctx, r.Bin, ThumbnailArgs(input, output, at)...)
	return err
}
