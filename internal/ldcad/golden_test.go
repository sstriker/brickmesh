// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package ldcad

import (
	"os"
	"path/filepath"
	"testing"

	"brickmesh/internal/geom"
)

// goldenPath is the script tests/test_animation_lua.py actually runs. Nothing
// here can tell whether the Lua is valid Lua or whether its arithmetic comes
// out right; that test can, because it executes it against a stub of LDCad's
// API. Keeping the file in the tree is what lets the two languages meet.
const goldenPath = "testdata/shift.lua"

// shiftSample is a three-speed gearbox in the shape the pipeline produces: an
// input turning at one, an output whose speed changes with the state, three
// free-running gears, and a ring per state that engages in exactly one of them.
func shiftSample() Script {
	x := geom.Vec3{X: 1}
	ratios := []float64{-1.0 / 3, -0.6, -1}
	states := []string{"1st", "2nd", "3rd"}

	turningIn := func(s int) []Turning {
		out := []Turning{
			{Group: "shaft_input", Axis: x, Speed: 1},
			{Group: "shaft_output", Axis: x, Speed: ratios[s]},
		}
		for i, name := range []string{"first", "second", "third"} {
			out = append(out, Turning{Group: "shaft_" + name, Axis: x, Speed: ratios[i]})
		}
		return out
	}
	slidingIn := func(s int) []Sliding {
		var out []Sliding
		for i := range states {
			at := 1.0
			if i == s {
				at = 0
			}
			out = append(out, Sliding{
				Group: []string{"ring_1", "ring_2", "ring_3"}[i],
				Axis:  x, Speed: ratios[s],
				Engaged:    geom.Vec3{X: 30 + float64(70*i), Z: -40},
				Disengaged: geom.Vec3{X: 40 + float64(70*i), Z: -40},
				At:         at,
			})
		}
		return out
	}

	script := Script{Model: "3-speed gearbox", Seconds: 10, InputTurns: 4}
	for s, state := range states {
		script.Animations = append(script.Animations, Animation{
			Name: state, Turning: turningIn(s), Sliding: slidingIn(s),
		})
	}
	shift := Animation{Name: "shift", Turning: turningIn(0)}
	for s, state := range states {
		shift.Segments = append(shift.Segments, Segment{
			State: state, Turning: turningIn(s), Sliding: slidingIn(s),
		})
	}
	script.Animations = append(script.Animations, shift)
	return script
}

func TestTheGoldenScriptIsCurrent(t *testing.T) {
	got := shiftSample().Render()
	if os.Getenv("BRICKMESH_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("%v — run with BRICKMESH_UPDATE_GOLDEN=1 to write it", err)
	}
	if string(want) != got {
		t.Errorf("%s is out of date. The Python test runs that file, so it has "+
			"to match what the writer emits; rerun with BRICKMESH_UPDATE_GOLDEN=1",
			goldenPath)
	}
}
