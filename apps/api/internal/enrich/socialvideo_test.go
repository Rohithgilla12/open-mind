package enrich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsSocialVideoURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.instagram.com/reel/Cabc123/", true},
		{"https://instagram.com/p/Cabc123/", true},
		{"https://www.tiktok.com/@user/video/123", true},
		{"https://vm.tiktok.com/ZM123/", true},
		{"https://vt.tiktok.com/ZM123/", true},
		{"https://www.youtube.com/watch?v=abc", false},
		{"https://blog.example.com/instagram.com", false},
		{"://bad", false},
	}
	for _, tt := range tests {
		if got := IsSocialVideoURL(tt.url); got != tt.want {
			t.Errorf("IsSocialVideoURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestParseOpenGraph(t *testing.T) {
	tests := []struct {
		name string
		html string
		want Extraction
	}{
		{
			name: "full tags",
			html: `<html><head>
				<meta property="og:title" content="chef.eats on Instagram"/>
				<meta property="og:description" content="3 cafes in Lisbon you must try 📍"/>
				<meta property="og:image" content="https://cdn.example.com/thumb.jpg"/>
			</head><body>login wall</body></html>`,
			want: Extraction{
				Title:        "chef.eats on Instagram",
				Body:         "3 cafes in Lisbon you must try 📍",
				LeadImageURL: "https://cdn.example.com/thumb.jpg",
			},
		},
		{
			name: "missing description",
			html: `<head><meta property="og:title" content="a reel"/></head><body></body>`,
			want: Extraction{Title: "a reel"},
		},
		{
			name: "login wall without og tags",
			html: `<html><head><title>Login</title></head><body>Log in to continue</body></html>`,
			want: Extraction{},
		},
		{
			name: "first tag wins",
			html: `<head>
				<meta property="og:title" content="first"/>
				<meta property="og:title" content="second"/>
			</head>`,
			want: Extraction{Title: "first"},
		},
		{
			name: "meta after body start is ignored",
			html: `<head></head><body><meta property="og:title" content="late"/></body>`,
			want: Extraction{},
		},
		{
			name: "not html",
			html: `just plain text, no tags at all`,
			want: Extraction{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOpenGraph(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("parseOpenGraph: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseOpenGraph = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestExtractOpenGraph(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reel":
			_, _ = w.Write([]byte(`<head><meta property="og:title" content="t"/><meta property="og:description" content="d"/></head>`))
		case "/gone":
			http.Error(w, "nope", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got, err := extractOpenGraph(context.Background(), srv.Client(), srv.URL+"/reel")
	if err != nil {
		t.Fatalf("extractOpenGraph: %v", err)
	}
	if got.Title != "t" || got.Body != "d" {
		t.Errorf("extractOpenGraph = %+v", got)
	}

	if _, err := extractOpenGraph(context.Background(), srv.Client(), srv.URL+"/gone"); err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestNormalizeSocialVideo(t *testing.T) {
	longCaption := "Hyderabad has a food scene like no other — and these 3 Telugu restaurants are rewriting the rulebook! " +
		"Thamara — for the ones who love a slow meal. Gamyam — where tradition meets craft."

	tests := []struct {
		name      string
		ogTitle   string
		ogBody    string
		label     string
		wantTitle string
		wantBody  string
	}{
		{
			name:      "instagram author only",
			ogTitle:   "chef.eats on Instagram",
			ogBody:    "3 cafes in Lisbon you must try",
			label:     "Instagram reel",
			wantTitle: "chef.eats on Instagram",
			wantBody:  "3 cafes in Lisbon you must try",
		},
		{
			name:    "instagram title embeds full caption",
			ogTitle: "Vijay Rathod on Instagram: '" + longCaption + "'",
			ogBody:  longCaption,
			label:   "Instagram reel",
			wantTitle: truncateRunes(
				"Vijay Rathod: Hyderabad has a food scene like no other — and these 3 Telugu restaurants are rewriting the rulebook!",
				socialVideoTitleMax,
			),
			wantBody: longCaption,
		},
		{
			name:      "caption only in title fills body",
			ogTitle:   "Ada on Instagram: \"Best ramen in Shibuya tonight.\"",
			ogBody:    "",
			label:     "Instagram reel",
			wantTitle: "Ada: Best ramen in Shibuya tonight.",
			wantBody:  "Best ramen in Shibuya tonight.",
		},
		{
			name:      "empty falls back to label",
			ogTitle:   "",
			ogBody:    "",
			label:     "Instagram reel",
			wantTitle: "Instagram reel",
			wantBody:  "",
		},
		{
			name:      "long caption-only title is capped",
			ogTitle:   strings.Repeat("yummy noodles ", 20),
			ogBody:    "full caption kept",
			label:     "TikTok video",
			wantTitle: truncateRunes(strings.Repeat("yummy noodles ", 20), socialVideoTitleMax),
			wantBody:  "full caption kept",
		},
		{
			name:      "username containing TikTok stays Instagram",
			ogTitle:   "TikTokFan on Instagram",
			ogBody:    "a cafe in Lisbon",
			label:     "Instagram reel",
			wantTitle: "TikTokFan on Instagram",
			wantBody:  "a cafe in Lisbon",
		},
		{
			name:      "multiline caption uses first line as hook",
			ogTitle:   "Sam on Instagram: 'Line one hook!\nLine two details'",
			ogBody:    "Line one hook!\nLine two details",
			label:     "Instagram reel",
			wantTitle: "Sam: Line one hook!",
			wantBody:  "Line one hook!\nLine two details",
		},
		{
			name:      "tiktok author only uses label platform",
			ogTitle:   "chef.eats on TikTok",
			ogBody:    "street food tour",
			label:     "TikTok video",
			wantTitle: "chef.eats on TikTok",
			wantBody:  "street food tour",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTitle, gotBody := normalizeSocialVideo(tt.ogTitle, tt.ogBody, tt.label)
			if gotTitle != tt.wantTitle {
				t.Errorf("title = %q, want %q", gotTitle, tt.wantTitle)
			}
			if gotBody != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
			if runes := []rune(gotTitle); len(runes) > socialVideoTitleMax {
				t.Errorf("title rune length = %d, want ≤ %d", len(runes), socialVideoTitleMax)
			}
		})
	}
}

func TestParseTaggedLocation(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"present", `<script>{"location":{"id":"123","name":"Blue Bottle Coffee"}}</script>`, "Blue Bottle Coffee"},
		{"unicode escapes", `{"location":{"name":"Café Lisboa"}}`, "Café Lisboa"},
		{"absent", `<html><head><meta property="og:title" content="x"></head></html>`, ""},
		{"malformed no name", `{"location":{"id":"9"}}`, ""},
		{"empty", ``, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseTaggedLocation([]byte(tt.html)); got != tt.want {
				t.Fatalf("parseTaggedLocation() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("short", 10); got != "short" {
		t.Errorf("truncateRunes short = %q", got)
	}
	got := truncateRunes("one two three four five six", 14)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis, got %q", got)
	}
	if len([]rune(got)) > 15 { // 14 + ellipsis rune, or shorter if word-broken
		t.Errorf("too long: %q (%d runes)", got, len([]rune(got)))
	}
}
