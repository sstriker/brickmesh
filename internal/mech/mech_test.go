// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package mech

import (
	"math"
	"testing"
)

// These mirror tests/test_mech.py case for case. Two implementations of the
// same arithmetic are only worth having if they are held to the same answers.

func simpleTrain() *Mechanism {
	// 8t driving 24t: one input, one output, ratio 3.
	m := New("simple")
	m.Shaft("in", 2)
	m.Shaft("out", 2)
	m.Mesh("in", "out", 8, 24)
	m.Drive("in", 1)
	m.Output("out")
	return m
}

func subtractor() *Mechanism {
	// A differential fed from two sides: the case averages both outputs.
	m := New("subtractor")
	for _, s := range []string{"drive", "steer", "case"} {
		m.Shaft(s, 2)
	}
	m.Differential("case", "drive", "steer")
	return m
}

func spurPair(ta, tb int) *Mechanism {
	m := New("pair")
	m.Shaft("a", 2)
	m.Shaft("b", 2)
	m.Mesh("a", "b", ta, tb)
	return m
}

func hasFinding(fs []Finding, level, check string) bool {
	for _, f := range fs {
		if f.Level == level && f.Check == check {
			return true
		}
	}
	return false
}

func TestDOFOfAPlainTrainIsOne(t *testing.T) {
	if got := simpleTrain().DOF(); got != 1 {
		t.Errorf("DOF = %d, want 1", got)
	}
}

func TestSubtractorHasTwoDegreesOfFreedom(t *testing.T) {
	// The whole reason a subtractor works: three shafts, one equation.
	if got := subtractor().DOF(); got != 2 {
		t.Errorf("DOF = %d, want 2", got)
	}
}

func TestSpeedsFollowTheGearRatio(t *testing.T) {
	sol, ok := simpleTrain().Solve()
	if !ok {
		t.Fatal("not solvable")
	}
	if math.Abs(sol["in"]-1) > 1e-9 {
		t.Errorf("in = %v, want 1", sol["in"])
	}
	// 8t driving 24t turns the output three times slower, and a spur pair
	// reverses direction.
	if math.Abs(sol["out"]+1.0/3.0) > 1e-9 {
		t.Errorf("out = %v, want -1/3", sol["out"])
	}
}

func TestDifferentialCaseIsTheAverageOfItsOutputs(t *testing.T) {
	m := subtractor()
	m.Drive("drive", 4)
	m.Drive("steer", 2)
	sol, ok := m.Solve()
	if !ok {
		t.Fatal("not solvable")
	}
	if math.Abs(sol["case"]-3) > 1e-9 {
		t.Errorf("case = %v, want 3", sol["case"])
	}
}

func TestDrivingBothTracksTogetherTurnsTheCaseWithThem(t *testing.T) {
	m := subtractor()
	m.Drive("drive", 1)
	m.Drive("steer", 1)
	sol, ok := m.Solve()
	if !ok {
		t.Fatal("not solvable")
	}
	if math.Abs(sol["case"]-1) > 1e-9 {
		t.Errorf("case = %v, want 1", sol["case"])
	}
}

func TestDrivingTheTracksOppositePivotsOnTheSpot(t *testing.T) {
	// Equal and opposite tracks: the machine spins in place, so the case, the
	// forward-motion port, stands still.
	m := subtractor()
	m.Drive("drive", 1)
	m.Drive("steer", -1)
	sol, ok := m.Solve()
	if !ok {
		t.Fatal("not solvable")
	}
	if math.Abs(sol["case"]) > 1e-9 {
		t.Errorf("case = %v, want 0", sol["case"])
	}
}

func TestUnderdrivenMechanismIsUnsolvable(t *testing.T) {
	m := subtractor()
	m.Drive("drive", 1)
	if _, ok := m.Solve(); ok {
		t.Error("two degrees of freedom and one input should not resolve")
	}
}

func TestLockedTrainIsReported(t *testing.T) {
	// Three shafts in a ring is one equation too many: nothing can turn.
	m := New("locked")
	for _, s := range []string{"a", "b", "c"} {
		m.Shaft(s, 2)
	}
	m.Mesh("a", "b", 8, 8)
	m.Mesh("b", "c", 8, 8)
	m.Mesh("a", "c", 8, 8)
	if got := m.DOF(); got != 0 {
		t.Errorf("DOF = %d, want 0", got)
	}
	if !hasFinding(m.CheckDOF(), "FAIL", "dof") {
		t.Error("expected a dof failure")
	}
}

func TestTooManyDrivesIsOverdetermined(t *testing.T) {
	m := simpleTrain()
	m.Drive("out", 1)
	if !hasFinding(m.CheckDOF(), "FAIL", "dof") {
		t.Error("expected a dof failure")
	}
}

func TestSingleBearingShaftFails(t *testing.T) {
	m := New("wobbly")
	m.Shaft("lonely", 1)
	fs := m.CheckBearings()
	if len(fs) != 1 || fs[0].Level != "FAIL" {
		t.Fatalf("got %+v", fs)
	}
}

func TestMixedGridDomainsAreFlagged(t *testing.T) {
	m := New("mixed")
	m.ShaftIn("a", 2, "technic-studless")
	m.ShaftIn("b", 2, "technic-brick")
	m.Mesh("a", "b", 8, 24)
	if !hasFinding(m.CheckDomains(), "FAIL", "grid") {
		t.Error("expected a grid failure")
	}
}

func TestCenterDistanceOnTheLatticeIsAccepted(t *testing.T) {
	// 8+24 = 32, a multiple of 8, so the pair sits 4 whole half studs apart.
	fs := spurPair(8, 24).CheckCenterDistances()
	if len(fs) != 1 || fs[0].Level != "OK" {
		t.Errorf("got %+v", fs)
	}
}

