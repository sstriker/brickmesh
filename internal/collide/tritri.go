// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package collide

import (
	"math"

	"brickmesh/internal/geom"
)

// eps is the tolerance for calling a signed distance zero. LDraw coordinates
// are in LDU and parts span tens to hundreds of them, so this is far below any
// real feature and far above the noise of transforming a vertex.
const eps = 1e-9

// Triangle is three points, in whatever frame the caller is working in.
type Triangle [3]geom.Vec3

// TrianglesOverlap reports whether two triangles share any point.
//
// Möller's test: each triangle is checked against the other's plane first,
// which rejects most pairs outright. What survives has both triangles crossing
// the line where the two planes meet, so the test reduces to whether their
// intervals on that line overlap. The coplanar case has no such line and falls
// back to a 2D test.
//
// This is what replaces FCL on the Go side. The gate on it is not that it looks
// right but that two 24-tooth gears mesh at 60 LDU and jam at 58 — see
// internal/interfere.
func TrianglesOverlap(t1, t2 Triangle) bool {
	// T1 against the plane of T2.
	n2 := t2[1].Sub(t2[0]).Cross(t2[2].Sub(t2[0]))
	d2 := -n2.Dot(t2[0])
	dv := [3]float64{
		n2.Dot(t1[0]) + d2,
		n2.Dot(t1[1]) + d2,
		n2.Dot(t1[2]) + d2,
	}
	if sameSide(dv) {
		return false
	}

	// T2 against the plane of T1.
	n1 := t1[1].Sub(t1[0]).Cross(t1[2].Sub(t1[0]))
	d1 := -n1.Dot(t1[0])
	du := [3]float64{
		n1.Dot(t2[0]) + d1,
		n1.Dot(t2[1]) + d1,
		n1.Dot(t2[2]) + d1,
	}
	if sameSide(du) {
		return false
	}

	if zero(dv[0]) && zero(dv[1]) && zero(dv[2]) {
		return coplanarOverlap(n1, t1, t2)
	}

	// Both straddle the line where the planes meet. Project onto its dominant
	// axis and compare intervals.
	axis := dominantAxis(n1.Cross(n2))
	lo1, hi1, ok1 := interval(t1, dv, axis)
	lo2, hi2, ok2 := interval(t2, du, axis)
	if !ok1 || !ok2 {
		return coplanarOverlap(n1, t1, t2)
	}
	return lo1 <= hi2+eps && lo2 <= hi1+eps
}

// sameSide reports whether all three distances sit strictly on one side, which
// means the triangle misses the plane entirely.
func sameSide(d [3]float64) bool {
	return (d[0] > eps && d[1] > eps && d[2] > eps) ||
		(d[0] < -eps && d[1] < -eps && d[2] < -eps)
}

func zero(v float64) bool { return math.Abs(v) < eps }

func axisValue(v geom.Vec3, axis int) float64 {
	switch axis {
	case 0:
		return v.X
	case 1:
		return v.Y
	}
	return v.Z
}

func dominantAxis(v geom.Vec3) int {
	ax, ay, az := math.Abs(v.X), math.Abs(v.Y), math.Abs(v.Z)
	if ay > ax && ay >= az {
		return 1
	}
	if az > ax {
		return 2
	}
	return 0
}

// interval is where a triangle crosses the intersection line, as a range on the
// projection axis. ok is false when the triangle lies in the other's plane, in
// which case there is no line to speak of.
func interval(t Triangle, d [3]float64, axis int) (lo, hi float64, ok bool) {
	i, j, k, ok := loneVertex(d)
	if !ok {
		return 0, 0, false
	}
	p := [3]float64{
		axisValue(t[i], axis), axisValue(t[j], axis), axisValue(t[k], axis),
	}
	// Where the two edges leaving the lone vertex cross the plane.
	a := p[0] + (p[1]-p[0])*d[i]/(d[i]-d[j])
	b := p[0] + (p[2]-p[0])*d[i]/(d[i]-d[k])
	return math.Min(a, b), math.Max(a, b), true
}

