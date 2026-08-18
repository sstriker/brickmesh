// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// Package ldcad writes the animation script that turns a model's shafts.
//
// LDCad animates through Lua rather than through meta commands. The model
// declares groups — GROUP_DEF for each, GROUP_NXT tagging the part lines — and
// a script reaches them by name and sets their orientation once per frame.
// The API used here is exactly the one in LDCad's own 5510 example: a matrix
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
	// Speed in turns per turn of the input, signed: the ratio the functional
	// layer solved for.
	Speed float64
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
	// At is where it sits: 0 engaged, 1 disengaged.
	At float64
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
	for j, t := range ani.Turning {
		fmt.Fprintf(b, "  grp%d_%d=sf:getGroup(%q)\n", i, j, t.Group)
	}
	for k, sl := range slidingOf(ani) {
		fmt.Fprintf(b, "  ring%d_%d=sf:getGroup(%q)\n", i, k, sl.Group)
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
	b.WriteString("  local m=ldc.matrix()\n")
	for j, t := range ani.Turning {
		axis := t.Axis.Unit()
		fmt.Fprintf(b, "\n  --%s turns %.4f per turn of the input\n", t.Group, t.Speed)
		fmt.Fprintf(b, "  m:setRotate(input*%.6f, %g, %g, %g)\n",
			t.Speed, round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "  grp%d_%d:setOri(m)\n", i, j)
	}
	for k, sl := range ani.Sliding {
		axis := sl.Axis.Unit()
		pos := sl.Position(sl.At)
		where := "engaged"
		if sl.At > 0.5 {
			where = "clear of its gear"
		}
		fmt.Fprintf(b, "\n  --%s turns with its shaft and sits %s\n", sl.Group, where)
		fmt.Fprintf(b, "  m:setRotate(input*%.6f, %g, %g, %g)\n",
			sl.Speed, round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "  m:setPos(%g, %g, %g)\n",
			round6(pos.X), round6(pos.Y), round6(pos.Z))
		fmt.Fprintf(b, "  ring%d_%d:setPosOri(m)\n", i, k)
	}
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
	b.WriteString("  local seg=math.floor(t*segs)\n")
	b.WriteString("  if seg>segs-1 then seg=segs-1 end\n")
	b.WriteString("  local u=t*segs-seg --0..1 within this segment\n")
	fmt.Fprintf(b, "  local perSeg=%g*360/segs --input degrees per segment\n", turns)
	b.WriteString("  local m=ldc.matrix()\n\n")

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
    for k=1,seg do a=a+sp[k]*perSeg end
    return a+sp[seg+1]*u*perSeg
  end

`)
	for j, t := range ani.Turning {
		axis := t.Axis.Unit()
		fmt.Fprintf(b, "  m:setRotate(angle(speed[%d]), %g, %g, %g)\n",
			j+1, round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "  grp%d_%d:setOri(m)\n", i, j)
	}

	if len(slidingOf(ani)) == 0 {
		return
	}
	fmt.Fprintf(b, `
  --A ring holds its place for most of a segment and moves near the end of it,
  --so the shift is a thing that happens rather than a thing between frames.
  local shift=%g
  local f=0
  if u>1-shift then f=(u-(1-shift))/shift end
  local nxt=seg+1
  if nxt>segs-1 then nxt=segs-1 end

`, shiftFraction)

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
		fmt.Fprintf(b, "    m:setRotate(angle(ringSpeed[%d]), %g, %g, %g)\n",
			k+1, round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "    m:setPos(%g+(%g)*at, %g+(%g)*at, %g+(%g)*at)\n",
			round6(e.X), round6(d.X-e.X),
			round6(e.Y), round6(d.Y-e.Y),
			round6(e.Z), round6(d.Z-e.Z))
		fmt.Fprintf(b, "    ring%d_%d:setPosOri(m)\n", i, k)
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
