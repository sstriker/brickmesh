// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/synth"
)

// MarkerPart is a thin liftarm put on the end of the input and output shafts.
//
// Half a stud thick, so it takes almost no room, and two studs long, so it
// sticks out far enough to read. Its hole goes over the axle end.
const MarkerPart = "41677.dat"

// markerHalf is half the arm's thickness, in LDU. It goes on the last of the
// axle rather than a stud short of it: a stud short is where the gear is.
const markerHalf = 5

// addMarkers puts a flag on the end of every input and output shaft.
//
// A gearbox animates as a lot of black axles turning at once, and which one is
// the input is not something a model says. A coloured arm on each end is
// readable at a glance, and because it turns with its shaft the ratio between
// two of them can be watched rather than read off the table: one arm going
// round twice while the other goes round once IS the ratio.
func addMarkers(res *Result, model *ldr.Model, m *mech.Mechanism) {
	if res.Layout == nil {
		return
	}
	seen := map[string]bool{}
	// Two shafts can share an end: a subtractor's case and its outputs are
	// coaxial by definition. The first one there keeps it, and the second goes
	// unmarked rather than being buried in it.
	taken := map[[3]float64]bool{}
	var skipped []string
	mark := func(shaft string, color int, what string) {
		if seen[shaft] {
			return
		}
		place, ok := res.Layout.Place[shaft]
		if !ok {
			return
		}
		end, ok := shaftEnd(res, shaft, middleOf(res, place.Direction.Unit()))
		if !ok {
			skipped = append(skipped, shaft)
			return
		}
		key := [3]float64{math.Round(end.X), math.Round(end.Y), math.Round(end.Z)}
		if taken[key] {
			skipped = append(skipped, shaft)
			return
		}
		taken[key] = true
		seen[shaft] = true
		d := place.Direction.Unit()
		// A liftarm's holes run through its thickness, so the axle goes along
		// the part's own y and its length points away from the shaft.
		out := anyPerpendicular(d)
		t := d.Cross(out)
		rot := geom.Mat3{
			{t.X, d.X, out.X},
			{t.Y, d.Y, out.Y},
			{t.Z, d.Z, out.Z},
		}
		model.Add(MarkerPart, color, rot, end,
			fmt.Sprintf("%s marker on shaft '%s'", what, shaft))
	}
	// Sorted, because ranging a map is ranging it in a different order every
	// time: two inputs would be marked in either order and the same mechanism
	// would produce two different files.
	ins := make([]string, 0, len(m.Inputs))
	for id := range m.Inputs {
		ins = append(ins, id)
	}
	sort.Strings(ins)
	for _, id := range ins {
		mark(id, ldr.ColorRed, "input")
	}
	for _, id := range m.Outputs {
		mark(id, ldr.ColorYellow, "output")
	}
	if len(skipped) > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
				"no free shaft end to mark on %v: something else on that line "+
					"reaches the end first, so those turn unmarked", skipped)})
	}
}

// shaftEnd is the free end of a shaft: the outer face of its outermost axle,
// on whichever side points away from the mechanism.
//
// Both sides have to be considered. Taking the larger of the two puts the
// marker on the inner face of any shaft that runs the other way, which is
// where the gears are — in the subtractor that landed one inside the
// differential.
func shaftEnd(res *Result, shaft string, mid float64) (geom.Vec3, bool) {
	place, ok := res.Layout.Place[shaft]
	if !ok {
		return geom.Vec3{}, false
	}
	d := place.Direction.Unit()
	// How far the whole line runs, not just this shaft. Shafts share a line —
	// a subtractor's case is coaxial with both its inputs — and an end that is
	// free for one of them is the middle of the mechanism for another.
	var lineLo, lineHi float64
	lineFound := false
	for _, a := range res.axles {
		other, ok := res.Layout.Place[a.shaft]
		if !ok || other.Key() != place.Key() {
			continue
		}
		half := float64(a.studs) * synth.HalfStud
		at := a.center.Dot(d)
		if !lineFound {
			lineLo, lineHi, lineFound = at-half, at+half, true
			continue
		}
		lineLo, lineHi = math.Min(lineLo, at-half), math.Max(lineHi, at+half)
	}
	var lo, hi float64
	found := false
	for _, a := range res.axles {
		if a.shaft != shaft {
			continue
		}
		half := float64(a.studs) * synth.HalfStud
		at := a.center.Dot(d)
		if !found {
			lo, hi, found = at-half, at+half, true
			continue
		}
		lo, hi = math.Min(lo, at-half), math.Max(hi, at+half)
	}
	if !found {
		return geom.Vec3{}, false
	}
	// The end further from the middle of everything, and step back half the
	// arm's thickness so the hole is full of axle rather than hanging off.
	at, end := hi-markerHalf, hi
	if math.Abs(lo-mid) > math.Abs(hi-mid) {
		at, end = lo+markerHalf, lo
	}
	// Only a real end of the line will do. A shaft whose own axle stops short
	// of it has the mechanism in the way, and marking it there buries the arm
	// in whatever is standing at that spot.
	if lineFound && math.Abs(end-lineLo) > 1e-6 && math.Abs(end-lineHi) > 1e-6 {
		return geom.Vec3{}, false
	}
	base := place.Point.Scale(synth.HalfStud)
	base = base.Sub(d.Scale(base.Dot(d)))
	return base.Add(d.Scale(at)), true
}

// middleOf is where the mechanism sits along a shaft, used to tell that shaft's
// two ends apart.
func middleOf(res *Result, d geom.Vec3) float64 {
	sum, n := 0.0, 0
	for _, a := range res.axles {
		sum += a.center.Dot(d)
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// anyPerpendicular picks a direction across a shaft, preferring an axis the
// model already uses so the marker lies square rather than at an angle.
func anyPerpendicular(d geom.Vec3) geom.Vec3 {
	for _, c := range []geom.Vec3{{Y: -1}, {Z: 1}, {X: 1}} {
		if abs(c.Dot(d)) < 1e-6 {
			return c
		}
	}
	return geom.Vec3{Y: -1}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
