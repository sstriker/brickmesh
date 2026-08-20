// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// Package ldcad writes the animation script that turns a model's shafts.
//
// LDCad animates through Lua rather than through meta commands. The model
// declares groups — GROUP_DEF for each, GROUP_NXT tagging the part lines — and
// a script reaches them by name and sets their orientation once per frame.
// The API used here is exactly the one in LDCad's own 5510 example: a matrix
// mulRotateAB is the one that turns a group about a model axis.
//
// Measured, not read. LDCad's reference says mulRotateAB is self=self*rotate
// and mulRotateBA is self=rotate*self, and on that reading BA is what applies a
// rotation in the model's frame. It is the other way round in effect: asked to
// turn a 20t gear a quarter turn about x, AB left the gear's axis on x and BA
// threw it to (0, 0.16, -0.99).
//
// The difference is invisible for a group whose main item is placed square,
// because then the two orders coincide — which is why the shafts carrying an
// axle animated correctly while every group holding only a gear or a ring
// tumbled. Whatever the naming means, the measurement decides.
//
// A group's placement is its center, not the model's origin.
//
// That is the whole of it, and getting it backwards is what made the first
// animation LDCad ever ran come out with the parts scattered. The scripting
// reference says it plainly: setPos "applies to the groups center position not
// the main item's true position", and getPos "returns the position of the
// linked LDCad group current center point". So setOri turns a group about its
// own center already, and a rotation is all a spinning shaft needs.
//
// The mistake came from reading "absolute" in LDCad's own examples — "this
// group has a main item with identity placement so we can apply the rotation
// around y absolutely" — as "about the model origin". It means the opposite of
// incremental, not the opposite of local. On that reading the code added an
// offset t = q - R*q to move the pivot back onto the shaft, which is a correct
// correction to a problem that was not there, and it threw every group off its
// axis by twice its distance from the origin.
//
// A ring still gets a position, because it slides along its shaft as well as
// turning with it. That position is where its center goes.
//
// from ldc.matrix(), setRotate in degrees about an axis, and setOri on the
// group, which the example is explicit about being absolute rather than
// incremental.
//
// The speeds are not guessed. They come from the mechanism's own solution, so
// a shaft geared three to one turns three times slower on screen, and a gearbox
// gets one animation per state showing what that state actually does.
package ldcad

import (
	"fmt"
	"strings"

	"brickmesh/internal/geom"
)

// Turning is a group that rotates, and how fast.
type Turning struct {
	Group string
	// Axis it turns about, in the model's coordinates.
	Axis geom.Vec3
	// Through is a point on that axis, in the model's coordinates.
	//
	// Needed because a group turns about the model's origin and not about the
	// center its GROUP_DEF declares — which is the whole reason a model came
	// out with one shaft spinning correctly and every other part orbiting it.
	// The one that worked was the one whose axis ran through the origin.
	Through geom.Vec3
	// Speed in turns per turn of the input, signed: the ratio the functional
	// layer solved for.
	Speed float64
	// ThroughShift marks a shaft the inputs reach only through a shift.
	//
	// It matters during a shift and nowhere else. While a ring is sliding it is
	// in neither gear, so nothing passes through it: a shaft on the far side is
	// driven by nothing at all, and so is everything downstream of that shaft.
	// Turning them anyway draws a drive that is not there.
	//
	// Whether a shaft is on the far side is a question about the whole graph,
	// not about which shaft a ring happens to ride — in a compound gearbox the
	// second stage's gears are driven by the first stage's output, so they stop
	// when it does.
	ThroughShift bool
}

// Sliding is a group that moves along its shaft as well as turning about it.
//
// In a gearbox that is the driving ring and nothing else: it is splined to its
// shaft, so it turns with it, and it slides to decide which gear that shaft is
// locked to. Showing it parked in one place is what makes a shift look like a
// jump cut.
type Sliding struct {
	Group string
	// Axis it turns about, which is also the axis it slides along.
	Axis  geom.Vec3
	Speed float64
	// Engaged and Disengaged are its two positions, in the model's coordinates.
	Engaged, Disengaged geom.Vec3
	// At is where it sits: 0 engaged, 1 disengaged. A ring that serves two
	// gears engages one at 0 and the other at 1, with neutral halfway.
	At float64
	// ThroughShift is whether the shaft this rides is itself reached only
	// through a shift, in which case the ring holds along with it.
	ThroughShift bool
}

