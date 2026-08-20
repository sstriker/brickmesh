// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package collide

import (
	"math"
	"math/rand"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
)

// boxMesh is a closed axis-aligned box, the simplest thing with an inside.
func boxMesh(cx, cy, cz, sx, sy, sz float64) *Mesh {
	x0, x1 := cx-sx/2, cx+sx/2
	y0, y1 := cy-sy/2, cy+sy/2
	z0, z1 := cz-sz/2, cz+sz/2
	corner := func(i int) geom.Vec3 {
		x, y, z := x0, y0, z0
		if i&1 != 0 {
			x = x1
		}
		if i&2 != 0 {
			y = y1
		}
		if i&4 != 0 {
			z = z1
		}
		return geom.Vec3{X: x, Y: y, Z: z}
	}
	quads := [6][4]int{
		{0, 2, 6, 4}, {1, 5, 7, 3}, // -X, +X
		{0, 4, 5, 1}, {2, 3, 7, 6}, // -Y, +Y
		{0, 1, 3, 2}, {4, 6, 7, 5}, // -Z, +Z
	}
	var tris []Triangle
	for _, q := range quads {
		tris = append(tris,
			Triangle{corner(q[0]), corner(q[1]), corner(q[2])},
			Triangle{corner(q[0]), corner(q[2]), corner(q[3])})
	}
	return NewMesh(tris)
}

func TestOverlappingBoxesCollide(t *testing.T) {
	a := boxMesh(0, 0, 0, 20, 20, 20)
	b := boxMesh(10, 0, 0, 20, 20, 20)
	if !Intersects(a, Identity(), b, Identity()) {
		t.Error("boxes sharing half their width should collide")
	}
}

func TestSeparatedBoxesDoNot(t *testing.T) {
	a := boxMesh(0, 0, 0, 20, 20, 20)
	b := boxMesh(100, 0, 0, 20, 20, 20)
	if Intersects(a, Identity(), b, Identity()) {
		t.Error("boxes 100 LDU apart should not collide")
	}
}

func TestTranslationIsHonored(t *testing.T) {
	a := boxMesh(0, 0, 0, 20, 20, 20)
	b := boxMesh(0, 0, 0, 20, 20, 20)
	if Intersects(a, Identity(), b, Transform{Rot: Identity().Rot, Pos: geom.Vec3{X: 100}}) {
		t.Error("moving the second box away should separate them")
	}
	if !Intersects(a, Identity(), b, Transform{Rot: Identity().Rot, Pos: geom.Vec3{X: 10}}) {
		t.Error("moving it halfway should still overlap")
	}
}

func TestRotationIsHonored(t *testing.T) {
	// A long thin bar, and the same bar turned across it. Placed apart along
	// the bar's length they miss; turned, the second sweeps into the first.
	a := boxMesh(0, 0, 0, 100, 6, 6)
	b := boxMesh(0, 0, 0, 100, 6, 6)
	// a spans -50..50; at 120 the second spans 70..170, genuinely clear.
	along := Transform{Rot: Identity().Rot, Pos: geom.Vec3{X: 120}}
	if Intersects(a, Identity(), b, along) {
		t.Error("two bars end to end should not overlap")
	}

	var acrossRot geom.Mat3
	for _, m := range geom.Rotations {
		if m.Apply(geom.Vec3{X: 1}).Sub(geom.Vec3{Y: 1}).Len() < 1e-9 {
			acrossRot = m
			break
		}
	}
	across := Transform{Rot: acrossRot, Pos: geom.Vec3{X: 40}}
	if !Intersects(a, Identity(), b, across) {
		t.Error("a bar turned across the other should overlap it")
	}
}

// Both meshes are moved, so the relative transform is what has to be right.
func TestBothTransformsCompose(t *testing.T) {
	a := boxMesh(0, 0, 0, 20, 20, 20)
	b := boxMesh(0, 0, 0, 20, 20, 20)
	ta := Transform{Rot: geom.Rotations[3], Pos: geom.Vec3{X: 50, Y: -30}}
	// Same place, so they must collide however both are moved.
	if !Intersects(a, ta, b, ta) {
		t.Error("two meshes at the same transform overlap")
	}
	tb := ta
	tb.Pos = tb.Pos.Add(geom.Vec3{X: 200})
	if Intersects(a, ta, b, tb) {
		t.Error("200 LDU apart is 200 LDU apart, whatever the frame")
	}
}

// A box inside another shares no surface, and the hierarchy must not conclude
// from that alone that nothing is happening — but with surface meshes there is
// genuinely no triangle contact, which is the same convention the voxel grid
// uses. Pinning it so a change of convention is deliberate.
func TestAContainedBoxTouchesNoSurface(t *testing.T) {
	outer := boxMesh(0, 0, 0, 100, 100, 100)
	inner := boxMesh(0, 0, 0, 10, 10, 10)
	if Intersects(outer, Identity(), inner, Identity()) {
		t.Error("surfaces that do not cross are not in contact")
	}
}

