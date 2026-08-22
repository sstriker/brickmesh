// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
)

// A model this engine wrote, read back. It is the one model whose contents are
// known exactly, so it is the only honest test of a reader.
func TestAModelReadsBackWithTheGearsItWasBuiltWith(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{"reduction", "gearbox-2-speed", "gearbox-3-speed-compound"} {
		t.Run(name, func(t *testing.T) {
			res := runSpec(t, deps, filepath.Join("..", "..", "examples", name+".json"))
			parts, err := ldr.Decode(strings.NewReader(res.Model.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			got := Inspect(parts)

			// Every gear the engine placed, found again with its tooth count.
			want := map[int]int{}
			for _, st := range res.Stations {
				want[st.Teeth]++
			}
			have := map[int]int{}
			for _, f := range got.Parts {
				if f.Teeth > 0 {
					have[f.Teeth]++
				}
			}
			for teeth, n := range want {
				if have[teeth] != n {
					t.Errorf("%d gear(s) of %dt were placed and %d read back",
						n, teeth, have[teeth])
				}
			}

			// And every mesh the mechanism has, found as a pair standing right.
			meshes := 0
			for _, l := range res.Layout.Mech.Links {
				if m, ok := l.(mech.Mesh); ok && m.Kind == mech.Spur {
					meshes++
				}
			}
			// Exactly, not at least. A tolerance loose enough for a model a
			// person wrote must still not invent a pair in one this engine
			// wrote, where every position is exact.
			if len(got.Meshes) != meshes {
				t.Errorf("the mechanism has %d spur mesh(es) and reading the "+
					"model back found %d", meshes, len(got.Meshes))
			}
			t.Logf("%d part(s), %d mesh(es) found, %d kind(s) unknown",
				len(got.Parts), len(got.Meshes), len(got.Unknown))
		})
	}
}

// A reader that quietly ignores what it does not know is worse than one that
// says so: a model may be mostly bodywork, and a ratio worked out from the
// third of it that was understood is not a ratio.
func TestWhatIsNotUnderstoodIsReported(t *testing.T) {
	parts := []ldr.Placed{
		{Name: "3001.dat"}, {Name: "3001.dat"}, {Name: "3623.dat"},
	}
	got := Inspect(parts)
	if got.Unknown["3001.dat"] != 2 {
		t.Errorf("two bricks should read as two unknowns, got %d",
			got.Unknown["3001.dat"])
	}
	var said bool
	for _, f := range got.Findings {
		if f.Level == "WARN" && strings.Contains(f.Detail, "not anything this knows") {
			said = true
		}
	}
	if !said {
		t.Error("a reading full of parts it cannot name should say so")
	}
}

// A model this engine wrote should read back with nothing unaccounted for.
// Anything left over is a part it places and cannot name, which is a gap
// between the two halves rather than a fact about the model.
func TestTheEngineUnderstandsItsOwnOutputCompletely(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{
		"reduction", "subtractor", "gearbox-2-speed",
		"gearbox-3-speed-compound", "gearbox-2-speed-auto",
	} {
		t.Run(name, func(t *testing.T) {
			res := runSpec(t, deps, filepath.Join("..", "..", "examples", name+".json"))
			parts, err := ldr.Decode(strings.NewReader(res.Model.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			if got := Inspect(parts); len(got.Unknown) > 0 {
				t.Errorf("its own model came back with %v unrecognised", got.Unknown)
			}
		})
	}
}

// Every part the engine can place, it can also name.
//
// Stronger than reading the examples back, and it does not go stale: adding a
// part to Placeable without teaching Inspect about it fails here rather than
// showing up as a mystery in somebody's reading months later.
func TestEveryPlaceablePartIsRecognised(t *testing.T) {
	var parts []ldr.Placed
	for _, name := range Placeable() {
		parts = append(parts, ldr.Placed{Name: name, Rot: geom.Rotations[0]})
	}
	got := Inspect(parts)
	if len(got.Unknown) > 0 {
		t.Errorf("the engine places these and cannot name them: %v. Either "+
			"teach Inspect what they are or stop placing them", got.Unknown)
	}
}

// The whole round trip: a description becomes a model, the model is written to
// LDraw, and the file is read back by something that never saw the description.
// The ratio has to survive that.
//
// It is the strongest test available here, because the answer is known exactly
// and nothing about the reading is told it.
func TestARatioSurvivesBeingWrittenAndReadBack(t *testing.T) {
	deps := requireLibraries(t)
	for _, c := range []struct {
		name  string
		state string
	}{
		{"reduction", ""},
		{"gearbox-2-speed", "low"},
	} {
		t.Run(c.name, func(t *testing.T) {
			res := runSpec(t, deps, filepath.Join("..", "..", "examples", c.name+".json"))
			want, ok := res.Layout.Mech.Solve(c.state)
			if !ok {
				t.Fatal("the mechanism it was built from does not solve")
			}

			parts, err := ldr.Decode(strings.NewReader(res.Model.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			read := Inspect(parts)
			got, _ := read.Mechanism(deps.Shadow)

			// Which read shaft is the input? The one on the same axis line.
			drive := lineNamed(t, res, read, got, "input")
			out := lineNamed(t, res, read, got, "output")
			got.Drive(drive, want["input"])
			speeds, ok := got.Solve("")
			if !ok {
				t.Fatalf("turning %s does not determine the model read back", drive)
			}
			if math.Abs(speeds[out]-want["output"]) > 1e-6 {
				t.Errorf("built to turn the output at %+.4f and read back at "+
					"%+.4f", want["output"], speeds[out])
			}
		})
	}
}

// lineNamed finds the shaft in a reading that sits on the same axis as a named
// shaft of the mechanism it was built from.
func lineNamed(t *testing.T, res *Result, read *Reading, m *mech.Mechanism, id string) string {
	t.Helper()
	place, ok := res.Layout.Place[id]
	if !ok {
		t.Fatalf("no shaft %q in what it was built from", id)
	}
	want := lineKey(place.Point.Scale(10), place.Direction.Unit())
	i := 0
	keys := sortedLineKeys(read)
	for _, k := range keys {
		if !carriesDrive(read, k) {
			continue
		}
		i++
		if k == want {
			return fmt.Sprintf("line%d", indexOf(keys, k)+1)
		}
	}
	t.Fatalf("the line %q sits on was not found in the reading", id)
	return ""
}
