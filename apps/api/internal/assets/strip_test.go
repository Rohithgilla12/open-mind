package assets

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"hash/crc32"
	"image/jpeg"
	"image/png"
	"testing"
)

// tinyImage returns a 2x2 RGBA image for encoding into fixtures.
func tinyImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	return img
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, tinyImage(), nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

// injectAfterSOI inserts seg immediately after the SOI marker (FFD8).
func injectAfterSOI(t *testing.T, jpg, seg []byte) []byte {
	t.Helper()
	if len(jpg) < 2 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		t.Fatalf("not a jpeg")
	}
	out := make([]byte, 0, len(jpg)+len(seg))
	out = append(out, jpg[:2]...)
	out = append(out, seg...)
	out = append(out, jpg[2:]...)
	return out
}

// jpegSegment builds an FF<marker> segment with a 2-byte BE length covering the
// length bytes plus payload.
func jpegSegment(marker byte, payload []byte) []byte {
	seg := []byte{0xFF, marker}
	l := len(payload) + 2
	seg = append(seg, byte(l>>8), byte(l))
	seg = append(seg, payload...)
	return seg
}

func TestStripJPEG(t *testing.T) {
	// Go's jpeg encoder emits no APP0/JFIF marker, so inject one to form the
	// "clean" baseline the stripper must preserve verbatim.
	app0 := jpegSegment(0xE0, append([]byte("JFIF\x00"), 0x01, 0x02, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00))
	clean := injectAfterSOI(t, jpegBytes(t), app0)

	exif := jpegSegment(0xE1, append([]byte("Exif\x00\x00"), 0x49, 0x49, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00))
	app13 := jpegSegment(0xED, []byte("Photoshop 3.0\x00some iptc"))
	com := jpegSegment(0xFE, []byte("a private comment"))

	withMeta := injectAfterSOI(t, clean, exif)
	withMeta = injectAfterSOI(t, withMeta, app13)
	withMeta = injectAfterSOI(t, withMeta, com)

	out, err := StripMetadata("image/jpeg", withMeta)
	if err != nil {
		t.Fatalf("StripMetadata: %v", err)
	}
	if bytes.Contains(out, []byte("Exif\x00\x00")) {
		t.Errorf("output still contains Exif marker payload")
	}
	if bytes.Contains(out, []byte("Photoshop 3.0")) {
		t.Errorf("output still contains APP13 payload")
	}
	if bytes.Contains(out, []byte("a private comment")) {
		t.Errorf("output still contains COM payload")
	}
	// No FFE1, FFED, FFFE segment markers remain.
	for _, m := range []byte{0xE1, 0xED, 0xFE} {
		if bytes.Contains(out, []byte{0xFF, m}) {
			t.Errorf("output still contains FF%02X marker", m)
		}
	}
	// APP0 (JFIF) survives.
	if !bytes.Contains(out, []byte{0xFF, 0xE0}) {
		t.Errorf("APP0/JFIF marker was dropped")
	}
	// Still decodes to a 2x2 image.
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode stripped jpeg: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Errorf("bounds = %v, want 2x2", b)
	}
	// Scan data byte-identical to the clean (APP0-only) baseline.
	if !bytes.Equal(out, clean) {
		t.Errorf("stripped output differs from clean baseline (%d vs %d bytes)", len(out), len(clean))
	}
}

func TestStripJPEGFillBytes(t *testing.T) {
	// A spec-legal JPEG may pad a marker with extra 0xFF fill bytes. Inject an
	// EXIF segment whose marker is preceded by two extra fill bytes and confirm
	// the stripper handles it (removes EXIF, still decodes) rather than rejecting.
	exif := jpegSegment(0xE1, append([]byte("Exif\x00\x00"), 0x49, 0x49, 0x2A, 0x00))
	withFill := append([]byte{0xFF, 0xFF}, exif...) // FF FF FF E1 ...
	jpg := injectAfterSOI(t, jpegBytes(t), withFill)

	out, err := StripMetadata("image/jpeg", jpg)
	if err != nil {
		t.Fatalf("StripMetadata rejected a fill-byte jpeg: %v", err)
	}
	if bytes.Contains(out, []byte("Exif\x00\x00")) {
		t.Errorf("EXIF survived the fill-byte path")
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("decode stripped fill-byte jpeg: %v", err)
	}
}