func TestAnEmptyMeshCollidesWithNothing(t *testing.T) {
	empty := NewMesh(nil)
	solid := boxMesh(0, 0, 0, 20, 20, 20)
	if Intersects(empty, Identity(), solid, Identity()) ||
		Intersects(solid, Identity(), empty, Identity()) {
		t.Error("an empty mesh has nothing to collide with")
	}
}

// The hierarchy is an optimization, so it has to agree with the brute-force
// answer every time. Random placements, checked both ways.
func TestHierarchyAgreesWithBruteForce(t *testing.T) {
	a := boxMesh(0, 0, 0, 40, 20, 20)
	b := boxMesh(0, 0, 0, 30, 30, 10)
	rng := rand.New(rand.NewSource(7))

	for i := 0; i < 300; i++ {
		tb := Transform{
			Rot: geom.Rotations[rng.Intn(len(geom.Rotations))],
			Pos: geom.Vec3{
				X: rng.Float64()*80 - 40,
				Y: rng.Float64()*80 - 40,
				Z: rng.Float64()*80 - 40,
			},
		}
		want := bruteForce(a, Identity(), b, tb)
		if got := Intersects(a, Identity(), b, tb); got != want {
			t.Fatalf("placement %d at %+v: hierarchy said %v, brute force %v",
				i, tb.Pos, got, want)
		}
	}
}

func bruteForce(a *Mesh, ta Transform, b *Mesh, tb Transform) bool {
	rel := tb.relativeTo(ta)
	for _, x := range a.Triangles() {
		for _, y := range b.Triangles() {
			moved := Triangle{rel.Apply(y[0]), rel.Apply(y[1]), rel.Apply(y[2])}
			if TrianglesOverlap(x, moved) {
				return true
			}
		}
	}
	return false
}

func TestRelativeTransformRoundTrips(t *testing.T) {
	ta := Transform{Rot: geom.Rotations[5], Pos: geom.Vec3{X: 10, Y: 20, Z: 30}}
	tb := Transform{Rot: geom.Rotations[11], Pos: geom.Vec3{X: -5, Y: 7, Z: 2}}
	rel := tb.relativeTo(ta)

	p := geom.Vec3{X: 1, Y: 2, Z: 3}
	// A point of B, expressed in A's frame, then back out to the world.
	got := ta.Apply(rel.Apply(p))
	if want := tb.Apply(p); got.Sub(want).Len() > 1e-9 {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestBoxesGrowToContainTheirCorners(t *testing.T) {
	b := emptyBox().grow(geom.Vec3{X: 1, Y: 2, Z: 3}).grow(geom.Vec3{X: -4, Y: 0, Z: 5})
	if b.lo != (geom.Vec3{X: -4, Y: 0, Z: 3}) || b.hi != (geom.Vec3{X: 1, Y: 2, Z: 5}) {
		t.Errorf("bounds %+v..%+v", b.lo, b.hi)
	}
}

// A rotated box needs a bigger axis-aligned box around it, never a smaller one:
// a broad phase that under-reports would drop real collisions.
func TestTransformedBoxNeverShrinks(t *testing.T) {
	b := box{lo: geom.Vec3{X: -10, Y: -10, Z: -10}, hi: geom.Vec3{X: 10, Y: 10, Z: 10}}
	for _, m := range geom.Rotations {
		moved := transformedBox(b, Transform{Rot: m, Pos: geom.Vec3{X: 5}})
		for _, c := range b.corners() {
			w := m.Apply(c).Add(geom.Vec3{X: 5})
			if w.X < moved.lo.X-1e-9 || w.X > moved.hi.X+1e-9 ||
				w.Y < moved.lo.Y-1e-9 || w.Y > moved.hi.Y+1e-9 ||
				w.Z < moved.lo.Z-1e-9 || w.Z > moved.hi.Z+1e-9 {
				t.Fatalf("corner %+v fell outside %+v..%+v", w, moved.lo, moved.hi)
			}
		}
	}
}

func TestEveryTriangleEndsUpInExactlyOneLeaf(t *testing.T) {
	m := boxMesh(0, 0, 0, 40, 20, 20)
	seen := map[int]int{}
	for i := range m.nodes {
		if !m.isLeaf(i) {
			continue
		}
		n := m.nodes[i]
		for _, idx := range m.order[n.first : n.first+n.n] {
			seen[idx]++
		}
	}
	if len(seen) != len(m.Triangles()) {
		t.Errorf("%d triangles in leaves, mesh has %d", len(seen), len(m.Triangles()))
	}
	for idx, count := range seen {
		if count != 1 {
			t.Errorf("triangle %d appears in %d leaves", idx, count)
		}
	}
}

func BenchmarkIntersects(b *testing.B) {
	x := boxMesh(0, 0, 0, 40, 20, 20)
	y := boxMesh(0, 0, 0, 40, 20, 20)
	tb := Transform{Rot: Identity().Rot, Pos: geom.Vec3{X: 39}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !Intersects(x, Identity(), y, tb) {
			b.Fatal("expected an overlap")
		}
	}
}

var _ = math.Abs
