// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package part

import "brickmesh/internal/geom"

// Shape is a part's triangles in its own frame.
//
// Named rather than reused from internal/ldraw because it is what the engine
// needs and not what a .dat file happens to hold: a browser reads the same
// triangles out of meshes.bin, with no .dat and no parser anywhere near it.
type Shape struct {
	Name  string
	Title string
	Verts []geom.Vec3
	Tris  [][3]geom.Vec3
}

// BBox returns the low and high corners.
func (g *Shape) BBox() (lo, hi geom.Vec3) {
	if len(g.Verts) == 0 {
		return
	}
	lo, hi = g.Verts[0], g.Verts[0]
	for _, v := range g.Verts[1:] {
		lo = geom.Vec3{X: min(lo.X, v.X), Y: min(lo.Y, v.Y), Z: min(lo.Z, v.Z)}
		hi = geom.Vec3{X: max(hi.X, v.X), Y: max(hi.Y, v.Y), Z: max(hi.Z, v.Z)}
	}
	return
}

func (g *Shape) Size() geom.Vec3 {
	lo, hi := g.BBox()
	return hi.Sub(lo)
}

// ThinAxis is the index of the shortest bbox dimension. For disc-shaped parts
// that is the rotation axis in the part's default orientation. Reported so it
// can be eyeballed, never trusted blindly — the shadow library knows better.
func (g *Shape) ThinAxis() int {
	s := g.Size()
	idx, best := 0, s.X
	if s.Y < best {
		idx, best = 1, s.Y
	}
	if s.Z < best {
		idx = 2
	}
	return idx
}

// Shapes is where triangles come from.
//
// The whole of what the engine asks of a parts library. internal/ldraw answers
// it by parsing .dat files and following their references; internal/assets
// answers it from the mesh blob a browser downloads. Everything that measures
// geometry — the voxel grid, the meshing sweep, the tooth phase, the clearance
// check — takes one of these rather than a library, so none of them care which.
type Shapes interface {
	Geometry(part string) (*Shape, error)
}

// Hole is a connection point on a part, in the part's own frame.
//
// A neutral shape for what the shadow library describes and what the published
// catalogue carries, so the engine can be handed either. Round or cross is the
// distinction that matters most: an axle spins inside a round hole and is keyed
// into a cross one, which decides both what bears a shaft and what turns with
// it.
type Hole struct {
	Pos, Axis geom.Vec3
	Cross     bool
}

// Holes is where connection points come from — the shadow library on a machine
// with one, the published catalogue in a browser.
type Holes interface {
	AxisSource
	Holes(part string) []Hole
}
