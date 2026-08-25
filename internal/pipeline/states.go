// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"fmt"
	"sort"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/interfere"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
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
