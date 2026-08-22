// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/part"
	"github.com/sstriker/brickmesh/internal/voxel"
)

// Bearing is an axis a model already offers to hold a shaft on.
//
// Two round holes facing the same way with their centres on one line is a
// shaft's worth of support: that is what a bearing IS, and it is the thing a
// mechanism has to be fitted to when the frame is given rather than searched
// for.
type Bearing struct {
	// At is the foot of the line from the origin, and Axis the way it runs.
	At, Axis geom.Vec3
	// Holes is how many the line passes through, and From/To how far apart the
	// outermost two are, measured along the axis from At.
	Holes    int
	From, To float64
	// Parts is how many distinct parts contribute a hole to it. One part with
	// several holes in a row is not two bearings, it is one beam.
	Parts int
}

// Span is how long a shaft on this line could be.
func (b Bearing) Span() float64 { return b.To - b.From }

// Bearings is every axis the model already supports a shaft on.
//
// Round holes only. An axle is keyed into a cross hole and turns in a round
// one, so a cross hole is somewhere a shaft is HELD, not somewhere it turns —
// part.Hole.Cross again, and the same distinction that separates a gear which
// drives its shaft from one that freewheels on it.
func (r *Reading) Bearings(src part.Holes) []Bearing {
	if src == nil {
		return nil
	}
	type hole struct {
		at   geom.Vec3
		part int
	}
	byLine := map[[6]float64][]hole{}
	for i, f := range r.Parts {
		// Structure only. A clutch gear has a round bore and an axle really
		// does turn in it, but a gear is not support: it is a thing the shaft
		// carries, not a thing that carries the shaft. Counting bores made the
		// two-speed's output line look like a four-point bearing when it is a
		// two-point one with two gears strung between.
		if f.Class != classStructure {
			continue
		}
		// Carried over by the part's own matrix rather than by a lattice
		// index. part.WorldPorts takes one of the 24 rotations, which is what
		// the structural search works in — but a model being READ is under no
		// such obligation. A suspension arm sits where the geometry puts it,
		// and this engine's own gears are turned by a fraction of a tooth to
		// interleave, so almost nothing in a real model is on the lattice.
		for _, h := range src.Holes(f.Name) {
			if h.Cross {
				continue // keyed, so nothing turns in it
			}
			at := f.Rot.Apply(h.Pos).Add(f.Pos)
			axis := f.Rot.Apply(h.Axis).Unit()
			byLine[lineKey(at, axis)] = append(byLine[lineKey(at, axis)], hole{at, i})
		}
	}

	var out []Bearing
	for k, holes := range byLine {
		if len(holes) < 2 {
			continue // one hole holds a pin, not a shaft
		}
		axis := geom.Vec3{X: k[3], Y: k[4], Z: k[5]}
		at := geom.Vec3{X: k[0], Y: k[1], Z: k[2]}
		lo, hi := math.Inf(1), math.Inf(-1)
		parts := map[int]bool{}
		for _, h := range holes {
			t := h.at.Sub(at).Dot(axis)
			lo, hi = math.Min(lo, t), math.Max(hi, t)
			parts[h.part] = true
		}
		out = append(out, Bearing{
			At: at, Axis: axis, Holes: len(holes),
			From: lo, To: hi, Parts: len(parts),
		})
	}
	// Longest first: the most room for a mechanism is the most interesting.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Span() != out[j].Span() {
			return out[i].Span() > out[j].Span()
		}
		return lessKey(lineKey(out[i].At, out[i].Axis), lineKey(out[j].At, out[j].Axis))
	})
	return out
}

// ReportBearings says what a model offers to build in.
func (r *Reading) ReportBearings(src part.Holes) []mech.Finding {
	all := r.Bearings(src)
	if len(all) == 0 {
		return []mech.Finding{{Level: "WARN", Check: "fit", Detail: "no line in " +
			"this model has two round holes on it, so there is nowhere a shaft " +
			"could turn. Either it is not a Technic model or none of it was " +
			"recognised"}}
	}
	// Only the ones a mechanism could actually use: a shaft wants its bearings
	// apart, and two holes in one beam a stud apart is not a bearing base.
	usable := 0
	for _, b := range all {
		if b.Span() >= 2*geom.Stud && b.Parts >= 2 {
			usable++
		}
	}
	out := []mech.Finding{{Level: "OK", Check: "fit", Detail: fmt.Sprintf(
		"%d axis line(s) with two or more round holes; %d of them span two "+
			"studs or more across two or more parts, which is what a shaft "+
			"needs to turn in", len(all), usable)}}
	for i, b := range all {
		if i >= 5 || b.Parts < 2 || b.Span() < 2*geom.Stud {
			break
		}
		out = append(out, mech.Finding{Level: "OK", Check: "fit", Detail: fmt.Sprintf(
			"  along %v through %v: %.0f LDU between %d hole(s) in %d part(s)",
			b.Axis, b.At, b.Span(), b.Holes, b.Parts)})
	}
	return out
}

// Occupied is the space a mechanism fitted into this model may not enter.
//
// Structure and bodywork, not the parts of a mechanism. A gear already in the
// model is not an obstacle in the same sense: it is somebody's drivetrain, and
// whether a new one can share a model with it is a question about what drives
// what, which this does not answer. Counting them made a model fitted with its
// OWN mechanism report four gears in the way — all four of them the ones it was
// asking about.
//
// Built with VoxelsAt rather than Voxels because a model being read is not on
// the lattice: 42110's chassis is turned about three thousandths of a radian,
// and asking for a lattice rotation index would have skipped nearly all of it.
func (r *Reading) Occupied(rast *voxel.Rasterizer) map[geom.Cell]bool {
	if rast == nil {
		return nil
	}
	out := map[geom.Cell]bool{}
	for _, f := range r.Parts {
		switch f.Class {
		case classGear, classRing, classJoiner, classAxle, classSelector:
			continue // a mechanism, not the room around one
		}
		cells, err := rast.VoxelsAt(f.Name, f.Rot)
		if err != nil {
			continue
		}
		shift := geom.Cell{
			X: int32(math.Round(f.Pos.X / geom.VoxelPitch)),
			Y: int32(math.Round(f.Pos.Y / geom.VoxelPitch)),
			Z: int32(math.Round(f.Pos.Z / geom.VoxelPitch)),
		}
		for _, c := range cells {
			out[c.Add(shift)] = true
		}
	}
	// Eroded, for the reason turningCells erodes: the rasteriser marks every
	// cell a part so much as touches, and parts in a model touch each other by
	// design. Without it a gear resting against the wall that bears its shaft
	// counts as a gear inside the wall, and the two-speed reported two of its
	// own gears clashing with the frame built to clear them.
	return erode(out)
}
