// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package mech

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ShiftPoints say when a gearbox changes gear on its own.
//
// The rule is the one a car uses: watch a shaft, and when it turns faster than
// the point set for the gear it is in, take the next gear up. Which is a stated
// rule and not a modelled governor — what actually senses the speed and moves
// the rings is a mechanism in its own right, and builders have found many
// answers to it. See docs/shifting.md.
type ShiftPoints struct {
	// Watch is the shaft whose speed decides. In a vehicle this is the engine.
	Watch string
	// UpAt is the speed at which each gear gives way to the next, so there is
	// one fewer of them than there are states.
	UpAt []float64
	// DownAt is the speed at which each gear is given up again on the way
	// down, in the same order. Optional: without it the box's behaviour going
	// down is not described, and hunting cannot be judged.
	DownAt []float64
}

// Gear is one state's place in the schedule.
type Gear struct {
	State string
	// Ratio is the output's turns per turn of the input, as solved.
	Ratio float64
	// From and To are the speeds of the watched shaft over which this gear is
	// held. To is zero for the top gear, which is held to any speed.
	From, To float64
	// EntrySpeed is what the watched shaft drops to on the way in, and the
	// number that decides whether the box hunts: it has to stay below the
	// speed at which this gear gives way in turn.
	EntrySpeed float64
}

// Schedule is the whole plan: which gear is held over which speeds.
type Schedule struct {
	Watch string
	Gears []Gear
}

// Solve works out the schedule, or says why it cannot.
//
// The states are taken in the order they were declared, which is the order the
// box shifts through them.
func (m *Mechanism) Schedule(p ShiftPoints) (Schedule, error) {
	states := m.States()
	if len(states) < 2 {
		return Schedule{}, fmt.Errorf("a schedule needs at least two states, got %d",
			len(states))
	}
	if len(p.UpAt) != len(states)-1 {
		return Schedule{}, fmt.Errorf(
			"%d shift point(s) for %d states: there is one shift between each "+
				"pair, so %d are needed", len(p.UpAt), len(states), len(states)-1)
	}
	if _, ok := m.shafts[p.Watch]; !ok {
		return Schedule{}, fmt.Errorf("no shaft called %q to watch", p.Watch)
	}
	if len(m.Outputs) == 0 {
		return Schedule{}, fmt.Errorf("a schedule needs an output to measure against")
	}

	out := Schedule{Watch: p.Watch}
	for i, state := range states {
		speeds, ok := m.Solve(state)
		if !ok {
			return Schedule{}, fmt.Errorf("%q does not resolve to definite speeds", state)
		}
		ratio := speeds[m.Outputs[0]]
		if ratio == 0 {
			return Schedule{}, fmt.Errorf("the output does not turn in %q", state)
		}
		g := Gear{State: state, Ratio: ratio}
		if i > 0 {
			g.From = p.UpAt[i-1]
		}
		if i < len(p.UpAt) {
			g.To = p.UpAt[i]
		}
		out.Gears = append(out.Gears, g)
	}

	// Entering a gear, the watched shaft drops by the ratio between the gear
	// left and the gear taken.
	for i := 1; i < len(out.Gears); i++ {
		prev, cur := out.Gears[i-1], out.Gears[i]
		out.Gears[i].EntrySpeed = math.Abs(p.UpAt[i-1] * prev.Ratio / cur.Ratio)
	}
	return out, nil
}

// CheckShiftPoints reports whether a schedule holds together.
func (m *Mechanism) CheckShiftPoints(p ShiftPoints) []Finding {
	const check = "shift points"
	sched, err := m.Schedule(p)
	if err != nil {
		return []Finding{{Level: "FAIL", Check: check, Detail: err.Error()}}
	}

	var out []Finding
	if !sort.SliceIsSorted(p.UpAt, func(i, j int) bool { return p.UpAt[i] < p.UpAt[j] }) {
		return append(out, Finding{Level: "FAIL", Check: check, Detail: fmt.Sprintf(
			"the shift points %v are not in increasing order, so the box would "+
				"be asked to change up at a speed it has already passed", p.UpAt)})
	}
	for _, v := range p.UpAt {
		if v <= 0 {
			return append(out, Finding{Level: "FAIL", Check: check, Detail: fmt.Sprintf(
				"a shift point of %g is not a speed the box can reach", v)})
		}
	}

	if len(p.DownAt) > 0 && len(p.DownAt) != len(p.UpAt) {
		return append(out, Finding{Level: "FAIL", Check: check, Detail: fmt.Sprintf(
			"%d downshift point(s) against %d upshift points: there is one of each "+
				"per shift", len(p.DownAt), len(p.UpAt))})
	}
	out = append(out, ratiosRise(sched, check)...)
	out = append(out, hysteresis(p, check)...)
	out = append(out, huntCheck(sched, p, check)...)
	out = append(out, Finding{Level: "OK", Check: check, Detail: describe(sched)})
	return out
}

