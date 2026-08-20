// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package synth

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/ldraw"
	"github.com/sstriker/brickmesh/internal/part"
	"github.com/sstriker/brickmesh/internal/rigidity"
	"github.com/sstriker/brickmesh/internal/voxel"
)

// A stand-in for the shadow library: the fixture beam's holes run along Y,
// across its length. Offline throughout — the geometry comes from the same
// synthetic parts the Python suite uses.
type axisAlongY struct{}

func (axisAlongY) RotationAxis(string) (geom.Vec3, string, bool) {
	return geom.Vec3{Y: 1}, "test", true
}

// The holes themselves, which is what the search asks for now: five in a line
// one stud apart, all facing along Y, which is what the fixture beam is.
func (axisAlongY) Holes(name string) []part.Hole {
	out := make([]part.Hole, 0, 5)
	for _, off := range part.HoleOffsets(5) {
		out = append(out, part.Hole{Pos: off, Axis: geom.Vec3{Y: 1}})
	}
	return out
}

var testInventory = []part.Beam{{Part: "fixbeam", Holes: 5}}

func searcher(t *testing.T) *Searcher {
	t.Helper()
	lib := ldraw.New(filepath.Join("..", "..", "tests", "fixtures", "ldraw"))
	lib.Offline = true
	return NewSearcher(voxel.NewRasterizer(lib), axisAlongY{}, testInventory)
}

// oneShaft is a shaft through the origin along Y, with nothing on it.
func oneShaft() *layout.Layout {
	return &layout.Layout{Place: map[string]layout.Placement{
		"s": layout.NewPlacement(geom.Vec3{}, geom.Vec3{Y: 1}),
	}}
}

func TestBearingRequirementsSitAtTheEndsOfTheFreeStretch(t *testing.T) {
	reqs := BearingRequirements(oneShaft(), nil, 2, 8)
	if len(reqs) != 2 {
		t.Fatalf("got %d requirements, want 2", len(reqs))
	}
	// Far apart: a short bearing base lets the shaft whip anyway.
	if reqs[0].Point.Y != -80 || reqs[1].Point.Y != 80 {
		t.Errorf("bearings at %v and %v, want the ends of the free stretch",
			reqs[0].Point, reqs[1].Point)
	}
	for _, r := range reqs {
		if !r.Point.OnLattice(HalfStud) {
			t.Errorf("%v is off the half-stud lattice", r.Point)
		}
	}
}

func TestAGearLeavesLessRoomForBearings(t *testing.T) {
	stations := []layout.Station{{Shaft: "s", Teeth: 24, Axial: 0, Thickness: 2}}
	reqs := BearingRequirements(oneShaft(), stations, 2, 8)
	if len(reqs) != 2 {
		t.Fatalf("got %d requirements, want 2", len(reqs))
	}
	// The gear occupies -1..1, so the bearings still go to the extremes.
	if reqs[0].Point.Y != -80 || reqs[1].Point.Y != 80 {
		t.Errorf("got %v and %v", reqs[0].Point, reqs[1].Point)
	}
}

func TestRequirementsOnACommonLineAreDeduplicated(t *testing.T) {
	// Differential ports share a line, so their bearing requirements coincide.
	l := &layout.Layout{Place: map[string]layout.Placement{
		"case":  layout.NewPlacement(geom.Vec3{}, geom.Vec3{Y: 1}),
		"left":  layout.NewPlacement(geom.Vec3{}, geom.Vec3{Y: 1}),
		"right": layout.NewPlacement(geom.Vec3{}, geom.Vec3{Y: 1}),
	}}
	if got := BearingRequirements(l, nil, 2, 8); len(got) != 2 {
		t.Errorf("got %d requirements for three ports on one line, want 2", len(got))
	}
}

