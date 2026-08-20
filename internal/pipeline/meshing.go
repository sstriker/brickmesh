// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"

	"brickmesh/internal/geom"
	"brickmesh/internal/mech"
	"brickmesh/internal/synth"
)

// checkMeshing asks the placed model the question the arithmetic cannot: do the
// two gears of each mesh actually stand where they would turn each other.
//
// mech.CheckCenterDistances works on tooth counts alone and never sees a
// placement, and SolveStations puts a pair in one plane by construction. Both
// were passing while a three-speed's first stage sat diagonal — its two gears a
// stud apart along their shafts, meshing nothing, because the two lines
// disagreed about where axial zero was. Nothing looked at the finished
// positions, so nothing noticed.
//
// This does, in world coordinates, which is the only place the answer is not a
// matter of trust.
func checkMeshing(res *Result, m *mech.Mechanism) {
	if res.Layout == nil {
		return
	}
	var bad []string
	pairs := 0
	for _, link := range m.Links {
		mesh, ok := link.(mech.Mesh)
		if !ok || mesh.Kind != mech.Spur {
			continue
		}
		as := gearWorlds(res, mesh.A, mesh.TeethA)
		bs := gearWorlds(res, mesh.B, mesh.TeethB)
		if len(as) == 0 || len(bs) == 0 {
			continue // not placed; the station checks have that
		}
		pairs++
		want := geom.PitchDistance(mesh.TeethA, mesh.TeethB)

		// A shaft can carry more than one gear of the same size — a two-speed
		// drives two 16t off one input — and nothing in a Station says which
		// mesh it was put there for. So the question is whether SOME pair of
		// them stands right, not whether the first two do. Asking it of the
		// first two called a working gearbox broken.
		best, ok := math.Inf(1), false
		for _, a := range as {
			for _, b := range bs {
				if got := b.Sub(a).Len(); math.Abs(got-want) < 1e-6 {
					ok = true
				} else if math.Abs(got-want) < math.Abs(best-want) {
					best = got
				}
			}
		}
		if ok {
			continue
		}
		// Say which way it is wrong: a pair in one plane at the wrong distance
		// is a different mistake from a pair at the right distance in two.
		d := res.Layout.Place[mesh.A].Direction.Unit()
		along := math.Abs(bs[0].Sub(as[0]).Dot(d))
		why := fmt.Sprintf("nearest standing %.1f LDU apart, want %.1f", best, want)
		if along > 1e-6 {
			why = fmt.Sprintf("nearest standing %.1f LDU apart along their "+
				"shafts, so they are not in one plane at all", along)
		}
		bad = append(bad, fmt.Sprintf("%s %dt / %s %dt: %s",
			mesh.A, mesh.TeethA, mesh.B, mesh.TeethB, why))
	}
	if len(bad) > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "FAIL", Check: "meshing", Detail: fmt.Sprintf(
				"%d gear pair(s) do not mesh where they were placed: %v", len(bad), bad)})
		return
	}
	if pairs > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "meshing", Detail: fmt.Sprintf(
				"all %d gear pair(s) stand at their pitch distance in one plane", pairs)})
	}
}

// gearWorlds is every place a shaft carries a gear of this many teeth. More
// than one is normal: two ratios off one input is two gears the same size.
func gearWorlds(res *Result, shaft string, teeth int) []geom.Vec3 {
	place, ok := res.Layout.Place[shaft]
	if !ok {
		return nil
	}
	var out []geom.Vec3
	for _, st := range res.Stations {
		if st.Shaft != shaft || st.Teeth != teeth {
			continue
		}
		out = append(out, place.Point.Scale(synth.HalfStud).
			Add(place.Direction.Unit().Scale(st.Axial*synth.HalfStud)))
	}
	return out
}
