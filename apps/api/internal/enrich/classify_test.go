package enrich

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name, url string
		want      string
	}{
		{"youtube", "https://www.youtube.com/watch?v=abc", "video"},
		{"youtube uppercase www and host", "https://WWW.YOUTUBE.COM/watch?v=x", "video"},
		{"tweet", "https://x.com/user/status/123", "tweet"},
		{"twitter", "https://twitter.com/user/status/123", "tweet"},
		{"image ext", "https://cdn.site.com/photo.jpg", "image"},
		{"instagram reel", "https://www.instagram.com/reel/Cabc123/", "video"},
		{"instagram short link", "https://instagr.am/p/Cabc123/", "video"},
		{"tiktok", "https://www.tiktok.com/@user/video/123", "video"},
		{"tiktok share link", "https://vm.tiktok.com/ZM123/", "video"},
		{"default article", "https://blog.example.com/post", "article"},
		{"repo root", "https://github.com/sqlc-dev/sqlc", "repo"},
		{"repo sub-page", "https://github.com/sqlc-dev/sqlc/pull/42", "repo"},
		{"repo blob with image extension", "https://github.com/o/r/blob/main/logo.png", "repo"},
		{"repo trailing slash", "https://github.com/o/r/", "repo"},
		{"repo with query", "https://github.com/o/r?tab=readme-ov-file", "repo"},
		{"uppercase host and owner", "https://GitHub.com/O/R", "repo"},
		{"uppercase www and host", "https://WWW.GITHUB.COM/O/R", "repo"},
		{"github profile", "https://github.com/torvalds", "article"},
		{"github reserved segment", "https://github.com/features/copilot", "article"},
		{"github reserved segment cased", "https://github.com/Topics/go", "article"},
		{"github bare host", "https://github.com", "article"},
		{"gitlab nested group", "https://gitlab.com/group/sub/project", "repo"},
		{"gitlab dash route", "https://gitlab.com/-/profile", "article"},
		{"codeberg repo", "https://codeberg.org/forgejo/forgejo", "repo"},
		{"bitbucket repo", "https://bitbucket.org/workspace/project", "repo"},
		{"gist is not a repo", "https://gist.github.com/user/abc123", "article"},
		{"raw image host unaffected", "https://raw.githubusercontent.com/o/r/main/a.png", "image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.url, Extraction{}); got != tt.want {
				t.Errorf("Classify(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
