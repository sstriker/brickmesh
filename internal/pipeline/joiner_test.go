// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"math"
	"testing"

	"github.com/sstriker/brickmesh/internal/clutch"
	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
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

	// Whichever generation the gearbox ended up using. Two clutch gears sharing
	// one ring settle that between them, so naming a system here would test the
	// choice rather than the property.
	var rings, joiners []ldr.Part
	for _, p := range res.Model.Parts {
		switch {
		case isRing(p.Name):
			rings = append(rings, p)
		case isJoiner(p.Name):
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
			if r.Pos.Sub(j.Pos).Len() <= (clutch.First.Travel()+clutch.First.JoinerHalf)*10+1e-6 {
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
		if isJoiner(a.name) {
			continue // this loop is about the axles that butt into them
		}
		axis := a.rot.Apply(geom.Vec3{X: 1}).Unit()
		half := float64(a.studs) * geom.Stud / 2
		for _, j := range res.axles {
			if !isJoiner(j.name) {
				continue
			}
			d := j.center.Sub(a.center)
			// Only joiners on this axle's own line.
			if d.Sub(axis.Scale(d.Dot(axis))).Len() > 1e-6 {
				continue
			}
			gap := math.Abs(d.Dot(axis))
			if gap > half+clutch.Second.JoinerHalf*10 {
				continue // not the joiner this end goes into
			}
			// The axle's end, measured from the joiner's center.
			end := half - gap
			if end > 1e-6 {
				t.Errorf("axle %d at %+v runs %.1f LDU past the stop in the joiner "+
					"at %+v", a.studs, a.center, end, j.center)
			}
			if end < -clutch.Second.JoinerReach*10-1e-6 {
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
		Name: clutch.First.Ring, Pos: geom.Vec3{},
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
	// The ring turns about its own shaft, which runs along x through the origin.
	spin := turning{about: map[int]axis{0: {dir: geom.Vec3{X: 1}}}}
	inside, _, err := sharesSpace(context.Background(), deps, ring, beam(30), spin, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if inside {
		t.Error("a beam resting against the end of the ring is right, not a clash")
	}

	// Half a stud further in and it is inside the ring.
	inside, overlap, err := sharesSpace(context.Background(), deps, ring, beam(20), spin, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !inside {
		t.Error("a beam a half stud inside the ring should be reported")
	} else if overlap <= 0 {
		t.Errorf("reported inside but with an overlap of %v", overlap)
	}
}
