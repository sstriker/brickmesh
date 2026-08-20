// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/synth"
)

// checkMeshing asks the placed model the question the arithmetic cannot: does
// every link stand where it would actually transmit.
//
// mech.CheckCenterDistances works on tooth counts alone and never sees a
// placement, and the station solver puts a pair where its rule says by
// construction. Both were passing while a three-speed's first stage sat
// diagonal — its two gears a stud apart along their shafts, meshing nothing,
// because the two lines disagreed about where axial zero was. Nothing looked at
// the finished positions, so nothing noticed.
//
// This does, in world coordinates, which is the only place the answer is not a
// matter of trust. Every kind of link, and what it cannot check it says.
func checkMeshing(res *Result, m *mech.Mechanism) {
	if res.Layout == nil {
		return
	}
	c := &meshCheck{res: res}
	for _, link := range m.Links {
		switch v := link.(type) {
		case mech.Mesh:
			c.mesh(v)
		case mech.Differential:
			c.coaxial("differential", v.Case, v.OutA, v.OutB)
		case mech.Coupling:
			c.coaxial("coupling", v.A, v.B)
		}
	}
	c.report()
}

// meshCheck accumulates what stood up and what did not.
type meshCheck struct {
	res *Result
	// did counts what was checked, by the property checked rather than by the
	// link, so the report can say what it actually covered. Keyed by the
	// singular phrase, with the plural kept beside it.
	did    map[string]int
	plural map[string]string
	bad    []string
	open   []string
}

func (c *meshCheck) ok(one, many string) {
	if c.did == nil {
		c.did, c.plural = map[string]int{}, map[string]string{}
	}
	c.did[one]++
	c.plural[one] = many
}

func (c *meshCheck) fail(format string, args ...any) {
	c.bad = append(c.bad, fmt.Sprintf(format, args...))
}

// unchecked records a property this cannot decide, so the report does not read
// as blanket cover. Saying "all N pairs stand correctly" while quietly skipping
// three kinds of link is the same mistake as not checking at all.
func (c *meshCheck) unchecked(what string) {
	for _, s := range c.open {
		if s == what {
			return
		}
	}
	c.open = append(c.open, what)
}

func (c *meshCheck) mesh(mesh mech.Mesh) {
	pa, okA := c.res.Layout.Place[mesh.A]
	pb, okB := c.res.Layout.Place[mesh.B]
	if !okA || !okB {
		return // not placed; the station checks have that
	}
	switch mesh.Kind {
	case mech.Spur:
		c.spur(mesh, pa)
	case mech.Bevel:
		c.bevel(mesh, pa, pb)
	case mech.Worm:
		// Perpendicular is all the layout requires and all that is settled.
		// How far a worm sits from its wheel is not measured anywhere here.
		if !layout.Perpendicular(pa, pb) {
			c.fail("%s/%s: a worm and its wheel are at %.1f degrees, not square",
				mesh.A, mesh.B, angleBetween(pa, pb))
			return
		}
		c.ok("worm pair square to its wheel", "worm pairs square to their wheels")
		c.unchecked("how far a worm sits from its wheel")
	case mech.Chain:
		// A chain runs in a plane, so the sprockets are parallel and coplanar.
		// The distance between them is free, which is the point of a chain.
		if _, parallel := layout.ParallelDistance(pa, pb); !parallel {
			c.fail("%s/%s: sprockets on shafts that are not parallel",
				mesh.A, mesh.B)
			return
		}
		c.coplanar(mesh, pa)
	default:
		c.unchecked("meshes of kind " + mesh.Kind)
	}
}

// spur: both gears in one plane, at their pitch distance.
func (c *meshCheck) spur(mesh mech.Mesh, pa layout.Placement) {
	as := gearWorlds(c.res, mesh.A, mesh.TeethA)
	bs := gearWorlds(c.res, mesh.B, mesh.TeethB)
	if len(as) == 0 || len(bs) == 0 {
		return
	}
	want := geom.PitchDistance(mesh.TeethA, mesh.TeethB)

	// A shaft can carry more than one gear of the same size — a two-speed
	// drives two 16t off one input — and nothing in a Station says which mesh
	// it was put there for. So the question is whether SOME pair of them stands
	// right, not whether the first two do. Asking it of the first two called a
	// working gearbox broken.
	best := math.Inf(1)
	for _, a := range as {
		for _, b := range bs {
			if got := b.Sub(a).Len(); math.Abs(got-want) < 1e-6 {
				c.ok("spur pair at its pitch distance in one plane", "spur pairs at their pitch distance in one plane")
				return
			} else if math.Abs(got-want) < math.Abs(best-want) {
				best = got
			}
		}
	}
	d := pa.Direction.Unit()
	along := math.Abs(bs[0].Sub(as[0]).Dot(d))
	why := fmt.Sprintf("nearest standing %.1f LDU apart, want %.1f", best, want)
	if along > 1e-6 {
		why = fmt.Sprintf("nearest standing %.1f LDU apart along their shafts, "+
			"so they are not in one plane at all", along)
	}
	c.fail("%s %dt / %s %dt: %s", mesh.A, mesh.TeethA, mesh.B, mesh.TeethB, why)
}

