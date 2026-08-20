// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package layout

import (
	"math"
	"testing"

	"brickmesh/internal/geom"
)

// A line has no origin along itself, and two ways of naming the same line have
// to agree — otherwise a station's axial position means something different on
// each, and two gears put in one plane land a stud apart.
func TestTheSameLineNamedTwoWaysIsOnePlacement(t *testing.T) {
	d := geom.Vec3{X: 1}
	a := NewPlacement(geom.Vec3{Y: 2, Z: -4}, d)
	for _, slide := range []float64{-7, -1, 0, 3, 40} {
		b := NewPlacement(geom.Vec3{X: slide, Y: 2, Z: -4}, d)
		if a.Key() != b.Key() {
			t.Errorf("sliding the point %g along the line gives a different "+
				"placement: %v vs %v", slide, a.Point, b.Point)
		}
		if got := math.Abs(b.Point.Dot(b.Direction)); got > 1e-9 {
			t.Errorf("slide %g: point keeps %g of a component along the line; "+
				"axial zero has to mean the same thing on every line", slide, got)
		}
	}
}

// The perpendicular part is what identifies the line, and must survive.
func TestCanonicalizingKeepsTheLineItself(t *testing.T) {
	p := NewPlacement(geom.Vec3{X: 9, Y: 2, Z: -4}, geom.Vec3{X: 1})
	if p.Point.Y != 2 || p.Point.Z != -4 {
		t.Errorf("point %v: the part across the line is which line it is and "+
			"cannot be dropped", p.Point)
	}
	// Two genuinely different lines stay different.
	q := NewPlacement(geom.Vec3{X: 9, Y: 2, Z: -6}, geom.Vec3{X: 1})
	if p.Key() == q.Key() {
		t.Error("two parallel lines two studs apart came out as one")
	}
}
