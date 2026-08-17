// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package geom

import (
	"math"
	"testing"
)

func TestRotationsAreThe24ProperRotations(t *testing.T) {
	if len(Rotations) != 24 {
		t.Fatalf("got %d rotations, want 24", len(Rotations))
	}
	// A rotation has determinant +1; a reflection has -1 and would mirror the
	// part, which no amount of building can achieve.
	for i, m := range Rotations {
		if math.Abs(m.Det()-1) > 1e-9 {
			t.Errorf("rotation %d has determinant %v, want +1", i, m.Det())
		}
	}
	seen := make(map[Mat3]bool, 24)
	for _, m := range Rotations {
		if seen[m] {
			t.Errorf("duplicate rotation %v", m)
		}
		seen[m] = true
	}
}

func TestRotationsPreserveLength(t *testing.T) {
	v := Vec3{X: 1, Y: 2, Z: 3}
	want := v.Len()
	for i, m := range Rotations {
		if got := m.Apply(v).Len(); math.Abs(got-want) > 1e-9 {
			t.Errorf("rotation %d changed length: got %v, want %v", i, got, want)
		}
	}
}

// The meshing rule the whole geometric layer rests on: two parallel gears mesh
// at (t1+t2)/16 studs.
func TestPitchDistance(t *testing.T) {
	cases := []struct {
		t1, t2 int
		want   float64
	}{
		{8, 8, 20},   // 1 stud
		{8, 24, 40},  // 2 studs
		{12, 20, 40}, // 2 studs
		{16, 16, 40}, // 2 studs
		{20, 28, 60}, // 3 studs
		{24, 40, 80}, // 4 studs
		{8, 16, 30},  // 1.5 studs
		{12, 12, 30}, // 1.5 studs
		{8, 12, 25},  // 1.25 studs — off the half-stud lattice
		{36, 40, 95}, // 4.75 studs — likewise
	}
	for _, c := range cases {
		got := PitchDistance(c.t1, c.t2)
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("PitchDistance(%d, %d) = %v, want %v", c.t1, c.t2, got, c.want)
		}
	}
}

// Every standard tooth count is a multiple of 4, but that alone does NOT put
// every pair on the half-stud lattice: the distance is a whole number of half
// studs exactly when the two counts SUM to a multiple of 8. 8t+12t (25 LDU) and
// 36t+40t (95 LDU) are the counterexamples, and they are buildable pairs.
func TestPitchDistanceIsOnLatticeOnlyWhenSumIsMultipleOf8(t *testing.T) {
	counts := []int{8, 12, 16, 20, 24, 28, 36, 40}
	for _, a := range counts {
		for _, b := range counts {
			d := PitchDistance(a, b)
			onLattice := (Vec3{X: d}).OnLattice(HalfStud)
			want := (a+b)%8 == 0
			if onLattice != want {
				t.Errorf("%dt+%dt = %v LDU: on half-stud lattice = %v, want %v",
					a, b, d, onLattice, want)
			}
		}
	}
}

func TestEffectiveRadiiSumToPitchDistance(t *testing.T) {
	for _, p := range [][2]int{{8, 24}, {12, 20}, {16, 36}, {40, 40}} {
		sum := EffectiveRadius(p[0]) + EffectiveRadius(p[1])
		if want := PitchDistance(p[0], p[1]); math.Abs(sum-want) > 1e-9 {
			t.Errorf("radii of %dt and %dt sum to %v, want %v", p[0], p[1], sum, want)
		}
	}
}

func TestOnLattice(t *testing.T) {
	cases := []struct {
		v    Vec3
		step float64
		want bool
	}{
		{Vec3{X: 20, Y: 40, Z: -60}, Stud, true},
		{Vec3{X: 10, Y: 0, Z: 0}, Stud, false},
		{Vec3{X: 10, Y: 0, Z: 0}, HalfStud, true},
		{Vec3{X: 24, Y: 0, Z: 0}, HalfStud, false}, // a brick height is not
		{Vec3{}, Stud, true},
	}
	for _, c := range cases {
		if got := c.v.OnLattice(c.step); got != c.want {
			t.Errorf("%v.OnLattice(%v) = %v, want %v", c.v, c.step, got, c.want)
		}
	}
}

// Holes have no direction: a hole along +Y is the same hole as along -Y.
func TestParallelIgnoresSign(t *testing.T) {
	y := Vec3{Y: 1}
	if !y.Parallel(Vec3{Y: -1}) {
		t.Error("+Y should be parallel to -Y")
	}
	if !y.Parallel(Vec3{Y: 7}) {
		t.Error("Parallel should not care about magnitude")
	}
	if y.Parallel(Vec3{X: 1}) {
		t.Error("+Y should not be parallel to +X")
	}
}

func TestToCellFloorsTowardNegative(t *testing.T) {
	// Truncation instead of floor would collapse -1 and 0 into the same cell
	// and let parts overlap across the origin.
	if got := ToCell(Vec3{X: -1, Y: -VoxelPitch, Z: 0}); got != (Cell{X: -1, Y: -1, Z: 0}) {
		t.Errorf("ToCell = %+v, want {-1 -1 0}", got)
	}
	if got := ToCell(Vec3{X: VoxelPitch, Y: 2 * VoxelPitch, Z: VoxelPitch - 0.01}); got != (Cell{X: 1, Y: 2, Z: 0}) {
		t.Errorf("ToCell = %+v, want {1 2 0}", got)
	}
}

func TestVec3Arithmetic(t *testing.T) {
	a, b := Vec3{X: 1, Y: 2, Z: 3}, Vec3{X: 4, Y: 5, Z: 6}
	if got := a.Add(b); got != (Vec3{X: 5, Y: 7, Z: 9}) {
		t.Errorf("Add = %+v", got)
	}
	if got := a.Sub(b); got != (Vec3{X: -3, Y: -3, Z: -3}) {
		t.Errorf("Sub = %+v", got)
	}
	if got := a.Dot(b); got != 32 {
		t.Errorf("Dot = %v, want 32", got)
	}
	// X cross Y is Z, right-handed.
	if got := (Vec3{X: 1}).Cross(Vec3{Y: 1}); got != (Vec3{Z: 1}) {
		t.Errorf("Cross = %+v, want {0 0 1}", got)
	}
	if got := (Vec3{X: 3, Y: 4}).Len(); math.Abs(got-5) > 1e-9 {
		t.Errorf("Len = %v, want 5", got)
	}
	if got := (Vec3{}).Unit(); got != (Vec3{}) {
		t.Errorf("Unit of the zero vector should not divide by zero, got %+v", got)
	}
}
