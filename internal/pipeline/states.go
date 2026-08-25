// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"fmt"
	"sort"

	"github.com/sstriker/brickmesh/internal/collide"
	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/interfere"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/synth"
)

// The geometry checks look at the model as drawn, in one state, and a gearbox
// is not in one state — that is the whole point of it.
//
// Five faults in a row got past them for that reason, every one of them visible
// the moment somebody selected a gear and looked: a ring travelling twice as
// far as it should, a ball leaving the drum it rides, a drum turned to a phase
// that put the ball inside it, a catch walking out of the groove it holds, a
// clutch gear presenting its closed face. The parts that move are exactly the
// parts a shift is about, and they were only ever checked where they were
// parked.
//
// So the moving parts are put where each state puts them, and checked there.
// Only the moving ones: everything else is where it was when the drawn model
// was checked, and their pairs have not changed.

// movedParts is the model's parts as a given state arranges them, along with
// the indices of those the state actually moved.
//
// The parts are found by what they are and where they stand, not by the
// animation's groups: those are only attached when an animation was asked for,
// and this check has to work whether one was or not. It did not, at first, and
// said nothing at all — which is the very fault it exists to catch, one level
// up.
func movedParts(res *Result, rings []ringGroup, state string) ([]ldr.Part, []int) {
	parts := make([]ldr.Part, len(res.Model.Parts))
	copy(parts, res.Model.Parts)

	var moved []int
	// near finds the part of a kind standing closest to a point, within a stud.
	near := func(at geom.Vec3, is func(string) bool) int {
		best, dist := -1, 20.0
		for i, p := range parts {
			if !is(p.Name) {
				continue
			}
			if d := p.Pos.Sub(at).Len(); d < dist {
				best, dist = i, d
			}
		}
		return best
	}
	shift := func(i int, by geom.Vec3) {
		if i < 0 {
			return
		}
		parts[i].Pos = parts[i].Pos.Add(by)
		moved = append(moved, i)
	}
	turn := func(i int, axis geom.Vec3, deg float64, about geom.Vec3) {
		if i < 0 || deg == 0 {
			return
		}
		r := interfere.RotAbout(axis, deg)
		parts[i].Rot = r.Mul(parts[i].Rot)
		parts[i].Pos = about.Add(r.Apply(parts[i].Pos.Sub(about)))
		moved = append(moved, i)
	}

	for _, r := range rings {
		at := atFor(r, state)
		by := r.engaged.Add(r.disengaged.Sub(r.engaged).Scale(at)).Sub(r.rest)
		shift(near(r.rest, isRing), by)

		catchAt := r.rest.Add(r.catchAt)
		if r.catchSlides {
			// A fork travels with its ring, and the ball pinned into it goes
			// along.
			shift(near(catchAt, isSelector), by)
			shift(near(catchAt, isBall), by)
		} else if r.swingHalf != 0 {
			turn(near(catchAt, isSelector), r.swingAxis,
				-r.swingHalf+2*r.swingHalf*at, r.swingPivot)
		}
		if r.drumHalf != 0 {
			turn(near(r.drumAt, isDrum), r.drumAxis,
				-r.drumHalf+2*r.drumHalf*at, r.drumAt)
		}
	}
	sort.Ints(moved)
	return parts, uniq(moved)
}

func uniq(v []int) []int {
	out := v[:0]
	last := -1
	for _, n := range v {
		if n != last {
			out = append(out, n)
			last = n
		}
	}
	return out
}

// checkClearanceInEveryState puts the moving parts where each state puts them
// and asks whether anything shares space there.
func checkClearanceInEveryState(ctx context.Context, res *Result, deps Deps,
	m *mech.Mechanism) error {

	if res.Model == nil || deps.Lib == nil || m == nil {
		return nil
	}
	states := m.States()
	if len(states) < 2 {
		return nil // nothing moves, and the drawn model was already checked
	}
	rings := ringGroups(m, res)
	if len(rings) == 0 {
		return nil
	}
	spin := checkTurning(res, deps)

	checked, bad := 0, 0
	for _, state := range states {
		parts, moved := movedParts(res, rings, state)
		if len(moved) == 0 {
			continue
		}
		isMoved := map[int]bool{}
		for _, i := range moved {
			isMoved[i] = true
		}
		for _, i := range moved {
			for j := range parts {
				if i == j {
					continue
				}
				// Each pair once: two moving parts would otherwise be asked
				// twice, and the drawn model already covered still-vs-still.
				if isMoved[j] && j < i {
					continue
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				a, b := parts[i], parts[j]
				if mayBeInside(a, b) {
					continue
				}
				checked++
				inside, overlap, err := sharesSpace(ctx, deps, a, b, spin, i, j)
				if err != nil {
					return err
				}
				if !inside {
					continue
				}
				bad++
				res.Findings = append(res.Findings, mech.Finding{
					Level: "FAIL", Check: "clearance", Detail: fmt.Sprintf(
						"in %q: %s at %+v is inside %s at %+v, by %.1f LDU. The "+
							"model as drawn is clear — this is where the shift "+
							"puts them", state, a.Name, a.Pos, b.Name, b.Pos,
						overlap)})
			}
		}
	}
	if bad == 0 && checked > 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "clearance", Detail: fmt.Sprintf(
				"and clear in every state: %d pair(s) checked with the rings, "+
					"catches and barrels where each of the %d state(s) puts them",
				checked, len(states))})
	}
	return nil
}