// ratiosRise reports gears that are not faster than the one below them. A
// schedule that changes up into a lower gear is a schedule the box will never
// climb out of.
func ratiosRise(s Schedule, check string) []Finding {
	var out []Finding
	for i := 1; i < len(s.Gears); i++ {
		prev, cur := s.Gears[i-1], s.Gears[i]
		if math.Abs(cur.Ratio) <= math.Abs(prev.Ratio) {
			out = append(out, Finding{Level: "FAIL", Check: check, Detail: fmt.Sprintf(
				"%q turns the output %.3f per turn of the input and %q only %.3f: "+
					"changing up into it would slow the output down",
				prev.State, math.Abs(prev.Ratio), cur.State, math.Abs(cur.Ratio))})
		}
	}
	return out
}

// huntCheck is the one worth having.
//
// Changing up drops the watched shaft by the step between the two gears.  If it
// drops past the speed at which the box changes back down, it changes down at
// once -- and then straight back up, because changing down puts the speed back
// where it was.  That is hunting: the box sits between two gears banging
// between them, and it is a property of the ratios and both sets of points
// together rather than of any of them alone.
//
// It cannot be read off the upshift points by themselves. Changing up always
// leaves the watched shaft below the speed it changed up at, and the next
// upshift point is higher still, so an upshift can never trigger an upshift.
// The trap is one gear down, not one gear up.
func huntCheck(s Schedule, p ShiftPoints, check string) []Finding {
	if len(p.DownAt) == 0 {
		return []Finding{{Level: "WARN", Check: check, Detail: ceilings(s)}}
	}

	var out []Finding
	for i := 1; i < len(s.Gears); i++ {
		g := s.Gears[i]
		down := p.DownAt[i-1] // the speed at which this gear is given up again
		if g.EntrySpeed <= down {
			out = append(out, Finding{Level: "FAIL", Check: check, Detail: fmt.Sprintf(
				"changing up into %q at %g leaves %s turning at %.3f, at or below "+
					"the %g it changes back down at: the box hunts between %q and "+
					"%q. Either drop that downshift point below %.3f or close the "+
					"gap between the ratios",
				g.State, g.From, s.Watch, g.EntrySpeed, down, s.Gears[i-1].State,
				g.State, g.EntrySpeed)})
			continue
		}
		// And the same trap taken the other way: changing down must not put the
		// watched shaft straight back over the point it changed up at.
		up := math.Abs(down * g.Ratio / s.Gears[i-1].Ratio)
		if up >= g.From {
			out = append(out, Finding{Level: "FAIL", Check: check, Detail: fmt.Sprintf(
				"changing down out of %q at %g leaves %s turning at %.3f, at or "+
					"above the %g it changes up at: the box hunts",
				g.State, down, s.Watch, up, g.From)})
			continue
		}
		if margin := (g.EntrySpeed - down) / g.EntrySpeed; margin < 0.1 {
			out = append(out, Finding{Level: "WARN", Check: check, Detail: fmt.Sprintf(
				"changing up into %q leaves %s at %.3f against a downshift point "+
					"of %g, a margin of %.0f%%: a little drag and the box hunts",
				g.State, s.Watch, g.EntrySpeed, down, margin*100)})
		}
	}
	return out
}

// hysteresis reports downshift points at or above the upshift points they sit
// under. A box that changes down at a higher speed than it changed up at has
// no gear it can settle in.
func hysteresis(p ShiftPoints, check string) []Finding {
	var out []Finding
	for i := range p.DownAt {
		if i < len(p.UpAt) && p.DownAt[i] >= p.UpAt[i] {
			out = append(out, Finding{Level: "FAIL", Check: check, Detail: fmt.Sprintf(
				"changing down at %g and up at %g leaves no speed at which the "+
					"box holds a gear: the down point has to be the lower of the two",
				p.DownAt[i], p.UpAt[i])})
		}
	}
	return out
}

// ceilings says how low each downshift point would have to be, which is the
// useful thing to say when none were given.
func ceilings(s Schedule) string {
	var parts []string
	for i := 1; i < len(s.Gears); i++ {
		parts = append(parts, fmt.Sprintf("out of %q below %.3f",
			s.Gears[i].State, s.Gears[i].EntrySpeed))
	}
	return "no downshift points given, so what the box does coming back down is " +
		"not described and hunting cannot be judged. These ratios would need " +
		strings.Join(parts, ", ")
}

func describe(s Schedule) string {
	var b strings.Builder
	fmt.Fprintf(&b, "watching %s: ", s.Watch)
	for i, g := range s.Gears {
		if i > 0 {
			b.WriteString(", ")
		}
		switch {
		case g.To == 0:
			fmt.Fprintf(&b, "%s above %g", g.State, g.From)
		case i == 0:
			fmt.Fprintf(&b, "%s to %g", g.State, g.To)
		default:
			fmt.Fprintf(&b, "%s %g to %g (entered at %.3f)", g.State, g.From, g.To,
				g.EntrySpeed)
		}
	}
	return b.String()
}
