// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"math"
	"testing"

	"brickmesh/internal/clutch"
	"brickmesh/internal/geom"
	"brickmesh/internal/ldr"
)

// A driving ring cannot grip a plain axle: its bore is ridged, and the shadow
// library pairs it with an axle joiner. So a shaft carrying a ring is two
// axles, not one.
func TestEveryRingRidesAJoiner(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, gearbox), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}

	var rings, joiners []ldr.Part
	for _, p := range res.Model.Parts {
		switch p.Name {
		case DrivingRing:
			rings = append(rings, p)
		case clutch.Joiner:
			joiners = append(joiners, p)
		}
	}
	if len(rings) == 0 {
		t.Fatal("no rings to check")
	}
	if len(joiners) != len(rings) {
		t.Fatalf("%d ring(s) but %d joiner(s): each ring needs one to slide on",
			len(rings), len(joiners))
	}
	for _, r := range rings {
		var found bool
		for _, j := range joiners {
			// Coaxial, and close enough along the shaft that the ring is on it
			// at both ends of its travel.
			if r.Pos.Sub(j.Pos).Len() <= clutch.Travel*10+1e-6 {
				found = true
			}
		}
		if !found {
			t.Errorf("the ring at %+v has no joiner under it", r.Pos)
		}
	}
}

// The joiner's two holes are capped in the middle, so the axles meet there
// rather than passing. An axle that reaches past the stop cannot be pushed in.
func TestTheAxlesMeetInsideTheJoiner(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, gearbox), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}

	for _, a := range res.axles {
		if a.name == clutch.Joiner {
			continue
		}
		axis := a.rot.Apply(geom.Vec3{X: 1}).Unit()
		half := float64(a.studs) * geom.Stud / 2
		for _, j := range res.axles {
			if j.name != clutch.Joiner {
				continue
			}
			d := j.center.Sub(a.center)
			// Only joiners on this axle's own line.
			if d.Sub(axis.Scale(d.Dot(axis))).Len() > 1e-6 {
				continue
			}
			gap := math.Abs(d.Dot(axis))
			if gap > half+clutch.JoinerHalf*10 {
				continue // not the joiner this end goes into
			}
			// The axle's end, measured from the joiner's center.
			end := half - gap
			if end > 1e-6 {
				t.Errorf("axle %d at %+v runs %.1f LDU past the stop in the joiner "+
					"at %+v", a.studs, a.center, end, j.center)
			}
			if end < -clutch.JoinerReach*10-1e-6 {
				t.Errorf("axle %d at %+v stops %.1f LDU short of the joiner at %+v, "+
					"so it never reaches it", a.studs, a.center, -end, j.center)
			}
		}
	}
}

// Parts that meet face to face are ordinary and must not be reported. Parts
// that are inside one another must be. The tolerance sits between the two, and
// this is what keeps it there.
func TestTouchingIsAllowedAndOverlapIsNot(t *testing.T) {
	deps := requireLibraries(t)
	ring := ldr.Part{
		Name: DrivingRing, Pos: geom.Vec3{},
		Rot: geom.Mat3{{0, 0, 1}, {1, 0, 0}, {0, 1, 0}},
	}
	beam := func(x float64) ldr.Part {
		return ldr.Part{
			Name: "32316.dat", Pos: geom.Vec3{X: x},
			Rot: geom.Mat3{{0, 1, 0}, {1, 0, 0}, {0, 0, -1}},
		}
	}

	// The ring runs 20 LDU each way along its shaft and the beam 10, so at 30
	// their faces meet exactly.
	res := &Result{}
	got, err := sweepAgainst(context.Background(), res, deps, []ldr.Part{ring},
		[]ldr.Part{beam(30)})
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Errorf("a beam resting against the end of the ring is right, got %d "+
			"clash(es): %v", got, res.Findings)
	}

	// Half a stud further in and it is inside the ring.
	res = &Result{}
	got, err = sweepAgainst(context.Background(), res, deps, []ldr.Part{ring},
		[]ldr.Part{beam(20)})
	if err != nil {
		t.Fatal(err)
	}
	if got == 0 {
		t.Error("a beam a half stud inside the ring should be reported")
	}
}
