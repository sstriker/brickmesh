// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package collide answers whether two parts share space.
//
// The voxel grid is the coarse filter and answers most questions; this is the
// exact test behind it, for the cases where a hole has to stay a hole and a
// tooth flank has to miss another by half an LDU.
//
// A real gear is a few thousand triangles, so comparing every pair against
// every other is out of the question: two 24-tooth gears would be six million
// tests per orientation, and a meshing check sweeps hundreds of orientations.
// Each mesh therefore carries a bounding-volume hierarchy, and the traversal
// descends only into boxes that actually overlap.
package collide

import (
	"math"

	"github.com/sstriker/brickmesh/internal/geom"
)

// Transform places a mesh in the world.
type Transform struct {
	Rot geom.Mat3
	Pos geom.Vec3
}

// Identity leaves a mesh where it is.
func Identity() Transform {
	return Transform{Rot: geom.Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}
}

// Apply maps a point from the mesh's frame into the world.
func (t Transform) Apply(v geom.Vec3) geom.Vec3 { return t.Rot.Apply(v).Add(t.Pos) }

// relativeTo expresses this transform in the frame of another, so one mesh can
// be tested against the other without moving either into world coordinates.
func (t Transform) relativeTo(other Transform) Transform {
	inv := other.Rot.Transpose()
	return Transform{
		Rot: inv.Mul(t.Rot),
		Pos: inv.Apply(t.Pos.Sub(other.Pos)),
	}
}

type box struct{ lo, hi geom.Vec3 }

func (b box) overlaps(o box) bool {
	return b.lo.X <= o.hi.X && o.lo.X <= b.hi.X &&
		b.lo.Y <= o.hi.Y && o.lo.Y <= b.hi.Y &&
		b.lo.Z <= o.hi.Z && o.lo.Z <= b.hi.Z
}

func (b box) grow(v geom.Vec3) box {
	return box{
		lo: geom.Vec3{X: math.Min(b.lo.X, v.X), Y: math.Min(b.lo.Y, v.Y), Z: math.Min(b.lo.Z, v.Z)},
		hi: geom.Vec3{X: math.Max(b.hi.X, v.X), Y: math.Max(b.hi.Y, v.Y), Z: math.Max(b.hi.Z, v.Z)},
	}
}

func emptyBox() box {
	inf := math.Inf(1)
	return box{lo: geom.Vec3{X: inf, Y: inf, Z: inf},
		hi: geom.Vec3{X: -inf, Y: -inf, Z: -inf}}
}

// corners are needed when a box has to be re-bounded in another frame.
func (b box) corners() [8]geom.Vec3 {
	return [8]geom.Vec3{
		{X: b.lo.X, Y: b.lo.Y, Z: b.lo.Z}, {X: b.hi.X, Y: b.lo.Y, Z: b.lo.Z},
		{X: b.lo.X, Y: b.hi.Y, Z: b.lo.Z}, {X: b.hi.X, Y: b.hi.Y, Z: b.lo.Z},
		{X: b.lo.X, Y: b.lo.Y, Z: b.hi.Z}, {X: b.hi.X, Y: b.lo.Y, Z: b.hi.Z},
		{X: b.lo.X, Y: b.hi.Y, Z: b.hi.Z}, {X: b.hi.X, Y: b.hi.Y, Z: b.hi.Z},
	}
}

// transformedBox re-bounds a box in another frame. Axis-aligned boxes do not
// survive rotation, so the result is the box around the rotated one: bigger
// than necessary, but never smaller, which is what a broad phase must be.
func transformedBox(b box, t Transform) box {
	out := emptyBox()
	for _, c := range b.corners() {
		out = out.grow(t.Apply(c))
	}
	return out
}

const leafSize = 4

type node struct {
	bounds      box
	left, right int // -1 at a leaf
	first, n    int // triangle range, at a leaf
}

// Mesh is a triangle soup with a hierarchy over it.
type Mesh struct {
	tris  []Triangle
	order []int // triangle indices, permuted by the build
	nodes []node
}

// NewMesh builds the hierarchy. Do this once per part; the result is read-only
// and safe to share between goroutines.
func NewMesh(tris []Triangle) *Mesh {
	m := &Mesh{tris: tris, order: make([]int, len(tris))}
	for i := range m.order {
		m.order[i] = i
	}
	if len(tris) == 0 {
		return m
	}
	m.build(0, len(tris))
	return m
}

