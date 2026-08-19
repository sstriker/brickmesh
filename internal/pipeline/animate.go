// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"strings"

	"brickmesh/internal/geom"

	"brickmesh/internal/ldcad"
	"brickmesh/internal/ldr"
	"brickmesh/internal/mech"
	"brickmesh/internal/synth"
)

// animate groups the parts by the shaft they turn with, and writes a script
// that turns each group at the ratio the functional layer solved for.
//
// A gearbox gets one animation per state, because that is what a state is: the
// same parts turning at different speeds. The freewheeling gears keep turning
// in every state — they are always meshed — which is what makes a gearbox worth
// watching rather than merely looking at.
func animate(m *mech.Mechanism, res *Result, opts Options) {
	groupOf := shaftGroups(res)
	if len(groupOf) == 0 {
		return
	}

	// One group per shaft, centered on its own line, which is what it turns
	// about.
	seen := map[string]bool{}
	for _, id := range m.Order() {
		name, ok := groupOf[id]
		if !ok || seen[name] {
			continue
		}
		place, ok := res.Layout.Place[id]
		if !ok {
			continue
		}
		seen[name] = true
		res.Model.Groups = append(res.Model.Groups, ldr.Group{
			Name: name, Center: place.Point.Scale(synth.HalfStud),
		})
	}
	tagParts(res, groupOf)
	for _, r := range ringGroups(m, res) {
		res.Model.Groups = append(res.Model.Groups, ldr.Group{
			Name: r.group, Center: r.engaged,
		})
	}
	tagRings(m, res)

	script := &ldcad.Script{
		Model: m.Name, Seconds: opts.Seconds, InputTurns: opts.InputTurns,
	}
	states := m.States()
	if len(states) == 0 {
		states = []string{""}
	}
	rings := ringGroups(m, res)
	for _, state := range states {
		if ani, ok := animationFor(m, res, groupOf, rings, state); ok {
			script.Animations = append(script.Animations, ani)
		}
	}
	if ani, ok := shiftAnimation(m, res, groupOf, rings, states); ok {
		script.Animations = append(script.Animations, ani)
	}
	if len(script.Animations) == 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "WARN", Check: "animation",
			Detail: "no state resolves to definite speeds, so there is nothing to animate",
		})
		return
	}
	res.Script = script
	res.Model.Script = opts.ScriptName
	res.Findings = append(res.Findings, mech.Finding{
		Level: "OK", Check: "animation", Detail: animationSummary(script),
	})
}

func animationSummary(s *ldcad.Script) string {
	names := make([]string, 0, len(s.Animations))
	for _, a := range s.Animations {
		names = append(names, a.Name)
	}
	return "animating " + strings.Join(names, ", ") +
		"; every group turns at the ratio the mechanism solved for"
}

// shaftGroups names a group per shaft that carries something.
func shaftGroups(res *Result) map[string]string {
	out := map[string]string{}
	for _, st := range res.Stations {
		out[st.Shaft] = "shaft_" + st.Shaft
	}
	for _, a := range res.axles {
		out[a.shaft] = "shaft_" + a.shaft
	}
	return out
}

// tagParts puts each gear and axle in the group of the shaft it turns with.
func tagParts(res *Result, groupOf map[string]string) {
	for i := range res.Model.Parts {
		p := &res.Model.Parts[i]
		shaft, ok := shaftFromLabel(p.Label)
		if !ok {
			continue
		}
		if name, ok := groupOf[shaft]; ok {
			p.Group = name
		}
	}
}

