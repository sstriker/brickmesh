// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package mech

import (
	"math"
	"strings"
	"testing"
)

// autoBox is the three-speed of the examples: 8+24, 12+20, 16+16, so the
// output turns -1/3, -3/5 and -1 per turn of the input.
func autoBox() *Mechanism {
	m := New("auto")
	for _, id := range []string{"input", "output", "first", "second", "third"} {
		m.Shaft(id, 2)
	}
	for _, s := range []string{"1st", "2nd", "3rd"} {
		m.State(s)
	}
	m.Mesh("input", "first", 8, 24)
	m.Mesh("input", "second", 12, 20)
	m.Mesh("input", "third", 16, 16)
	m.Couple("output", "first", "ring 1", "1st")
	m.Couple("output", "second", "ring 2", "2nd")
	m.Couple("output", "third", "ring 3", "3rd")
	m.Drive("input", 1)
	m.Output("output")
	return m
}

func points(up, down []float64) ShiftPoints {
	return ShiftPoints{Watch: "input", UpAt: up, DownAt: down}
}

func TestTheScheduleSaysWhichGearIsHeldWhen(t *testing.T) {
	s, err := autoBox().Schedule(points([]float64{1.0, 1.6}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Gears) != 3 {
		t.Fatalf("got %d gears, want one per state", len(s.Gears))
	}
	if s.Gears[0].To != 1.0 || s.Gears[1].From != 1.0 || s.Gears[1].To != 1.6 {
		t.Errorf("the gears do not hand over at the shift points: %+v", s.Gears)
	}
	// Changing up from 1st to 2nd drops the input by the step between the
	// ratios: 1.0 * (1/3) / (3/5).
	if want := 1.0 * (1.0 / 3) / 0.6; math.Abs(s.Gears[1].EntrySpeed-want) > 1e-9 {
		t.Errorf("entering 2nd at %v, want %v", s.Gears[1].EntrySpeed, want)
	}
	if s.Gears[0].EntrySpeed != 0 {
		t.Error("first gear is not entered from anywhere")
	}
}

// The trap this check fell into first time round. Changing up always leaves the
// watched shaft below the speed it changed up at, and the next upshift point is
// higher still, so no arrangement of upshift points alone can make the box
// hunt. A check written against them would never fire, whatever it was given.
func TestUpshiftPointsAloneCannotDescribeHunting(t *testing.T) {
	m := autoBox()
	for _, up := range [][]float64{
		{1.0, 1.6}, {0.1, 0.11}, {1.0, 1.0001}, {5, 500},
	} {
		s, err := m.Schedule(points(up, nil))
		if err != nil {
			t.Fatalf("%v: %v", up, err)
		}
		for i := 1; i < len(s.Gears); i++ {
			if s.Gears[i].EntrySpeed >= s.Gears[i].From {
				t.Fatalf("%v: entering %s at %v, which is not below the %v it "+
					"changed up at — the premise of the hunt check is wrong",
					up, s.Gears[i].State, s.Gears[i].EntrySpeed, s.Gears[i].From)
			}
		}
	}
}

func TestAScheduleWithRoomToSettleIsFine(t *testing.T) {
	got := autoBox().CheckShiftPoints(points([]float64{1.0, 1.6}, []float64{0.45, 0.8}))
	for _, f := range got {
		if f.Level != "OK" {
			t.Errorf("unexpected %s: %s", f.Level, f.Detail)
		}
	}
}

func TestADownshiftPointAboveTheEntrySpeedHunts(t *testing.T) {
	// Entering 2nd leaves the input at 0.556; changing back down at 0.6 means
	// it changes down the moment it has changed up.
	got := autoBox().CheckShiftPoints(points([]float64{1.0, 1.6}, []float64{0.6, 0.85}))
	if !hasFinding(got, "FAIL", "shift points") {
		t.Errorf("a box that changes down as soon as it changes up should fail: %v", got)
	}
	var said bool
	for _, f := range got {
		if strings.Contains(f.Detail, "hunts between") {
			said = true
		}
	}
	if !said {
		t.Errorf("the finding should name it as hunting: %v", got)
	}
}

func TestADownPointAboveItsUpPointIsRejected(t *testing.T) {
	got := autoBox().CheckShiftPoints(points([]float64{1.0, 1.6}, []float64{1.2, 0.85}))
	if !hasFinding(got, "FAIL", "shift points") {
		t.Errorf("changing down higher than it changed up should fail: %v", got)
	}
}

// Without downshift points the box's behaviour coming down is not described, so
// the honest answer is to say so and to say how low they would have to be.
func TestMissingDownshiftPointsAreReportedWithTheCeiling(t *testing.T) {
	got := autoBox().CheckShiftPoints(points([]float64{1.0, 1.6}, nil))
	var warned bool
	for _, f := range got {
		if f.Level == "WARN" && strings.Contains(f.Detail, "0.556") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("should warn, and name the 0.556 a downshift point has to stay "+
			"under: %v", got)
	}
}

func TestAScheduleThatDoesNotAddUpIsRejected(t *testing.T) {
	m := autoBox()
	for _, c := range []struct {
		why string
		p   ShiftPoints
	}{
		{"too few points", points([]float64{1.0}, nil)},
		{"out of order", points([]float64{1.6, 1.0}, nil)},
		{"a point at zero", points([]float64{0, 1.6}, nil)},
		{"no such shaft", ShiftPoints{Watch: "crank", UpAt: []float64{1, 1.6}}},
		{"down points that do not pair up", points([]float64{1, 1.6}, []float64{0.5})},
	} {
		if !hasFinding(m.CheckShiftPoints(c.p), "FAIL", "shift points") {
			t.Errorf("%s should be rejected", c.why)
		}
	}
}

// A box whose gears do not climb cannot be shifted up through.
func TestGearsThatDoNotClimbAreRejected(t *testing.T) {
	m := New("backwards")
	for _, id := range []string{"input", "output", "high", "low"} {
		m.Shaft(id, 2)
	}
	m.State("1st")
	m.State("2nd")
	m.Mesh("input", "high", 16, 16) // ratio 1
	m.Mesh("input", "low", 8, 24)   // ratio 1/3, slower
	m.Couple("output", "high", "ring 1", "1st")
	m.Couple("output", "low", "ring 2", "2nd")
	m.Drive("input", 1)
	m.Output("output")

	got := m.CheckShiftPoints(points([]float64{1.0}, []float64{0.5}))
	if !hasFinding(got, "FAIL", "shift points") {
		t.Errorf("changing up into a lower gear should fail: %v", got)
	}
}
