// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/mech"
)

// Mirrors tests/test_layout.py.

var (
	xAxis = geom.Vec3{X: 1}
	yAxis = geom.Vec3{Y: 1}
	zAxis = geom.Vec3{Z: 1}
)

func TestPlacementNormalizesItsDirection(t *testing.T) {
	p := NewPlacement(geom.Vec3{}, geom.Vec3{Z: 5})
	if math.Abs(p.Direction.Len()-1) > 1e-9 || p.Direction.Z != 1 {
		t.Errorf("direction = %+v", p.Direction)
	}
}

func TestPlacementKeyIsStable(t *testing.T) {
	a := NewPlacement(geom.Vec3{X: 1, Y: 2, Z: 3}, zAxis).Key()
	b := NewPlacement(geom.Vec3{X: 1, Y: 2, Z: 3}, geom.Vec3{Z: 2}).Key()
	if a != b {
		t.Errorf("%v != %v", a, b)
	}
}

func TestParallelDistanceMeasuresPerpendicularOffset(t *testing.T) {
	a := NewPlacement(geom.Vec3{}, zAxis)
	b := NewPlacement(geom.Vec3{X: 4, Z: 100}, zAxis) // sideways, and along the shaft
	d, ok := ParallelDistance(a, b)
	if !ok || math.Abs(d-4) > 1e-9 {
		t.Errorf("distance = %v, ok = %v; sliding along the shaft must not matter", d, ok)
	}
}

func TestParallelDistanceIsUndefinedForSkewShafts(t *testing.T) {
	if _, ok := ParallelDistance(NewPlacement(geom.Vec3{}, zAxis),
		NewPlacement(geom.Vec3{}, xAxis)); ok {
		t.Error("expected not-parallel")
	}
}

func TestPerpendicularAndIntersecting(t *testing.T) {
	a := NewPlacement(geom.Vec3{}, zAxis)
	b := NewPlacement(geom.Vec3{}, xAxis)
	if !Perpendicular(a, b) || !AxesIntersect(a, b) {
		t.Error("a bevel pair needs both")
	}
	// Lifted apart in Y: still perpendicular, no longer intersecting.
	c := NewPlacement(geom.Vec3{Y: 30}, xAxis)
	if !Perpendicular(a, c) {
		t.Error("still perpendicular")
	}
	if AxesIntersect(a, c) {
		t.Error("lifted apart, they no longer meet")
	}
}

func TestAxesIntersectIsFalseForParallelLines(t *testing.T) {
	if AxesIntersect(NewPlacement(geom.Vec3{}, zAxis),
		NewPlacement(geom.Vec3{X: 10}, zAxis)) {
		t.Error("parallel lines never intersect")
	}
}

func TestLineDistanceHandlesParallelAndSkew(t *testing.T) {
	if got := LineDistance(NewPlacement(geom.Vec3{}, zAxis),
		NewPlacement(geom.Vec3{X: 3, Y: 4, Z: 50}, zAxis)); math.Abs(got-5) > 1e-9 {
		t.Errorf("parallel: %v, want 5", got)
	}
	if got := LineDistance(NewPlacement(geom.Vec3{}, xAxis),
		NewPlacement(geom.Vec3{Z: 7}, yAxis)); math.Abs(got-7) > 1e-9 {
		t.Errorf("skew: %v, want 7", got)
	}
	if got := LineDistance(NewPlacement(geom.Vec3{}, xAxis),
		NewPlacement(geom.Vec3{}, yAxis)); math.Abs(got) > 1e-9 {
		t.Errorf("intersecting: %v, want 0", got)
	}
}

func TestSumOfTwoSquaresFindsLatticeOffsets(t *testing.T) {
	got := map[[2]int]bool{}
	for _, p := range SumOfTwoSquares(25) {
		got[p] = true
		if p[0]*p[0]+p[1]*p[1] != 25 {
			t.Errorf("%v is not a solution", p)
		}
	}
	// The classic buildable offsets.
	for _, want := range [][2]int{{3, 4}, {4, 3}, {5, 0}, {0, 5}, {3, -4}, {-5, 0}} {
		if !got[want] {
			t.Errorf("missing %v", want)
		}
	}
}

