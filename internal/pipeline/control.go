// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/part"
	"github.com/sstriker/brickmesh/internal/rigidity"
	"github.com/sstriker/brickmesh/internal/synth"
)

// controlAxle is the axle a catch turns on.
//
// Determinate, unlike the rest of the shift linkage: the catch's own axle hole
// says where it is and which way it runs, and the catch's placement is already
// measured. What is left after this is the lever a hand turns and whatever
// joins two control axles together, and neither of those follows from the
// gears — they follow from the housing, which is why they stay named.
type controlAxle struct {
	// ring is what it shifts, for reporting.
	ring string
	// at is a point on the axle and dir the way it runs.
	at, dir geom.Vec3
	// from and to bound it along dir, relative to at.
	from, to float64
	// alongShaft is whether it runs parallel to the shaft it serves, which is
	// what a cam does and a lever does not.
	alongShaft bool
}

// controlAxles works out the axle under each catch.
func controlAxles(res *Result) []controlAxle {
	var out []controlAxle
	for _, site := range res.ringSites {
		if site.catchRot == (geom.Mat3{}) || site.system.CatchTurnAxis == 0 {
			continue
		}
		place, ok := res.Layout.Place[site.station.Shaft]
		if !ok {
			continue
		}
		col := func(ax byte) geom.Vec3 {
			c := int(ax - 'x')
			return geom.Vec3{X: site.catchRot[0][c], Y: site.catchRot[1][c],
				Z: site.catchRot[2][c]}
		}
		ringAt := place.Point.Scale(synth.HalfStud).
			Add(place.Direction.Unit().Scale(site.engaged * synth.HalfStud))
		catchAt := ringAt.Add(site.catchAt)
		// The hole sits along the part's own z, whichever axis it turns about.
		pivot := catchAt.Add(col('z').Scale(site.system.CatchPivot))
		dir := col(site.system.CatchTurnAxis).Unit()

		c := controlAxle{ring: site.rides, at: pivot, dir: dir}
		c.alongShaft = math.Abs(dir.Dot(place.Direction.Unit())) > 0.999
		if c.alongShaft {
			// As far as its bearings, not as far as the shaft. Taking the
			// shaft's whole length gave the three-speed a 360 LDU axle, longer
			// than any that exists, and both of its control axles were dropped
			// without a word.
			if lo, hi, ok := bearingSpan(res, c); ok {
				c.from, c.to = lo-geom.Stud, hi+geom.Stud
			}
		}
		if c.from == 0 && c.to == 0 {
			// A lever's axle crosses the shafts and has nothing to run
			// alongside. One stud past the catch either way is the least that
			// fills its hole.
			c.from, c.to = -geom.Stud, geom.Stud
		}
		out = append(out, c)
	}
	return out
}

// bearingSpan is how far apart this control axle's bearings are, measured from
// its pivot along its own direction.
func bearingSpan(res *Result, c controlAxle) (float64, float64, bool) {
	line := layout.LineOf(res.Layout, c.ring)
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range synth.BearingRequirements(res.Layout, res.Stations, 2, 8) {
		if layout.LineOf(res.Layout, r.Shaft) != line {
			continue
		}
		t := r.Point.Sub(c.at).Dot(c.dir)
		lo, hi = math.Min(lo, t), math.Max(hi, t)
	}
	if math.IsInf(lo, 1) {
		return 0, 0, false
	}
	return lo, hi, true
}

// placeControlAxles puts an axle under each catch and says whether the frame
// holds it.
//
// It is placed rather than named because where it goes follows from the catch,
// which is measured. Whether anything holds it is a different question and gets
// a different answer: today's frames are two walls in the plane of the shafts,
// and a control axle sits a couple of studs off that plane, so they do not
// reach it. Saying so is the point — a gearbox whose shift falls out is worth
// knowing about before it is built.
func placeControlAxles(res *Result, deps Deps, model *ldr.Model) {
	axles := controlAxles(res)
	if len(axles) == 0 {
		return
	}
	var loose []string
	placed := 0
	for _, c := range axles {
		length := c.to - c.from
		studs, name, ok := axleFor(length)
		if !ok {
			continue
		}
		rot, ok := alignXTo(c.dir)
		if !ok {
			continue
		}
		centre := c.at.Add(c.dir.Scale((c.from + c.to) / 2))
		model.Add(name, ldr.ColorBlack, rot, centre,
			fmt.Sprintf("control axle %d for the catch on '%s'", studs, c.ring))
		placed++
		// The structure has to know it is there, the same as any other shaft.
		res.Axles = append(res.Axles, rigidity.Axle{
			Point: centre, Dir: c.dir,
			From: -length / 2, To: length / 2,
		})
		if !borne(res, deps, c, centre, length) {
			loose = append(loose, fmt.Sprintf("the one on '%s'", c.ring))
		}
	}
	if placed == 0 {
		return
	}
	if len(loose) > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "bearings", Detail: fmt.Sprintf(
				"%d control axle(s) placed and %d of them borne by nothing: %v. "+
					"The frame is two walls in the plane of the shafts, and a "+
					"catch sits a couple of studs off that plane, so the walls "+
					"do not reach its axle. The gears are held; the shift is not",
				placed, len(loose), loose)})
	} else {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "bearings", Detail: fmt.Sprintf(
				"%d control axle(s) placed, each borne by the frame", placed)})
	}
	res.Findings = append(res.Findings, mech.Finding{
		Level: "OK", Check: "parts", Detail: "the axle each catch turns on is " +
			"placed, since the catch's own hole fixes it. What is still not " +
			"placed is the lever a hand turns and whatever joins two control " +
			"axles: those follow from the housing, not from the gears"})
}

// borne reports whether any frame part has a round hole on this axle's line.
func borne(res *Result, deps Deps, c controlAxle, centre geom.Vec3, length float64) bool {
	if res.Structure == nil || deps.Shadow == nil {
		return false
	}
	for _, p := range res.Structure.Parts {
		ports, err := part.WorldPorts(deps.Shadow, p)
		if err != nil {
			continue
		}
		for _, h := range ports {
			if h.Cross {
				continue // a shaft seizes in one; it is not a bearing
			}
			d := h.Pos.Sub(c.at)
			if d.Sub(c.dir.Scale(d.Dot(c.dir))).Len() > 1e-6 {
				continue // not on the line
			}
			if math.Abs(math.Abs(h.Axis.Unit().Dot(c.dir))-1) > 1e-6 {
				continue // not facing along it
			}
			if math.Abs(h.Pos.Sub(centre).Dot(c.dir)) > length/2+1e-6 {
				continue // on the line, but past the end of the axle
			}
			return true
		}
	}
	return false
}
