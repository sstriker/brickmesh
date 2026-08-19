// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/spec"
)

// The frame has to stay around the mechanism it holds.
//
// This is here because it did not. The rigidity report counts the shafts as
// what ties the bearings on a line together, and the structural search did not,
// so the search believed a reduction's two bearings were loose pieces and
// braced them until Grubler was satisfied: three 13-hole beams marching 35
// studs off the end of a 10-stud mechanism, each pinned to the last and none of
// them reaching the far bearing, which the axle had been holding all along.
//
// Nothing failed. The structure was connected, it was rigid, no two parts
// shared space, and every check said OK. It was visible only once the model was
// drawn. So the property is written down: a bearing sits near what it bears.
func TestTheFrameStaysAroundTheMechanism(t *testing.T) {
	deps := requireLibraries(t)
	for _, path := range examples(t) {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".json"), func(t *testing.T) {
			res := runSpec(t, deps, path)

			// The space the turning parts actually fill is what the frame
			// exists to hold — their geometry, not their origins. An axle is
			// ten studs long and sits at the middle of them, and a frame
			// measured against that point would look like an overhang.
			lo, hi := geom.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)},
				geom.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
			turning := 0
			for _, p := range res.Model.Parts {
				if classOf(p) == classStructure {
					continue
				}
				plo, phi, err := placedBox(deps, p)
				if err != nil {
					t.Fatal(err)
				}
				turning++
				lo, _ = expand(lo, hi, plo)
				_, hi = expand(lo, hi, phi)
			}
			if turning == 0 {
				t.Skip("nothing turns in this one")
			}

			// A generous allowance: a bearing lies alongside what it carries,
			// and a beam tying two of them together runs past both. Three studs
			// of margin is room for all of that, and the failure it is there to
			// catch overshot by thirty-five.
			const margin = 3 * geom.Stud
			for _, p := range res.Model.Parts {
				if classOf(p) != classStructure {
					continue
				}
				plo, phi, err := placedBox(deps, p)
				if err != nil {
					t.Fatal(err)
				}
				out := math.Max(outside(lo, hi, plo, margin), outside(lo, hi, phi, margin))
				if out > 0 {
					t.Errorf("%s at %+v reaches %.0f LDU (%.1f studs) beyond the "+
						"mechanism it is meant to hold, which spans %+v..%+v",
						p.Name, p.Pos, out, out/geom.Stud, lo, hi)
				}
			}
		})
	}
}

// And the control: the stiffening that produced the appendage still runs, so
// the test above is not passing because bracing quietly stopped happening.
func TestStiffeningStillAddsBeamsWhereTheyAreNeeded(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "gearbox-2-speed.json"))
	added := false
	for _, f := range res.Findings {
		if f.Check == "structure" && strings.Contains(f.Detail, "to stop it hinging") {
			added = true
		}
	}
	if !added {
		t.Error("no beam was added to a two-speed gearbox; the frame is two " +
			"bearing walls and a shaft line, and something has to stop them " +
			"folding")
	}
}

func runSpec(t *testing.T, deps Deps, path string) *Result {
	t.Helper()
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := spec.Read(strings.NewReader(string(doc)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Build()
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), m, deps, Options{Restarts: 8, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model == nil {
		t.Fatal("no model")
	}
	return res
}

func expand(lo, hi, p geom.Vec3) (geom.Vec3, geom.Vec3) {
	return geom.Vec3{X: math.Min(lo.X, p.X), Y: math.Min(lo.Y, p.Y), Z: math.Min(lo.Z, p.Z)},
		geom.Vec3{X: math.Max(hi.X, p.X), Y: math.Max(hi.Y, p.Y), Z: math.Max(hi.Z, p.Z)}
}

// outside is how far a point lies beyond a box grown by margin, along whichever
// axis it is furthest out.
func outside(lo, hi, p geom.Vec3, margin float64) float64 {
	worst := 0.0
	for _, a := range [][3]float64{{lo.X, hi.X, p.X}, {lo.Y, hi.Y, p.Y}, {lo.Z, hi.Z, p.Z}} {
		worst = math.Max(worst, math.Max(a[0]-margin-a[2], a[2]-(a[1]+margin)))
	}
	return worst
}
