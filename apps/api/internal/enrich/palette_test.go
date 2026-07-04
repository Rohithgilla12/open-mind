package enrich

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// encodePNG renders fn over a w×h canvas and returns the PNG bytes.
func encodePNG(t *testing.T, w, h int, fn func(x, y int) color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, fn(x, y))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding png: %v", err)
	}
	return buf.Bytes()
}

// redish reports whether hex is a red-dominant colour: R clearly above G and B.
func redish(t *testing.T, hex string) bool {
	t.Helper()
	if len(hex) != 7 || hex[0] != '#' {
		t.Fatalf("not a hex colour: %q", hex)
	}
	var r, g, b int
	if _, err := fmtSscan(hex, &r, &g, &b); err != nil {
		t.Fatalf("parsing %q: %v", hex, err)
	}
	return r > 150 && r > g+60 && r > b+60
}

func fmtSscan(hex string, r, g, b *int) (int, error) {
	// hex is "#RRGGBB"
	var err error
	*r, err = hexByte(hex[1:3])
	if err != nil {
		return 0, err
	}
	*g, err = hexByte(hex[3:5])
	if err != nil {
		return 0, err
	}
	*b, err = hexByte(hex[5:7])
	if err != nil {
		return 0, err
	}
	return 3, nil
}

func hexByte(s string) (int, error) {
	v := 0
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= int(c - '0')
		case c >= 'a' && c <= 'f':
			v |= int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= int(c-'A') + 10
		}
	}
	return v, nil
}

func TestDominantColorsSolidRed(t *testing.T) {
	data := encodePNG(t, 4, 4, func(_, _ int) color.Color {
		return color.RGBA{R: 255, G: 0, B: 0, A: 255}
	})
	colors, err := DominantColors(data, 5)
	if err != nil {
		t.Fatalf("DominantColors: %v", err)
	}
	if len(colors) == 0 {
		t.Fatal("want at least one colour, got none")
	}
	found := false
	for _, c := range colors {
		if redish(t, c) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no red-ish colour in %v", colors)
	}
}

func TestDominantColorsTwoColours(t *testing.T) {
	// Left half red, right half blue.
	data := encodePNG(t, 8, 8, func(x, _ int) color.Color {
		if x < 4 {
			return color.RGBA{R: 220, G: 20, B: 20, A: 255}
		}
		return color.RGBA{R: 20, G: 20, B: 220, A: 255}
	})
	colors, err := DominantColors(data, 5)
	if err != nil {
		t.Fatalf("DominantColors: %v", err)
	}
	distinct := map[string]struct{}{}
	for _, c := range colors {
		distinct[c] = struct{}{}
	}
	if len(distinct) < 2 {
		t.Fatalf("want >=2 distinct colours, got %v", colors)
	}
}

func TestDominantColorsGarbage(t *testing.T) {
	colors, err := DominantColors([]byte("not an image at all"), 5)
	if err != nil {
		t.Fatalf("garbage input must not error, got %v", err)
	}
	if colors != nil {
		t.Fatalf("garbage input must return nil, got %v", colors)
	}
}

func TestDominantColorsEmpty(t *testing.T) {
	colors, err := DominantColors(nil, 5)
	if err != nil || colors != nil {
		t.Fatalf("empty input: want nil,nil; got %v,%v", colors, err)
	}
}

func TestDominantColorsRespectsN(t *testing.T) {
	// A gradient with many buckets; ask for 3.
	data := encodePNG(t, 30, 30, func(x, y int) color.Color {
		return color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 128, A: 255}
	})
	colors, err := DominantColors(data, 3)
	if err != nil {
		t.Fatalf("DominantColors: %v", err)
	}
	if len(colors) > 3 {
		t.Fatalf("want at most 3 colours, got %d: %v", len(colors), colors)
	}
	if !strings.HasPrefix(colors[0], "#") {
		t.Fatalf("colour not hex: %q", colors[0])
	}
}
