// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package ldcad

import (
	"strings"
	"testing"

	"brickmesh/internal/geom"
)

func sample() Script {
	return Script{
		Model: "3-speed", Seconds: 10, InputTurns: 4,
		Animations: []Animation{
			{Name: "1st", Turning: []Turning{
				{Group: "shaft_input", Axis: geom.Vec3{X: 1}, Speed: 1},
				{Group: "shaft_output", Axis: geom.Vec3{X: 1}, Speed: -1.0 / 3.0},
			}},
			{Name: "2nd", Turning: []Turning{
				{Group: "shaft_input", Axis: geom.Vec3{X: 1}, Speed: 1},
				{Group: "shaft_output", Axis: geom.Vec3{X: 1}, Speed: -0.6},
			}},
		},
	}
}

// The API is the one from LDCad's own 5510 example. These are the calls it
// makes, and getting any of their names wrong produces a file that loads and
// does nothing.
func TestTheScriptUsesTheLDCadAPI(t *testing.T) {
	out := sample().Render()
	for _, want := range []string{
		"ldc.animation(\"1st\")",
		"ani0:setLength(10)",
		"ani0:setEvent('start', 'onStart0')",
		"ani0:setEvent('frame', 'onFrame0')",
		"local sf=ldc.subfile()",
		"sf:getGroup(\"shaft_input\")",
		"ldc.animation.getCurrent()",
		"ani:getFrameTime()",
		"local m=ldc.matrix()",
		"m:setRotate(",
		// setOri, not setPosOri: a group's placement is its own center, so a
		// rotation turns it about its own axis already. Setting a position as
		// well is what threw every group off its axis the first time LDCad ran
		// one of these. A ring is the exception below, since it slides.
		":setOri(m)",
		"register()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestOneAnimationPerState(t *testing.T) {
	out := sample().Render()
	for _, name := range []string{"1st", "2nd"} {
		if !strings.Contains(out, "ldc.animation(\""+name+"\")") {
			t.Errorf("no animation for %q", name)
		}
	}
	if strings.Count(out, "function onFrame") != 2 {
		t.Errorf("got %d frame handlers, want 2", strings.Count(out, "function onFrame"))
	}
}

// The ratio the mechanism solved for has to reach the file, or the animation is
// decorative rather than accurate.
func TestTheSolvedRatioIsWhatTurns(t *testing.T) {
	out := sample().Render()
	if !strings.Contains(out, "input*-0.333333") {
		t.Error("the 1:3 reduction did not reach the script")
	}
	if !strings.Contains(out, "input*-0.600000") {
		t.Error("the second ratio did not reach the script")
	}
}

func TestEveryGroupFetchedIsAlsoTurned(t *testing.T) {
	out := sample().Render()
	for _, line := range strings.Split(out, "\n") {
		name, ok := strings.CutPrefix(strings.TrimSpace(line), "grp")
		if !ok || !strings.Contains(name, "=sf:getGroup") {
			continue
		}
		v := "grp" + name[:strings.Index(name, "=")]
		if !strings.Contains(out, v+":setOri(m)") {
			t.Errorf("%s is fetched but never turned", v)
		}
	}
}

// Crude, but it catches the generation bug that matters most: no Lua
// interpreter is available here, so the file cannot be run to find out.
func TestFunctionsAndEndsBalance(t *testing.T) {
	out := sample().Render()
	var opens, closes int
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--") {
			continue
		}
		if strings.HasPrefix(line, "function ") {
			opens++
		}
		if line == "end" {
			closes++
		}
	}
	if opens == 0 || opens != closes {
		t.Errorf("%d functions and %d ends", opens, closes)
	}
}

// Two renders of the same script have to match, so re-exporting a model does
// not churn the file.
func TestRenderIsDeterministic(t *testing.T) {
	first := sample().Render()
	second := sample().Render()
	if first != second {
		t.Error("the same script rendered differently twice")
	}
}

func TestDefaultsAreFilledIn(t *testing.T) {
	s := Script{Model: "m", Animations: []Animation{{Name: "a",
		Turning: []Turning{{Group: "g", Axis: geom.Vec3{Y: 1}, Speed: 1}}}}}
	out := s.Render()
	if !strings.Contains(out, "setLength(10)") {
		t.Error("a length should default rather than come out as zero")
	}
	if strings.Contains(out, "t*0*360") {
		t.Error("the input turns should default rather than stand still")
	}
}

func TestTheAxisIsNormalizedAndNeverNegativeZero(t *testing.T) {
	s := Script{Model: "m", Animations: []Animation{{Name: "a",
		Turning: []Turning{{Group: "g", Axis: geom.Vec3{X: -5}, Speed: 1}}}}}
	out := s.Render()
	if !strings.Contains(out, "-1, 0, 0)") {
		t.Errorf("the axis should be a unit vector:\n%s", out)
	}
	if strings.Contains(out, "-0,") || strings.Contains(out, "-0)") {
		t.Error("no component should be written as -0")
	}
}

// A shaft group must not be given a position. It turns about its own center
// already, and a position is a second displacement on top of that.
//
// This is the shape of the bug that made the first animation LDCad ever ran
// come out scattered, so it is stated rather than left to the eye.
func TestOnlyTheSlidingGroupsAreGivenAPosition(t *testing.T) {
	out := shiftSample().Render()
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "m:setPos(") {
			continue
		}
		// The only thing that may follow a setPos is a ring being placed.
		var next string
		for _, l := range lines[i+1:] {
			if t := strings.TrimSpace(l); t != "" {
				next = t
				break
			}
		}
		if !strings.HasPrefix(next, "ring") {
			t.Errorf("line %d sets a position for something that is not a "+
				"sliding ring:\n  %s\n  %s", i+1, strings.TrimSpace(line), next)
		}
	}
}

// And the ring does get one, or it turns with its shaft and never shifts.
func TestASlidingRingIsGivenAPosition(t *testing.T) {
	out := shiftSample().Render()
	if !strings.Contains(out, "m:setPos(") {
		t.Error("no position is set anywhere, so no ring can slide")
	}
	if !strings.Contains(out, ":setPosOri(m)") {
		t.Error("a sliding ring needs both, so setPosOri has to appear")
	}
}
