package assets

import (
	"encoding/binary"
	"fmt"
)

// StripMetadata removes non-essential metadata (EXIF, XMP, IPTC, textual
// comments) from an image while leaving the pixel/scan data byte-identical.
// It is lossless: the returned bytes decode to the same image.
//
// contentType selects the per-format parser. JPEG, PNG, and WebP are parsed
// and rewritten; GIF is returned as an unchanged copy (its metadata surface is
// negligible and the container is awkward to walk safely). Unrecognised types
// return a copy unchanged. Malformed or truncated input for a strippable
// format returns an error so a bad upload never reaches storage.
func StripMetadata(contentType string, data []byte) ([]byte, error) {
	switch contentType {
	case "image/jpeg":
		return stripJPEG(data)
	case "image/png":
		return stripPNG(data)
	case "image/webp":
		return stripWebP(data)
	default:
		// GIF and anything else: return an independent copy, unchanged.
		return append([]byte(nil), data...), nil
	}
}

// stripJPEG walks the JPEG marker stream, dropping APP1 (EXIF/XMP), APP13
// (Photoshop/IPTC), and COM segments, and copies everything else verbatim.
// After the SOS marker the entropy-coded scan is copied as-is.
func stripJPEG(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("jpeg: missing SOI marker")
	}
	out := make([]byte, 0, len(data))
	out = append(out, 0xFF, 0xD8)

	i := 2
	for {
		if i+1 >= len(data) {
			return nil, fmt.Errorf("jpeg: truncated at marker")
		}
		if data[i] != 0xFF {
			return nil, fmt.Errorf("jpeg: expected marker at offset %d", i)
		}
		// A marker may be preceded by any number of 0xFF fill bytes; skip them
		// so a spec-legal FF FF ... <marker> sequence isn't misread as a segment.
		// data[j] lands on the marker byte; keep the last 0xFF adjacent to it so
		// the copy/segment logic below still sees a well-formed FF <marker> pair.
		j := i + 1
		for j < len(data) && data[j] == 0xFF {
			j++
		}
		if j >= len(data) {
			return nil, fmt.Errorf("jpeg: truncated at marker")
		}
		// Emit any leading fill bytes (all but the final 0xFF) verbatim.
		out = append(out, data[i:j-1]...)
		i = j - 1
		marker := data[i+1]

		// Standalone markers with no length/payload: RSTn (D0–D7), TEM (01).
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			out = append(out, 0xFF, marker)
			i += 2
			continue
		}

		// Start of Scan: copy SOS + the entire remainder verbatim and stop.
		if marker == 0xDA {
			out = append(out, data[i:]...)
			return out, nil
		}

		// Segment marker with a 2-byte big-endian length (covers the length
		// field itself). Bounds-check every read.
		if i+3 >= len(data) {
			return nil, fmt.Errorf("jpeg: truncated segment length")
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if segLen < 2 {
			return nil, fmt.Errorf("jpeg: invalid segment length %d", segLen)
		}
		end := i + 2 + segLen // marker(2) not counted in segLen; segLen covers length+payload
		if end > len(data) {
			return nil, fmt.Errorf("jpeg: segment overruns buffer")
		}

		switch marker {
		case 0xE1, 0xED, 0xFE: // APP1 (EXIF/XMP), APP13 (IPTC), COM
			// drop
		default:
			out = append(out, data[i:end]...)
		}
		i = end
	}
}

// stripPNG walks the PNG chunk stream, dropping textual/metadata chunks
// (tEXt, zTXt, iTXt, eXIf) and copying all others verbatim, stopping after
// IEND.
func stripPNG(data []byte) ([]byte, error) {
	sig := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if len(data) < len(sig) || string(data[:8]) != string(sig) {
		return nil, fmt.Errorf("png: bad signature")
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:8]...)

	drop := map[string]struct{}{
		"tEXt": {}, "zTXt": {}, "iTXt": {}, "eXIf": {},
	}

	i := 8
	for {
		if i+8 > len(data) {
			return nil, fmt.Errorf("png: truncated chunk header")
		}
		dataLen := int(binary.BigEndian.Uint32(data[i : i+4]))
		typ := string(data[i+4 : i+8])
		end := i + 8 + dataLen + 4 // length(4) + type(4) + data + crc(4)
		if end < i || end > len(data) {
			return nil, fmt.Errorf("png: chunk %q overruns buffer", typ)
		}
		if _, skip := drop[typ]; !skip {
			out = append(out, data[i:end]...)
		}
		i = end
		if typ == "IEND" {
			return out, nil
		}
	}
}

// stripWebP walks a RIFF/WEBP container, dropping EXIF and XMP chunks and
// clearing the corresponding flag bits in any VP8X chunk, then rewrites the
// RIFF size field. Image data chunks (VP8/VP8L/ALPH/ANMF/...) are preserved
// byte-identically.
func stripWebP(data []byte) ([]byte, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return nil, fmt.Errorf("webp: not a RIFF/WEBP container")
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:12]...) // RIFF + size (rewritten below) + WEBP

	i := 12
	for i < len(data) {
		if i+8 > len(data) {
			return nil, fmt.Errorf("webp: truncated chunk header")
		}
		fourcc := string(data[i : i+4])
		size := int(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		payloadStart := i + 8
		payloadEnd := payloadStart + size
		if payloadEnd < payloadStart || payloadEnd > len(data) {
			return nil, fmt.Errorf("webp: chunk %q overruns buffer", fourcc)
		}
		// Chunks are padded to an even number of bytes.
		next := payloadEnd
		if size%2 == 1 {
			next++
		}
		if next > len(data) {
			// Missing pad byte at EOF is tolerated; clamp.
			next = len(data)
		}

		switch fourcc {
		case "EXIF", "XMP ":
			// drop
		case "VP8X":
			chunk := append([]byte(nil), data[i:next]...)
			// Clear EXIF (0x08) and XMP (0x04) flag bits in payload byte 0.
			if size >= 1 {
				chunk[8] &^= 0x08 | 0x04
			}
			out = append(out, chunk...)
		default:
			out = append(out, data[i:next]...)
		}
		i = next
	}

	// Rewrite the RIFF size field: total output minus the 8-byte RIFF header.
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, nil
}
