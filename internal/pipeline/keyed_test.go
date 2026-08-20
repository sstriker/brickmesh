// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/ldr"
)

// A part in the frame must not turn.
//
// A cross-shaped hole is what keys a part to a shaft: an axle in one cannot
// spin, so the part goes round with the axle instead, sweeping a circle and
// taking whatever is pinned to it along. A frame member that does that is not
// a frame member.
//
// This used to be asserted one level up, as "no part in the inventory has a
// cross hole", with a note saying that adding one would fail this and point at
// what else had to change. Adding one did, and the answer turned out to be
// nothing: the clearance sweep stopped deciding what turns from what a part is
// called and started propagating it from the joints, so a keyed structural part
// is swept correctly rather than tested standing still.
//
// What remains is the real rule, and it is about placement rather than about
// the inventory. A connector has cross holes and belongs in the frame; laid
// along a shaft it would be driven by it. So the question is asked of the models
// the search actually produces.
func TestNoFramePartEndsUpTurning(t *testing.T) {
	deps := requireLibraries(t)
	for _, path := range examples(t) {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".json"), func(t *testing.T) {
			res := runSpec(t, deps, path)
			spin := checkTurning(res, deps)
			for i, p := range res.Model.Parts {
				if classOf(p) != classStructure {
					continue
				}
				if a, ok := spin.about[i]; ok {
					t.Errorf("%s at %+v is in the frame and turns about %+v. "+
						"A frame that turns holds nothing", p.Name, p.Pos, a.dir)
				}
			}
		})
	}
}

// The control: the parts that are supposed to turn still do, so the test above
// is not passing because the propagation stopped finding anything.
func TestTheThingsThatShouldTurnStillDo(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "gearbox-2-speed.json"))
	spin := checkTurning(res, deps)
	turning := 0
	for i, p := range res.Model.Parts {
		if classOf(p) == classStructure || isAxle(p.Name) {
			continue
		}
		if _, ok := spin.about[i]; ok {
			turning++
		}

	}
	if turning == 0 {
		t.Error("nothing in a two-speed gearbox was found to turn")
	}
}

// And the classification itself, stated plainly so a reader can see what it
// rests on.
func TestWhatCountsAsTurning(t *testing.T) {
	for _, c := range []struct {
		what  ldr.Part
		turns bool
	}{
		{ldr.Part{Name: "3648b.dat", Label: "24t on shaft 'output'"}, true},
		{ldr.Part{Name: "6539.dat"}, true},
		{ldr.Part{Name: "18947.dat"}, true},
		{ldr.Part{Name: "6538a.dat"}, true},
		{ldr.Part{Name: "18948.dat"}, true},
		{ldr.Part{Name: "32523.dat"}, false},
		{ldr.Part{Name: "3707.dat"}, false}, // an axle: it turns, but it is
		// round about its own axis, so sweeping it says nothing
	} {
		if got := turns(c.what); got != c.turns {
			t.Errorf("%s: turns=%v, want %v", c.what.Name, got, c.turns)
		}
	}
}
