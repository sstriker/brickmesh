// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package teeth finds where a gear's teeth sit, and the phase two of them need
// to interleave.
//
// It is derivable from the part itself: sweep the angle around the rotation
// axis, keep the points out near the tip radius, and the teeth show up as
// peaks. From that follows how far each gear has to be turned so a tooth of one
// meets a gap of the other rather than its tooth — and that phase is what fixes
// the rotation of the axle running through them.
//
// Without it a model has its gears at the right centers with their teeth passing
// through each other, which is right where it is measured and wrong wherever it
// is looked at.
package teeth

import (
	"fmt"
	"math"

	"brickmesh/internal/geom"
	"brickmesh/internal/ldraw"
)

// TrustThreshold is the sharpness below which a phase should not be relied on.
//
// The standard gears read between 0.51 and 0.90 except the 40t, which reads
// 0.26: its teeth are small against its radius, so the rim band picks up more
// that is not a tooth. Both implementations agree on that number, so it is the
// gear and not the reading.
const TrustThreshold = 0.45

// Frame builds two directions across an axis, so a part's points can be read as
// an angle around it.
func Frame(axis geom.Vec3) (u, v geom.Vec3) {
	a := axis.Unit()
	tmp := geom.Vec3{X: 1}
	if math.Abs(a.X) >= 0.9 {
		tmp = geom.Vec3{Y: 1}
	}
	u = a.Cross(tmp).Unit()
	return u, a.Cross(u)
}

// Angles is the angular position of each tooth, in degrees, in the part's own
// frame. It returns one per tooth, ascending.
func Angles(lib *ldraw.Library, part string, axis geom.Vec3, teeth int) ([]float64, error) {
	offset, _, err := phaseOf(lib, part, axis, teeth)
	if err != nil {
		return nil, err
	}
	pitch := 360.0 / float64(teeth)
	out := make([]float64, teeth)
	for i := range out {
		out[i] = math.Mod(offset+float64(i)*pitch, 360)
	}
	return out, nil
}

// Sharpness is how cleanly the teeth cluster, from 0 to 1. A low value means
// the reading is unreliable and the phase it implies should not be trusted.
func Sharpness(lib *ldraw.Library, part string, axis geom.Vec3, teeth int) (float64, error) {
	_, sharp, err := phaseOf(lib, part, axis, teeth)
	return sharp, err
}

// phaseOf folds every tooth onto one pitch window and takes the circular mean,
// which is far steadier than picking peaks out of a sparse cloud of vertices.
func phaseOf(lib *ldraw.Library, part string, axis geom.Vec3, teeth int) (offset, sharpness float64, err error) {
	if teeth <= 0 {
		return 0, 0, fmt.Errorf("%s: a gear needs teeth", part)
	}
	g, err := lib.Geometry(part)
	if err != nil {
		return 0, 0, err
	}
	u, v := Frame(axis)

	// The teeth are the points furthest from the axis.
	tip := 0.0
	radii := make([]float64, len(g.Verts))
	for i, p := range g.Verts {
		radii[i] = math.Hypot(p.Dot(u), p.Dot(v))
		if radii[i] > tip {
			tip = radii[i]
		}
	}
	if tip == 0 {
		return 0, 0, fmt.Errorf("%s: no geometry away from the axis", part)
	}

	angles := tipAngles(g, radii, u, v, tip, 0.93)
	if len(angles) < teeth*4 {
		// Too sparse a rim to read: take a wider band rather than guess from
		// a handful of points.
		angles = tipAngles(g, radii, u, v, tip, 0.85)
	}
	if len(angles) == 0 {
		return 0, 0, fmt.Errorf("%s: no rim points to read the teeth from", part)
	}

	pitch := 360.0 / float64(teeth)
	var sumSin, sumCos float64
	for _, a := range angles {
		phase := math.Mod(a, pitch) / pitch * 2 * math.Pi
		sumSin += math.Sin(phase)
		sumCos += math.Cos(phase)
	}
	n := float64(len(angles))
	mean := math.Atan2(sumSin/n, sumCos/n)
	offset = math.Mod(mean/(2*math.Pi)*pitch, pitch)
	if offset < 0 {
		offset += pitch
	}
	return offset, math.Hypot(sumSin/n, sumCos/n), nil
}

func tipAngles(g *ldraw.Geometry, radii []float64, u, v geom.Vec3, tip, frac float64) []float64 {
	var out []float64
	for i, p := range g.Verts {
		if radii[i] <= frac*tip {
			continue
		}
		a := math.Mod(math.Atan2(p.Dot(v), p.Dot(u))*180/math.Pi, 360)
		if a < 0 {
			a += 360
		}
		out = append(out, a)
	}
	return out
}

// SeatingIsFree reports whether the four ways a gear can sit on a cross axle
// all put teeth where teeth were.
//
// A cross axle has four-fold symmetry, so a gear can be seated four ways. Where
// the tooth count is a multiple of four those seatings map teeth onto teeth and
// the choice does not affect the phase at all.
func SeatingIsFree(teeth int) bool { return teeth%4 == 0 }

// Phase is how far each of two meshing gears must turn about its own axis.
type Phase struct {
	RotA, RotB           float64 // degrees
	PitchA, PitchB       float64
	SharpA, SharpB       float64
	SeatFreeA, SeatFreeB bool
}

// MeshPhase works out the rotation that puts a tooth of A on the line of
// centers and a gap of B facing back at it.
func MeshPhase(lib *ldraw.Library, partA string, teethA int, axisA geom.Vec3,
	partB string, teethB int, axisB geom.Vec3, towardB geom.Vec3) (Phase, error) {

	uA, vA := Frame(axisA)
	uB, vB := Frame(axisB)
	d := towardB.Unit()

	betaA := degrees(math.Atan2(d.Dot(vA), d.Dot(uA)))
	betaB := degrees(math.Atan2(-d.Dot(vB), -d.Dot(uB)))

	anglesA, err := Angles(lib, partA, axisA, teethA)
	if err != nil {
		return Phase{}, err
	}
	anglesB, err := Angles(lib, partB, axisB, teethB)
	if err != nil {
		return Phase{}, err
	}
	sharpA, err := Sharpness(lib, partA, axisA, teethA)
	if err != nil {
		return Phase{}, err
	}
	sharpB, err := Sharpness(lib, partB, axisB, teethB)
	if err != nil {
		return Phase{}, err
	}

	pitchA, pitchB := 360.0/float64(teethA), 360.0/float64(teethB)
	return Phase{
		// A brings a tooth onto the line of centers.
		RotA: wrap(betaA-anglesA[0], pitchA),
		// B brings a GAP onto it, which is a tooth half a pitch away.
		RotB:      wrap(betaB+pitchB/2-anglesB[0], pitchB),
		PitchA:    pitchA,
		PitchB:    pitchB,
		SharpA:    sharpA,
		SharpB:    sharpB,
		SeatFreeA: SeatingIsFree(teethA),
		SeatFreeB: SeatingIsFree(teethB),
	}, nil
}

func degrees(rad float64) float64 { return rad * 180 / math.Pi }

func wrap(v, period float64) float64 {
	r := math.Mod(v, period)
	if r < 0 {
		r += period
	}
	return r
}
