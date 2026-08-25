// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"encoding/binary"
	"math"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/part"
)

// DrawBuffer is a model flattened into something a graphics card can take:
// every triangle in the model's own coordinates, with a normal and a colour.
//
// Flattened rather than sent as parts and transforms because the parts are few
// and the transforms are fixed. Drawing is not where this engine is interesting,
// and a buffer the page uploads once is less machinery than an instancing
// scheme that would draw the same picture.
//
// Eleven float32 a vertex: position, normal, colour, whether a finding is about
// this part, and which animation group it belongs to. About six megabytes for a
// compound gearbox, handed over once per build.
//
// The group is an index rather than a name because it is read by a shader: the
// page uploads one transform per group and each vertex picks its own. That is
// what lets the model move without re-uploading it every frame — a compound
// gearbox is seventy thousand vertices, and rebuilding that buffer sixty times
// a second is not a thing to do on the page's thread.
const drawStride = 11

// DrawGroups is the most groups a model may have for the page to animate it.
//
// A shader's uniform space is small and guaranteed smaller: WebGL 1 promises
// only 128 vertex uniform vectors, and a mat4 costs four. Twenty-four leaves
// room for the camera and covers what this engine builds — a four-speed
// compound, its widest, uses eleven. Past that the model still draws, and the
// parts beyond simply do not move.
const DrawGroups = 24

// Draw flattens a model for the page to render.
//
// Normals per triangle rather than per vertex. LDraw parts are open shells with
// inconsistent winding — the differential alone has 835 boundary loops, see
// docs/findings.md — so averaging normals across a vertex produces nonsense at
// every seam. Flat shading is honest about what the geometry is.
func Draw(model *ldr.Model, shapes part.Shapes) []byte {
	return DrawFlagging(model, shapes, nil)
}

// DrawFlagging is Draw with some parts marked, by index into the model.
//
// A tenth float per vertex: 0 for an ordinary part, 1 for one a finding is
// about. The page decides what that looks like — the engine's job is to say
// which parts the sentence means, not to choose a colour for them.
//
// This is the reason for drawing a model at all. Stud.io renders better and
// docs/architecture.md says so; what nothing else does is show WHICH parts a
// verdict is about. "60485.dat at {X:80 Y:0 Z:-40} is inside 32523.dat" is a
// sentence the reader has to go and find. A part lit up is not.
func DrawFlagging(model *ldr.Model, shapes part.Shapes, flagged map[int]bool) []byte {
	return drawWith(model, shapes, flagged)
}

// GroupOrder is the group names in the order DrawFlagging numbers them, so the
// page can match a transform to the index its vertices carry.
func GroupOrder(model *ldr.Model) []string {
	if model == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range model.Parts {
		if p.Group == "" || seen[p.Group] {
			continue
		}
		seen[p.Group] = true
		out = append(out, p.Group)
	}
	return out
}

func drawWith(model *ldr.Model, shapes part.Shapes, flagged map[int]bool) []byte {
	if model == nil {
		return nil
	}
	var out []byte
	buf := make([]byte, 4)
	put := func(v float64) {
		binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(v)))
		out = append(out, buf...)
	}

	// Group names in the order the model declares them, so a vertex can carry
	// an index the shader understands.
	group := map[string]int{}
	for _, p := range model.Parts {
		if p.Group == "" {
			continue
		}
		if _, seen := group[p.Group]; !seen {
			group[p.Group] = len(group) + 1 // 0 is "moves with nothing"
		}
	}

	for i, p := range model.Parts {
		g, err := shapes.Geometry(p.Name)
		if err != nil {
			continue // reported elsewhere; a missing part is not a reason to draw nothing
		}
		r, gr, b := colourOf(p.Color)
		mark := 0.0
		if flagged[i] {
			mark = 1
		}
		at := float64(group[p.Group])
		if at >= DrawGroups {
			at = 0 // past what the shader holds: drawn, but it will not move
		}
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
				put(mark)
				put(at)
			}
		}
	}
	return out
}

// colourOf maps the LDraw colours this engine places, at LDraw's own values.
// Anything else comes out grey, which is what an unrecognised part should look
// like.
//
// The list grew when parts started being placed in the colours they are
// actually made in rather than in whatever was convenient. Three entries
// covered black axles, red rings and grey gears; a white joiner and an orange
// catch both came out the same anonymous grey until they were added, which is
// worse than the wrong colour because it looks deliberate.
func colourOf(code int) (r, g, b float64) {
	switch code {
	case 0: // black
		return 0.02, 0.07, 0.11
	case 1: // blue: the long pins
		return 0.00, 0.33, 0.75
	case 4: // red
		return 0.79, 0.10, 0.04
	case 7: // light grey, the older one
		return 0.61, 0.63, 0.62
	case 8: // dark grey, the older one
		return 0.43, 0.43, 0.36
	case 14: // yellow
		return 0.95, 0.80, 0.22
	case 15: // white
		return 1.00, 1.00, 1.00
	case 25: // orange
		return 1.00, 0.54, 0.09
	case 71: // light bluish grey
		return 0.63, 0.65, 0.66
	case 72: // dark bluish grey
		return 0.42, 0.43, 0.41
	case 322: // medium azure
		return 0.42, 0.77, 0.87
	}
	return 0.45, 0.47, 0.50
}
