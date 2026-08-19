// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package part is the vocabulary the structural search and the rigidity check
// share: a placed part, where a beam's holes are, and the inventory to draw
// from.
//
// It exists as its own package because both of those need it and they need each
// other: the search repairs connectivity, which is the rigidity check's job.
// Python resolves that with an import inside the function; Go wants the shared
// nouns to live somewhere neither side owns.
package part

import (
	"fmt"

	"brickmesh/internal/geom"
)

// Stud in LDU. Beam holes sit one stud apart.
const Stud = 20.0

// Placed is a part at a lattice position in one of the 24 orientations.
type Placed struct {
	Part   string
	Rot    int
	Origin geom.Vec3
}

// Beam is a load-bearing part and its hole count.
type Beam struct {
	Part  string
	Holes int
}

// Beams is the inventory the structural search draws from: the straight
// liftarms, shortest first.
var Beams = []Beam{
	{"32523.dat", 3}, {"32316.dat", 5}, {"32524.dat", 7},
	{"40490.dat", 9}, {"32525.dat", 11}, {"41239.dat", 13},
}

// HoleCounts indexes an inventory by part.
func HoleCounts(inventory []Beam) map[string]int {
	out := make(map[string]int, len(inventory))
	for _, b := range inventory {
		out[b.Part] = b.Holes
	}
	return out
}

// HoleOffsets are the local hole positions along a beam's length (Z), in LDU,
// centered on the part origin.
func HoleOffsets(n int) []geom.Vec3 {
	out := make([]geom.Vec3, n)
	for i := range out {
		k := float64(i) - float64(n-1)/2
		out[i] = geom.Vec3{Z: k * Stud}
	}
	return out
}

// AxisSource answers which way a part's holes point. The shadow library is the
// real one; a test can supply its own without downloading anything.
type AxisSource interface {
	RotationAxis(part string) (axis geom.Vec3, source string, ok bool)
}

// LocalHoleAxis is the hole direction in the part's own frame.
func LocalHoleAxis(src AxisSource, part string) (geom.Vec3, error) {
	axis, _, ok := src.RotationAxis(part)
	if !ok {
		return geom.Vec3{}, fmt.Errorf("%s: hole axis unknown", part)
	}
	return axis.Unit(), nil
}

// WorldPorts returns a placed part's connection points in world coordinates,
// each carrying its own axis.
//
// This is what WorldHoles could not say. A part had one hole axis, because the
// shadow library's RotationAxis gives one per part and a straight liftarm has
// only one — but an angle connector has holes facing two ways, and it is the
// part that ties two bearing walls together. Ports now come from the library
// rather than being laid out from a hole count, so a part is whatever shape it
// is. See extract.EntryForWith.
func WorldPorts(src Holes, p Placed) ([]Hole, error) {
	local := src.Holes(p.Part)
	if len(local) == 0 {
		return nil, fmt.Errorf("%s: no connection points", p.Part)
	}
	r := geom.Rotations[p.Rot]
	out := make([]Hole, 0, len(local))
	for _, h := range local {
		out = append(out, Hole{
			Pos:   r.Apply(h.Pos).Add(p.Origin),
			Axis:  r.Apply(h.Axis).Unit(),
			Cross: h.Cross,
		})
	}
	return out, nil
}

// WorldHoles returns a placed part's hole positions and its hole axis, both in
// world coordinates.
//
// Deprecated: it cannot describe a part whose holes face more than one way, and
// it lays holes out from a count rather than reading them. Use WorldPorts.
func WorldHoles(src AxisSource, p Placed, nHoles int) ([]geom.Vec3, geom.Vec3, error) {
	local, err := LocalHoleAxis(src, p.Part)
	if err != nil {
		return nil, geom.Vec3{}, err
	}
	r := geom.Rotations[p.Rot]
	pts := make([]geom.Vec3, 0, nHoles)
	for _, off := range HoleOffsets(nHoles) {
		pts = append(pts, r.Apply(off).Add(p.Origin))
	}
	return pts, r.Apply(local).Unit(), nil
}
