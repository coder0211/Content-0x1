package ports

import (
	"context"
	"time"
)

// StreamInfo describes one stream reported by ffprobe.
type StreamInfo struct {
	Index      int
	Type       string // video | audio | subtitle | data
	Codec      string
	Width      int
	Height     int
	FPS        float64
	PixFmt     string
	SampleRate int
	Channels   int
}

// MediaInfo is the subset of ffprobe output the system relies on.
type MediaInfo struct {
	FormatName string
	Duration   time.Duration
	SizeBytes  int64
	BitRate    int64
	Streams    []StreamInfo
}

// Video returns the first video stream, if any.
func (m MediaInfo) Video() (StreamInfo, bool) { return m.first("video") }

// Audio returns the first audio stream, if any.
func (m MediaInfo) Audio() (StreamInfo, bool) { return m.first("audio") }

func (m MediaInfo) first(t string) (StreamInfo, bool) {
	for _, s := range m.Streams {
		if s.Type == t {
			return s, true
		}
	}
	return StreamInfo{}, false
}

// MediaProber inspects media. src is a local path or an internal signed URL.
type MediaProber interface {
	Probe(ctx context.Context, src string) (MediaInfo, error)
}

// OutputFormat is the canonical render format (OQ-07): H.264/AAC 48 kHz,
// yuv420p, 30 fps.
type OutputFormat struct {
	Width  int
	Height int
	FPS    int
}

// NormalizeSpec converts a provider clip into the canonical format, adding a
// silent audio track when missing.
type NormalizeSpec struct {
	Input      string
	Output     string
	Format     OutputFormat
	DurationMs int // trim/pad to this length; 0 keeps the source length
}

// StillClipSpec turns a still image into a clip with a slow zoom ("kenburns"
// fallback video provider, OQ-01).
type StillClipSpec struct {
	Image      string
	Output     string
	Format     OutputFormat
	DurationMs int
}

// AudioTrack is an audio input to the final mix.
type AudioTrack struct {
	Path   string
	Volume float64 // 1.0 = unchanged
	// Duck lowers this track while the voice-over plays (music only).
	Duck bool
}

// RenderSpec describes the final MP4. All inputs are local files inside
// WorkDir; user text only ever reaches FFmpeg through subtitle files.
type RenderSpec struct {
	WorkDir      string
	Clips        []string // normalized clips in order
	Transition   string   // "" (hard cut) | "fade"
	TransitionMs int
	Format       OutputFormat
	Voice        *AudioTrack
	Music        *AudioTrack
	Subtitles    string // path to .srt/.ass; "" for none
	BurnIn       bool   // burn subtitles into frames instead of a soft track
	Output       string
	// ClipDurationsMs are the clip lengths in order; required for
	// cross-fade transitions (xfade offsets). Empty = hard cuts.
	ClipDurationsMs []int
	// VoiceSegments place several voice-over files on the timeline (used
	// instead of Voice when set); volume comes from Voice when non-nil.
	VoiceSegments []VoiceSegment
	// FontsDir is passed to the subtitles filter for burn-in (fonts with
	// Vietnamese glyphs); "" uses fontconfig defaults.
	FontsDir string
}

// VoiceSegment is one voice-over file starting at StartMs on the final
// timeline, trimmed to MaxMs when > 0.
type VoiceSegment struct {
	Path    string
	StartMs int
	MaxMs   int
}

// RenderResult describes the rendered output.
type RenderResult struct {
	Output   string
	Duration time.Duration
}

// VideoRenderer wraps FFmpeg. Implementations exec FFmpeg with an argument
// vector, never through a shell.
type VideoRenderer interface {
	Normalize(ctx context.Context, spec NormalizeSpec) error
	StillToClip(ctx context.Context, spec StillClipSpec) error
	Render(ctx context.Context, spec RenderSpec) (RenderResult, error)
	Thumbnail(ctx context.Context, input, output string, at time.Duration) error
}
