// Package reelmedia is the optional deep-media rung for reel place extraction:
// it downloads a social video with yt-dlp and samples frames with ffmpeg,
// returning JPEG bytes. It knows nothing about AI or places. All binaries are
// user-installed and optional; absence is handled by the caller (Detect).
package reelmedia

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Mode is the resolved REEL_MEDIA ladder ceiling.
type Mode int

const (
	ModeOff Mode = iota
	ModeThumbnail
	ModeVideo
)

// String renders Mode for logging.
func (m Mode) String() string {
	switch m {
	case ModeOff:
		return "off"
	case ModeThumbnail:
		return "thumbnail"
	case ModeVideo:
		return "video"
	default:
		return "unknown"
	}
}

// ModeFromEnv reads REEL_MEDIA (off|thumbnail|video), defaulting to thumbnail
// (preserving Phase 2 behaviour). Unknown values fall back to thumbnail.
func ModeFromEnv() Mode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("REEL_MEDIA"))) {
	case "off":
		return ModeOff
	case "video":
		return ModeVideo
	default:
		return ModeThumbnail
	}
}

const (
	maxFramesDefault = 8
	maxFileSize      = "50M"
	frameLongEdge    = 768
	extractTimeout   = 60 * time.Second
)

type runFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func execRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Extractor holds resolved binary paths and limits. Construct via Detect.
type Extractor struct {
	ytDLP, ffmpeg, ffprobe string
	maxFrames              int
	run                    runFunc
	tempBase               string // "" = os.TempDir(); overridden in tests
}

// Detect resolves yt-dlp, ffmpeg, and ffprobe on PATH. ok=false (nil Extractor)
// means deep media is unavailable and the caller should downgrade to thumbnail.
func Detect() (*Extractor, bool) {
	yt, err1 := exec.LookPath("yt-dlp")
	ff, err2 := exec.LookPath("ffmpeg")
	fp, err3 := exec.LookPath("ffprobe")
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, false
	}
	return &Extractor{ytDLP: yt, ffmpeg: ff, ffprobe: fp, maxFrames: maxFramesDefault, run: execRun}, true
}

// Frames downloads the video at url and returns up to maxFrames JPEG frames
// sampled evenly across it. Best-effort: any failure returns an error the
// caller logs and ignores. The working dir is always removed.
func (e *Extractor) Frames(ctx context.Context, url string) (frames [][]byte, err error) {
	ctx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()

	dir, err := os.MkdirTemp(e.tempBase, "reelmedia-*")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	video := filepath.Join(dir, "v.mp4")
	if _, err := e.run(ctx, e.ytDLP,
		"--no-playlist", "--no-warnings", "--max-filesize", maxFileSize,
		"--socket-timeout", "20", "-f", "mp4/best[ext=mp4]/best",
		"-o", video, "--", url,
	); err != nil {
		return nil, fmt.Errorf("yt-dlp: %w", err)
	}
	if _, statErr := os.Stat(video); statErr != nil {
		return nil, fmt.Errorf("yt-dlp produced no file: %w", statErr)
	}

	dur := e.probeDuration(ctx, video) // seconds; 0 on failure
	fps := "1"
	if dur > 0 {
		fps = strconv.FormatFloat(float64(e.maxFrames)/dur, 'f', 4, 64)
	}
	pattern := filepath.Join(dir, "frame_%03d.jpg")
	if _, err := e.run(ctx, e.ffmpeg,
		"-hide_banner", "-loglevel", "error", "-i", video,
		"-vf", fmt.Sprintf("fps=%s,scale=%d:%d:force_original_aspect_ratio=decrease", fps, frameLongEdge, frameLongEdge),
		"-frames:v", strconv.Itoa(e.maxFrames), "-q:v", "4", pattern,
	); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "frame_*.jpg"))
	sort.Strings(matches)
	for _, m := range matches {
		b, rerr := os.ReadFile(m)
		if rerr == nil && len(b) > 0 {
			frames = append(frames, b)
		}
	}
	return frames, nil
}

// probeDuration returns the clip length in seconds, or 0 if ffprobe fails.
func (e *Extractor) probeDuration(ctx context.Context, video string) float64 {
	out, err := e.run(ctx, e.ffprobe,
		"-v", "error", "-show_entries", "format=duration",
		"-of", "default=nw=1:nk=1", video,
	)
	if err != nil {
		return 0
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}
