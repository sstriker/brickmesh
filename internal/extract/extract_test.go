// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package extract

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/shadow"
)

// The fixtures are shared with the Python suite, so both implementations are
// held to the same synthetic parts. See tests/fixtures/generate_fixtures.py.
func fixtureLibrary(t *testing.T) *shadow.Library {
	t.Helper()
	root := filepath.Join("..", "..", "tests", "fixtures", "shadow",
		"LDCadShadowLibrary-main")
	return shadow.Open(root)
}

func TestGridAxesReadsTheStructure(t *testing.T) {
	// Six tokens is two centered axes or three uncentered ones; the token
	// count alone cannot tell them apart.
	cases := map[string]int{
		"":                  0,
		"2 2 20 20":         2,
		"C 3 1 20 0":        2,
		"C 2 C 2 20 20":     2,
		"2 2 2 20 20 20":    3,
		"1 C 2 C 2 0 80 60": 3,
		"1 C 2 1 0 110 0":   3,
	}
	for spec, want := range cases {
		if got := GridAxes(spec); got != want {
			t.Errorf("GridAxes(%q) = %d, want %d", spec, got, want)
		}
	}
}

func TestParseGrid(t *testing.T) {
	counts, spacings, centered := ParseGrid("C 3 1 20 0")
	if len(counts) != 2 || counts[0] != 3 || counts[1] != 1 {
		t.Errorf("counts = %v", counts)
	}
	if spacings[0] != 20 || spacings[1] != 0 {
		t.Errorf("spacings = %v", spacings)
	}
	if !centered[0] || centered[1] {
		t.Errorf("centered = %v", centered)
	}
}

func TestParseGridDefaults(t *testing.T) {
	counts, spacings, _ := ParseGrid("")
	if len(counts) != 2 || counts[0] != 1 || counts[1] != 1 {
		t.Errorf("counts = %v", counts)
	}
	if spacings[0] != 0 || spacings[1] != 0 {
		t.Errorf("spacings = %v", spacings)
	}
}

func TestExpandCenteredGrid(t *testing.T) {
	s := shadow.Snap{Grid: "C 5 1 20 0", Ori: identity()}
	got := Expand(s)
	if len(got) != 5 {
		t.Fatalf("got %d positions, want 5", len(got))
	}
	want := []float64{-40, -20, 0, 20, 40}
	for i, p := range got {
		if math.Abs(p.X-want[i]) > 1e-9 {
			t.Errorf("position %d at X=%v, want %v", i, p.X, want[i])
		}
	}
}

func TestExpandFollowsTheSnapOrientation(t *testing.T) {
	// Rotated 90 degrees about Z, the snap's local X runs along world Y.
	rot := geom.Mat3{{0, -1, 0}, {1, 0, 0}, {0, 0, 1}}
	got := Expand(shadow.Snap{Grid: "C 3 1 20 0", Ori: rot})
	if len(got) != 3 {
		t.Fatalf("got %d positions, want 3", len(got))
	}
	for _, p := range got {
		if math.Abs(p.X) > 1e-9 {
			t.Errorf("expected the row along Y, got %+v", p)
		}
	}
}

// A third axis is declared by 92 grids in the library and its local direction
// is not established, so the snap's own position is kept and the repeats
// dropped rather than invented. Python does the same, which is what keeps the
// two catalogs identical.
func TestExpandKeepsOnlyTheStatedPositionForThreeAxisGrids(t *testing.T) {
	s := shadow.Snap{Grid: "1 C 2 C 2 0 80 60", Pos: geom.Vec3{Y: 5}, Ori: identity()}
	got := Expand(s)
	if len(got) != 1 {
		t.Fatalf("got %d positions, want 1", len(got))
	}
	if got[0] != (geom.Vec3{Y: 5}) {
		t.Errorf("got %+v, want the snap position", got[0])
	}
}

func TestTierOf(t *testing.T) {
	cases := map[string]uint8{
		"Technic Beam  3":                  1,
		"Technic Pin with Friction Ridges": 1,
		"Technic Bush":                     1,
		"Technic Panel Fairing #1":         2,
		"Brick  2 x  4":                    3,
		"Horse Barding":                    3,
	}
	for title, want := range cases {
		if got := TierOf(title); got != want {
			t.Errorf("TierOf(%q) = %d, want %d", title, got, want)
		}
	}
}