// bevel: square shafts whose axes meet, each gear at the other's effective
// radius from where they meet.
//
// That last rule is the layout's own, and docs/findings.md records that it is
// not settled — the sweep disagrees and nine measurements disagreed with each
// other. So this checks the model against the rule the engine applied, which
// catches a placement that drifted from it, and says the rule itself is open.
func (c *meshCheck) bevel(mesh mech.Mesh, pa, pb layout.Placement) {
	if !layout.Perpendicular(pa, pb) {
		c.fail("%s/%s: bevel shafts at %.1f degrees, not square",
			mesh.A, mesh.B, angleBetween(pa, pb))
		return
	}
	if !layout.AxesIntersect(pa, pb) {
		c.fail("%s/%s: bevel shafts are square but their axes never meet, so "+
			"there is no point for the gears to sit either side of",
			mesh.A, mesh.B)
		return
	}
	c.ok("bevel pair square with axes that meet", "bevel pairs square with axes that meet")

	at, ok := axesMeetAt(pa, pb)
	if !ok {
		return
	}
	for _, side := range []struct {
		shaft        string
		teeth, other int
		place        layout.Placement
	}{
		{mesh.A, mesh.TeethA, mesh.TeethB, pa},
		{mesh.B, mesh.TeethB, mesh.TeethA, pb},
	} {
		gears := gearWorlds(c.res, side.shaft, side.teeth)
		if len(gears) == 0 {
			continue
		}
		want := layout.EffectiveRadius(side.other) * synth.HalfStud
		best := math.Inf(1)
		for _, g := range gears {
			if got := math.Abs(g.Sub(at).Dot(side.place.Direction.Unit())); math.Abs(got-want) < math.Abs(best-want) {
				best = got
			}
		}
		if math.Abs(best-want) > 1e-6 {
			c.fail("%s %dt: %.1f LDU from where the axes meet, and the rule the "+
				"layout used puts it at %.1f", side.shaft, side.teeth, best, want)
			continue
		}
		c.ok("bevel gear at the radius the layout's rule gives", "bevel gears at the radius the layout's rule gives")
	}
	c.unchecked("whether that rule is the right one — see docs/findings.md")
}

// coplanar: two gears on parallel shafts standing in one plane across them.
func (c *meshCheck) coplanar(mesh mech.Mesh, pa layout.Placement) {
	as := gearWorlds(c.res, mesh.A, mesh.TeethA)
	bs := gearWorlds(c.res, mesh.B, mesh.TeethB)
	if len(as) == 0 || len(bs) == 0 {
		return
	}
	d := pa.Direction.Unit()
	for _, a := range as {
		for _, b := range bs {
			if math.Abs(b.Sub(a).Dot(d)) < 1e-6 {
				c.ok("chain pair in one plane", "chain pairs in one plane")
				return
			}
		}
	}
	c.fail("%s/%s: the sprockets are %.1f LDU apart along their shafts, and a "+
		"chain runs in a plane", mesh.A, mesh.B,
		math.Abs(bs[0].Sub(as[0]).Dot(d)))
}

// coaxial: shafts a link forces onto one line.
func (c *meshCheck) coaxial(what string, shafts ...string) {
	first, ok := c.res.Layout.Place[shafts[0]]
	if !ok {
		return
	}
	for _, s := range shafts[1:] {
		p, ok := c.res.Layout.Place[s]
		if !ok {
			return
		}
		if p.Key() != first.Key() {
			c.fail("%s: '%s' and '%s' are on different lines, and a %s holds "+
				"its shafts on one", what, shafts[0], s, what)
			return
		}
	}
	c.ok(what+" with its shafts on one line", what+"s with their shafts on one line")
}

func (c *meshCheck) report() {
	if len(c.bad) > 0 {
		c.res.Findings = append(c.res.Findings, mech.Finding{
			Level: "FAIL", Check: "meshing", Detail: fmt.Sprintf(
				"%d link(s) do not stand where they would transmit: %v",
				len(c.bad), c.bad)})
		return
	}
	if len(c.did) == 0 {
		return
	}
	kinds := make([]string, 0, len(c.did))
	for k, n := range c.did {
		phrase := k
		if n > 1 {
			phrase = c.plural[k]
		}
		kinds = append(kinds, fmt.Sprintf("%d %s", n, phrase))
	}
	sort.Strings(kinds)
	detail := "checked against the finished positions: " + strings.Join(kinds, ", ")
	if len(c.open) > 0 {
		detail += ". Not checked: " + strings.Join(c.open, ", ")
	}
	c.res.Findings = append(c.res.Findings, mech.Finding{
		Level: "OK", Check: "meshing", Detail: detail})
}

// axesMeetAt is where two intersecting lines cross.
func axesMeetAt(pa, pb layout.Placement) (geom.Vec3, bool) {
	a := pa.Point.Scale(synth.HalfStud)
	b := pb.Point.Scale(synth.HalfStud)
	da, db := pa.Direction.Unit(), pb.Direction.Unit()
	n := da.Cross(db)
	denom := db.Cross(n).Dot(da)
	if math.Abs(denom) < 1e-9 {
		return geom.Vec3{}, false
	}
	t := -db.Cross(n).Dot(a.Sub(b)) / denom
	return a.Add(da.Scale(t)), true
}

func angleBetween(pa, pb layout.Placement) float64 {
	d := math.Abs(pa.Direction.Unit().Dot(pb.Direction.Unit()))
	if d > 1 {
		d = 1
	}
	return math.Acos(d) * 180 / math.Pi
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
