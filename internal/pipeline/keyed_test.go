// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"strings"
	"testing"

	"brickmesh/internal/ldr"
	"brickmesh/internal/part"
)

// The clearance check decides what turns by what a part is: a gear, a driving
// ring, a joiner. Everything else it treats as structure and tests where it
// stands, rather than sweeping it through a revolution.
//
// That is only sound while nothing in the structure can be keyed to a turning
// shaft. A cross-shaped hole is what does the keying — an axle in one cannot
// turn, so the part turns with the axle instead, sweeping a circle and taking
// whatever is pinned to it along. 61408, a thin liftarm with an axle hole
// through the middle, is exactly such a part.
//
// The beams in the inventory have round holes only, so the assumption holds.
// This is what says so, rather than leaving it to luck: add a part with an axle
// hole to the inventory and this fails, pointing at what else has to change.
func TestNoStructuralPartCanBeKeyedToATurningShaft(t *testing.T) {
	deps := requireLibraries(t)
	for _, beam := range part.Beams {
		holes := deps.Shadow.Holes(strings.TrimSuffix(beam.Part, ".dat"))
		if len(holes) == 0 {
			t.Errorf("%s has no connection points, so this cannot tell whether "+
				"it is keyed to anything", beam.Part)
			continue
		}
		for _, h := range holes {
			if !h.Cross {
				continue
			}
			t.Errorf("%s has a cross hole at %+v. A part keyed to a turning "+
				"shaft turns with it, and the structural search would still "+
				"place this as if it stood still", beam.Part, h.Pos)
		}
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