func TestUsableDropsSubpartsAndObsolete(t *testing.T) {
	entries := map[string]*Entry{
		"real":  {Part: "real", Holes: []Port{{}}},
		"sub":   {Part: "sub", Holes: []Port{{}}},
		"moved": {Part: "moved", Holes: []Port{{}}},
		"old":   {Part: "old", Holes: []Port{{}}},
		"alias": {Part: "alias", Holes: []Port{{}}},
	}
	titles := map[string]string{
		"real":  "Technic Beam  3",
		"sub":   "~Technic Beam  3 Subpart",
		"moved": "Moved to 12345",
		"old":   "Technic Beam  3 (Obsolete)",
		"alias": "=Technic Beam  3 Alias",
	}
	kept := Usable(entries, titles)
	if len(kept) != 1 || kept["real"] == nil {
		t.Fatalf("kept %v, want only real", keys(kept))
	}
	if kept["real"].Title != "Technic Beam  3" {
		t.Errorf("title = %q", kept["real"].Title)
	}
}

func TestEntryExpandsTheGridAndMarksAxleHoles(t *testing.T) {
	lib := fixtureLibrary(t)
	e := EntryFor(lib, "fixbeam")
	if e == nil {
		t.Fatal("no entry for the fixture beam")
	}
	// Five grid holes plus one axle hole; the grouped snap must not appear.
	if len(e.Holes) != 6 {
		t.Fatalf("got %d holes, want 6", len(e.Holes))
	}
	if e.AxleHoles != 1 {
		t.Errorf("got %d axle holes, want 1", e.AxleHoles)
	}
	if len(e.Pins) != 1 {
		t.Errorf("got %d pins, want 1", len(e.Pins))
	}
	for _, h := range e.Holes {
		if h[0] == 0 && h[1] == 10 && h[2] == 0 {
			t.Error("the grouped craneArm snap reached the catalog")
		}
	}
}

func TestEntryIsNilWithoutShadowData(t *testing.T) {
	if e := EntryFor(fixtureLibrary(t), "nosuchpart"); e != nil {
		t.Errorf("got %+v, want nil", e)
	}
}

func TestRecordsAreSortedAndCarryTheEngineFieldNames(t *testing.T) {
	entries := map[string]*Entry{
		"b": {Part: "b", Title: "Technic Beam  3", Holes: []Port{{}}},
		"a": {Part: "a", Title: "Technic Beam  5", Pins: []Port{{}}},
		"c": {Part: "c", Title: "Brick  2 x  4", Holes: []Port{{}}},
	}
	got := Records(entries, 3)
	if len(got) != 3 {
		t.Fatalf("got %d records, want 3", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" {
		t.Errorf("not sorted: %v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
	// Nil slices would serialize as null; the reader expects a list.
	for _, r := range got {
		if r.Holes == nil || r.Pins == nil {
			t.Errorf("%s has a nil port slice", r.ID)
		}
	}
}

func TestRecordsHonorTheTierCeiling(t *testing.T) {
	entries := map[string]*Entry{
		"beam":  {Part: "beam", Title: "Technic Beam  3", Holes: []Port{{}}},
		"panel": {Part: "panel", Title: "Technic Panel Fairing #1", Holes: []Port{{}}},
		"brick": {Part: "brick", Title: "Brick  2 x  4", Holes: []Port{{}}},
	}
	if got := Records(entries, 1); len(got) != 1 || got[0].ID != "beam" {
		t.Errorf("tier 1 gave %v", ids(got))
	}
	if got := Records(entries, 2); len(got) != 2 {
		t.Errorf("tier 2 gave %v", ids(got))
	}
	if got := Records(entries, 3); len(got) != 3 {
		t.Errorf("tier 3 gave %v", ids(got))
	}
}

func TestRecordsDropPortlessParts(t *testing.T) {
	entries := map[string]*Entry{"empty": {Part: "empty", Title: "Technic Beam  3"}}
	if got := Records(entries, 3); len(got) != 0 {
		t.Errorf("got %v, want nothing", ids(got))
	}
}

// numpy rounds half to even, and the catalogs are compared value by value, so
// half-away-from-zero would disagree on any coordinate landing on a half.
func TestRoundingMatchesNumpy(t *testing.T) {
	// Values verified against numpy directly. 2.675 rounding up rather than
	// down is not a half-to-even decision: 2.675 * 100 lands on exactly 267.5
	// in float64, and numpy scales the same way, artifact included.
	cases := []struct{ in, want float64 }{
		{0.125, 0.12}, {0.135, 0.14}, {-0.125, -0.12}, {2.675, 2.68},
	}
	for _, c := range cases {
		if got := roundTo(c.in, 2); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("roundTo(%v, 2) = %v, want %v", c.in, got, c.want)
		}
	}
	// Half to even where the value really is a half. Away-from-zero would give
	// 1 and 3 here.
	for _, c := range []struct{ in, want float64 }{{0.5, 0}, {1.5, 2}, {2.5, 2}} {
		if got := roundTo(c.in, 0); got != c.want {
			t.Errorf("roundTo(%v, 0) = %v, want %v", c.in, got, c.want)
		}
	}
}

func identity() geom.Mat3 { return geom.Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} }

func keys(m map[string]*Entry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func ids(rs []Record) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}
