// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"github.com/sstriker/brickmesh/internal/spec"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/voxel"
)

// A model fits its own mechanism, exactly, without moving.
//
// The one case where the answer is known: the frame was built for these shafts,
// so every one of them lands on a line it bears, and the offset is nothing. A
// fitter that cannot find that is not going to find anything harder.
func TestAModelFitsItsOwnMechanismWithoutMoving(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{"reduction", "gearbox-2-speed", "gearbox-first-system"} {
		t.Run(name, func(t *testing.T) {
			res := runSpec(t, deps, filepath.Join("..", "..", "examples", name+".json"))
			parts, err := ldr.Decode(strings.NewReader(res.Model.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			fits := FitTo(res.Layout, Inspect(parts).Bearings(deps.Shadow), 1)
			if len(fits) == 0 {
				t.Fatal("its own frame offered nowhere to put it")
			}
			best := fits[0]
			if best.Borne != best.Total {
				t.Errorf("its own frame bears %d of %d shaft(s): %v",
					best.Borne, best.Total, best.On)
			}
			if best.Offset != (geom.Vec3{}) {
				t.Errorf("it had to be moved by %v to fit the frame built for "+
					"it; the best placement should be where it already is",
					best.Offset)
			}
		})
	}
}

// And the score means something: a frame with nothing running the right way
// bears nothing, and says so rather than reporting a placement.
func TestAFrameThatBearsNothingSaysSo(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "reduction.json"))

	// Bearings across the shafts rather than along them.
	across := []Bearing{
		{At: geom.Vec3{}, Axis: geom.Vec3{Y: 1}, Holes: 2, From: 0, To: 100, Parts: 2},
	}
	fits := FitTo(res.Layout, across, 1)
	for _, f := range fits {
		if f.Borne > 0 {
			t.Errorf("a bearing at right angles to every shaft bore %d of them",
				f.Borne)
		}
	}
	for _, fi := range ReportFit(res.Layout, across) {
		if fi.Level == "OK" && strings.Contains(fi.Detail, "best placement") {
			t.Error("it should not offer a best placement when nothing is borne")
		}
	}
}

// A model fits its own mechanism without any of it ending up inside the frame.
//
// The frame was built to clear those gears, so a fitter that reports a clash
// there is measuring contact rather than overlap — which is the mistake the
// voxel grid invites, since it marks every cell a part so much as touches.
func TestItsOwnMechanismDoesNotClashWithItsOwnFrame(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "gearbox-2-speed.json"))
	parts, err := ldr.Decode(strings.NewReader(res.Model.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	// Its own mechanism taken out, since the question is whether the FRAME has
	// room for it: with the gears left in, every one of them is in the way and
	// every one of them is a gear the answer is about.
	read := Inspect(parts).WithoutMechanism()
	solids := SolidsOf(res.Layout, res.Stations, deps.Rast, res.ringSites, res.slip)
	// Where it already is must be among the placements that work — not
	// necessarily the one chosen. A frame that bears a mechanism at one place
	// along its shafts usually bears it at the next stud too, and between two
	// that both work the fitter prefers the one touching least. Insisting on
	// the offset being nothing asserts that tiebreak rather than the fit.
	fits := FitToIn(res.Layout, read.Bearings(deps.Shadow),
		read.Occupied(deps.Rast), solids, 0)
	if len(fits) == 0 {
		t.Fatal("its own frame offered nowhere to put it")
	}
	if fits[0].Clashes != 0 {
		t.Errorf("the best placement puts %d gear(s) inside the frame built to "+
			"clear them", fits[0].Clashes)
	}
	found := false
	for _, f := range fits {
		if f.Offset == (geom.Vec3{}) && f.Borne == f.Total && f.Clashes == 0 {
			found = true
		}
	}
	if !found {
		t.Error("the frame built for this mechanism does not accept it where " +
			"it already is")
	}
}

// Space that is filled reads as filled.
//
// Asked of the clash test rather than of the ranking: the ranking is allowed to
// slide the mechanism somewhere else, and does, which is the right answer to a
// different question. This one is whether a gear standing in occupied space is
// seen at all.
func TestAGearInFilledSpaceIsSeen(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "reduction.json"))

	solids := SolidsOf(res.Layout, res.Stations, deps.Rast, res.ringSites, res.slip)
	if len(solids) == 0 {
		t.Fatal("a reduction has gears; none were rasterised")
	}
	full := map[geom.Cell]bool{}
	for _, s := range solids {
		for _, c := range s.Cells {
			full[c] = true
		}
	}
	if got, _ := clashesAt(solids, full, geom.Vec3{}); got != len(solids) {
		t.Errorf("%d of %d gears read as clashing with space they fill exactly",
			got, len(solids))
	}
	// And moved well clear of it, none of them do.
	away := geom.Vec3{X: 400, Y: 400, Z: 400}
	if got, _ := clashesAt(solids, full, away); got != 0 {
		t.Errorf("%d gear(s) still clash after moving twenty studs away", got)
	}
}

