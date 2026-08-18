// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"fmt"
	"math"

	"brickmesh/internal/clutch"
	"brickmesh/internal/collide"
	"brickmesh/internal/geom"
	"brickmesh/internal/interfere"
	"brickmesh/internal/ldr"
	"brickmesh/internal/mech"
	"brickmesh/internal/teeth"
)

// checkTurningClearance turns every part that rides a shaft against everything
// it is not meant to be inside.
//
// A joiner is 20 LDU across and a beam's hole is 12, so a joiner that lands
// where a bearing is cannot be built at all — the shaft simply does not go
// through. A driving ring is 40 across and no more forgiving. That is worth
// catching in the run rather than in the box of parts.
//
// The search is kept off this space already, but on a voxel lattice and with a
// little slack. This is the check that decides, because it works on the
// geometry.
func checkTurningClearance(ctx context.Context, res *Result, deps Deps) error {
	if res.Model == nil || deps.Lib == nil {
		return nil
	}
	var joiners, rings, gears, structure []ldr.Part
	for _, p := range res.Model.Parts {
		switch {
		case p.Name == clutch.Joiner:
			joiners = append(joiners, p)
		case p.Name == DrivingRing:
			rings = append(rings, p)
		case isAxle(p.Name):
			// An axle belongs inside a joiner and inside a gear's bore.
		default:
			if _, _, ok := gearFromLabel(p.Label); ok {
				gears = append(gears, p)
			} else {
				structure = append(structure, p)
			}
		}
	}
	if len(joiners)+len(rings) == 0 {
		return nil
	}

	// A joiner has business with nothing at all. A ring has business with the
	// gear it engages — that is the whole point of it — so it is only asked
	// about the structure.
	joinerClashes, err := sweepAgainst(ctx, res, deps, joiners,
		append(append([]ldr.Part(nil), gears...), structure...))
	if err != nil {
		return err
	}
	ringClashes, err := sweepAgainst(ctx, res, deps, rings, structure)
	if err != nil {
		return err
	}
	clashes := joinerClashes + ringClashes

	if clashes == 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "clearance", Detail: fmt.Sprintf(
				"%d joiner(s) and %d ring(s) turn clear of the structure",
				len(joiners), len(rings))})
	}
	return nil
}

// touchTolerance is how much two parts may share before it is an overlap
// rather than a touch, in LDU.
//
// Parts that meet face to face are ordinary, and the triangle test counts two
// coplanar faces as intersecting: a bearing right against the end of a ring's
// travel reads as blocked at every angle, which is the same answer a ring
// buried in a beam gives. Boxes that overlap by less than this are touching,
// and touching is allowed.
const touchTolerance = 1.0

// sweepAgainst turns each rider a full revolution against each obstacle.
func sweepAgainst(ctx context.Context, res *Result, deps Deps,
	riders, obstacles []ldr.Part) (int, error) {

	clashes := 0
	for _, r := range riders {
		rider, err := interfere.MeshFor(deps.Lib, r.Name)
		if err != nil {
			continue
		}
		rlo, rhi, err := sweptBox(deps, r)
		if err != nil {
			continue
		}
		for _, o := range obstacles {
			other, err := interfere.MeshFor(deps.Lib, o.Name)
			if err != nil {
				continue
			}
			olo, ohi, err := placedBox(deps, o)
			if err != nil || overlapOf(rlo, rhi, olo, ohi) < touchTolerance {
				continue
			}
			got, err := interfere.MeshLock(ctx,
				other, collide.Transform{Rot: o.Rot, Pos: o.Pos},
				rider, collide.Transform{Rot: r.Rot, Pos: r.Pos},
				16, interfere.Options{Steps: 72})
			if err != nil {
				return clashes, err
			}
			if got.Verdict == interfere.NoEngagement {
				continue
			}
			clashes++
			res.Findings = append(res.Findings, mech.Finding{
				Level: "FAIL", Check: "clearance", Detail: fmt.Sprintf(
					"the %s at %+v runs into the %s at %+v: %s. It rides a shaft "+
						"and turns, so it needs the space to itself",
					r.Name, r.Pos, o.Name, o.Pos, got.Verdict)})
		}
	}
	return clashes, nil
}

func isAxle(name string) bool {
	for _, a := range AxleParts {
		if a == name {
			return true
		}
	}
	return false
}

// placedBox is a part's bounding box where it stands.
func placedBox(deps Deps, p ldr.Part) (geom.Vec3, geom.Vec3, error) {
	g, err := deps.Lib.Geometry(p.Name)
	if err != nil {
		return geom.Vec3{}, geom.Vec3{}, err
	}
	lo := geom.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi := geom.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	for _, t := range g.Tris {
		for _, v := range [3]geom.Vec3{t[0], t[1], t[2]} {
			w := p.Rot.Apply(v).Add(p.Pos)
			lo = geom.Vec3{X: math.Min(lo.X, w.X), Y: math.Min(lo.Y, w.Y), Z: math.Min(lo.Z, w.Z)}
			hi = geom.Vec3{X: math.Max(hi.X, w.X), Y: math.Max(hi.Y, w.Y), Z: math.Max(hi.Z, w.Z)}
		}
	}
	return lo, hi, nil
}

// sweptBox is the box a turning part needs: its own, widened across its axis to
// the circle it sweeps out.
func sweptBox(deps Deps, p ldr.Part) (geom.Vec3, geom.Vec3, error) {
	g, err := deps.Lib.Geometry(p.Name)
	if err != nil {
		return geom.Vec3{}, geom.Vec3{}, err
	}
	axis := p.Rot.Apply(geom.Vec3{Z: 1}).Unit()
	var along, radius float64
	alongLo := math.Inf(1)
	for _, t := range g.Tris {
		for _, v := range [3]geom.Vec3{t[0], t[1], t[2]} {
			w := p.Rot.Apply(v)
			d := w.Dot(axis)
			along = math.Max(along, d)
			alongLo = math.Min(alongLo, d)
			radius = math.Max(radius, w.Sub(axis.Scale(d)).Len())
		}
	}
	u, v := teeth.Frame(axis)
	lo := geom.Vec3{X: math.Inf(1), Y: math.Inf(1), Z: math.Inf(1)}
	hi := geom.Vec3{X: math.Inf(-1), Y: math.Inf(-1), Z: math.Inf(-1)}
	for _, d := range []float64{alongLo, along} {
		for _, a := range []float64{-radius, radius} {
			for _, b := range []float64{-radius, radius} {
				w := axis.Scale(d).Add(u.Scale(a)).Add(v.Scale(b)).Add(p.Pos)
				lo = geom.Vec3{X: math.Min(lo.X, w.X), Y: math.Min(lo.Y, w.Y), Z: math.Min(lo.Z, w.Z)}
				hi = geom.Vec3{X: math.Max(hi.X, w.X), Y: math.Max(hi.Y, w.Y), Z: math.Max(hi.Z, w.Z)}
			}
		}
	}
	return lo, hi, nil
}

// overlapOf is how far two boxes share, along whichever axis they share least.
// Zero or less means they are apart.
func overlapOf(alo, ahi, blo, bhi geom.Vec3) float64 {
	return math.Min(
		math.Min(math.Min(ahi.X, bhi.X)-math.Max(alo.X, blo.X),
			math.Min(ahi.Y, bhi.Y)-math.Max(alo.Y, blo.Y)),
		math.Min(ahi.Z, bhi.Z)-math.Max(alo.Z, blo.Z))
}
