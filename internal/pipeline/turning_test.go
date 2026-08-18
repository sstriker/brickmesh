// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"testing"

	"brickmesh/internal/ldr"

	"brickmesh/internal/geom"
)

// Two pins in different places hold a part still, so it is carried round
// rigidly and sweeps a circle. One pin does not: it is a hinge, and the part is
// free to swing about it while it goes round, so no single axis says where it
// can be.
func TestWhatHoldsAPartStill(t *testing.T) {
	along := geom.Vec3{X: 1}
	for _, c := range []struct {
		why   string
		pins  []port
		still bool
	}{
		{"nothing", nil, false},
		{"one pin", []port{{at: geom.Vec3{}, dir: along}}, false},
		{"the same pin twice", []port{
			{at: geom.Vec3{}, dir: along}, {at: geom.Vec3{}, dir: along},
		}, false},
		{"two pins on one line", []port{
			{at: geom.Vec3{}, dir: along}, {at: geom.Vec3{X: 40}, dir: along},
		}, false},
		{"two pins side by side", []port{
			{at: geom.Vec3{}, dir: along}, {at: geom.Vec3{Z: 40}, dir: along},
		}, true},
		{"three pins", []port{
			{at: geom.Vec3{}, dir: along}, {at: geom.Vec3{Z: 20}, dir: along},
			{at: geom.Vec3{Z: 40}, dir: along},
		}, true},
	} {
		if got := holdsStill(c.pins); got != c.still {
			t.Errorf("%s: holdsStill=%v, want %v", c.why, got, c.still)
		}
	}
}

// Two pins strung along one line are the case that looks like restraint and is
// not: that is how a hinge with a long pin is built, and the part spins about
// the line they share.
func TestTwoPinsOnOneLineAreStillAHinge(t *testing.T) {
	along := geom.Vec3{Y: 1}
	pins := []port{
		{at: geom.Vec3{X: 10}, dir: along},
		{at: geom.Vec3{X: 10, Y: 60}, dir: along}, // further along the same axis
	}
	if holdsStill(pins) {
		t.Error("two pins on one line leave the part free to spin about it")
	}
}

// A cross hole is what keys a part to a shaft. A round one lets the shaft spin
// inside it, which is a bearing, and nothing is carried.
func TestOnlyACrossHoleIsCarried(t *testing.T) {
	shaft := axis{at: geom.Vec3{Z: -40}, dir: geom.Vec3{X: 1}}
	onShaft := geom.Vec3{X: 20, Z: -40}

	round := []port{{at: onShaft, dir: geom.Vec3{X: 1}, cross: false}}
	if _, ok := onAShaft(placedAt(onShaft), round, []axis{shaft}, false); ok {
		t.Error("a round hole on a shaft is a bearing: the shaft turns inside it")
	}

	cross := []port{{at: onShaft, dir: geom.Vec3{X: 1}, cross: true}}
	got, ok := onAShaft(placedAt(onShaft), cross, []axis{shaft}, false)
	if !ok {
		t.Fatal("a cross hole on a shaft is keyed to it and goes round with it")
	}
	if !got.sameAs(shaft) {
		t.Errorf("carried about %+v, want the shaft %+v", got, shaft)
	}

	// And a cross hole that is nowhere near the shaft is not carried by it.
	elsewhere := []port{{at: geom.Vec3{X: 20, Z: 100}, dir: geom.Vec3{X: 1}, cross: true}}
	if _, ok := onAShaft(placedAt(geom.Vec3{Z: 100}), elsewhere, []axis{shaft}, false); ok {
		t.Error("a cross hole on a different line is not on this shaft")
	}
}

// Two points on one line describe the same axis however far apart they are, and
// two parallel lines a stud apart do not.
func TestWhenTwoAxesAreTheSame(t *testing.T) {
	a := axis{at: geom.Vec3{}, dir: geom.Vec3{X: 1}}
	for _, c := range []struct {
		why  string
		b    axis
		same bool
	}{
		{"further along", axis{at: geom.Vec3{X: 200}, dir: geom.Vec3{X: 1}}, true},
		{"pointing the other way", axis{at: geom.Vec3{}, dir: geom.Vec3{X: -1}}, true},
		{"a stud across", axis{at: geom.Vec3{Z: 20}, dir: geom.Vec3{X: 1}}, false},
		{"at right angles", axis{at: geom.Vec3{}, dir: geom.Vec3{Z: 1}}, false},
	} {
		if got := a.sameAs(c.b); got != c.same {
			t.Errorf("%s: sameAs=%v, want %v", c.why, got, c.same)
		}
	}
}

// placedAt is a part at a position, with no rotation: enough for the seeding
// tests, which care about where its ports are and not what it looks like.
func placedAt(at geom.Vec3) ldr.Part {
	return ldr.Part{Pos: at, Rot: geom.Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}
}
