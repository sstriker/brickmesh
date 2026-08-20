// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/sstriker/brickmesh/internal/clutch"
)

// One ring between two clutch gears, not one ring per shift.
//
// A driving ring has dogs on both faces. A builder puts one between the two
// gears and slides it either way; this put two of them back to back, and said
// so in its own report every time it ran while going ahead anyway.
//
// The hardware generation is what makes it work or not, and it is chosen per
// gear: the 20t exists only in the second system, the 16t in both, so picking
// the first that fits each landed them in different generations — and a ring of
// one generation does not grip the other's gears. The pair has to settle it.
func TestTwoShiftsOnOneShaftShareOneRing(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "gearbox-2-speed.json"))

	rings := 0
	for _, p := range res.Model.Parts {
		if isRing(p.Name) {
			rings++
		}
	}
	if rings != 1 {
		t.Errorf("got %d driving rings for two shifts on one shaft, want 1", rings)
	}

	shared := 0
	for _, site := range res.ringSites {
		if site.mate == nil {
			continue
		}
		shared++
		// Its two positions engage a gear each, so both gears must be reachable
		// and the ring must sit between them rather than beyond one.
		lo := math.Min(site.station.Axial, site.mate.station.Axial)
		hi := math.Max(site.station.Axial, site.mate.station.Axial)
		for _, at := range []float64{site.engaged, site.disengaged} {
			if at < lo || at > hi {
				t.Errorf("a shared ring sits at %.2f, outside the gears at "+
					"%.2f and %.2f: it cannot be between them", at, lo, hi)
			}
		}
		// And both gears have to be the same generation, or the dogs of one
		// meet a gear that has no recesses for them.
		if _, ok := site.system.Gears[site.station.Teeth]; !ok {
			t.Errorf("%s has no %dt gear, so the ring cannot grip it",
				site.system.Name, site.station.Teeth)
		}
		if _, ok := site.system.Gears[site.mate.station.Teeth]; !ok {
			t.Errorf("%s has no %dt gear, so the ring cannot grip its far side",
				site.system.Name, site.mate.station.Teeth)
		}
	}
	if shared != 1 {
		t.Errorf("got %d shared rings, want 1", shared)
	}
}

// The control: a shift with nothing to share with still gets its own ring, with
// a clear position rather than a second engagement.
func TestALoneShiftStillGetsItsOwnRing(t *testing.T) {
	if _, ok := clutch.ForBoth(16, 24); ok {
		t.Skip("24t became shiftable, so this is no longer a lone shift")
	}
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "gearbox-3-speed-compound.json"))

	// Two shaft pairs, each with two shifts, so two shared rings — and every
	// site here should be a pair, which is what makes the count 2 and not 4.
	lone := 0
	for _, site := range res.ringSites {
		if site.mate == nil {
			lone++
		}
	}
	if len(res.ringSites) != 2 {
		t.Errorf("got %d ring sites for a three speed over two shaft pairs, want 2",
			len(res.ringSites))
	}
	if lone != 0 {
		t.Errorf("%d of them serve a single gear; both pairs should have "+
			"settled on one ring each", lone)
	}
}
