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
	ring, err := interfere.MeshFor(lib, First.Ring)
	if err != nil {
		t.Fatalf("%s: %v", First.Ring, err)
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

// The claim the First.Engaged constant rests on: at that distance the clutch gear is
// blocked for most of a turn, and what is free is sixteen windows a clutch
// tooth apart. That signature is the ring's dogs in the gear's recesses.
func TestTheClutchGearInterlocksAtTheEngagedDistance(t *testing.T) {
	lib := requireLibraries(t)
	for _, part := range []string{"6542a.dat", "6542b.dat"} {
		got := sweepAt(t, lib, part, First.Engaged*halfStud)
		if got.Windows != 16 {
			t.Errorf("%s at %g LDU: %d free windows, want 16 — one per clutch tooth",
				part, First.Engaged*halfStud, got.Windows)
		}
		if got.FreeFraction == 1 {
			t.Errorf("%s at %g LDU turns freely, so nothing engages it",
				part, First.Engaged*halfStud)
		}
	}
}

// A plain gear has no recesses, so a ring beside one is scenery. This is why
// Gears maps only the counts that have a clutch variant.
func TestAPlainGearShowsNoInterlock(t *testing.T) {
	lib := requireLibraries(t)
	for _, part := range []string{"4019.dat", "32269.dat", "3648b.dat"} {
		got := sweepAt(t, lib, part, First.Engaged*halfStud)
		if got.Windows == 16 {
			t.Errorf("%s reads as sixteen clutch teeth, which it does not have", part)
		}
	}
}

// One LDU past the flush position every gear is free at every angle, so First.Clear
// really does clear — and First.Engaged really is as close as a ring can come.
func TestClearIsClearAndEngagedIsNotDeeper(t *testing.T) {
	lib := requireLibraries(t)
	gears := []string{"6542a.dat", "6542b.dat", "4019.dat", "32269.dat", "3648b.dat"}
	for _, part := range gears {
		if got := sweepAt(t, lib, part, First.Clear*halfStud); got.Verdict != interfere.NoEngagement {
			t.Errorf("%s at the clear position: %s, want the ring to turn freely",
				part, got.Verdict)
		}
		// A half stud closer than engaged and the ring is inside the gear.
		if got := sweepAt(t, lib, part, (First.Engaged-1)*halfStud); got.Verdict != interfere.TooDeep {
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
			"First.Engaged being %g is that two is solid overlap", got.Verdict, First.Engaged)
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

// The two generations do not mix, and this is the measurement that says so:
// each ring shows a window pattern against its own gears and none against the
// other's. Getting this wrong would put a ring beside a gear it cannot grip and
// call the gearbox finished.
func TestEachRingEngagesOnlyItsOwnGears(t *testing.T) {
	lib := requireLibraries(t)
	for _, c := range []struct {
		system  System
		teeth   int
		at      float64 // LDU from the gear's center
		windows int
	}{
		// The first ring against the 16t it was made for: sixteen windows, one
		// per recess in the clutch face.
		{First, 16, First.Engaged * halfStud, 16},
		// The second against its own 16t and 20t: four windows, and over a band
		// rather than at a single distance, because its dogs have depth.
		{Second, 16, Second.Engaged * halfStud, 4},
		{Second, 20, Second.Engaged * halfStud, 4},
	} {
		gear := c.system.Gears[c.teeth]
		got := sweepRing(t, lib, c.system.Ring, gear, c.at)
		if got.Windows != c.windows {
			t.Errorf("%s ring against %s at %g LDU: %d windows, want %d",
				c.system.Name, gear, c.at, got.Windows, c.windows)
		}
		if got.FreeFraction == 1 {
			t.Errorf("%s ring turns freely against %s, so nothing engages it",
				c.system.Name, gear)
		}
	}
}

// A plain gear has no recesses, at any distance either ring can reach. This is
// the control: without it the window counts above could be the gear's own teeth
// rather than a clutch.
func TestNeitherRingEngagesAPlainGear(t *testing.T) {
	lib := requireLibraries(t)
	for _, system := range Systems {
		for _, plain := range []string{"4019.dat", "32269.dat", "3648b.dat"} {
			for at := 28.0; at <= 40; at++ {
				got := sweepRing(t, lib, system.Ring, plain, at)
				if got.Verdict == interfere.Meshes {
					t.Errorf("%s ring reads as engaging the plain %s at %g LDU",
						system.Name, plain, at)
				}
			}
		}
	}
}

// A 24-tooth gear cannot be dog-shifted, and the parts named "Gear 24 Tooth
// Clutch" do not change that: they are torque limiters with a slipping centre,
// and they read exactly like a plain gear to both rings.
func TestNothingShiftsATwentyFourToothGear(t *testing.T) {
	lib := requireLibraries(t)
	if Shiftable(24) {
		t.Error("24 is listed as shiftable; no driving ring has a clutch gear that size")
	}
	for _, system := range Systems {
		for _, socalled := range []string{"76019.dat", "76244.dat"} {
			engaged := false
			for at := 24.0; at <= 44; at++ {
				if got := sweepRing(t, lib, system.Ring, socalled, at); got.Windows > 0 {
					engaged = true
				}
			}
			if engaged {
				t.Errorf("%s reads as engaging %s somewhere; if that is real, 24 "+
					"belongs in the tables", system.Name, socalled)
			}
		}
	}
}

// sweepRing turns a ring a full revolution against a gear at a distance.
func sweepRing(t *testing.T, lib *ldraw.Library, ring, gear string,
	at float64) interfere.Result {

	t.Helper()
	rm, err := interfere.MeshFor(lib, ring)
	if err != nil {
		t.Fatalf("%s: %v", ring, err)
	}
	gm, err := interfere.MeshFor(lib, gear)
	if err != nil {
		t.Fatalf("%s: %v", gear, err)
	}
	got, err := interfere.MeshLock(context.Background(), gm, collide.Identity(),
		rm, collide.Transform{Rot: collide.Identity().Rot, Pos: geom.Vec3{Z: at}},
		16, interfere.Options{Steps: 360})
	if err != nil {
		t.Fatal(err)
	}
	return got
}
