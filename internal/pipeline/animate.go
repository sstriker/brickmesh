// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"strings"

	"github.com/sstriker/brickmesh/internal/geom"

	"github.com/sstriker/brickmesh/internal/ldcad"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/synth"
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
		if r.catchGroup != "" {
			res.Model.Groups = append(res.Model.Groups, ldr.Group{
				Name: r.catchGroup, Center: r.engaged.Add(r.catchAt),
			})
		}
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
		if isControlAxle(p.Label) {
			// It names a shaft and does not turn with it. A catch's axle is
			// held by the frame — the catch turns or slides ON it — and it
			// sits a couple of studs off that shaft's line, so carrying it
			// round with the shaft swings it about the model at the length of
			// the catch's reach. Which is what it did: the axle through the
			// selector orbited the structure instead of standing still.
			continue
		}
		shaft, ok := shaftFromLabel(p.Label)
		if !ok {
			continue
		}
		if name, ok := groupOf[shaft]; ok {
			p.Group = name
		}
	}
}

// isControlAxle reads a label written by placeControlAxles. The shaft it names
// is the one whose ring the catch moves, not one it turns with.
func isControlAxle(label string) bool {
	return strings.HasPrefix(label, "control axle ")
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
	ani := ldcad.Animation{Name: name,
		Sliding:  slidingIn(rings, speeds, state),
		Swinging: swingingIn(rings, state)}
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
	// catchAt is where the catch that moves this ring sits relative to it, and
	// catchGroup names its group. Empty when no catch was placed.
	catchAt    geom.Vec3
	catchGroup string
	// catchSlides is a catch that travels with its ring instead of turning.
	catchSlides bool
	// How the catch moves: about swingAxis through swingPivot, by swingHalf
	// degrees either side of square. swingAssumed marks an angle that was
	// chosen rather than derived.
	swingAxis, swingPivot geom.Vec3
	swingHalf             float64
	swingAssumed          bool
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
		catchGroup := ""
		if site.catchAt != (geom.Vec3{}) {
			catchGroup = fmt.Sprintf("catch_%d", i+1)
		}
		// Where the catch's own axle points and sits, once placed: the column
		// of its orientation for the axis the part's hole runs along, and the
		// hole's offset down the part's own z.
		sys := site.system
		col := func(ax byte) geom.Vec3 {
			c := int(ax - 'x')
			return geom.Vec3{X: site.catchRot[0][c], Y: site.catchRot[1][c],
				Z: site.catchRot[2][c]}
		}
		var swingAxis, swingPivot geom.Vec3
		var swingHalf float64
		swingAssumed := false
		if catchGroup != "" && sys.CatchTurnAxis != 0 {
			swingAxis = col(sys.CatchTurnAxis)
			catchWorld := base.
				Add(place.Direction.Scale(site.engaged * synth.HalfStud)).
				Add(site.catchAt)
			swingPivot = catchWorld.Add(col('z').Scale(sys.CatchPivot))
			travel := math.Abs(site.disengaged-site.engaged) * synth.HalfStud
			switch {
			case sys.CatchArm > 0:
				// A lever: the arm has to carry its tip the ring's whole travel
				// along the shaft, which fixes the angle rather than leaving it
				// to be picked. Half either side of square.
				sin := travel / (2 * sys.CatchArm)
				if sin > 1 {
					sin = 1 // a travel longer than the arm can reach; say so by maxing out
				}
				swingHalf = math.Asin(sin) * 180 / math.Pi
			case sys.CatchPerLDU > 0:
				// A cam, whose rim carries the ring a fixed distance per degree.
				// Half either side of the seat in the middle, which is the
				// neutral one: a ring between two gears is engaged at one seat,
				// neutral at the next and engaged again at the third.
				swingHalf = travel / 2 * sys.CatchPerLDU
			}
		}
		out = append(out, ringGroup{
			group:        fmt.Sprintf("ring_%d", i+1),
			catchSlides:  sys.CatchSlides,
			swingAxis:    swingAxis,
			swingPivot:   swingPivot,
			swingHalf:    swingHalf,
			swingAssumed: swingAssumed,
			catchAt:      site.catchAt,
			catchGroup:   catchGroup,
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
	k, c := 0, 0
	for i := range res.Model.Parts {
		p := &res.Model.Parts[i]
		switch {
		case isRing(p.Name) && k < len(rings):
			p.Group = rings[k].group
			k++
		case isSelector(p.Name):
			// Its own group: it moves with its ring and does not turn with it.
			for c < len(rings) && rings[c].catchGroup == "" {
				c++
			}
			if c < len(rings) {
				p.Group = rings[c].catchGroup
				c++
			}
		}
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
		at := atFor(r, state)
		out = append(out, ldcad.Sliding{
			Group: r.group, Axis: r.axis, Speed: speeds[r.shaft],
			// A ring is splined to its shaft, so it holds when that shaft does.
			ThroughShift: r.throughShift,
			Engaged:      r.engaged, Disengaged: r.disengaged, At: at,
		})
		if r.catchSlides && r.catchGroup != "" {
			// A fork on a shaft-parallel axle travels with the ring it holds,
			// and does not turn: it is threaded on its axle, not splined to
			// the shaft. Same two positions, offset to where the catch sits.
			out = append(out, ldcad.Sliding{
				Group: r.catchGroup, Axis: r.axis, Speed: 0,
				ThroughShift: r.throughShift,
				Engaged:      r.engaged.Add(r.catchAt),
				Disengaged:   r.disengaged.Add(r.catchAt), At: at,
			})
		}
	}
	return out
}

// swingingIn turns every catch that turns, to match where it has pushed its
// ring.
//
// Which is not all of them, though it was when this was written. A lever's axle
// runs across the shaft and a cam's along it, and neither can travel — showing
// one gliding sideways down a shaft it is not mounted on is what gave the game
// away. But 4159 is threaded on a shaft-parallel axle and pushed along it, so
// for that one a slide is exactly right; slidingIn has it.
func swingingIn(rings []ringGroup, state string) []ldcad.Swinging {
	var out []ldcad.Swinging
	for _, r := range rings {
		if r.catchGroup == "" || r.catchSlides {
			continue
		}
		out = append(out, ldcad.Swinging{
			Group: r.catchGroup, Axis: r.swingAxis, Pivot: r.swingPivot,
			Rest:    r.engaged.Add(r.catchAt),
			Engaged: -r.swingHalf, Clear: r.swingHalf,
			At:      atFor(r, state),
			Assumed: r.swingAssumed,
		})
	}
	return out
}

// atFor is where one ring sits in a given state: 0 engaged, 1 clear or the far
// gear, 0.5 neutral for a shared ring no state claims.
func atFor(r ringGroup, state string) float64 {
	at := 1.0
	if len(r.mateStates) > 0 {
		at = 0.5
	}
	for _, s := range r.states {
		if s == state {
			return 0
		}
	}
	for _, s := range r.mateStates {
		if s == state {
			return 1
		}
	}
	return at
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
			State: state, Turning: seg.Turning,
			Sliding: seg.Sliding, Swinging: seg.Swinging,
		})
	}
	markHolds(m, res, rings, states, &ani)
	applySchedule(m, &ani)
	return ani, true
}

