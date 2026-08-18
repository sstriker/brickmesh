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
		snaps := deps.Shadow.Snaps(strings.TrimSuffix(beam.Part, ".dat"))
		if len(snaps) == 0 {
			t.Errorf("%s has no shadow data, so this cannot tell whether it is "+
				"keyed to anything", beam.Part)
			continue
		}
		for _, s := range snaps {
			// Sections are given as a kind and a size: R round, A axle-shaped,
			// S solid. An A section is a hole an axle is keyed into.
			for _, field := range strings.Fields(s.Secs) {
				if field != "A" {
					continue
				}
				t.Errorf("%s has an axle-shaped hole (%q). A part keyed to a "+
					"turning shaft turns with it, and clearance.classOf would "+
					"still call this structure and test it standing still. "+
					"Deriving what turns from the ports rather than from the "+
					"part is the fix", beam.Part, s.Secs)
			}
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