// checkEngagementInEveryState asks whether the gear a state selects is one the
// ring can actually grip where that state puts it.
//
// The clearance pass catches parts that overlap. This catches the opposite, and
// the opposite is half of what went wrong: a clutch gear turned to present its
// closed face never engages anything, and nothing overlapped to say so. The
// model solved a ratio through a coupling that, in metal, would have slipped.
//
// The instrument is the one that measured the engaged distances in the first
// place — sweep the ring against the gear and count the windows its dogs leave.
// Asked here of the model as built rather than of two parts in isolation.
func checkEngagementInEveryState(ctx context.Context, res *Result, deps Deps,
	m *mech.Mechanism) error {

	if res.Model == nil || deps.Lib == nil || m == nil || len(res.ringSites) == 0 {
		return nil
	}
	rings := ringGroups(m, res)
	if len(rings) == 0 {
		return nil
	}
	asked, bad := 0, 0
	for _, state := range m.States() {
		parts, _ := movedParts(res, rings, state)
		for i, r := range rings {
			if i >= len(res.ringSites) {
				break
			}
			site := res.ringSites[i]
			at := atFor(r, state)
			// Which gear this state asks the ring to hold, if any. Halfway is
			// neutral and holds nothing, which is not a fault.
			var want layout.Station
			switch {
			case at == 0:
				want = site.station
			case at == 1 && site.mate != nil:
				want = site.mate.station
			default:
				continue
			}
			ring := nearestPart(parts, r.rest.Add(
				r.disengaged.Sub(r.engaged).Scale(at)), isRing)
			gear := stationPart(res, parts, want)
			if ring < 0 || gear < 0 {
				continue
			}
			asked++
			ok, err := engages(ctx, deps, parts[gear], parts[ring], r.axis,
				site.system.EngageFit)
			if err != nil {
				return err
			}
			if ok {
				continue
			}
			bad++
			res.Findings = append(res.Findings, mech.Finding{
				Level: "FAIL", Check: "engagement", Detail: fmt.Sprintf(
					"in %q the ring is where it should be and grips nothing: "+
						"swept against the %dt on '%s' it leaves no window at "+
						"all, so its dogs are not in that gear's recesses. The "+
						"ratio is solved through a coupling that would slip",
					state, want.Teeth, want.Shaft)})
		}
	}
	if asked > 0 && bad == 0 {
		res.Findings = append(res.Findings, mech.Finding{
			Level: "OK", Check: "engagement", Detail: fmt.Sprintf(
				"%d coupling(s) checked where their state puts them: every ring "+
					"has its dogs in the gear its state selects", asked)})
	}
	return nil
}

// engages sweeps a ring against a gear where both stand and asks whether the
// dogs find recesses.
// The fit is the system's own. The first system's dogs meet the gear face to
// face and any slack at all separates them — swept at the blanket quarter-LDU
// tolerance it engages at no distance whatever, and this check called a
// perfectly good two-speed broken twice over before that was noticed.
func engages(ctx context.Context, deps Deps, gear, ring ldr.Part,
	axis geom.Vec3, fit float64) (bool, error) {

	gm, err := interfere.MeshFor(deps.Lib, gear.Name)
	if err != nil {
		return true, nil // nothing to check with, so nothing to say
	}
	rm, err := interfere.MeshFor(deps.Lib, ring.Name)
	if err != nil {
		return true, nil
	}
	got, err := interfere.MeshLock(ctx,
		gm, collide.Transform{Rot: gear.Rot, Pos: gear.Pos},
		rm, collide.Transform{Rot: ring.Rot, Pos: ring.Pos},
		8, interfere.Options{Steps: 36, SpinAxis: principal(axis), Fit: fit})
	if err != nil {
		return false, err
	}
	return got.Windows > 0, nil
}

// nearestPart is the index of the part of a kind closest to a point.
func nearestPart(parts []ldr.Part, at geom.Vec3, is func(string) bool) int {
	best, dist := -1, 30.0
	for i, p := range parts {
		if !is(p.Name) {
			continue
		}
		if d := p.Pos.Sub(at).Len(); d < dist {
			best, dist = i, d
		}
	}
	return best
}

// stationPart is the index of the gear standing at a station.
func stationPart(res *Result, parts []ldr.Part, st layout.Station) int {
	place, ok := res.Layout.Place[st.Shaft]
	if !ok {
		return -1
	}
	at := place.Point.Scale(synth.HalfStud).
		Add(place.Direction.Scale(st.Axial * synth.HalfStud))
	best, dist := -1, 5.0
	for i, p := range parts {
		if _, _, isGear := gearFromLabel(p.Label); !isGear {
			continue
		}
		if d := p.Pos.Sub(at).Len(); d < dist {
			best, dist = i, d
		}
	}
	return best
}