// markHolds works out which shifts cut which group's drive.
//
// One per segment and per group, because a compound gearbox shifts one ring at
// a time. The shaft fed through the ring that is NOT moving keeps its drive all
// the way through, and stopping it looked exactly like a bug: the middle axle
// halting while its own ring sat engaged and the input kept turning.
func markHolds(m *mech.Mechanism, res *Result, rings []ringGroup,
	states []string, ani *ldcad.Animation) {

	shaftOf := map[string]string{}
	for id := range res.Layout.Place {
		shaftOf["shaft_"+id] = id
	}
	for _, r := range rings {
		shaftOf[r.group] = r.shaft
	}

	// One driven set per transition. The last segment has no shift after it.
	held := make([]map[string]bool, len(states))
	for k := 0; k+1 < len(states); k++ {
		held[k] = drivenBetween(m, states[k], states[k+1])
	}

	holdsFor := func(group string) []bool {
		shaft, ok := shaftOf[group]
		if !ok {
			return nil
		}
		out := make([]bool, len(states))
		for k, driven := range held {
			out[k] = driven != nil && !driven[shaft]
		}
		return out
	}
	for j := range ani.Turning {
		ani.Turning[j].Holds = holdsFor(ani.Turning[j].Group)
	}
	for _, seg := range ani.Segments {
		for j := range seg.Turning {
			seg.Turning[j].Holds = holdsFor(seg.Turning[j].Group)
		}
		for k := range seg.Sliding {
			seg.Sliding[k].Holds = holdsFor(seg.Sliding[k].Group)
		}
	}
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
	// No shift is engaged here, so a coupling reaches nothing.
	return drivenWith(m, func(mech.Coupling) bool { return false })
}

// drivenBetween is what the inputs still reach midway through one shift.
//
// A ring that is not moving between these two states is still in whatever gear
// it was in, and still passes drive; only the rings that are travelling are in
// neither gear. A coupling engaged in both states is one whose ring stays put,
// which makes "engaged in both" exactly the test.
//
// Holding every shaft at every shift was the alternative, and it stopped the
// half of a compound gearbox whose ring was not even moving.
func drivenBetween(m *mech.Mechanism, a, b string) map[string]bool {
	return drivenWith(m, func(c mech.Coupling) bool {
		return hasState(c, a) && hasState(c, b)
	})
}

func hasState(c mech.Coupling, state string) bool {
	for _, s := range c.States {
		if s == state {
			return true
		}
	}
	return false
}

// drivenWith is reachability from the inputs, with engaged deciding which of
// the shiftable couplings pass drive.
func drivenWith(m *mech.Mechanism, engaged func(mech.Coupling) bool) map[string]bool {
	driven := make(map[string]bool, len(m.Inputs))
	for id := range m.Inputs {
		driven[id] = true
	}
	for changed := true; changed; {
		changed = false
		for _, link := range m.Links {
			if c, ok := link.(mech.Coupling); ok && len(c.States) > 0 && !engaged(c) {
				continue
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