func TestCandidatesPutAHoleOnThePoint(t *testing.T) {
	s := searcher(t)
	point := geom.Vec3{X: 20, Y: 0, Z: 40}
	cands, err := s.CandidatesFor(point, geom.Vec3{Y: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("no candidates")
	}
	for _, c := range cands {
		if !c.Origin.OnLattice(HalfStud) {
			t.Errorf("%+v is off the lattice", c)
		}
		holes, axis, err := worldHoles(s.Ports, c)
		if err != nil {
			t.Fatal(err)
		}
		// The hole axis has to lie along the shaft, or the shaft cannot turn.
		if got := axis.Dot(geom.Vec3{Y: 1}); got < 0.999 && got > -0.999 {
			t.Errorf("hole axis %v is not along the shaft", axis)
		}
		found := false
		for _, h := range holes {
			if h.Sub(point).Len() < 1e-6 {
				found = true
			}
		}
		if !found {
			t.Errorf("%+v has no hole on %v", c, point)
		}
	}
}

func TestCandidatesRefuseAnOffLatticePoint(t *testing.T) {
	s := searcher(t)
	// 3 LDU is neither a stud nor a half stud.
	got, err := s.CandidatesFor(geom.Vec3{X: 3}, geom.Vec3{Y: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d candidates for an off-lattice point, want none", len(got))
	}
}

func TestSynthesizeCoversEveryRequirement(t *testing.T) {
	s := searcher(t)
	l := oneShaft()
	sols, err := s.Synthesize(context.Background(), l, nil, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(sols) == 0 {
		t.Fatal("no structure found")
	}

	reqs := BearingRequirements(l, nil, 2, 8)
	for _, sol := range sols {
		for _, r := range reqs {
			borne := false
			for _, p := range sol.Parts {
				ports, err := part.WorldPorts(s.Ports, p)
				if err != nil {
					t.Fatal(err)
				}
				for _, h := range positions(ports) {
					if h.Sub(r.Point).Len() < 1e-6 {
						borne = true
					}
				}
			}
			if !borne {
				t.Errorf("no part bears the requirement at %v", r.Point)
			}
		}
	}
}

func TestSolutionsComeBackSmallestFirst(t *testing.T) {
	s := searcher(t)
	sols, err := s.Synthesize(context.Background(), oneShaft(), nil, Options{Restarts: 8, Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(sols); i++ {
		if sols[i-1].Count > sols[i].Count {
			t.Errorf("solution %d uses more parts than %d", i-1, i)
		}
	}
}

// The restarts run in parallel, so the result must not depend on which worker
// happened to take which attempt.
func TestSynthesizeIsReproducibleForASeed(t *testing.T) {
	first, err := searcher(t).Synthesize(context.Background(), oneShaft(), nil,
		Options{Restarts: 8, Seed: 42, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	second, err := searcher(t).Synthesize(context.Background(), oneShaft(), nil,
		Options{Restarts: 8, Seed: 42, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("%d solutions with four workers, %d with one", len(first), len(second))
	}
	for i := range first {
		if first[i].Count != second[i].Count || len(first[i].Parts) != len(second[i].Parts) {
			t.Errorf("solution %d differs between worker counts", i)
		}
	}
}

func TestSynthesizeWithNothingToBearReturnsNothing(t *testing.T) {
	empty := &layout.Layout{Place: map[string]layout.Placement{}}
	sols, err := searcher(t).Synthesize(context.Background(), empty, nil, Options{Restarts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(sols) != 0 {
		t.Errorf("got %d solutions for an empty layout", len(sols))
	}
}

func TestStudSpanAcceptsOnlyStraightWholeStudRuns(t *testing.T) {
	cases := []struct {
		a, b geom.Vec3
		want int
		ok   bool
	}{
		{geom.Vec3{}, geom.Vec3{Z: 40}, 2, true},
		{geom.Vec3{}, geom.Vec3{X: 20}, 1, true},
		{geom.Vec3{}, geom.Vec3{Z: 30}, 0, false},        // not a whole stud
		{geom.Vec3{}, geom.Vec3{X: 20, Z: 20}, 0, false}, // diagonal
		{geom.Vec3{}, geom.Vec3{}, 0, false},             // the same hole
	}
	for _, c := range cases {
		got, ok := studSpan(c.a, c.b)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("studSpan(%v, %v) = %d, %v; want %d, %v",
				c.a, c.b, got, ok, c.want, c.ok)
		}
	}
}

func TestConnectorsSpanTwoHolesOnALine(t *testing.T) {
	s := searcher(t)
	a := map[geom.Vec3]bool{{}: true}
	b := map[geom.Vec3]bool{{Z: 40}: true}
	got, err := s.ConnectorsBetween(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("nothing spans two holes two studs apart")
	}
	// Every connector has to reach both — either landing on the holes, or
	// lying alongside within a pin's reach along the hole axis, which is how
	// two parts are actually joined.
	for _, p := range got {
		holes, axis, err := worldHoles(s.Ports, p)
		if err != nil {
			t.Fatal(err)
		}
		if !reachesByPin(holes, axis, geom.Vec3{}) || !reachesByPin(holes, axis, geom.Vec3{Z: 40}) {
			t.Errorf("%+v reaches neither hole, even by pin", p)
		}
	}
}

// reaches reports whether any of a part's holes could take a pin to the target:
// on the same axis line, and within a pin's span along it.
func reachesByPin(holes []geom.Vec3, axis, target geom.Vec3) bool {
	for _, h := range holes {
		d := target.Sub(h)
		along := d.Dot(axis)
		across := d.Sub(axis.Scale(along))
		if across.Len() < 1e-6 && math.Abs(along) <= rigidity.PinReach+1e-6 {
			return true
		}
	}
	return false
}

func TestNothingSpansHolesThatDoNotLineUp(t *testing.T) {
	s := searcher(t)
	a := map[geom.Vec3]bool{{}: true}
	b := map[geom.Vec3]bool{{X: 20, Z: 20}: true} // diagonal
	got, err := s.ConnectorsBetween(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d connectors for a diagonal pair, want none", len(got))
	}
}

func TestRepairLeavesAConnectedStructureAlone(t *testing.T) {
	s := searcher(t)
	// Two beams overlapping by two holes: already one piece.
	parts := []Placed{
		{Part: "fixbeam", Rot: 0, Origin: geom.Vec3{}},
		{Part: "fixbeam", Rot: 0, Origin: geom.Vec3{Z: part.Stud}},
	}
	got, err := s.RepairConnectivity(parts)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(parts) {
		t.Errorf("added %d parts to an already connected structure", len(got)-len(parts))
	}
}

func positions(ports []part.Hole) []geom.Vec3 {
	out := make([]geom.Vec3, len(ports))
	for i, h := range ports {
		out[i] = h.Pos
	}
	return out
}

// worldHoles is the positions and the shared axis of a placed part's holes.
//
// part.WorldHoles said the same thing and is deprecated: it laid holes out from
// a count rather than reading them, so it could not describe a part whose holes
// face more than one way. These fixtures are all single-axis beams, so the
// question still has one answer, and asking WorldPorts for it keeps the tests
// on the API the engine uses.
func worldHoles(src part.Holes, p part.Placed) ([]geom.Vec3, geom.Vec3, error) {
	ports, err := part.WorldPorts(src, p)
	if err != nil {
		return nil, geom.Vec3{}, err
	}
	pts := make([]geom.Vec3, 0, len(ports))
	for _, h := range ports {
		pts = append(pts, h.Pos)
	}
	return pts, ports[0].Axis, nil
}