// shaftFromLabel reads the shaft out of a label like "24t on shaft 'first'" or
// "axle 10 for shaft 'input'". The labels are written when the parts are
// placed, so this saves carrying a second index alongside the model.
func shaftFromLabel(label string) (string, bool) {
	i := strings.Index(label, "'")
	if i < 0 {
		return "", false
	}
	rest := label[i+1:]
	j := strings.Index(rest, "'")
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// animationFor works out how fast each group turns in one state.
func animationFor(m *mech.Mechanism, res *Result, groupOf map[string]string,
	rings []ringGroup, state string) (ldcad.Animation, bool) {

	speeds, ok := m.Solve(state)
	if !ok {
		return ldcad.Animation{}, false
	}
	name := state
	if name == "" {
		name = m.Name
	}
	ani := ldcad.Animation{Name: name, Sliding: slidingIn(rings, speeds, state)}
	// Which shafts nothing reaches while every ring is between gears. Those
	// hold still rather than turning at a ratio no engagement is delivering.
	always := alwaysDriven(m)

	for _, id := range m.Order() {
		group, ok := groupOf[id]
		if !ok {
			continue
		}
		place, ok := res.Layout.Place[id]
		if !ok {
			continue
		}
		ani.Turning = append(ani.Turning, ldcad.Turning{
			Group: group, Axis: place.Direction, Speed: speeds[id],
			Through:      place.Point.Scale(synth.HalfStud),
			ThroughShift: !always[id],
		})
	}
	if len(ani.Turning) == 0 {
		return ldcad.Animation{}, false
	}
	return ani, true
}

// ringGroup is a driving ring's own group: it turns with its shaft like any
// gear, and slides along it, which no other part does.
type ringGroup struct {
	group string
	shaft string // the one it is splined to, whose speed it turns at
	// throughShift is whether that shaft is itself driven only through a shift,
	// in which case the ring holds with it.
	throughShift bool
	states       []string
	// mateStates are the states in which this ring engages the gear on its
	// other face, when one ring sits between two of them. Empty for a ring that
	// serves a single gear, which then only has engaged and clear.
	mateStates []string
	// axis it turns about, which is also the one it slides along.
	axis                geom.Vec3
	engaged, disengaged geom.Vec3
}

// ringGroups names a group per driving ring and works out its two positions.
func ringGroups(m *mech.Mechanism, res *Result) []ringGroup {
	always := alwaysDriven(m)
	var out []ringGroup
	for i, site := range res.ringSites {
		place, ok := res.Layout.Place[site.station.Shaft]
		if !ok {
			continue
		}
		base := place.Point.Scale(synth.HalfStud)
		var mateStates []string
		if site.mate != nil {
			mateStates = site.mate.coupling.States
		}
		out = append(out, ringGroup{
			group:        fmt.Sprintf("ring_%d", i+1),
			shaft:        site.rides,
			throughShift: !always[site.rides],
			states:       site.coupling.States,
			mateStates:   mateStates,
			axis:         place.Direction,
			engaged:      base.Add(place.Direction.Scale(site.engaged * synth.HalfStud)),
			disengaged:   base.Add(place.Direction.Scale(site.disengaged * synth.HalfStud)),
		})
	}
	return out
}

// tagRings puts each ring in its own group rather than its shaft's, so it can
// be moved on its own.
func tagRings(m *mech.Mechanism, res *Result) {
	rings := ringGroups(m, res)
	k := 0
	for i := range res.Model.Parts {
		p := &res.Model.Parts[i]
		if !isRing(p.Name) || k >= len(rings) {
			continue
		}
		p.Group = rings[k].group
		k++
	}
}

// slidingIn places every ring for one state: engaged where its coupling is,
// clear of its gear where it is not.
func slidingIn(rings []ringGroup, speeds map[string]float64,
	state string) []ldcad.Sliding {

	var out []ldcad.Sliding
	for _, r := range rings {
		// A ring that serves one gear has two positions, engaged and clear. One
		// that sits between two has three, and the middle is neutral: it drives
		// nothing there, which is why the shaft it rides holds still.
		at := 1.0 // clear, or engaging the far gear if there is one
		if len(r.mateStates) > 0 {
			at = 0.5 // neutral until a state claims it
		}
		for _, s := range r.states {
			if s == state {
				at = 0
				break
			}
		}
		for _, s := range r.mateStates {
			if s == state {
				at = 1
				break
			}
		}
		out = append(out, ldcad.Sliding{
			Group: r.group, Axis: r.axis, Speed: speeds[r.shaft],
			// A ring is splined to its shaft, so it holds when that shaft does.
			ThroughShift: r.throughShift,
			Engaged:      r.engaged, Disengaged: r.disengaged, At: at,
		})
	}
	return out
}

// shiftAnimation walks the states in order so a shift can be watched.
func shiftAnimation(m *mech.Mechanism, res *Result, groupOf map[string]string,
	rings []ringGroup, states []string) (ldcad.Animation, bool) {

	if len(states) < 2 || len(rings) == 0 {
		return ldcad.Animation{}, false // nothing shifts, so nothing to watch
	}
	ani := ldcad.Animation{Name: "shift"}
	for _, state := range states {
		seg, ok := animationFor(m, res, groupOf, rings, state)
		if !ok {
			return ldcad.Animation{}, false // a state that does not resolve
		}
		if len(ani.Turning) == 0 {
			ani.Turning = seg.Turning
		}
		ani.Segments = append(ani.Segments, ldcad.Segment{
			State: state, Turning: seg.Turning, Sliding: seg.Sliding,
		})
	}
	applySchedule(m, &ani)
	return ani, true
}

// topGearTail is how far past the last shift point the animation carries on, so
// the top gear is held for a while rather than arrived at and stopped in.
const topGearTail = 0.5

// applySchedule gives each state as much of the animation as the shift points
// say it is held for.
//
// Without them every state gets an equal share, which is all there is to go on
// for a box shifted by hand. With them the animation sweeps the watched shaft
// from a standstill to half again past the last shift point, and each gear
// takes the share of that sweep it is actually in.
func applySchedule(m *mech.Mechanism, ani *ldcad.Animation) {
	p, ok := m.ShiftPointsSet()
	if !ok || len(p.UpAt) == 0 {
		return
	}
	sched, err := m.Schedule(p)
	if err != nil || len(sched.Gears) != len(ani.Segments) {
		return
	}
	top := p.UpAt[len(p.UpAt)-1] * (1 + topGearTail)
	for i, g := range sched.Gears {
		to := g.To
		if to == 0 {
			to = top
		}
		if span := to - g.From; span > 0 {
			ani.Segments[i].Fraction = span
		}
	}
}

// alwaysDriven is every shaft the inputs reach with no shift engaged.
//
// The rule the animation has to obey, and the converse of it: a gear that turns
// turns whatever it meshes with, a shaft keyed to a turning gear turns, and
// anything nothing reaches does not turn at all. During a shift every ring is
// between gears, so nothing passes through a shift — and a shaft the inputs
// cannot reach without one has to stand still, along with everything downstream
// of it.
//
// Holding only the shafts a ring rides was not enough: in a compound gearbox the
// gears on the second stage are driven by the first stage's output, so when that
// output stops they stop too, and they were still turning.
//
// A differential is the exception that has to be spelled out. Two of its three
// shafts determine the third; one determines nothing, since the other two are
// free to turn against each other. Driving the case alone leaves both outputs
// undetermined, which is exactly what a differential is for.
func alwaysDriven(m *mech.Mechanism) map[string]bool {
	driven := make(map[string]bool, len(m.Inputs))
	for id := range m.Inputs {
		driven[id] = true
	}
	for changed := true; changed; {
		changed = false
		for _, link := range m.Links {
			if c, ok := link.(mech.Coupling); ok && len(c.States) > 0 {
				continue // a shift, and no shift is engaged here
			}
			shafts := link.Shafts()
			need := 1
			if _, ok := link.(mech.Differential); ok {
				need = 2
			}
			known := 0
			for _, id := range shafts {
				if driven[id] {
					known++
				}
			}
			if known < need {
				continue
			}
			for _, id := range shafts {
				if !driven[id] {
					driven[id] = true
					changed = true
				}
			}
		}
	}
	return driven
}