func TestSumOfTwoSquaresIsEmptyWhenImpossible(t *testing.T) {
	// 6 is not a sum of two squares, so a pair needing sqrt(6) half studs has
	// nowhere on the lattice to go.
	if got := SumOfTwoSquares(6); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestEffectiveRadiiSumToTheCenterDistance(t *testing.T) {
	for _, p := range [][2]int{{8, 24}, {12, 20}} {
		if got := EffectiveRadius(p[0]) + EffectiveRadius(p[1]); math.Abs(got-4) > 1e-9 {
			t.Errorf("%dt+%dt radii sum to %v, want 4 half studs", p[0], p[1], got)
		}
	}
}

func twoShaftMech(ta, tb int) *mech.Mechanism {
	m := mech.New("pair")
	m.Shaft("a", 2)
	m.Shaft("b", 2)
	m.Mesh("a", "b", ta, tb)
	return m
}

func TestRealizePlacesAPairThatFitsTheLattice(t *testing.T) {
	// 8+24 = 32 -> 4 half studs, and 16 = 0^2+4^2, so positions exist.
	sols := Realize(twoShaftMech(8, 24), Options{MaxSolutions: 3, Span: 1})
	if len(sols) == 0 {
		t.Fatal("no layout found")
	}
	d, ok := ParallelDistance(sols[0].Place["a"], sols[0].Place["b"])
	if !ok || math.Abs(d-4) > 1e-9 {
		t.Errorf("distance = %v, want 4 half studs", d)
	}
}

// 8t+12t needs 2.5 half studs. Squaring and rounding that to 6 used to invent
// candidates at sqrt(6); there is genuinely nowhere to put it.
func TestRealizeRefusesAnOffLatticePairRatherThanMisplacingIt(t *testing.T) {
	if sols := Realize(twoShaftMech(8, 12), Options{MaxSolutions: 3, Span: 1}); len(sols) != 0 {
		t.Errorf("got %d layouts, want none", len(sols))
	}
}

// 36t+40t needs 9.5. Rounding 90.25 to 90 would place the gear at sqrt(90) =
// 9.487: close enough to look right, and wrong.
func TestRealizeDoesNotPlaceTheNineAndAHalfPairAFractionOut(t *testing.T) {
	for _, l := range Realize(twoShaftMech(36, 40), Options{MaxSolutions: 3, Span: 1}) {
		d, _ := ParallelDistance(l.Place["a"], l.Place["b"])
		if math.Abs(d-9.5) > 1e-9 {
			t.Errorf("placed at %v, which does not mesh", d)
		}
	}
}

func TestRealizeSortsCompactFirst(t *testing.T) {
	sols := Realize(twoShaftMech(8, 24), Options{MaxSolutions: 10, Span: 2})
	for i := 1; i < len(sols); i++ {
		if sols[i-1].BBoxVolume() > sols[i].BBoxVolume() {
			t.Errorf("solution %d is bulkier than %d", i-1, i)
		}
	}
}

func TestRealizeKeepsDifferentialPortsOnOneLine(t *testing.T) {
	m := mech.New("diff")
	for _, s := range []string{"case", "left", "right"} {
		m.Shaft(s, 2)
	}
	m.Differential("case", "left", "right")
	sols := Realize(m, Options{MaxSolutions: 1, Span: 1})
	if len(sols) == 0 {
		t.Fatal("no layout")
	}
	p := sols[0].Place
	if p["case"] != p["left"] || p["left"] != p["right"] {
		t.Errorf("the three ports should share a line: %+v", p)
	}
}

func station(shaft string, teeth int, axial, thickness float64) Station {
	return Station{Shaft: shaft, Teeth: teeth, Axial: axial, Thickness: thickness}
}

func TestStationSpanIsCenteredOnItsAxialPosition(t *testing.T) {
	lo, hi := station("a", 24, 4, 2).Span()
	if lo != 3 || hi != 5 {
		t.Errorf("span = %v..%v, want 3..5", lo, hi)
	}
}

// The room left is the room beside the gear, less half a beam at each end: a
// bearing is a point but the beam giving it is a stud thick, and one placed
// hard against a gear ends up half inside it.
func TestFreeIntervalsLeavesRoomBesideAGear(t *testing.T) {
	got := FreeIntervals([]Station{station("a", 24, 0, 2)}, "a", 12)
	want := [][2]float64{{-12, -2}, {2, 12}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFreeIntervalsIgnoresOtherShafts(t *testing.T) {
	stations := []Station{station("a", 24, 0, 2), station("b", 24, 5, 2)}
	got := FreeIntervals(stations, "b", 8)
	// Shaft a's gear is not b's business. Each gap is pulled in by half a beam
	// at the end where b's own gear sits, and left alone at the far end, where
	// there is nothing to be inside of.
	want := [][2]float64{{-8, 3}, {7, 8}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFreeIntervalsMergesOverlappingGears(t *testing.T) {
	// Two gears side by side leave one gap each side, not three.
	stations := []Station{station("a", 24, 0, 2), station("a", 24, 1, 2)}
	got := FreeIntervals(stations, "a", 10)
	want := [][2]float64{{-10, -2}, {3, 10}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestACrowdedShaftHasNowhereForABearing(t *testing.T) {
	if got := FreeIntervals([]Station{station("a", 40, 0, 24)}, "a", 8); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestSolveStationsPutsASpurPairInOnePlane(t *testing.T) {
	m := twoShaftMech(8, 24)
	sols := Realize(m, Options{MaxSolutions: 1, Span: 1})
	if len(sols) == 0 {
		t.Fatal("no layout")
	}
	stations, findings := SolveStations(m, sols[0])
	if len(stations) != 2 {
		t.Fatalf("got %d stations, want 2", len(stations))
	}
	if math.Abs(stations[0].Axial-stations[1].Axial) > 1e-9 {
		t.Errorf("a spur pair has to share an axial plane: %v", stations)
	}
	for _, f := range findings {
		if f.Level == "FAIL" {
			t.Errorf("unexpected failure: %+v", f)
		}
	}
}

// Two pairs sharing a shaft are two different gears at two different places
// along it. Stacking them in one plane was the old behavior and it is what
// makes a multi-speed gearbox impossible to lay out.
func TestPairsSharingAShaftAreSpreadAlongIt(t *testing.T) {
	m := mech.New("two-speed")
	for _, s := range []string{"input", "a", "b"} {
		m.Shaft(s, 2)
	}
	m.Mesh("input", "a", 8, 24)
	m.Mesh("input", "b", 24, 8)

	sols := Realize(m, Options{MaxSolutions: 1, Span: 2})
	if len(sols) == 0 {
		t.Fatal("no layout")
	}
	stations, findings := SolveStations(m, sols[0])

	var onInput []float64
	for _, st := range stations {
		if st.Shaft == "input" {
			onInput = append(onInput, st.Axial)
		}
	}
	if len(onInput) != 2 {
		t.Fatalf("got %d gears on the input shaft, want 2", len(onInput))
	}
	if onInput[0] == onInput[1] {
		t.Errorf("both gears landed at %v; they have to be spread", onInput[0])
	}
	for _, f := range findings {
		if f.Level == "FAIL" {
			t.Errorf("unexpected failure: %+v", f)
		}
	}
}

// Both gears of one pair still share a plane: that equality is real.
func TestBothGearsOfAPairShareAPlane(t *testing.T) {
	m := twoShaftMech(8, 24)
	sols := Realize(m, Options{MaxSolutions: 1, Span: 1})
	if len(sols) == 0 {
		t.Fatal("no layout")
	}
	stations, _ := SolveStations(m, sols[0])
	if len(stations) != 2 {
		t.Fatalf("got %d stations, want 2", len(stations))
	}
	if math.Abs(stations[0].Axial-stations[1].Axial) > 1e-9 {
		t.Errorf("a meshing pair must share a plane, got %v and %v",
			stations[0].Axial, stations[1].Axial)
	}
}

// Overlap detection itself still works when two gears genuinely share space.
func TestOverlapOnAShaftIsReported(t *testing.T) {
	stations := []Station{
		station("a", 24, 0, 2),
		station("a", 24, 0.5, 2),
	}
	findings := checkOverlap(stations, lineLayout())
	if len(findings) != 1 || findings[0].Level != "FAIL" {
		t.Fatalf("got %+v, want one failure", findings)
	}
	if !strings.Contains(findings[0].Detail, "overlap") {
		t.Errorf("detail = %q", findings[0].Detail)
	}
}

func TestGearsThatOnlyTouchAreNotAnOverlap(t *testing.T) {
	// Centers two apart, each two thick: they meet exactly, which is how gears
	// sit side by side on a shaft.
	stations := []Station{station("a", 24, 0, 2), station("a", 24, 2, 2)}
	if got := checkOverlap(stations, lineLayout()); len(got) != 0 {
		t.Errorf("got %+v, want nothing", got)
	}
}

// lineLayout puts shaft "a" on a line so the overlap check can group by it.
func lineLayout() *Layout {
	return &Layout{Place: map[string]Placement{
		"a": NewPlacement(geom.Vec3{}, geom.Vec3{X: 1}),
	}}
}

// A coupling holds two shafts on one axis, so their gears are as much in each
// other's way as if they shared a name. Grouping by shaft name let a gearbox
// stack every ratio in one plane.
func TestCoaxialShaftsShareTheirSlots(t *testing.T) {
	m := mech.New("2-speed")
	for _, s := range []string{"input", "output", "low", "high"} {
		m.Shaft(s, 2)
	}
	m.Mesh("input", "low", 16, 24)
	m.Mesh("input", "high", 24, 16)
	m.State("low")
	m.State("high")
	m.Couple("output", "low", "ring low", "low")
	m.Couple("output", "high", "ring high", "high")

	sols := Realize(m, Options{MaxSolutions: 1, Span: 2})
	if len(sols) == 0 {
		t.Fatal("no layout")
	}
	stations, findings := SolveStations(m, sols[0])
	for _, f := range findings {
		if f.Level == "FAIL" {
			t.Errorf("unexpected failure: %+v", f)
		}
	}

	// The two ratio gears ride on one line and must not be in the same place.
	var lowAxial, highAxial float64
	for _, st := range stations {
		switch st.Shaft {
		case "low":
			lowAxial = st.Axial
		case "high":
			highAxial = st.Axial
		}
	}
	if lowAxial == highAxial {
		t.Errorf("both ratio gears landed at %v; they share an axis", lowAxial)
	}
}

// A shifted gear needs a gap beside it for the ring that engages it.
func TestAShiftedGearGetsRoomForItsRing(t *testing.T) {
	m := mech.New("shifted")
	for _, s := range []string{"input", "output", "a", "b"} {
		m.Shaft(s, 2)
	}
	m.Mesh("input", "a", 16, 24)
	m.Mesh("input", "b", 24, 16)
	m.State("1")
	m.State("2")
	m.Couple("output", "a", "ring", "1")
	m.Couple("output", "b", "ring", "2")

	sols := Realize(m, Options{MaxSolutions: 1, Span: 2})
	if len(sols) == 0 {
		t.Fatal("no layout")
	}
	stations, _ := SolveStations(m, sols[0])

	var axials []float64
	for _, st := range stations {
		if st.Shaft == "a" || st.Shaft == "b" {
			axials = append(axials, st.Axial)
		}
	}
	if len(axials) != 2 {
		t.Fatalf("got %d ratio gears, want 2", len(axials))
	}
	gap := math.Abs(axials[0] - axials[1])
	// Two gears two half studs thick, plus a ring's two between them.
	if gap < 2+RingRoomHalfStuds {
		t.Errorf("gears %v apart; a ring needs %v of room between them",
			gap, RingRoomHalfStuds)
	}
}

// The overlap check has to see across a shared axis too.
func TestOverlapIsSeenAcrossACommonLine(t *testing.T) {
	// Two differently named shafts on the same line, gears in the same place.
	l := &Layout{Place: map[string]Placement{
		"a": NewPlacement(geom.Vec3{}, geom.Vec3{X: 1}),
		"b": NewPlacement(geom.Vec3{}, geom.Vec3{X: 1}),
	}}
	stations := []Station{station("a", 24, 0, 2), station("b", 24, 0, 2)}
	if got := checkOverlap(stations, l); len(got) != 1 || got[0].Level != "FAIL" {
		t.Errorf("got %+v, want one failure: they are the same place", got)
	}
}