// Position of a sliding group at a given fraction of its travel.
func (s Sliding) Position(at float64) geom.Vec3 {
	return s.Engaged.Add(s.Disengaged.Sub(s.Engaged).Scale(at))
}

// Segment is one state's worth of a walk through the states.
type Segment struct {
	State   string
	Turning []Turning
	Sliding []Sliding
	// Fraction of the animation this state is held for. Zero in every segment
	// means share the time equally, which is all there is to go on for a box
	// that is shifted by hand.
	Fraction float64
}

// Animation is one thing to watch — a gearbox has one per state, plus one that
// walks through them.
type Animation struct {
	Name    string
	Turning []Turning
	Sliding []Sliding
	// Segments, when set, make this a walk through the states rather than a
	// single one held throughout. The groups are the same in every segment;
	// only the speeds and the ring positions change.
	Segments []Segment
}

// Script is a whole file.
type Script struct {
	Model      string
	Seconds    float64
	InputTurns float64 // how many turns of the input the animation covers
	Animations []Animation
}

// Render writes the Lua.
func (s Script) Render() string {
	seconds := s.Seconds
	if seconds <= 0 {
		seconds = 10
	}
	turns := s.InputTurns
	if turns <= 0 {
		turns = 4
	}

	var b strings.Builder
	fmt.Fprintf(&b, `--[[
  Animation for %s, written by brickmesh.

  Every group below turns at the ratio the functional layer solved for, so what
  you see is the mechanism this model actually is. Speeds are turns of that
  group per turn of the input shaft; the input itself makes %g turns over the
  %g seconds of the animation.
]]

`, s.Model, turns, seconds)

	for i, ani := range s.Animations {
		writeAnimation(&b, ani, i, seconds, turns)
	}

	b.WriteString("function register()\n")
	for i, ani := range s.Animations {
		fmt.Fprintf(&b, "  local ani%d=ldc.animation(%q)\n", i, ani.Name)
		fmt.Fprintf(&b, "  ani%d:setLength(%g)\n", i, seconds)
		fmt.Fprintf(&b, "  ani%d:setEvent('start', 'onStart%d')\n", i, i)
		fmt.Fprintf(&b, "  ani%d:setEvent('frame', 'onFrame%d')\n", i, i)
		if i < len(s.Animations)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("end\n\nregister()\n")
	return b.String()
}

func writeAnimation(b *strings.Builder, ani Animation, i int, seconds, turns float64) {
	// The start event caches the group lookups, as LDCad's own examples do, so
	// the per-frame work stays small.
	fmt.Fprintf(b, "function onStart%d()\n", i)
	b.WriteString("  local sf=ldc.subfile()\n")
	// The orientation each group starts with, kept because setOri is absolute:
	// it replaces a group's orientation rather than adding to it, so turning a
	// group by an angle means multiplying that angle onto what it already had.
	// Only a group whose parts are placed square to the model would come out
	// right without this, and a gear on a shaft never is.
	for j, t := range ani.Turning {
		fmt.Fprintf(b, "  grp%d_%d=sf:getGroup(%q)\n", i, j, t.Group)
		fmt.Fprintf(b, "  ori%d_%d=grp%d_%d:getOri()\n", i, j, i, j)
	}
	for k, sl := range slidingOf(ani) {
		fmt.Fprintf(b, "  ring%d_%d=sf:getGroup(%q)\n", i, k, sl.Group)
		fmt.Fprintf(b, "  rori%d_%d=ring%d_%d:getOri()\n", i, k, i, k)
	}
	b.WriteString("end\n\n")

	fmt.Fprintf(b, "function onFrame%d()\n", i)
	b.WriteString("  local ani=ldc.animation.getCurrent()\n")
	b.WriteString("  local t=ani:getFrameTime()/ani:getLength()\n")
	if len(ani.Segments) > 0 {
		writeWalk(b, ani, i, turns)
	} else {
		writeHeld(b, ani, i, turns)
	}
	b.WriteString("end\n\n")
}

// slidingOf lists the sliding groups, which are the same in every segment.
func slidingOf(ani Animation) []Sliding {
	if len(ani.Segments) > 0 {
		return ani.Segments[0].Sliding
	}
	return ani.Sliding
}

// writeHeld turns everything at one state's speeds, with the rings parked where
// that state puts them.
func writeHeld(b *strings.Builder, ani Animation, i int, turns float64) {
	fmt.Fprintf(b, "  local input=t*%g*360 --degrees turned by the input\n", turns)
	for j, t := range ani.Turning {
		axis := t.Axis.Unit()
		fmt.Fprintf(b, "\n  --%s turns %.4f per turn of the input\n", t.Group, t.Speed)
		fmt.Fprintf(b, "  local a=input*%.6f\n", t.Speed)
		// R times what it started with, about the group's own center, which is
		// where a group turns. Position is left alone: setOri does not touch it.
		fmt.Fprintf(b, "  local m%d=ori%d_%d:clone()\n", j, i, j)
		fmt.Fprintf(b, "  m%d:mulRotateAB(a, %g, %g, %g)\n",
			j, round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "  grp%d_%d:setOri(m%d)\n", i, j, j)
	}
	for k, sl := range ani.Sliding {
		axis := sl.Axis.Unit()
		pos := sl.Position(sl.At)
		where := "engaged"
		if sl.At > 0.5 {
			where = "clear of its gear"
		}
		turns := "turns with its shaft and"
		if sl.Speed == 0 {
			// A catch does not turn: it sits still in the frame and pushes the
			// ring along, which spins inside its fork.
			turns = "does not turn, and"
		}
		fmt.Fprintf(b, "\n  --%s %s sits %s\n", sl.Group, turns, where)
		fmt.Fprintf(b, "  local a=input*%.6f\n", sl.Speed)
		fmt.Fprintf(b, "  local rm%d=rori%d_%d:clone()\n", k, i, k)
		fmt.Fprintf(b, "  rm%d:mulRotateAB(a, %g, %g, %g)\n",
			k, round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "  ring%d_%d:setOri(rm%d)\n", i, k, k)
		// A ring slides as well as turning, so unlike a shaft it also gets a
		// position — where its center goes, which setPos sets outright.
		fmt.Fprintf(b, "  ring%d_%d:setPos(%g, %g, %g)\n", i, k,
			round6(pos.X), round6(pos.Y), round6(pos.Z))
	}
}

// fractions is each state's share of the animation, from the schedule if there
// is one and shared equally if there is not.
func fractions(ani Animation) []float64 {
	out := make([]float64, len(ani.Segments))
	var total float64
	for k, seg := range ani.Segments {
		out[k] = seg.Fraction
		total += seg.Fraction
	}
	if total <= 0 {
		for k := range out {
			out[k] = 1 / float64(len(out))
		}
		return out
	}
	for k := range out {
		out[k] /= total
	}
	return out
}

// shiftFraction is how much of a segment the ring spends moving. The rest of it
// is spent sitting still in gear, which is what a gearbox mostly does.
const shiftFraction = 0.25

// writeWalk turns the states into one animation that moves between them.
//
// Speeds are piecewise constant, so the angle a group has turned by is the sum
// over the segments already finished plus what it has turned in this one. That
// sum is what keeps a shaft from jumping backwards when the ratio changes: the
// orientation LDCad is given is absolute, so it has to be the whole history and
// not just the current segment's share.
func writeWalk(b *strings.Builder, ani Animation, i int, turns float64) {
	segs := len(ani.Segments)
	fmt.Fprintf(b, "  local segs=%d\n", segs)

	frac := fractions(ani)
	b.WriteString("  --how long each state is held, as a share of the animation\n")
	b.WriteString("  local frac={")
	for k, f := range frac {
		if k > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "%.6f", f)
	}
	b.WriteString("}\n")
	b.WriteString(`  local seg,acc=0,0
  for k=1,segs do
    --The last state takes whatever is left, so t=1 lands in it rather than
    --running off the end of the table.
    if t<acc+frac[k] or k==segs then seg=k-1 break end
    acc=acc+frac[k]
  end
  local u=(t-acc)/frac[seg+1] --0..1 within this segment
  if u<0 then u=0 elseif u>1 then u=1 end
`)
	fmt.Fprintf(b, "  local turns=%g\n", turns)
	// Declared here rather than beside the rings, because the shafts the rings
	// drive need it too: it is the share of a segment in which nothing is
	// engaged.
	fmt.Fprintf(b, "  local shift=%g\n", shiftFraction)

	// Speeds, group by group, segment by segment.
	b.WriteString("  --speed[group][segment], in turns per turn of the input\n")
	b.WriteString("  local speed={\n")
	for j := range ani.Turning {
		fmt.Fprintf(b, "    {")
		for si, seg := range ani.Segments {
			if si > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(b, "%.6f", seg.Turning[j].Speed)
		}
		fmt.Fprintf(b, "}, --%s\n", ani.Turning[j].Group)
	}
	b.WriteString("  }\n")

	b.WriteString(`
  --Degrees a group has turned by now: every finished segment in full, plus
  --this segment's share.
  local function angle(sp)
    local a=0
    for k=1,seg do a=a+sp[k]*frac[k]*turns*360 end
    return a+sp[seg+1]*u*frac[seg+1]*turns*360
  end

  --And for a shaft the inputs reach only through a shift: it turns while a ring
  --is in a gear, and holds still while the rings are sliding, when nothing
  --reaches it at all. Whatever the inputs still reach through fixed meshes
  --keeps going either way.
  local function angleThroughShift(sp)
    local a=0
    for k=1,seg do a=a+sp[k]*(1-shift)*frac[k]*turns*360 end
    local held=u
    if held>1-shift then held=1-shift end
    return a+sp[seg+1]*held*frac[seg+1]*turns*360
  end

`)
	for j, t := range ani.Turning {
		axis := t.Axis.Unit()
		fn := "angle"
		if t.ThroughShift {
			fn = "angleThroughShift"
		}
		fmt.Fprintf(b, "  local a%d=%s(speed[%d])\n", j, fn, j+1)
		fmt.Fprintf(b, "  local m%d=ori%d_%d:clone()\n", j, i, j)
		fmt.Fprintf(b, "  m%d:mulRotateAB(a%d, %g, %g, %g)\n",
			j, j, round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "  grp%d_%d:setOri(m%d)\n", i, j, j)
	}

	if len(slidingOf(ani)) == 0 {
		return
	}
	fmt.Fprintf(b, `
  --A ring holds its place for most of a segment and moves near the end of it,
  --so the shift is a thing that happens rather than a thing between frames.
  local f=0
  if u>1-shift then f=(u-(1-shift))/shift end
  local nxt=seg+1
  if nxt>segs-1 then nxt=segs-1 end

`)

	b.WriteString("  --ringSpeed[ring][segment]: a ring is splined to its shaft\n")
	b.WriteString("  local ringSpeed={\n")
	for k := range slidingOf(ani) {
		b.WriteString("    {")
		for si, seg := range ani.Segments {
			if si > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(b, "%.6f", seg.Sliding[k].Speed)
		}
		fmt.Fprintf(b, "}, --%s\n", slidingOf(ani)[k].Group)
	}
	b.WriteString("  }\n\n")

	b.WriteString("  --where[ring][segment]: 0 engaged, 1 clear\n")
	b.WriteString("  local where={\n")
	for k := range slidingOf(ani) {
		b.WriteString("    {")
		for si, seg := range ani.Segments {
			if si > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(b, "%g", seg.Sliding[k].At)
		}
		fmt.Fprintf(b, "}, --%s\n", slidingOf(ani)[k].Group)
	}
	b.WriteString("  }\n\n")

	for k, sl := range slidingOf(ani) {
		axis := sl.Axis.Unit()
		e, d := sl.Engaged, sl.Disengaged
		fmt.Fprintf(b, "  do --%s\n", sl.Group)
		fmt.Fprintf(b, "    local a=where[%d][seg+1]\n", k+1)
		fmt.Fprintf(b, "    local at=a+(where[%d][nxt+1]-a)*f\n", k+1)
		// A ring is splined to the shaft it rides, so it holds when that shaft
		// does — and turns straight through if that shaft is driven whatever
		// the rings are doing.
		ringFn := "angle"
		if sl.ThroughShift {
			ringFn = "angleThroughShift"
		}
		fmt.Fprintf(b, "    local ra=%s(ringSpeed[%d])\n", ringFn, k+1)
		fmt.Fprintf(b, "    local rm=rori%d_%d:clone()\n", i, k)
		fmt.Fprintf(b, "    rm:mulRotateAB(ra, %g, %g, %g)\n",
			round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "    ring%d_%d:setOri(rm)\n", i, k)
		// Where the center goes: engaged, plus however far along it has slid.
		fmt.Fprintf(b, "    ring%d_%d:setPos(%g+(%g)*at, %g+(%g)*at, %g+(%g)*at)\n",
			i, k,
			round6(e.X), round6(d.X-e.X),
			round6(e.Y), round6(d.Y-e.Y),
			round6(e.Z), round6(d.Z-e.Z))
		b.WriteString("  end\n")
	}
}

func round6(v float64) float64 {
	const f = 1e6
	r := float64(int64(v*f+copysign(0.5, v))) / f
	if r == 0 {
		return 0 // never -0, which reads oddly in a script
	}
	return r
}

func copysign(v, sign float64) float64 {
	if sign < 0 {
		return -v
	}
	return v
}
