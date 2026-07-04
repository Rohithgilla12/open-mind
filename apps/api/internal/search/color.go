package search

import (
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

// ErrBadColor is returned when a colour-search term is neither a valid
// "#RRGGBB" hex string nor a recognised colour name.
var ErrBadColor = errors.New("search: unrecognised colour")

// rgb is an sRGB colour with 0–255 channels.
type rgb struct{ r, g, b float64 }

// lab is a CIELAB colour, used because Euclidean distance in Lab space
// (ΔE*76) approximates perceptual colour difference far better than raw RGB.
type lab struct{ l, a, b float64 }

// namedColors maps human colour words to hex. It covers the common web
// colours plus Openmind's own accent palette (see docs/design/README.md) so a
// query like "cobalt" or "terracotta" resolves to the brand tokens.
var namedColors = map[string]string{
	// Openmind accents.
	"cobalt":     "#1B3FD1",
	"terracotta": "#C24A2E",
	"gold":       "#E0B23A",
	"green":      "#2E7D5B",
	// Common colours.
	"red":     "#D1291B",
	"orange":  "#E07B2A",
	"yellow":  "#E8C33A",
	"lime":    "#7DC24A",
	"teal":    "#2E7D7D",
	"cyan":    "#2AB6E0",
	"blue":    "#1B54D1",
	"navy":    "#16255C",
	"indigo":  "#3A2EE0",
	"purple":  "#7D2EC2",
	"magenta": "#C22E9E",
	"pink":    "#E07BA6",
	"brown":   "#7D5A2E",
	"beige":   "#D8C9A8",
	"cream":   "#F4F0E6",
	"black":   "#1C1A16",
	"white":   "#FCFBF6",
	"grey":    "#8A857A",
	"gray":    "#8A857A",
}

// ValidColor reports whether s is a colour term Run accepts: a hex string
// ("#RRGGBB", shorthand, or bare) or a recognised colour name. Callers use it
// to decide whether a machine-parsed colour is worth forwarding to search.
func ValidColor(s string) bool {
	_, ok := parseColor(s)
	return ok
}

// parseColor resolves a hex ("#RRGGBB" or "RRGGBB") or named colour to sRGB.
// It is case-insensitive and tolerant of surrounding whitespace.
func parseColor(s string) (rgb, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return rgb{}, false
	}
	if hex, ok := namedColors[s]; ok {
		s = hex
	}
	return hexToRGB(s)
}

// hexToRGB parses "#RRGGBB" / "RRGGBB" (and the 3-digit shorthand) into sRGB.
func hexToRGB(s string) (rgb, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(s)), "#")
	if len(s) == 3 { // shorthand #abc -> #aabbcc
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return rgb{}, false
	}
	var v [3]float64
	for i := 0; i < 3; i++ {
		hi, ok1 := hexNibble(s[i*2])
		lo, ok2 := hexNibble(s[i*2+1])
		if !ok1 || !ok2 {
			return rgb{}, false
		}
		v[i] = float64(hi*16 + lo)
	}
	return rgb{v[0], v[1], v[2]}, true
}

func hexNibble(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	default:
		return 0, false
	}
}

// toLab converts sRGB to CIELAB via linearised RGB → XYZ (D65) → Lab.
func (c rgb) toLab() lab {
	lin := func(ch float64) float64 {
		v := ch / 255.0
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	r, g, b := lin(c.r), lin(c.g), lin(c.b)
	// sRGB → XYZ (D65).
	x := (r*0.4124 + g*0.3576 + b*0.1805) / 0.95047
	y := (r*0.2126 + g*0.7152 + b*0.0722) / 1.00000
	z := (r*0.0193 + g*0.1192 + b*0.9505) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116.0
	}
	fx, fy, fz := f(x), f(y), f(z)
	return lab{
		l: 116.0*fy - 16.0,
		a: 500.0 * (fx - fy),
		b: 200.0 * (fy - fz),
	}
}

// deltaE returns the ΔE*76 (Euclidean CIELAB) distance between two colours.
// 0 is identical; ~2.3 is the "just noticeable difference" threshold.
func deltaE(a, b lab) float64 {
	dl := a.l - b.l
	da := a.a - b.a
	dbb := a.b - b.b
	return math.Sqrt(dl*dl + da*da + dbb*dbb)
}

// paletteDistance returns the smallest ΔE between target and any colour in the
// item's palette. The bool is false when the item has no parseable palette
// colour, so callers can drop it from colour results.
func paletteDistance(target lab, palette []string) (float64, bool) {
	best := math.MaxFloat64
	found := false
	for _, hexes := range palette {
		c, ok := parseColor(hexes)
		if !ok {
			continue
		}
		if d := deltaE(target, c.toLab()); d < best {
			best = d
			found = true
		}
	}
	return best, found
}

// rankByColor orders items by ascending distance from target (closest palette
// match first), dropping items with no parseable palette colour. Ties break by
// newest-created then ID for determinism, mirroring the fused-score tiebreak.
func rankByColor(items []db.Item, target rgb) []db.Item {
	tl := target.toLab()
	type scored struct {
		item db.Item
		dist float64
	}
	ranked := make([]scored, 0, len(items))
	for _, it := range items {
		d, ok := paletteDistance(tl, it.Palette)
		if !ok {
			continue
		}
		ranked = append(ranked, scored{item: it, dist: d})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].dist != ranked[j].dist {
			return ranked[i].dist < ranked[j].dist
		}
		ci, cj := ranked[i].item.CreatedAt, ranked[j].item.CreatedAt
		if !ci.Time.Equal(cj.Time) {
			return ci.Time.After(cj.Time)
		}
		return ranked[i].item.ID.String() > ranked[j].item.ID.String()
	})
	out := make([]db.Item, len(ranked))
	for i, s := range ranked {
		out[i] = s.item
	}
	return out
}
