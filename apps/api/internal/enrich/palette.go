package enrich

import (
	"bytes"
	"fmt"
	"image"
	"sort"

	// Register the decoders DominantColors supports. Blank imports install the
	// format handlers used by image.Decode.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// maxPaletteSamples caps how many pixels DominantColors inspects; larger images
// are sampled on a coarse grid so extraction stays cheap regardless of size.
const maxPaletteSamples = 4000

// colourBucket accumulates the running RGB sum and count of the pixels quantised
// into one coarse-grid cell, so the representative colour is the bucket average.
type colourBucket struct {
	key            uint32
	rSum           uint64
	gSum           uint64
	bSum           uint64
	count          uint64
	firstSeenOrder int
}

// DominantColors decodes data as an image (JPEG/PNG/GIF via the standard library)
// and returns up to n of its most frequent colours as "#RRGGBB" hex strings,
// ordered by descending frequency. Near-identical pixels are grouped by rounding
// each channel to a coarse grid, and each returned colour is that group's average.
//
// Undecodable, empty, or fully transparent input yields (nil, nil): palette
// extraction is a nicety and must never fail the enrichment job. The result is
// deterministic for a given input, so re-running enrichment reproduces it.
func DominantColors(data []byte, n int) ([]string, error) {
	if n <= 0 || len(data) == 0 {
		return nil, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, nil
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, nil
	}

	// Choose a grid stride so the number of sampled pixels stays near the cap.
	step := 1
	for (w/step)*(h/step) > maxPaletteSamples {
		step++
	}

	buckets := map[uint32]*colourBucket{}
	order := 0
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16 < 0x8000 { // skip mostly-transparent pixels
				continue
			}
			r8 := uint32(r16 >> 8)
			g8 := uint32(g16 >> 8)
			b8 := uint32(b16 >> 8)
			// Quantise to a coarse grid (top 3 bits per channel) so near
			// colours collapse into one bucket.
			key := (r8&0xE0)<<16 | (g8&0xE0)<<8 | (b8 & 0xE0)
			bkt := buckets[key]
			if bkt == nil {
				bkt = &colourBucket{key: key, firstSeenOrder: order}
				buckets[key] = bkt
				order++
			}
			bkt.rSum += uint64(r8)
			bkt.gSum += uint64(g8)
			bkt.bSum += uint64(b8)
			bkt.count++
		}
	}
	if len(buckets) == 0 {
		return nil, nil
	}

	ranked := make([]*colourBucket, 0, len(buckets))
	for _, bkt := range buckets {
		ranked = append(ranked, bkt)
	}
	// Most frequent first; ties broken by first-seen order for determinism.
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].count != ranked[j].count {
			return ranked[i].count > ranked[j].count
		}
		return ranked[i].firstSeenOrder < ranked[j].firstSeenOrder
	})
	if n > len(ranked) {
		n = len(ranked)
	}

	out := make([]string, 0, n)
	for _, bkt := range ranked[:n] {
		r := bkt.rSum / bkt.count
		g := bkt.gSum / bkt.count
		bl := bkt.bSum / bkt.count
		out = append(out, fmt.Sprintf("#%02X%02X%02X", r, g, bl))
	}
	return out, nil
}