// Triangles returns the mesh's triangles.
func (m *Mesh) Triangles() []Triangle { return m.tris }

func (m *Mesh) centroid(i int) geom.Vec3 {
	t := m.tris[i]
	return t[0].Add(t[1]).Add(t[2]).Scale(1.0 / 3.0)
}

// build splits a range of triangles and returns the node index covering it.
func (m *Mesh) build(first, n int) int {
	bounds := emptyBox()
	for _, idx := range m.order[first : first+n] {
		for _, v := range m.tris[idx] {
			bounds = bounds.grow(v)
		}
	}
	self := len(m.nodes)
	m.nodes = append(m.nodes, node{bounds: bounds, left: -1, right: -1, first: first, n: n})
	if n <= leafSize {
		return self
	}

	// Split down the middle of the longest axis, by centroid.
	size := bounds.hi.Sub(bounds.lo)
	axis := 0
	if size.Y > size.X && size.Y >= size.Z {
		axis = 1
	} else if size.Z > size.X {
		axis = 2
	}
	mid := (axisValue(bounds.lo, axis) + axisValue(bounds.hi, axis)) / 2

	lo, hi := first, first+n-1
	for lo <= hi {
		if axisValue(m.centroid(m.order[lo]), axis) < mid {
			lo++
		} else {
			m.order[lo], m.order[hi] = m.order[hi], m.order[lo]
			hi--
		}
	}
	split := lo - first
	// Everything landed on one side: fall back to halving, which still
	// terminates and keeps the tree balanced enough.
	if split == 0 || split == n {
		split = n / 2
	}

	left := m.build(first, split)
	right := m.build(first+split, n-split)
	m.nodes[self].left = left
	m.nodes[self].right = right
	m.nodes[self].n = 0 // no longer a leaf
	return self
}

func (m *Mesh) isLeaf(i int) bool { return m.nodes[i].left < 0 }

// Intersects reports whether two placed meshes share any point.
func Intersects(a *Mesh, ta Transform, b *Mesh, tb Transform) bool {
	if len(a.tris) == 0 || len(b.tris) == 0 {
		return false
	}
	// Work in A's frame: only B has to move.
	rel := tb.relativeTo(ta)

	// B's triangles and node boxes, carried over once rather than per pair.
	moved := make([]Triangle, len(b.tris))
	for i, t := range b.tris {
		moved[i] = Triangle{rel.Apply(t[0]), rel.Apply(t[1]), rel.Apply(t[2])}
	}
	boxes := make([]box, len(b.nodes))
	for i, n := range b.nodes {
		boxes[i] = transformedBox(n.bounds, rel)
	}

	type pair struct{ a, b int }
	stack := []pair{{0, 0}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if !a.nodes[p.a].bounds.overlaps(boxes[p.b]) {
			continue
		}
		aLeaf, bLeaf := a.isLeaf(p.a), b.isLeaf(p.b)
		switch {
		case aLeaf && bLeaf:
			if leavesOverlap(a, p.a, moved, b, p.b) {
				return true
			}
		case aLeaf:
			stack = append(stack, pair{p.a, b.nodes[p.b].left}, pair{p.a, b.nodes[p.b].right})
		case bLeaf:
			stack = append(stack, pair{a.nodes[p.a].left, p.b}, pair{a.nodes[p.a].right, p.b})
		default:
			// Descend the bulkier side first; it prunes faster.
			if volume(a.nodes[p.a].bounds) > volume(boxes[p.b]) {
				stack = append(stack, pair{a.nodes[p.a].left, p.b}, pair{a.nodes[p.a].right, p.b})
			} else {
				stack = append(stack, pair{p.a, b.nodes[p.b].left}, pair{p.a, b.nodes[p.b].right})
			}
		}
	}
	return false
}

func leavesOverlap(a *Mesh, an int, moved []Triangle, b *Mesh, bn int) bool {
	na, nb := a.nodes[an], b.nodes[bn]
	for _, i := range a.order[na.first : na.first+na.n] {
		for _, j := range b.order[nb.first : nb.first+nb.n] {
			if TrianglesOverlap(a.tris[i], moved[j]) {
				return true
			}
		}
	}
	return false
}

func volume(b box) float64 {
	s := b.hi.Sub(b.lo)
	return math.Max(s.X, 0) * math.Max(s.Y, 0) * math.Max(s.Z, 0)
}