func TestCenterDistanceOffTheLatticeIsFailed(t *testing.T) {
	// Both counts are multiples of 4, but they sum to 20, so the pair lands on
	// 2.5 half studs and cannot be built.
	for _, c := range []struct{ ta, tb int }{{8, 12}, {36, 40}} {
		fs := spurPair(c.ta, c.tb).CheckCenterDistances()
		if len(fs) != 1 || fs[0].Level != "FAIL" {
			t.Errorf("%dt/%dt: got %+v", c.ta, c.tb, fs)
		}
	}
}

func TestBevelPairsAreNotCenterDistanceChecked(t *testing.T) {
	m := New("bevel")
	m.Shaft("a", 2)
	m.Shaft("b", 2)
	m.MeshOf("a", "b", 8, 12, Bevel, 5)
	fs := m.CheckCenterDistances()
	if len(fs) != 1 || fs[0].Level != "OK" {
		t.Errorf("got %+v", fs)
	}
	if _, ok := (Mesh{Kind: Bevel}).CenterDistanceHalfStuds(); ok {
		t.Error("a bevel pair has no center distance to speak of")
	}
	if !(Mesh{Kind: Bevel}).Reverses() {
		t.Error("a bevel pair still reverses")
	}
	if (Mesh{Kind: Worm}).Reverses() {
		t.Error("a worm does not")
	}
}

func TestDegenerateTriangleIsRejected(t *testing.T) {
	m := New("degenerate")
	for _, s := range []string{"a", "b", "c"} {
		m.Shaft(s, 2)
	}
	m.Mesh("a", "b", 8, 8)   // 2 half studs
	m.Mesh("b", "c", 8, 8)   // 2 half studs
	m.Mesh("a", "c", 40, 40) // 10 half studs, longer than the other two
	if !hasFinding(m.CheckClosure(), "FAIL", "loop closure") {
		t.Error("expected the triangle to be rejected")
	}
}

func TestNoLoopsReportsOK(t *testing.T) {
	fs := simpleTrain().CheckClosure()
	if len(fs) != 1 || fs[0].Level != "OK" {
		t.Errorf("got %+v", fs)
	}
}

func TestBacklashAccumulatesAlongAPath(t *testing.T) {
	m := New("backlash")
	for _, s := range []string{"a", "b", "c"} {
		m.Shaft(s, 2)
	}
	m.MeshOf("a", "b", 8, 24, Spur, 4)
	m.MeshOf("b", "c", 8, 24, Spur, 4)
	// The first stage's play is seen through the second stage's reduction, so
	// the total is less than the naive sum of 8.
	got := m.Backlash([]string{"a", "b", "c"})
	if got <= 4 || got >= 8 {
		t.Errorf("backlash = %v, want between 4 and 8", got)
	}
}

var standardTeeth = []int{8, 12, 16, 20, 24, 28, 36, 40}

func tripleCloses(t1, t2, t3 int) bool {
	m := New("triple")
	for _, s := range []string{"a", "b", "c"} {
		m.Shaft(s, 2)
	}
	m.Mesh("a", "b", t1, t2)
	m.Mesh("b", "c", t2, t3)
	m.Mesh("a", "c", t1, t3)
	found := false
	for _, f := range m.CheckClosure() {
		if f.Check != "loop closure" {
			continue
		}
		if f.Level != "OK" {
			return false
		}
		found = true
	}
	return found
}

// Documented in PLAN.md, and cheap to keep honest.
func TestExactlySevenOf512GearTriplesClose(t *testing.T) {
	closing := 0
	for _, a := range standardTeeth {
		for _, b := range standardTeeth {
			for _, c := range standardTeeth {
				if tripleCloses(a, b, c) {
					closing++
				}
			}
		}
	}
	if closing != 7 {
		t.Errorf("got %d closing triples of 512, want 7", closing)
	}
}

// Closure places the first shaft at the origin and the second at (d, 0), then
// asks whether the THIRD lands on the lattice — never whether the second one
// did. Two of the four therefore contain a pair that cannot be built.
func TestOnlyTwoOfThoseFourAreActuallyBuildable(t *testing.T) {
	buildable := map[string]bool{}
	for _, triple := range [][3]int{{8, 12, 40}, {8, 16, 24}, {12, 24, 36}, {16, 24, 24}} {
		m := New("t")
		for _, s := range []string{"a", "b", "c"} {
			m.Shaft(s, 2)
		}
		m.Mesh("a", "b", triple[0], triple[1])
		m.Mesh("b", "c", triple[1], triple[2])
		m.Mesh("a", "c", triple[0], triple[2])
		ok := true
		for _, f := range m.CheckCenterDistances() {
			if f.Level != "OK" {
				ok = false
			}
		}
		if ok {
			buildable[key(triple)] = true
		}
	}
	if len(buildable) != 2 || !buildable["8-16-24"] || !buildable["16-24-24"] {
		t.Errorf("buildable = %v, want 8-16-24 and 16-24-24", buildable)
	}
}

func TestRunChecksIncludesEveryCheck(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range spurPair(8, 12).RunChecks() {
		seen[f.Check] = true
	}
	for _, want := range []string{"dof", "bearings", "grid", "center dist", "loop closure"} {
		if !seen[want] {
			t.Errorf("RunChecks did not report %q; got %v", want, seen)
		}
	}
}

func key(t [3]int) string {
	a := t
	// insertion sort, three elements
	if a[0] > a[1] {
		a[0], a[1] = a[1], a[0]
	}
	if a[1] > a[2] {
		a[1], a[2] = a[2], a[1]
	}
	if a[0] > a[1] {
		a[0], a[1] = a[1], a[0]
	}
	return itoa(a[0]) + "-" + itoa(a[1]) + "-" + itoa(a[2])
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}