func TestStripJPEGNoMetadata(t *testing.T) {
	base := jpegBytes(t)
	out, err := StripMetadata("image/jpeg", base)
	if err != nil {
		t.Fatalf("StripMetadata: %v", err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !bytes.Equal(out, base) {
		t.Errorf("no-metadata jpeg altered")
	}
}

func TestStripJPEGCorrupt(t *testing.T) {
	// Truncated: SOI + APP1 marker + a length claiming more bytes than present.
	trunc := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x20, 0x01, 0x02}
	if _, err := StripMetadata("image/jpeg", trunc); err == nil {
		t.Errorf("expected error on truncated jpeg")
	}
	// Random non-jpeg bytes.
	if _, err := StripMetadata("image/jpeg", []byte("not a jpeg at all")); err == nil {
		t.Errorf("expected error on non-jpeg bytes")
	}
}

func pngFixture(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, tinyImage()); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func pngChunk(typ string, data []byte) []byte {
	out := make([]byte, 0, 12+len(data))
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	out = append(out, lenBuf[:]...)
	out = append(out, []byte(typ)...)
	out = append(out, data...)
	crc := crc32.ChecksumIEEE(append([]byte(typ), data...))
	var crcBuf [4]byte
	binary.BigEndian.PutUint32(crcBuf[:], crc)
	out = append(out, crcBuf[:]...)
	return out
}

// injectAfterIHDR inserts chunk bytes right after the IHDR chunk.
func injectAfterIHDR(t *testing.T, p, chunk []byte) []byte {
	t.Helper()
	// signature (8) + IHDR length(4)+type(4)+data(13)+crc(4) = 8 + 25 = 33
	const ihdrEnd = 8 + 4 + 4 + 13 + 4
	if len(p) < ihdrEnd {
		t.Fatalf("png too short")
	}
	out := make([]byte, 0, len(p)+len(chunk))
	out = append(out, p[:ihdrEnd]...)
	out = append(out, chunk...)
	out = append(out, p[ihdrEnd:]...)
	return out
}

func TestStripPNG(t *testing.T) {
	base := pngFixture(t)
	withMeta := injectAfterIHDR(t, base, pngChunk("tEXt", []byte("Comment\x00hello world")))
	withMeta = injectAfterIHDR(t, withMeta, pngChunk("eXIf", []byte{0x49, 0x49, 0x2A, 0x00}))

	out, err := StripMetadata("image/png", withMeta)
	if err != nil {
		t.Fatalf("StripMetadata: %v", err)
	}
	if bytes.Contains(out, []byte("tEXt")) {
		t.Errorf("tEXt chunk survived")
	}
	if bytes.Contains(out, []byte("eXIf")) {
		t.Errorf("eXIf chunk survived")
	}
	if bytes.Contains(out, []byte("hello world")) {
		t.Errorf("tEXt payload survived")
	}
	for _, typ := range []string{"IHDR", "IDAT", "IEND"} {
		if !bytes.Contains(out, []byte(typ)) {
			t.Errorf("required chunk %s missing", typ)
		}
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode stripped png: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Errorf("bounds = %v, want 2x2", b)
	}
	if !bytes.Equal(out, base) {
		t.Errorf("stripped png differs from clean baseline")
	}
}

func TestStripPNGCorrupt(t *testing.T) {
	// Valid signature then a chunk length past the end.
	sig := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	trunc := append(sig, 0x00, 0x00, 0x10, 0x00, 'I', 'H', 'D', 'R')
	if _, err := StripMetadata("image/png", trunc); err == nil {
		t.Errorf("expected error on truncated png")
	}
	if _, err := StripMetadata("image/png", []byte("not a png")); err == nil {
		t.Errorf("expected error on non-png")
	}
}

// webpChunk builds a FourCC + LE-size + payload (+pad to even).
func webpChunk(fourcc string, payload []byte) []byte {
	out := make([]byte, 0, 8+len(payload)+1)
	out = append(out, []byte(fourcc)...)
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(len(payload)))
	out = append(out, sz[:]...)
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0x00)
	}
	return out
}

