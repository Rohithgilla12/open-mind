package reelmedia

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestModeFromEnv(t *testing.T) {
	cases := map[string]Mode{"": ModeThumbnail, "thumbnail": ModeThumbnail, "off": ModeOff, "video": ModeVideo, "bogus": ModeThumbnail}
	for in, want := range cases {
		t.Setenv("REEL_MEDIA", in)
		if got := ModeFromEnv(); got != want {
			t.Errorf("ModeFromEnv(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestModeString(t *testing.T) {
	cases := map[Mode]string{ModeOff: "off", ModeThumbnail: "thumbnail", ModeVideo: "video"}
	for mode, want := range cases {
		if got := mode.String(); got != want {
			t.Errorf("Mode(%d).String() = %q, want %q", mode, got, want)
		}
	}
}

// fakeRunner records args and writes the output files each stage expects.
type fakeRunner struct{ calls [][]string }

func (r *fakeRunner) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	switch {
	case strings.Contains(name, "yt-dlp"):
		// last arg is the URL; the -o value is the output path.
		out := argValue(args, "-o")
		_ = os.WriteFile(out, []byte("fakevideo"), 0o600)
	case strings.Contains(name, "ffprobe"):
		return []byte("12.5\n"), nil
	case strings.Contains(name, "ffmpeg"):
		// output pattern is the last arg, e.g. /dir/frame_%03d.jpg
		pattern := args[len(args)-1]
		for i := 1; i <= 3; i++ {
			_ = os.WriteFile(strings.Replace(pattern, "%03d", pad3(i), 1), []byte{0xFF, 0xD8, byte(i)}, 0o600)
		}
	}
	return nil, nil
}

func TestFramesHappyPath(t *testing.T) {
	dir := t.TempDir()
	r := &fakeRunner{}
	e := &Extractor{ytDLP: "yt-dlp", ffmpeg: "ffmpeg", ffprobe: "ffprobe", maxFrames: 8, run: r.run, tempBase: dir}
	frames, err := e.Frames(context.Background(), "https://instagram.com/reel/abc")
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("want 3 frames, got %d", len(frames))
	}
	// yt-dlp got the URL and a size cap; ffmpeg got an fps filter.
	joined := ""
	for _, c := range r.calls {
		joined += strings.Join(c, " ") + "\n"
	}
	if !strings.Contains(joined, "--max-filesize") || !strings.Contains(joined, "instagram.com/reel/abc") {
		t.Errorf("yt-dlp args missing cap/url:\n%s", joined)
	}
	if !strings.Contains(joined, "fps=") {
		t.Errorf("ffmpeg args missing fps filter:\n%s", joined)
	}
	if !strings.Contains(joined, "scale=768:768") {
		t.Errorf("ffmpeg args missing box-fit scale filter:\n%s", joined)
	}
	// Temp working dir is cleaned up.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("temp dir not cleaned: %v", entries)
	}
}

// badDurationRunner behaves like fakeRunner but returns non-numeric ffprobe
// output, exercising the dur <= 0 -> fps fallback path in Frames.
type badDurationRunner struct{ calls [][]string }

func (r *badDurationRunner) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	switch {
	case strings.Contains(name, "yt-dlp"):
		out := argValue(args, "-o")
		_ = os.WriteFile(out, []byte("fakevideo"), 0o600)
	case strings.Contains(name, "ffprobe"):
		return []byte("N/A\n"), nil
	case strings.Contains(name, "ffmpeg"):
		pattern := args[len(args)-1]
		for i := 1; i <= 3; i++ {
			_ = os.WriteFile(strings.Replace(pattern, "%03d", pad3(i), 1), []byte{0xFF, 0xD8, byte(i)}, 0o600)
		}
	}
	return nil, nil
}

func TestFramesFpsFallbackOnBadDuration(t *testing.T) {
	dir := t.TempDir()
	r := &badDurationRunner{}
	e := &Extractor{ytDLP: "yt-dlp", ffmpeg: "ffmpeg", ffprobe: "ffprobe", maxFrames: 8, run: r.run, tempBase: dir}
	frames, err := e.Frames(context.Background(), "https://instagram.com/reel/abc")
	if err != nil {
		t.Fatalf("Frames: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("want 3 frames, got %d", len(frames))
	}
	joined := ""
	for _, c := range r.calls {
		joined += strings.Join(c, " ") + "\n"
	}
	if !strings.Contains(joined, "fps=1,") {
		t.Errorf("ffmpeg args missing fallback fps=1 filter:\n%s", joined)
	}
}

func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
func pad3(i int) string {
	return string(rune('0'+i/100%10)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10))
}
