// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package interfere

import (
	"math"
	"os"
	"testing"
	"time"

	"brickmesh/internal/collide"
	"brickmesh/internal/geom"
	"brickmesh/internal/ldraw"
)

const gear24 = "3648b.dat" // Technic Gear 24 Tooth

// requireLibraries gates the calibration on the real parts. Same switch the
// Python suite uses, so one environment variable turns both on.
func requireLibraries(t *testing.T) *ldraw.Library {
	t.Helper()
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	return ldraw.New("") // the shared cache
}

func gearMesh(t *testing.T, lib *ldraw.Library, part string) *collide.Mesh {
	t.Helper()
	m, err := MeshFor(lib, part)
	if err != nil {
		t.Fatalf("%s: %v", part, err)
	}
	return m
}

func at(x float64) collide.Transform {
	return collide.Transform{Rot: collide.Identity().Rot, Pos: geom.Vec3{X: x}}
}

// THE calibration. Everything else this package says is only worth as much as
// this: two 24-tooth gears have a pitch radius of 30 LDU, so a pair sits 60
// apart, and turning one a full revolution has to leave exactly 24 free
// windows, one per tooth.
func TestTwo24tGearsMeshAt60LDU(t *testing.T) {
	lib := requireLibraries(t)
	a := gearMesh(t, lib, gear24)
	b := gearMesh(t, lib, gear24)

	start := time.Now()
	got := MeshLock(a, collide.Identity(), b, at(60), 24, Options{Steps: 360})
	t.Logf("swept 360 orientations in %s", time.Since(start).Round(time.Millisecond))

	if got.Verdict != Meshes {
		t.Errorf("verdict %q, want %q", got.Verdict, Meshes)
	}
	if got.Windows != 24 {
		t.Errorf("%d free windows, want 24 — one per tooth", got.Windows)
	}
	if math.Abs(got.WindowSpacingDeg-15) > 0.5 {
		t.Errorf("windows %v deg apart, want 15", got.WindowSpacingDeg)
	}
}

// Two LDU too close and the teeth cannot be assembled at any phase.
func TestTheSamePairJamsAt58(t *testing.T) {
	lib := requireLibraries(t)
	a := gearMesh(t, lib, gear24)
	b := gearMesh(t, lib, gear24)

	got := MeshLock(a, collide.Identity(), b, at(58), 24, Options{Steps: 360})
	if got.Verdict != TooDeep {
		t.Errorf("verdict %q, want %q", got.Verdict, TooDeep)
	}
	if got.Windows != 0 {
		t.Errorf("%d free windows at 58 LDU, want none", got.Windows)
	}
}

func TestPullingThemApartWidensTheBacklash(t *testing.T) {
	lib := requireLibraries(t)
	a := gearMesh(t, lib, gear24)
	b := gearMesh(t, lib, gear24)

	tight := MeshLock(a, collide.Identity(), b, at(60), 24, Options{Steps: 360})
	loose := MeshLock(a, collide.Identity(), b, at(62), 24, Options{Steps: 360})
	if loose.FreeFraction <= tight.FreeFraction {
		t.Errorf("free fraction %v at 62 LDU, %v at 60; it should widen",
			loose.FreeFraction, tight.FreeFraction)
	}
}

func TestFarApartTheyDoNotEngageAtAll(t *testing.T) {
	lib := requireLibraries(t)
	a := gearMesh(t, lib, gear24)
	b := gearMesh(t, lib, gear24)

	got := MeshLock(a, collide.Identity(), b, at(200), 24, Options{Steps: 72})
	if got.Verdict != NoEngagement {
		t.Errorf("verdict %q at 200 LDU, want %q", got.Verdict, NoEngagement)
	}
}

