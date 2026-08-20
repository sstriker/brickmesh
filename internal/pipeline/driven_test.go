// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/spec"
)

// What turns, and what does not.
//
// The rule, and it has to hold both ways round: a gear that turns turns
// whatever it meshes with; a shaft keyed to a turning gear turns, whether it is
// keyed through an axle hole or through an engaged driving ring; and anything
// nothing reaches does not turn at all.
//
// The last clause is the one that was wrong. During a shift every ring is
// between gears, so nothing passes through one — and the animation kept turning
// the shafts beyond them at the ratio they had before, which draws a drive that
// is not there.
//
// alwaysDriven answers it for the whole graph rather than for the shaft a ring
// happens to ride, which is the difference that matters in a compound gearbox:
// the second stage's gears are driven by the first stage's output, so they stop
// when it stops.
func TestWhatTheInputsReachWithNoShiftEngaged(t *testing.T) {
	deps := requireLibraries(t)
	m := mechOf(t, filepath.Join("..", "..", "examples", "gearbox-3-speed-compound.json"))
	_ = deps

	got := alwaysDriven(m)
	// input drives the two stage-one gears through fixed meshes. Everything
	// past the first ring is unreachable until something engages.
	want := map[string]bool{
		"input": true, "s1low": true, "s1high": true,
		"mid": false, "output": false, "s2low": false, "s2high": false,
	}
	for id, expected := range want {
		if got[id] != expected {
			t.Errorf("%s: reached=%v, want %v", id, got[id], expected)
		}
	}
	if !got["s1low"] || got["s2low"] {
		t.Errorf("the stage two gears are driven by the stage one output, so "+
			"they must stop when it does: s1low=%v s2low=%v",
			got["s1low"], got["s2low"])
	}
}

// A differential needs two of its three shafts known before it says anything
// about the third. One is not enough — the other two are free to turn against
// each other, which is the whole of what the part is for.
func TestADifferentialNeedsTwoOfThree(t *testing.T) {
	both := mech.New("both outputs driven")
	both.Shaft("case", 2)
	both.Shaft("left", 2)
	both.Shaft("right", 2)
	both.Differential("case", "left", "right")
	both.Drive("left", 1)
	both.Drive("right", 1)
	if got := alwaysDriven(both); !got["case"] {
		t.Error("two outputs driven determine the case, and did not")
	}

	one := mech.New("case driven only")
	one.Shaft("case", 2)
	one.Shaft("left", 2)
	one.Shaft("right", 2)
	one.Differential("case", "left", "right")
	one.Drive("case", 1)
	got := alwaysDriven(one)
	if got["left"] || got["right"] {
		t.Errorf("driving the case alone leaves both outputs free, and this "+
			"claims left=%v right=%v", got["left"], got["right"])
	}
}

// And the flag reaches the script: every shaft the inputs cannot reach is
// marked, and none of the ones they can.
func TestTheScriptHoldsExactlyTheUnreachableShafts(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{"gearbox-2-speed", "gearbox-3-speed-compound"} {
		t.Run(name, func(t *testing.T) {
			m := mechOf(t, filepath.Join("..", "..", "examples", name+".json"))
			res := runSpecAnimated(t, deps,
				filepath.Join("..", "..", "examples", name+".json"))
			always := alwaysDriven(m)

			// Every animation, not just the first. Whether a shaft is reached
			// without a shift is a fact about the graph and not about the state,
			// so all of them have to say the same thing — which is worth
			// checking rather than assuming, since the flag is computed once and
			// copied into each.
			var held, turning []string
			seen := map[string]bool{}
			for _, ani := range res.Script.Animations {
				for _, tg := range ani.Turning {
					id := strings.TrimPrefix(tg.Group, "shaft_")
					was, already := seen[id]
					if already && was != tg.ThroughShift {
						t.Errorf("%s holds in one state and turns in another; "+
							"what a shift can reach does not depend on which "+
							"state it is in", id)
					}
					// Whether the key is there, not what it holds: a shaft that
					// turns maps to false, and testing the value listed it once
					// per state.
					if !already {
						if tg.ThroughShift {
							held = append(held, id)
						} else {
							turning = append(turning, id)
						}
					}
					seen[id] = tg.ThroughShift

					if tg.ThroughShift == always[id] {
						t.Errorf("%s is %sreached with no shift engaged, and the "+
							"script %s it", id,
							map[bool]string{true: "", false: "not "}[always[id]],
							map[bool]string{true: "holds", false: "turns"}[tg.ThroughShift])
					}
				}
			}
			sort.Strings(held)
			sort.Strings(turning)
			t.Logf("turns through a shift: %v; always driven: %v", held, turning)
			if len(held) == 0 {
				t.Error("nothing holds, so a gearbox has no neutral at all")
			}
		})
	}
}

func mechOf(t *testing.T, path string) *mech.Mechanism {
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
	return m
}
