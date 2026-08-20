// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package collide

import (
	"testing"

	"brickmesh/internal/geom"
)

func tri(a, b, c geom.Vec3) Triangle { return Triangle{a, b, c} }

func v(x, y, z float64) geom.Vec3 { return geom.Vec3{X: x, Y: y, Z: z} }

// flat sits in the z=0 plane, a unit right triangle at the origin.
var flat = tri(v(0, 0, 0), v(4, 0, 0), v(0, 4, 0))

func TestSeparatedTrianglesDoNotOverlap(t *testing.T) {
	far := tri(v(100, 100, 100), v(104, 100, 100), v(100, 104, 100))
	if TrianglesOverlap(flat, far) {
		t.Error("triangles 100 LDU apart should not overlap")
	}
}

func TestParallelPlanesDoNotOverlap(t *testing.T) {
	above := tri(v(0, 0, 5), v(4, 0, 5), v(0, 4, 5))
	if TrianglesOverlap(flat, above) {
		t.Error("the same triangle lifted 5 LDU should not overlap")
	}
}

func TestATriangleSkeweredByAnotherOverlaps(t *testing.T) {
	// A vertical triangle passing straight through the flat one.
	spike := tri(v(1, 1, -5), v(1, 1, 5), v(2, 1, 0))
	if !TrianglesOverlap(flat, spike) {
		t.Error("a triangle passing through should overlap")
	}
	// Order must not matter.
	if !TrianglesOverlap(spike, flat) {
		t.Error("overlap should be symmetric")
	}
}

func TestATriangleCrossingBesideTheOtherDoesNotOverlap(t *testing.T) {
	// Crosses the z=0 plane, but well outside the flat triangle.
	beside := tri(v(50, 50, -5), v(50, 50, 5), v(52, 50, 0))
	if TrianglesOverlap(flat, beside) {
		t.Error("crossing the plane elsewhere is not an overlap")
	}
}

func TestTouchingAtAVertexCounts(t *testing.T) {
	// Parts joined by a pin lie flat against each other; contact is contact.
	touching := tri(v(0, 0, 0), v(0, 0, 5), v(2, -3, 5))
	if !TrianglesOverlap(flat, touching) {
		t.Error("sharing a vertex should count as contact")
	}
}

func TestCoplanarOverlappingTrianglesAreContact(t *testing.T) {
	shifted := tri(v(1, 1, 0), v(5, 1, 0), v(1, 5, 0))
	n := geom.Vec3{Z: 1}
	if !coplanarOverlapIn2D(n, flat, shifted) {
		t.Error("the 2D test should see coplanar triangles that share area")
	}
	if TrianglesOverlap(flat, shifted) {
		t.Error("but sharing a plane is contact, not interpenetration")
	}
}

func TestCoplanarDisjointTrianglesDoNot(t *testing.T) {
	apart := tri(v(10, 10, 0), v(14, 10, 0), v(10, 14, 0))
	if TrianglesOverlap(flat, apart) {
		t.Error("coplanar triangles side by side should not overlap")
	}
}

// Containment in the shared plane is contact too, and the 2D routine still
// says so — which is the part that has to keep working, since it is what would
// decide if coplanarIsContact were ever turned off.
func TestCoplanarContainmentIsSeenBy2DAndTreatedAsContact(t *testing.T) {
	// Entirely inside, so no edge crosses any edge.
	inner := tri(v(0.5, 0.5, 0), v(1, 0.5, 0), v(0.5, 1, 0))
	n := geom.Vec3{Z: 1}
	if !coplanarOverlapIn2D(n, flat, inner) {
		t.Error("a triangle inside another should be seen in 2D")
	}
	if !coplanarOverlapIn2D(n, inner, flat) {
		t.Error("containment should be symmetric")
	}
	if TrianglesOverlap(flat, inner) {
		t.Error("but coplanar is contact, so it is not an overlap")
	}
}

// Faces sharing a plane are in contact, not overlapping.
//
// The predicate is about solids, and two solids that genuinely interpenetrate
// always have some pair of faces crossing at an angle. Coplanar faces are two
// surfaces resting against each other, which in LDraw is what every real fit
// looks like: nominal sizes mean a part that fits its slot fills it exactly.
//
// Test both ways round against the 2D routine, so what changed is stated rather
// than merely absent.
func TestCoplanarFacesAreContactRatherThanOverlap(t *testing.T) {
	mirrored := tri(v(0, 0, 0), v(4, 0, 0), v(0, -4, 0))
	if TrianglesOverlap(flat, mirrored) {
		t.Error("a shared edge is contact, not an overlap")
	}
	if TrianglesOverlap(flat, flat) {
		t.Error("a face against an identical face is contact, not an overlap")
	}
	// The 2D routine still knows the difference; it is simply not what decides.
	n := geom.Vec3{Z: 1}
	if !coplanarOverlapIn2D(n, flat, flat) {
		t.Error("the 2D test should still see a triangle covering itself")
	}
}

// What must still be caught: faces that cross at an angle. That is what
// interpenetration looks like, and no amount of contact tolerance may hide it.
func TestFacesCrossingAtAnAngleStillOverlap(t *testing.T) {
	upright := tri(v(1, 1, -4), v(1, 1, 4), v(3, 1, 0))
	if !TrianglesOverlap(flat, upright) {
		t.Error("a face passing through another is an overlap")
	}
}

// A degenerate triangle has no plane to speak of. It must not panic or report a
// false hit; real LDraw parts do contain slivers.
func TestDegenerateTrianglesDoNotPanic(t *testing.T) {
	degenerate := tri(v(0, 0, 0), v(0, 0, 0), v(0, 0, 0))
	line := tri(v(0, 0, 0), v(4, 0, 0), v(8, 0, 0))
	for _, pair := range [][2]Triangle{
		{flat, degenerate}, {degenerate, flat},
		{flat, line}, {line, flat}, {degenerate, line},
	} {
		TrianglesOverlap(pair[0], pair[1]) // must simply return
	}
}

// The case that matters for gears: two tooth flanks passing close by without
// touching, versus the same pair pushed together.
func TestNearMissVersusBite(t *testing.T) {
	flank := tri(v(0, 0, 0), v(0, 10, 0), v(0, 0, 10))
	clear := tri(v(0.5, 2, 2), v(0.5, 8, 2), v(0.5, 2, 8))
	if TrianglesOverlap(flank, clear) {
		t.Error("half an LDU of clearance is not contact")
	}
	bite := tri(v(-0.5, 2, 2), v(0.5, 8, 2), v(0.5, 2, 8))
	if !TrianglesOverlap(flank, bite) {
		t.Error("a flank crossing the other is contact")
	}
}