// The caveat, pinned. The free window is a couple of degrees wide, so five
// degrees per sample steps over it and the sweep reports a confident "cannot be
// assembled" for a pair that meshes perfectly.
func TestTooCoarseASweepMissesTheWindows(t *testing.T) {
	lib := requireLibraries(t)
	a := gearMesh(t, lib, gear24)
	b := gearMesh(t, lib, gear24)

	coarse := MeshLock(a, collide.Identity(), b, at(60), 24, Options{Steps: 72})
	if coarse.Windows != 0 {
		t.Errorf("72 steps found %d windows; if this now works the caveat can go",
			coarse.Windows)
	}
	fine := MeshLock(a, collide.Identity(), b, at(60), 24, Options{Steps: 144})
	if fine.Windows != 24 {
		t.Errorf("the documented default of 144 found %d windows, want 24", fine.Windows)
	}
}

// --------------------------------------------------------------------------
// the parts that need no libraries
// --------------------------------------------------------------------------

func TestRotIsARotation(t *testing.T) {
	for _, axis := range []byte{'x', 'y', 'z'} {
		m := Rot(axis, 90)
		if math.Abs(m.Det()-1) > 1e-9 {
			t.Errorf("%c: determinant %v, want 1", axis, m.Det())
		}
		// Four quarter turns is the identity again.
		full := m.Mul(m).Mul(m).Mul(m)
		v := geom.Vec3{X: 1, Y: 2, Z: 3}
		if full.Apply(v).Sub(v).Len() > 1e-9 {
			t.Errorf("%c: four quarter turns did not come back", axis)
		}
	}
	// Turning about Z leaves Z alone.
	if got := Rot('z', 37).Apply(geom.Vec3{Z: 1}); got.Sub(geom.Vec3{Z: 1}).Len() > 1e-9 {
		t.Errorf("rotation about Z moved Z to %+v", got)
	}
}

func TestWindowsJoinAcrossTheWrapPoint(t *testing.T) {
	// Free at the very start and the very end: one window straddling zero, not
	// two.
	free := make([]bool, 12)
	free[0], free[1], free[11] = true, true, true
	got := windowsOf(free, len(free))
	if len(got) != 1 {
		t.Fatalf("got %d windows, want 1 across the wrap", len(got))
	}
	if len(got[0]) != 3 {
		t.Errorf("the window holds %d samples, want 3", len(got[0]))
	}
}

func TestWindowsAreCountedSeparately(t *testing.T) {
	free := make([]bool, 12)
	free[2], free[3] = true, true
	free[8] = true
	if got := windowsOf(free, len(free)); len(got) != 2 {
		t.Errorf("got %d windows, want 2", len(got))
	}
}

func TestClassifyReportsJamAndFreeSpin(t *testing.T) {
	jammed := classify(make([]bool, 36), 36, 24)
	if jammed.Verdict != TooDeep {
		t.Errorf("no free angle should read as %q, got %q", TooDeep, jammed.Verdict)
	}
	all := make([]bool, 36)
	for i := range all {
		all[i] = true
	}
	if got := classify(all, 36, 24); got.Verdict != NoEngagement {
		t.Errorf("every angle free should read as %q, got %q", NoEngagement, got.Verdict)
	}
}

// A sweep with the right number of evenly spaced windows meshes; the same count
// bunched together does not.
func TestClassifyNeedsEvenlySpacedWindows(t *testing.T) {
	steps := 240
	even := make([]bool, steps)
	for tooth := 0; tooth < 24; tooth++ {
		even[tooth*10] = true
	}
	if got := classify(even, steps, 24); got.Verdict != Meshes {
		t.Errorf("evenly spaced windows should mesh, got %q", got.Verdict)
	}

	bunched := make([]bool, steps)
	for i := 0; i < 24; i++ {
		bunched[i*2] = true
	}
	if got := classify(bunched, steps, 24); got.Verdict != Doubtful {
		t.Errorf("bunched windows should read as %q, got %q", Doubtful, got.Verdict)
	}
}

func TestMedian(t *testing.T) {
	if got := median([]float64{3, 1, 2}); got != 2 {
		t.Errorf("odd count: %v", got)
	}
	if got := median([]float64{4, 1, 3, 2}); got != 2.5 {
		t.Errorf("even count: %v", got)
	}
	if got := median(nil); got != 0 {
		t.Errorf("empty: %v", got)
	}
}
