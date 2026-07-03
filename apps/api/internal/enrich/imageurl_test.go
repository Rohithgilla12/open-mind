package enrich

import "testing"

func TestIsImageURLByExtension(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"png", "https://cdn.x.com/a/photo.png", true},
		{"jpg with query", "https://cdn.x.com/pics/sunset-beach.jpg?w=200", true},
		{"jpeg", "https://cdn.x.com/a.jpeg", true},
		{"gif", "https://cdn.x.com/a.gif", true},
		{"webp with query and fragment", "https://cdn.x.com/a.webp?v=2#frag", true},
		{"avif", "https://cdn.x.com/a.avif", true},
		{"uppercase ext", "https://cdn.x.com/A.PNG", true},
		{"html not image", "https://example.com/page.html", false},
		{"no extension", "https://example.com/photo", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// nil client: extension check must not touch the network. A false
			// result here just means "fall through to the sniff", not an error.
			got, err := isImageURL(t.Context(), nil, tc.url)
			if err != nil {
				t.Fatalf("isImageURL(%q) error: %v", tc.url, err)
			}
			if got != tc.want {
				t.Errorf("isImageURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestImageTitle(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://cdn.x.com/pics/sunset-beach.jpg?w=200", "sunset-beach"},
		{"https://cdn.x.com/a/photo.png", "photo"},
		{"https://cdn.x.com/my%20photo.jpeg", "my photo"},
		{"https://cdn.x.com/nostem", "nostem"},
	}
	for _, tc := range tests {
		if got := imageTitle(tc.url); got != tc.want {
			t.Errorf("imageTitle(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}
