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
