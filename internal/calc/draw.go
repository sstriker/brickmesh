// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"encoding/binary"
	"math"

	"brickmesh/internal/geom"
	"brickmesh/internal/ldr"
	"brickmesh/internal/part"
)

// DrawBuffer is a model flattened into something a graphics card can take:
// every triangle in the model's own coordinates, with a normal and a colour.
//
// Flattened rather than sent as parts and transforms because the parts are few
// and the transforms are fixed. Drawing is not where this engine is interesting,
// and a buffer the page uploads once is less machinery than an instancing
// scheme that would draw the same picture.
//
// Nine float32 a vertex: position, normal, colour. About six megabytes for a
// compound gearbox, handed over once per build.
const drawStride = 9

// Draw flattens a model for the page to render.
//
// Normals per triangle rather than per vertex. LDraw parts are open shells with
// inconsistent winding — the differential alone has 835 boundary loops, see
// docs/findings.md — so averaging normals across a vertex produces nonsense at
// every seam. Flat shading is honest about what the geometry is.
func Draw(model *ldr.Model, shapes part.Shapes) []byte {
	if model == nil {
		return nil
	}
	var out []byte
	buf := make([]byte, 4)
	put := func(v float64) {
		binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(v)))
		out = append(out, buf...)
	}

	for _, p := range model.Parts {
		g, err := shapes.Geometry(p.Name)
		if err != nil {
			continue // reported elsewhere; a missing part is not a reason to draw nothing
		}
		r, gr, b := colourOf(p.Color)
		for _, t := range g.Tris {
			a := p.Rot.Apply(t[0]).Add(p.Pos)
			c := p.Rot.Apply(t[1]).Add(p.Pos)
			d := p.Rot.Apply(t[2]).Add(p.Pos)
			n := c.Sub(a).Cross(d.Sub(a))
			if n.Len() > 1e-12 {
				n = n.Unit()
			}
			for _, v := range [3]geom.Vec3{a, c, d} {
				// LDraw has +Y downward; the page's camera does not, so the
				// sign is flipped once here rather than in every shader.
				put(v.X)
				put(-v.Y)
				put(v.Z)
				put(n.X)
				put(-n.Y)
				put(n.Z)
				put(r)
				put(gr)
				put(b)
			}
		}
	}
	return out
}

// colourOf maps the LDraw colours this engine places. Anything else comes out
// grey, which is what an unrecognised part should look like.
func colourOf(code int) (r, g, b float64) {
	switch code {
	case 0: // black: the axles
		return 0.13, 0.13, 0.15
	case 4: // red: the driving rings
		return 0.78, 0.16, 0.14
	case 71: // light bluish grey: the gears
		return 0.63, 0.65, 0.66
	}
	return 0.45, 0.47, 0.50
}