// The whole thing: a mechanism placed inside somebody else's model, written out
// as a copy of that model with the mechanism in it, and read back to check it
// still does what it was asked for.
func TestAMechanismIsBuiltIntoTheModelItWasFittedTo(t *testing.T) {
	deps := requireLibraries(t)

	// Two nine-hole beams, five studs apart, holes facing along y — so a shaft
	// runs between them. Written out rather than taken from anywhere, because
	// the point is a model this engine did not build.
	chassis := `0 a chassis
1 4 0 0 0 1 0 0 0 1 0 0 0 1 40490.dat
1 4 0 100 0 1 0 0 0 1 0 0 0 1 40490.dat
`
	parts, err := ldr.Decode(strings.NewReader(chassis))
	if err != nil {
		t.Fatal(err)
	}
	read := Inspect(parts)
	into := &FitInto{
		Parts:    parts,
		Bearings: read.Bearings(deps.Shadow),
		Occupied: read.Occupied(deps.Rast),
		Rast:     deps.Rast,
	}
	if len(into.Bearings) == 0 {
		t.Fatal("the chassis offers no bearing; the rest of this proves nothing")
	}

	res := runSpecInto(t, deps, filepath.Join("..", "..", "examples", "reduction.json"), into)

	// Its parts are still there, at their own positions.
	beams := 0
	for _, p := range res.Model.Parts {
		if p.Name == "40490.dat" {
			beams++
		}
	}
	if beams != 2 {
		t.Errorf("the model it was fitted into has 2 beams and the result has %d; "+
			"what is written should be a copy of somebody's build with a "+
			"mechanism added", beams)
	}

	// And the mechanism is in it, clear of everything.
	gears := 0
	for _, p := range res.Model.Parts {
		if _, _, ok := gearFromLabel(p.Label); ok {
			gears++
		}
	}
	if gears != 2 {
		t.Errorf("a reduction has two gears and the result has %d", gears)
	}
	for _, f := range res.Findings {
		if f.Level == "FAIL" {
			t.Errorf("%s: %s", f.Check, f.Detail)
		}
	}

	// The chassis bears both shafts, so it should not have been given a frame.
	for _, p := range res.Model.Parts {
		if p.Name == "32523.dat" || p.Name == "32316.dat" {
			t.Errorf("%s was added: the chassis already bears every shaft, so "+
				"there is nothing for a frame to do", p.Name)
		}
	}
}

func runSpecInto(t *testing.T, deps Deps, path string, into *FitInto) *Result {
	t.Helper()
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := spec.Read(strings.NewReader(string(doc)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := sp.Build()
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), m, deps, Options{
		Restarts: 8, Seed: 1, Into: into,
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestABearingOnlyHoldsWhereItReaches(t *testing.T) {
	// A wall pair a hundred LDU long, on the Y axis at the origin.
	idx := indexBearings([]Bearing{{
		At: geom.Vec3{}, Axis: geom.Vec3{Y: 1},
		Holes: 2, Parts: 2, From: 0, To: 100,
	}})
	at, dir := geom.Vec3{}, geom.Vec3{Y: 1}

	if !idx.near(at, dir, 10, 60) {
		t.Error("a mechanism between the walls is not held by them")
	}
	if idx.near(at, dir, -220, -120) {
		t.Error("a mechanism 120 LDU clear of the walls claims to be held; " +
			"the line runs for ever but the walls do not")
	}
}

func TestTheFitAccountsForWhatRidesTheShaft(t *testing.T) {
	// A driving ring is 36 LDU across, fatter than most of the gears it sits
	// between, and was once left out of the fit's clash test entirely.
	deps := requireLibraries(t)
	rast := voxel.NewRasterizer(deps.Lib)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "gearbox-2-speed.json"))
	stations, _ := layout.SolveStations(res.Layout.Mech, res.Layout)
	sites := sitesFor(res, stations)
	if len(sites) == 0 {
		t.Fatal("a two-speed with no ring site to place a ring at")
	}
	solids := SolidsOf(res.Layout, stations, rast, sites, nil)

	var rings int
	for _, s := range solids {
		for _, site := range sites {
			if s.Part == site.system.Ring {
				rings++
			}
		}
	}
	if rings == 0 {
		t.Errorf("%d solids and not one of them a driving ring; the fit is "+
			"blind to the fattest part on the shaft", len(solids))
	}
}

func TestEverySolidKnowsWhatItIsAndHowItTurns(t *testing.T) {
	// The exact confirm needs the part named, not only sampled, and needs its
	// spin: a gear that clears a beam at rest may not clear it a few degrees on.
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "gearbox-2-speed.json"))
	stations, _ := layout.SolveStations(res.Layout.Mech, res.Layout)
	solids := SolidsOf(res.Layout, stations, voxel.NewRasterizer(deps.Lib),
		sitesFor(res, stations), nil)
	if len(solids) == 0 {
		t.Fatal("no solids at all")
	}
	for _, s := range solids {
		if s.Part == "" {
			t.Error("a solid with no part name cannot be put to the exact test")
		}
		if s.Spin == (geom.Vec3{}) {
			t.Errorf("%s has no spin axis, so it will be swept as if it stood still", s.Part)
		}
	}
}
