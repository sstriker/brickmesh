// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package teeth

import (
	"math"
	"os"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldraw"
)

func requireLibraries(t *testing.T) *ldraw.Library {
	t.Helper()
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	return ldraw.New("")
}

var zAxis = geom.Vec3{Z: 1}

// The documented fact: a 24t needs half its 15 degree pitch of phase against
// its partner. Python reports rot_a 0 and rot_b 7.5 for this pair.
func TestTwo24tNeedHalfAPitch(t *testing.T) {
	lib := requireLibraries(t)
	p, err := MeshPhase(lib, "3648b.dat", 24, zAxis, "3648b.dat", 24, zAxis,
		geom.Vec3{X: 60})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.PitchA-15) > 1e-9 {
		t.Errorf("pitch %v, want 15", p.PitchA)
	}
	if diff := math.Mod(p.RotB-p.RotA+15, 15); math.Abs(diff-7.5) > 0.01 {
		t.Errorf("phase difference %v, want half a pitch: 7.5", diff)
	}
}

// Python's own numbers for the same call, so the port is held to them.
func TestTheAnglesMatchThePythonReading(t *testing.T) {
	lib := requireLibraries(t)
	p, err := MeshPhase(lib, "3648b.dat", 24, zAxis, "3648b.dat", 24, zAxis,
		geom.Vec3{X: 60})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(p.RotA-0) > 0.01 {
		t.Errorf("rot A = %v, python reads 0", p.RotA)
	}
	if math.Abs(p.RotB-7.5) > 0.01 {
		t.Errorf("rot B = %v, python reads 7.5", p.RotB)
	}
	// Python reads sharpness 0.829 for this gear.
	if math.Abs(p.SharpA-0.8295) > 0.01 {
		t.Errorf("sharpness %v, python reads 0.8295", p.SharpA)
	}
}

// Sharpness as the Python reading gives it, gear by gear. Holding the port to
// these rather than to a threshold of my own choosing is the point: the numbers
// are what they are, including the 40t's poor showing.
var pythonSharpness = map[string]struct {
	teeth int
	sharp float64
}{
	"3647.dat":  {8, 0.9004},
	"32270.dat": {12, 0.6345},
	"4019.dat":  {16, 0.7396},
	"32269.dat": {20, 0.6107},
	"3648b.dat": {24, 0.8295},
	"32498.dat": {36, 0.5162},
	"3649.dat":  {40, 0.2630},
}

func TestEveryStandardGearReadsAsPythonReadsIt(t *testing.T) {
	lib := requireLibraries(t)
	for part, want := range pythonSharpness {
		angles, err := Angles(lib, part, zAxis, want.teeth)
		if err != nil {
			t.Errorf("%s: %v", part, err)
			continue
		}
		if len(angles) != want.teeth {
			t.Errorf("%s: %d angles, want %d", part, len(angles), want.teeth)
		}
		// Evenly spaced by definition. Modulo 360, since the last tooth wraps
		// past the first.
		pitch := 360 / float64(want.teeth)
		for i := 1; i < len(angles); i++ {
			gap := math.Mod(angles[i]-angles[i-1]+360, 360)
			if math.Abs(gap-pitch) > 1e-6 {
				t.Errorf("%s: teeth %d and %d are %v apart, want %v",
					part, i-1, i, gap, pitch)
			}
		}
		sharp, _ := Sharpness(lib, part, zAxis, want.teeth)
		if math.Abs(sharp-want.sharp) > 0.001 {
			t.Errorf("%s: sharpness %.4f, python reads %.4f", part, sharp, want.sharp)
		}
	}
}

// The 40t reads at 0.26, far below the rest, and both implementations agree on
// that. Its teeth are small against its radius, so the rim band picks up more
// that is not a tooth. A phase derived from it should carry a warning.
func TestThe40tReadsPoorlyAndSaysSo(t *testing.T) {
	lib := requireLibraries(t)
	forty, _ := Sharpness(lib, "3649.dat", zAxis, 40)
	eight, _ := Sharpness(lib, "3647.dat", zAxis, 8)
	if forty >= eight {
		t.Errorf("40t reads at %v and 8t at %v; the 40t is the unreliable one",
			forty, eight)
	}
	if forty > TrustThreshold {
		t.Errorf("40t sharpness %v should fall below the threshold of %v",
			forty, TrustThreshold)
	}
}

// A cross axle has four-fold symmetry, so a tooth count that is a multiple of
// four sits the same way whichever of the four seatings is used.
func TestSeatingIsFreeOnMultiplesOfFour(t *testing.T) {
	for _, teeth := range []int{8, 16, 20, 24, 36, 40} {
		if !SeatingIsFree(teeth) {
			t.Errorf("%dt is a multiple of four", teeth)
		}
	}
	// Every standard count is a multiple of four, so seating never matters for
	// them. The newer bevel gears are where it would: 14t and 22t are not.
	for _, teeth := range []int{14, 22} {
		if SeatingIsFree(teeth) {
			t.Errorf("%dt is not a multiple of four, so its seating fixes the phase", teeth)
		}
	}
}

func TestFrameIsPerpendicularToTheAxis(t *testing.T) {
	for _, axis := range []geom.Vec3{{X: 1}, {Y: 1}, {Z: 1}, {X: 1, Y: 1}} {
		u, v := Frame(axis)
		a := axis.Unit()
		if math.Abs(u.Dot(a)) > 1e-9 || math.Abs(v.Dot(a)) > 1e-9 {
			t.Errorf("axis %v: the frame is not across it", axis)
		}
		if math.Abs(u.Dot(v)) > 1e-9 {
			t.Errorf("axis %v: the frame is not square", axis)
		}
		if math.Abs(u.Len()-1) > 1e-9 || math.Abs(v.Len()-1) > 1e-9 {
			t.Errorf("axis %v: the frame is not unit length", axis)
		}
	}
}

func TestAGearWithoutTeethIsAnError(t *testing.T) {
	lib := requireLibraries(t)
	if _, err := Angles(lib, "3648b.dat", zAxis, 0); err == nil {
		t.Error("expected an error for a gear with no teeth")
	}
}
