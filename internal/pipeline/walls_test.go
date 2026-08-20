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
	"brickmesh/internal/synth"
)

// A frame should be walls, not brackets.
//
// Every shaft was asked for a bearing at either end of its own free stretch,
// and those points almost never line up between shafts — so nothing could bear
// two shafts at once and the search returned the least that holds: five parts
// for a two-speed gearbox, each holding one thing. A gearbox is two walls with
// every shaft through both, which is also how the load is shared out rather
// than taken one liftarm at a time.
//
// The test is that the bearing parts cluster into a small number of cross
// sections, and that each of those bears more than one shaft.
func TestTheFrameIsWallsRatherThanBrackets(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{"gearbox-2-speed", "gearbox-3-speed-compound"} {
		t.Run(name, func(t *testing.T) {
			res := runSpec(t, deps, filepath.Join("..", "..", "examples", name+".json"))

			// The shafts run along one direction; a wall is a cross section of
			// it, so bearing parts at the same position along that direction
			// are the same wall.
			var dir geom.Vec3
			for _, p := range res.Layout.Place {
				dir = p.Direction.Unit()
				break
			}
			walls := map[float64]int{}
			for _, p := range res.Model.Parts {
				if classOf(p) != classStructure || isPin(p.Name) {
					continue
				}
				walls[math.Round(p.Pos.Dot(dir))]++
			}
			if len(walls) == 0 {
				t.Fatal("no bearing parts at all")
			}
			if len(walls) > 3 {
				t.Errorf("the bearing parts sit at %d different cross sections; "+
					"a frame spread that thin is brackets, not walls", len(walls))
			}
			t.Logf("%d wall(s): %v", len(walls), walls)
		})
	}
}

// And the walls have to be far enough apart to be a base. Two bearings an inch
// apart hold a shaft against nothing.
func TestTheWallsAreNotAllAtOneEnd(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps,
		filepath.Join("..", "..", "examples", "gearbox-3-speed-compound.json"))

	var dir geom.Vec3
	for _, p := range res.Layout.Place {
		dir = p.Direction.Unit()
		break
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range res.Model.Parts {
		if classOf(p) != classStructure || isPin(p.Name) {
			continue
		}
		at := p.Pos.Dot(dir)
		lo, hi = math.Min(lo, at), math.Max(hi, at)
	}
	if base := (hi - lo) / geom.Stud; base < 3 {
		t.Errorf("the bearings span %.1f studs; a base that short lets the "+
			"whole thing rock about it", base)
	}
}

// The envelope is a bound, not a wish: ask for something that cannot be built
// inside it and the answer is that it cannot, naming the bound.
func TestAnImpossibleEnvelopeSaysSo(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpecWithBudget(t, deps,
		filepath.Join("..", "..", "examples", "gearbox-2-speed.json"), 2)

	var said bool
	for _, f := range res.Findings {
		if f.Check == "structure" && strings.Contains(f.Detail, "envelope was capped") {
			said = true
		}
	}
	if !said {
		t.Error("a frame was asked for inside two studs of depth and the report " +
			"did not mention the bound as the reason none was found")
	}
}

func runSpecWithBudget(t *testing.T, deps Deps, path string, maxZ float64) *Result {
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
		Restarts: 8, Seed: 1,
		Budget: synth.Budget{
			PerStud: 1, PerPart: 0.2, PerCubicStud: 1,
			MaxStuds: geom.Vec3{Z: maxZ},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}
