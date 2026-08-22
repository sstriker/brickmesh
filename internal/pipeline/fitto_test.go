// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
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
	read := Inspect(parts)
	solids := SolidsOf(res.Layout, res.Stations, deps.Rast, res.ringSites, res.slip)
	fits := FitToIn(res.Layout, read.Bearings(deps.Shadow),
		read.Occupied(deps.Rast), solids, 1)
	if len(fits) == 0 {
		t.Fatal("its own frame offered nowhere to put it")
	}
	if fits[0].Clashes != 0 {
		t.Errorf("%d of its gears read as inside the frame built to clear them",
			fits[0].Clashes)
	}
	if fits[0].Offset != (geom.Vec3{}) {
		t.Errorf("it should fit where it already is, not at %v", fits[0].Offset)
	}
}

// And a solid model has nowhere to put anything, which the fitter must say
// rather than offering the least bad spot as though it were a spot.
func TestSomethingSolidHasNoRoom(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "reduction.json"))

	// Fill everything the mechanism would occupy.
	solids := SolidsOf(res.Layout, res.Stations, deps.Rast, res.ringSites, res.slip)
	full := map[geom.Cell]bool{}
	for _, s := range solids {
		for _, c := range s.Cells {
			full[c] = true
		}
	}
	bearings := []Bearing{}
	for id, place := range res.Layout.Place {
		_ = id
		bearings = append(bearings, Bearing{
			At: place.Point.Scale(10), Axis: place.Direction.Unit(),
			Holes: 2, To: 100, Parts: 2,
		})
	}
	fits := FitToIn(res.Layout, bearings, full, solids, 1)
	if len(fits) == 0 {
		t.Fatal("expected placements to be offered and scored")
	}
	if fits[0].Clashes == 0 {
		t.Error("every gear sits in filled space and none was reported as clashing")
	}
}
