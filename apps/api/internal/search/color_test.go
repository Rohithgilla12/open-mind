package search

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

func TestParseColor(t *testing.T) {
	tests := []struct {
		in      string
		wantOK  bool
		r, g, b float64
	}{
		{"#1B3FD1", true, 0x1B, 0x3F, 0xD1},
		{"1b3fd1", true, 0x1B, 0x3F, 0xD1},
		{"  #FFFFFF  ", true, 255, 255, 255},
		{"#000", true, 0, 0, 0},
		{"#abc", true, 0xAA, 0xBB, 0xCC},
		{"cobalt", true, 0x1B, 0x3F, 0xD1},
		{"TERRACOTTA", true, 0xC2, 0x4A, 0x2E},
		{"", false, 0, 0, 0},
		{"notacolour", false, 0, 0, 0},
		{"#12", false, 0, 0, 0},
		{"#gggggg", false, 0, 0, 0},
	}
	for _, tt := range tests {
		got, ok := parseColor(tt.in)
		if ok != tt.wantOK {
			t.Errorf("parseColor(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			continue
		}
		if ok && (got.r != tt.r || got.g != tt.g || got.b != tt.b) {
			t.Errorf("parseColor(%q) = %+v, want {%v %v %v}", tt.in, got, tt.r, tt.g, tt.b)
		}
	}
}

func TestDeltaEOrdering(t *testing.T) {
	red, _ := parseColor("red")
	orange, _ := parseColor("orange")
	blue, _ := parseColor("blue")

	if d := deltaE(red.toLab(), red.toLab()); d != 0 {
		t.Errorf("deltaE(red, red) = %v, want 0", d)
	}
	// Orange is perceptually nearer red than blue is.
	near := deltaE(red.toLab(), orange.toLab())
	far := deltaE(red.toLab(), blue.toLab())
	if near >= far {
		t.Errorf("deltaE(red,orange)=%v should be < deltaE(red,blue)=%v", near, far)
	}
}

func itemWithPalette(palette []string) db.Item {
	return db.Item{
		ID:        uuid.New(),
		Palette:   palette,
		CreatedAt: pgtype.Timestamptz{Valid: true},
	}
}

func TestRankByColor(t *testing.T) {
	blue := itemWithPalette([]string{"#1B3FD1", "#F4F0E6"})
	red := itemWithPalette([]string{"#D1291B", "#FCFBF6"})
	noParse := itemWithPalette([]string{"not-a-hex"})

	target, _ := parseColor("cobalt")
	ranked := rankByColor([]db.Item{red, noParse, blue}, target)

	if len(ranked) != 2 {
		t.Fatalf("ranked len = %d, want 2 (unparseable palette dropped)", len(ranked))
	}
	if ranked[0].ID != blue.ID {
		t.Errorf("closest to cobalt = %v, want blue item %v", ranked[0].ID, blue.ID)
	}
}

func TestRankByColorEmpty(t *testing.T) {
	target, _ := parseColor("#123456")
	if got := rankByColor(nil, target); len(got) != 0 {
		t.Errorf("rankByColor(nil) = %v, want empty", got)
	}
}
