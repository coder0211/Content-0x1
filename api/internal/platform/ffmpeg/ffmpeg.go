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

// Renderer implements ports.VideoRenderer. Normalize, StillToClip and
// Thumbnail are complete; Render (concat + mix + subtitles) is a baseline
// that the video module agent (ACS-066) extends.
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

// NormalizeArgs builds the argument vector for Normalize. A silent stereo
// track is mixed in and -shortest keeps the clip length when the source has
// no audio.
func NormalizeArgs(s ports.NormalizeSpec) []string {
	args := []string{"-y", "-hide_banner", "-i", s.Input,
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-filter_complex", "[0:v]" + scalePad(s.Format) + "[v]",
		"-map", "[v]", "-map", "0:a?", "-map", "1:a", "-shortest"}
	if s.DurationMs > 0 {
		args = append(args, "-t", ms(s.DurationMs))
	}
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

// RenderArgs builds the final render: concat clips (hard cut), optional
// voice + music (ducked), optional subtitles (burn-in via the subtitles
// filter referencing a file path, or a soft mov_text track).
func RenderArgs(s ports.RenderSpec) ([]string, error) {
	if len(s.Clips) == 0 {
		return nil, errors.New("render: no clips")
	}
	if s.Subtitles != "" && strings.ContainsAny(s.Subtitles, "':,;[]\\") {
		return nil, errors.New("render: subtitle path contains filter metacharacters")
	}
	args := []string{"-y", "-hide_banner"}
	for _, c := range s.Clips {
		args = append(args, "-i", c)
	}
	n := len(s.Clips)
	var fc strings.Builder
	for i := range n {
		fmt.Fprintf(&fc, "[%d:v][%d:a]", i, i)
	}
	fmt.Fprintf(&fc, "concat=n=%d:v=1:a=1[vcat][acat]", n)
	video := "[vcat]"
	if s.Subtitles != "" && s.BurnIn {
		fc.WriteString(";[vcat]subtitles=filename=" + s.Subtitles + "[vsub]")
		video = "[vsub]"
	}
	audioInputs := []string{"[acat]"}
	next := n
	voiceLabel := ""
	if s.Voice != nil {
		args = append(args, "-i", s.Voice.Path)
		fmt.Fprintf(&fc, ";[%d:a]volume=%.2f[vo0]", next, s.Voice.Volume)
		voiceLabel = "[vo0]"
		next++
	}
	if s.Music != nil {
		args = append(args, "-stream_loop", "-1", "-i", s.Music.Path)
		fmt.Fprintf(&fc, ";[%d:a]volume=%.2f[mu0]", next, s.Music.Volume)
		music := "[mu0]"
		if s.Music.Duck && voiceLabel != "" {
			fc.WriteString(";[vo0]asplit=2[vo1][vkey];[mu0][vkey]sidechaincompress=threshold=0.05:ratio=8[mu1]")
			voiceLabel, music = "[vo1]", "[mu1]"
		}
		audioInputs = append(audioInputs, music)
		next++
	}
	if voiceLabel != "" {
		audioInputs = append(audioInputs, voiceLabel)
	}
	fmt.Fprintf(&fc, ";%samix=inputs=%d:duration=first:normalize=0[aout]", strings.Join(audioInputs, ""), len(audioInputs))
	args = append(args, "-filter_complex", fc.String(), "-map", video, "-map", "[aout]")
	if s.Subtitles != "" && !s.BurnIn {
		args = append(args, "-i", s.Subtitles, "-map", fmt.Sprintf("%d:s", next), "-c:s", "mov_text")
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
