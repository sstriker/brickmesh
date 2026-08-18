// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package clutch

import (
	"context"
	"os"
	"testing"

	"brickmesh/internal/collide"
	"brickmesh/internal/geom"
	"brickmesh/internal/interfere"
	"brickmesh/internal/ldraw"
)

// HalfStud in LDU, so the constants above can be checked against a measurement
// in the units the measurement comes in.
const halfStud = 10.0

func requireLibraries(t *testing.T) *ldraw.Library {
	t.Helper()
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	return ldraw.New("")
}

// sweepAt slides the ring to an offset along the shared axis and turns it a
// full revolution against the gear.
func sweepAt(t *testing.T, lib *ldraw.Library, gearPart string,
	offset float64) interfere.Result {

	t.Helper()
	gear, err := interfere.MeshFor(lib, gearPart)
	if err != nil {
		t.Fatalf("%s: %v", gearPart, err)
	}
	ring, err := interfere.MeshFor(lib, Ring)
	if err != nil {
		t.Fatalf("%s: %v", Ring, err)
	}
	got, err := interfere.MeshLock(context.Background(), gear, collide.Identity(),
		ring, collide.Transform{Rot: collide.Identity().Rot, Pos: geom.Vec3{Z: offset}},
		16, interfere.Options{Steps: 360})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// The reference from docs/findings.md, run through this file's own calls. A
// sweep set up wrongly reports everything free and every claim below would pass
// vacuously, so the control comes first.
func TestTheSweepIsSetUpRight(t *testing.T) {
	lib := requireLibraries(t)
	gear, err := interfere.MeshFor(lib, "3648b.dat")
	if err != nil {
		t.Fatal(err)
	}
	side := func(x float64) collide.Transform {
		return collide.Transform{Rot: collide.Identity().Rot, Pos: geom.Vec3{X: x}}
	}
	if got := control(t, gear, side(60)); got.Verdict != interfere.Meshes {
		t.Fatalf("two 24t at 60 LDU: %s, want %s", got.Verdict, interfere.Meshes)
	}
	if got := control(t, gear, side(58)); got.Verdict != interfere.TooDeep {
		t.Fatalf("two 24t at 58 LDU: %s, want %s", got.Verdict, interfere.TooDeep)
	}
}

// The claim the Engaged constant rests on: at that distance the clutch gear is
// blocked for most of a turn, and what is free is sixteen windows a clutch
// tooth apart. That signature is the ring's dogs in the gear's recesses.
func TestTheClutchGearInterlocksAtTheEngagedDistance(t *testing.T) {
	lib := requireLibraries(t)
	for _, part := range []string{"6542a.dat", "6542b.dat"} {
		got := sweepAt(t, lib, part, Engaged*halfStud)
		if got.Windows != 16 {
			t.Errorf("%s at %g LDU: %d free windows, want 16 — one per clutch tooth",
				part, Engaged*halfStud, got.Windows)
		}
		if got.FreeFraction == 1 {
			t.Errorf("%s at %g LDU turns freely, so nothing engages it",
				part, Engaged*halfStud)
		}
	}
}

// A plain gear has no recesses, so a ring beside one is scenery. This is why
// Gears maps only the counts that have a clutch variant.
func TestAPlainGearShowsNoInterlock(t *testing.T) {
	lib := requireLibraries(t)
	for _, part := range []string{"4019.dat", "32269.dat", "3648b.dat"} {
		got := sweepAt(t, lib, part, Engaged*halfStud)
		if got.Windows == 16 {
			t.Errorf("%s reads as sixteen clutch teeth, which it does not have", part)
		}
	}
}

// One LDU past the flush position every gear is free at every angle, so Clear
// really does clear — and Engaged really is as close as a ring can come.
func TestClearIsClearAndEngagedIsNotDeeper(t *testing.T) {
	lib := requireLibraries(t)
	gears := []string{"6542a.dat", "6542b.dat", "4019.dat", "32269.dat", "3648b.dat"}
	for _, part := range gears {
		if got := sweepAt(t, lib, part, Clear*halfStud); got.Verdict != interfere.NoEngagement {
			t.Errorf("%s at the clear position: %s, want the ring to turn freely",
				part, got.Verdict)
		}
		// A half stud closer than engaged and the ring is inside the gear.
		if got := sweepAt(t, lib, part, (Engaged-1)*halfStud); got.Verdict != interfere.TooDeep {
			t.Errorf("%s a half stud inside the engaged position: %s, want %s",
				part, got.Verdict, interfere.TooDeep)
		}
	}
}

// The bug this package exists to fix: the pipeline used to park the ring two
// half studs from the gear's center, which is inside it.
func TestTwoHalfStudsPutsTheRingInsideTheGear(t *testing.T) {
	lib := requireLibraries(t)
	got := sweepAt(t, lib, "6542a.dat", 2*halfStud)
	if got.Verdict != interfere.TooDeep {
		t.Errorf("at two half studs the ring reads %s; the whole point of "+
			"Engaged being %g is that two is solid overlap", got.Verdict, Engaged)
	}
}

// control runs the reference pair, which every claim here rests on.
func control(t *testing.T, gear *collide.Mesh, at collide.Transform) interfere.Result {
	t.Helper()
	got, err := interfere.MeshLock(context.Background(), gear, collide.Identity(),
		gear, at, 24, interfere.Options{Steps: 360})
	if err != nil {
		t.Fatal(err)
	}
	return got
}
