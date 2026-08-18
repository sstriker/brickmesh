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

// Animation is one thing to watch — a gearbox has one per state.
type Animation struct {
	Name    string
	Turning []Turning
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
	b.WriteString("end\n\n")

	fmt.Fprintf(b, "function onFrame%d()\n", i)
	b.WriteString("  local ani=ldc.animation.getCurrent()\n")
	b.WriteString("  local t=ani:getFrameTime()/ani:getLength()\n")
	fmt.Fprintf(b, "  local input=t*%g*360 --degrees turned by the input\n", turns)
	b.WriteString("  local m=ldc.matrix()\n")
	for j, t := range ani.Turning {
		axis := t.Axis.Unit()
		fmt.Fprintf(b, "\n  --%s turns %.4f per turn of the input\n", t.Group, t.Speed)
		fmt.Fprintf(b, "  m:setRotate(input*%.6f, %g, %g, %g)\n",
			t.Speed, round6(axis.X), round6(axis.Y), round6(axis.Z))
		fmt.Fprintf(b, "  grp%d_%d:setOri(m)\n", i, j)
	}
	b.WriteString("end\n\n")
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