func craftWebP(chunks ...[]byte) []byte {
	var body []byte
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := make([]byte, 0, 12+len(body))
	out = append(out, []byte("RIFF")...)
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(4+len(body)))
	out = append(out, sz[:]...)
	out = append(out, []byte("WEBP")...)
	out = append(out, body...)
	return out
}

func TestStripWebP(t *testing.T) {
	vp8 := webpChunk("VP8 ", []byte{0x01, 0x02, 0x03, 0x04, 0x05})
	exif := webpChunk("EXIF", []byte{0x49, 0x49, 0x2A, 0x00, 0x08})
	xmp := webpChunk("XMP ", []byte("<x:xmpmeta>hi</x:xmpmeta>"))
	in := craftWebP(vp8, exif, xmp)

	out, err := StripMetadata("image/webp", in)
	if err != nil {
		t.Fatalf("StripMetadata: %v", err)
	}
	if bytes.Contains(out, []byte("EXIF")) {
		t.Errorf("EXIF fourcc survived")
	}
	if bytes.Contains(out, []byte("XMP ")) {
		t.Errorf("XMP fourcc survived")
	}
	if !bytes.Contains(out, vp8) {
		t.Errorf("VP8 chunk not preserved byte-identical")
	}
	// RIFF size field == len(out) - 8.
	got := binary.LittleEndian.Uint32(out[4:8])
	if int(got) != len(out)-8 {
		t.Errorf("RIFF size = %d, want %d", got, len(out)-8)
	}
}

func TestStripWebPVP8XFlags(t *testing.T) {
	// VP8X payload byte 0 has EXIF (0x08) and XMP (0x04) flags set, plus an
	// unrelated flag 0x10 that must survive.
	vp8x := webpChunk("VP8X", []byte{0x08 | 0x04 | 0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	exif := webpChunk("EXIF", []byte{0x01, 0x02})
	in := craftWebP(vp8x, webpChunk("VP8 ", []byte{0xAA, 0xBB}), exif)

	out, err := StripMetadata("image/webp", in)
	if err != nil {
		t.Fatalf("StripMetadata: %v", err)
	}
	// VP8X payload byte 0: after RIFF(12) + "VP8X"(4) + size(4) = offset 20.
	flags := out[20]
	if flags&0x08 != 0 {
		t.Errorf("EXIF flag still set: %08b", flags)
	}
	if flags&0x04 != 0 {
		t.Errorf("XMP flag still set: %08b", flags)
	}
	if flags&0x10 == 0 {
		t.Errorf("unrelated flag 0x10 was cleared: %08b", flags)
	}
}

func TestStripWebPCorrupt(t *testing.T) {
	if _, err := StripMetadata("image/webp", []byte("RIFFxxxxNOTW")); err == nil {
		t.Errorf("expected error on bad webp")
	}
	// Chunk size past the end.
	bad := []byte("RIFF")
	bad = append(bad, 0x20, 0, 0, 0)
	bad = append(bad, []byte("WEBP")...)
	bad = append(bad, []byte("VP8 ")...)
	bad = append(bad, 0xFF, 0xFF, 0, 0) // huge size
	if _, err := StripMetadata("image/webp", bad); err == nil {
		t.Errorf("expected error on truncated webp chunk")
	}
}

func TestStripGIF(t *testing.T) {
	var buf bytes.Buffer
	pal := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	if err := gif.Encode(&buf, pal, nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	in := buf.Bytes()
	out, err := StripMetadata("image/gif", in)
	if err != nil {
		t.Fatalf("StripMetadata: %v", err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("gif altered")
	}
}
