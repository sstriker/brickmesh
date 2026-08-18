// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"math"

	"brickmesh/internal/extract"
	"brickmesh/internal/geom"
)

// FromRecords turns what the extractor produced into a catalog.
//
// The kind byte packs the two things a port is: whether it takes a pin or an
// axle, and whether it is the hole or the thing that goes in it. The engine's
// own catalog.PortKind and catalog.Gender are ordered so the same two bits mean
// the same two things on both sides.
func FromRecords(records []extract.Record) Catalog {
	c := Catalog{Parts: make([]Part, 0, len(records))}
	for _, r := range records {
		p := Part{ID: r.ID, Title: r.Title, Tier: r.Tier}
		for _, h := range r.Holes {
			p.Ports = append(p.Ports, portFrom(h, 0))
		}
		for _, m := range r.Pins {
			p.Ports = append(p.Ports, portFrom(m, PortMale))
		}
		c.Parts = append(c.Parts, p)
	}
	return c
}

func portFrom(v extract.Port, gender uint8) Port {
	kind := gender
	if v[6] != 0 {
		kind |= PortCross
	}
	return Port{
		X: float32(v[0]), Y: float32(v[1]), Z: float32(v[2]),
		AX: float32(v[3]), AY: float32(v[4]), AZ: float32(v[5]),
		Kind: kind,
	}
}

// IndexTriangles turns a flat run of triangles into shared vertices.
//
// LDraw parts are built from primitives that meet edge to edge, so the same
// corner turns up in many triangles: a 24-tooth gear comes to 2536 triangles
// and far fewer distinct corners. Sharing them is most of why the mesh file is
// worth generating rather than shipping the .dat files.
//
// Vertices are matched on their float32 bits rather than by comparing floats.
// Two corners that came from the same primitive are bit-identical, and two that
// merely happen to be close should stay separate — welding them would move
// geometry that the collision tests are measured against.
func IndexTriangles(tris [][3]geom.Vec3) Mesh {
	m := Mesh{
		Positions: make([]float32, 0, len(tris)*9),
		Indices:   make([]uint32, 0, len(tris)*3),
	}
	type key struct{ x, y, z uint32 }
	seen := make(map[key]uint32, len(tris)*3)

	for _, t := range tris {
		for _, v := range t {
			x, y, z := float32(v.X), float32(v.Y), float32(v.Z)
			k := key{
				math.Float32bits(x), math.Float32bits(y), math.Float32bits(z),
			}
			idx, ok := seen[k]
			if !ok {
				idx = uint32(len(m.Positions) / 3)
				seen[k] = idx
				m.Positions = append(m.Positions, x, y, z)
			}
			m.Indices = append(m.Indices, idx)
		}
	}
	return m
}
