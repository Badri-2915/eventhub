package events

import (
	"testing"

	"github.com/google/uuid"
)

// 10×16 grid, aisle columns 4 and 11, four blocked seats → 140 seats, 4 BLOCKED.
func TestGenerateSeatsPlanExample(t *testing.T) {
	cat := uuid.New()
	l := Layout{Rows: 10, Cols: 16}
	for r := 0; r < 10; r++ {
		row := make([]string, 16)
		for c := range row {
			switch {
			case c == 4 || c == 11:
				row[c] = CellAisle
			default:
				row[c] = cat.String()
			}
		}
		l.Cells = append(l.Cells, row)
	}
	l.Cells[9][0], l.Cells[9][1], l.Cells[9][2], l.Cells[9][3] = CellBlocked, CellBlocked, CellBlocked, CellBlocked

	seats, err := GenerateSeats(l, map[uuid.UUID]bool{cat: true}, cat)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 140 {
		t.Fatalf("want 140 seats, got %d", len(seats))
	}
	blocked := 0
	for _, s := range seats {
		if s.Blocked {
			blocked++
		}
	}
	if blocked != 4 {
		t.Fatalf("want 4 blocked, got %d", blocked)
	}
	// Row A: cols 0-3 → A1-A4, col 4 aisle, col 5 → A5.
	if seats[4].Label != "A5" || seats[4].GridCol != 5 {
		t.Fatalf("aisle should be skipped in numbering, got %s at col %d", seats[4].Label, seats[4].GridCol)
	}
	if last := seats[len(seats)-1]; last.Label != "J14" {
		t.Fatalf("want last seat J14, got %s", last.Label)
	}
}

func TestGenerateSeatsSkipsEmptyRowsInLettering(t *testing.T) {
	cat := uuid.New().String()
	l := Layout{Rows: 3, Cols: 2, Cells: [][]string{{cat, cat}, {CellAisle, CellAisle}, {cat, CellAisle}}}
	seats, err := GenerateSeats(l, map[uuid.UUID]bool{uuid.MustParse(cat): true}, uuid.MustParse(cat))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, s := range seats {
		got = append(got, s.Label)
	}
	want := []string{"A1", "A2", "B1"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("want %v, got %v", want, got)
	}
	if seats[2].GridRow != 2 {
		t.Fatalf("row B should keep grid row 2, got %d", seats[2].GridRow)
	}
}

func TestGenerateSeatsRejectsUnassignedAndUnknown(t *testing.T) {
	cat := uuid.New()
	cats := map[uuid.UUID]bool{cat: true}
	if _, err := GenerateSeats(Layout{Rows: 1, Cols: 1, Cells: [][]string{{CellUnassigned}}}, cats, cat); err == nil {
		t.Fatal("unassigned seat must fail")
	}
	if _, err := GenerateSeats(Layout{Rows: 1, Cols: 1, Cells: [][]string{{uuid.NewString()}}}, cats, cat); err == nil {
		t.Fatal("unknown category must fail")
	}
	if _, err := GenerateSeats(Layout{Rows: 1, Cols: 1, Cells: [][]string{{CellBlocked}}}, cats, cat); err == nil {
		t.Fatal("no sellable seat must fail")
	}
}

func TestRowLabel(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 27: "AB", 49: "AX"} {
		if got := RowLabel(i); got != want {
			t.Errorf("RowLabel(%d)=%s want %s", i, got, want)
		}
	}
}