// loneVertex finds the vertex on its own side of the plane, returning it first
// and the other two after.
func loneVertex(d [3]float64) (i, j, k int, ok bool) {
	switch {
	case d[0]*d[1] > 0: // 0 and 1 together, so 2 is alone
		return 2, 0, 1, true
	case d[0]*d[2] > 0:
		return 1, 0, 2, true
	case d[1]*d[2] > 0 || !zero(d[0]):
		return 0, 1, 2, true
	case !zero(d[1]):
		return 1, 0, 2, true
	case !zero(d[2]):
		return 2, 0, 1, true
	}
	return 0, 0, 0, false // all three in the plane: coplanar
}

// coplanarOverlap is the 2D case: project both triangles onto the plane they
// share and test edges against edges, then containment either way.
//
// Whether this should count as an overlap at all is the question, and the
// answer for solids is no: two faces sharing a plane are two surfaces in
// contact, which is what a LEGO part does to the part beside it. Parts that
// genuinely interpenetrate always have some pair of faces crossing at an angle,
// and those are found by the 3D case above.
//
// It matters because in LDraw everything is nominal, so every real fit is an
// exact one: a driving ring's dogs meet a clutch gear's face, a half-width
// liftarm fills a 10 LDU groove to the LDU, a fork's tine sits in a channel cut
// to take it. Counting contact as collision made all three unbuildable, and
// each was worked around separately — a tolerance in the clearance check, a
// list of permitted nestings, an engaged distance nudged half a stud off the
// truth — before the common cause was named.
//
// See coplanarIsContact.
func coplanarOverlap(n geom.Vec3, t1, t2 Triangle) bool {
	if coplanarIsContact {
		return false
	}
	return coplanarOverlapIn2D(n, t1, t2)
}

// coplanarIsContact is whether faces sharing a plane are touching rather than
// overlapping. Kept as a name rather than a deletion so the old behaviour can
// be put back for one test and the difference measured.
const coplanarIsContact = true

func coplanarOverlapIn2D(n geom.Vec3, t1, t2 Triangle) bool {
	// Drop the axis the normal points most strongly along; the other two carry
	// the shape without distortion.
	drop := dominantAxis(n)
	u, v := 0, 1
	switch drop {
	case 0:
		u, v = 1, 2
	case 1:
		u, v = 0, 2
	}
	a := project(t1, u, v)
	b := project(t2, u, v)

	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if segmentsCross(a[i], a[(i+1)%3], b[j], b[(j+1)%3]) {
				return true
			}
		}
	}
	// No edge crossings: one may still be entirely inside the other.
	return pointInTriangle(a[0], b) || pointInTriangle(b[0], a)
}

type pt2 struct{ x, y float64 }

func project(t Triangle, u, v int) [3]pt2 {
	var out [3]pt2
	for i, p := range t {
		out[i] = pt2{axisValue(p, u), axisValue(p, v)}
	}
	return out
}

func cross2(o, a, b pt2) float64 {
	return (a.x-o.x)*(b.y-o.y) - (a.y-o.y)*(b.x-o.x)
}

func segmentsCross(p1, p2, q1, q2 pt2) bool {
	d1, d2 := cross2(q1, q2, p1), cross2(q1, q2, p2)
	d3, d4 := cross2(p1, p2, q1), cross2(p1, p2, q2)
	if ((d1 > eps && d2 < -eps) || (d1 < -eps && d2 > eps)) &&
		((d3 > eps && d4 < -eps) || (d3 < -eps && d4 > eps)) {
		return true
	}
	// Touching counts: a shared edge or a vertex on a segment is contact.
	return (zero(d1) && onSegment(q1, q2, p1)) ||
		(zero(d2) && onSegment(q1, q2, p2)) ||
		(zero(d3) && onSegment(p1, p2, q1)) ||
		(zero(d4) && onSegment(p1, p2, q2))
}

func onSegment(a, b, p pt2) bool {
	return p.x >= math.Min(a.x, b.x)-eps && p.x <= math.Max(a.x, b.x)+eps &&
		p.y >= math.Min(a.y, b.y)-eps && p.y <= math.Max(a.y, b.y)+eps
}

func pointInTriangle(p pt2, t [3]pt2) bool {
	d1 := cross2(t[0], t[1], p)
	d2 := cross2(t[1], t[2], p)
	d3 := cross2(t[2], t[0], p)
	hasNeg := d1 < -eps || d2 < -eps || d3 < -eps
	hasPos := d1 > eps || d2 > eps || d3 > eps
	return !(hasNeg && hasPos)
}
