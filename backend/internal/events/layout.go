package events

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Layout is the host's grid as saved by the layout designer.
// Row 0 is nearest the screen/stage. Each cell is one of:
//
//	""      unassigned seat (blocks publishing)
//	"a"     aisle: a gap, not a seat, not numbered
//	"x"     blocked seat: numbered, never sold
//	<uuid>  seat in that ticket category
type Layout struct {
	Rows  int        `json:"rows"`
	Cols  int        `json:"cols"`
	Cells [][]string `json:"cells"`
}

const (
	CellUnassigned = ""
	CellAisle      = "a"
	CellBlocked    = "x"
	maxGridSide    = 50
)

// GeneratedSeat is one seat derived from the layout.
type GeneratedSeat struct {
	CategoryID uuid.UUID
	RowLabel   string
	SeatNumber int
	Label      string
	GridRow    int
	GridCol    int
	Blocked    bool
}

func ParseLayout(raw []byte) (Layout, error) {
	var l Layout
	if len(raw) == 0 {
		return l, fmt.Errorf("Design the seating layout first")
	}
	if err := json.Unmarshal(raw, &l); err != nil {
		return l, fmt.Errorf("Layout is malformed")
	}
	if l.Rows < 1 || l.Cols < 1 || l.Rows > maxGridSide || l.Cols > maxGridSide {
		return l, fmt.Errorf("Layout must be between 1×1 and %d×%d", maxGridSide, maxGridSide)
	}
	if len(l.Cells) != l.Rows {
		return l, fmt.Errorf("Layout has %d rows of cells, expected %d", len(l.Cells), l.Rows)
	}
	for _, row := range l.Cells {
		if len(row) != l.Cols {
			return l, fmt.Errorf("Every layout row must have %d cells", l.Cols)
		}
	}
	return l, nil
}

// GenerateSeats turns a layout into seats. Rows are lettered A, B, … from the
// screen side, skipping rows that are entirely aisle; seats are numbered left to
// right, skipping aisles. Blocked seats keep a number and take blockedCategory
// (seats.category_id is NOT NULL, but they are never sold).
func GenerateSeats(l Layout, categories map[uuid.UUID]bool, blockedCategory uuid.UUID) ([]GeneratedSeat, error) {
	var seats []GeneratedSeat
	letter := 0
	sellable := 0
	for r, row := range l.Cells {
		if isAllAisle(row) {
			continue
		}
		rowLabel := RowLabel(letter)
		letter++
		num := 0
		for c, cell := range row {
			switch cell {
			case CellAisle:
				continue
			case CellUnassigned:
				return nil, fmt.Errorf("Seat %s%d has no ticket category", rowLabel, num+1)
			}
			num++
			s := GeneratedSeat{
				RowLabel: rowLabel, SeatNumber: num, Label: fmt.Sprintf("%s%d", rowLabel, num),
				GridRow: r, GridCol: c,
			}
			if cell == CellBlocked {
				s.Blocked = true
				s.CategoryID = blockedCategory
			} else {
				id, err := uuid.Parse(cell)
				if err != nil || !categories[id] {
					return nil, fmt.Errorf("Seat %s uses an unknown ticket category", s.Label)
				}
				s.CategoryID = id
				sellable++
			}
			seats = append(seats, s)
		}
	}
	if sellable == 0 {
		return nil, fmt.Errorf("The layout needs at least one sellable seat")
	}
	return seats, nil
}

func isAllAisle(row []string) bool {
	for _, c := range row {
		if c != CellAisle {
			return false
		}
	}
	return true
}

// RowLabel maps 0→A … 25→Z, 26→AA, 27→AB …
func RowLabel(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return RowLabel(i/26-1) + string(rune('A'+i%26))
}
