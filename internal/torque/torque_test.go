// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package torque

import (
	"math"
	"strings"
	"testing"
)

// The reference: an XL motor at stall through an 8t driving a 24t.
//
// Worked by hand rather than taken from the code it is checking. 40 Ncm at the
// 8t is 0.4 Nm on a pitch radius of 4 mm, which is 100 N on the tooth — and the
// 8t's limit is 40 N, so this is the arrangement that famously strips.
func TestTheClassicWayToStripAnEightTooth(t *testing.T) {
	rows := Propagate(40, []Stage{{Name: "8t to 24t", DriverTeeth: 8, DrivenTeeth: 24}})
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	r := rows[0]
	if math.Abs(r.ForceDriverN-100) > 1e-9 {
		t.Errorf("tooth load %v N, want 100", r.ForceDriverN)
	}
	if want := 40 * 3.0 * 0.94; math.Abs(r.TorqueOutNcm-want) > 1e-9 {
		t.Errorf("torque out %v, want %v", r.TorqueOutNcm, want)
	}

	got := Assess(rows)
	var failed bool
	for _, a := range got {
		if a.Level == "FAIL" && strings.Contains(a.Detail, "8t") {
			failed = true
		}
	}
	if !failed {
		t.Errorf("an 8t at 100 N should be reported as skipping: %+v", got)
	}
}

// Torque rises through a reduction and falls through an overdrive, less what
// the mesh loses either way.
func TestTorqueFollowsTheRatioAndLosesToFriction(t *testing.T) {
	for _, c := range []struct {
		driver, driven int
		kind           string
		want           float64
	}{
		{8, 24, "spur", 10 * 3 * 0.94},
		{24, 8, "spur", 10 / 3.0 * 0.94},
		{12, 20, "bevel", 10 * (20 / 12.0) * 0.90},
		{1, 24, "worm", 10 * 24 * 0.45},
	} {
		rows := Propagate(10, []Stage{{
			Name: "s", DriverTeeth: c.driver, DrivenTeeth: c.driven, Kind: c.kind,
		}})
		if math.Abs(rows[0].TorqueOutNcm-c.want) > 1e-9 {
			t.Errorf("%d to %d (%s): %v Ncm out, want %v",
				c.driver, c.driven, c.kind, rows[0].TorqueOutNcm, c.want)
		}
	}
}

// Stages compound: what leaves one enters the next.
func TestAChainCompounds(t *testing.T) {
	rows := Propagate(5, []Stage{
		{Name: "one", DriverTeeth: 8, DrivenTeeth: 24},
		{Name: "two", DriverTeeth: 8, DrivenTeeth: 24},
	})
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[1].TorqueInNcm != rows[0].TorqueOutNcm {
		t.Errorf("stage two starts at %v but stage one ended at %v",
			rows[1].TorqueInNcm, rows[0].TorqueOutNcm)
	}
	if want := 5 * 3 * 0.94 * 3 * 0.94; math.Abs(rows[1].TorqueOutNcm-want) > 1e-9 {
		t.Errorf("after two stages %v, want %v", rows[1].TorqueOutNcm, want)
	}
}

// An unknown kind of mesh is not a reason to refuse an answer, but it should
// not silently claim a spur's efficiency either.
func TestAnUnknownMeshKindGetsTheCautiousFigure(t *testing.T) {
	if got := (Stage{Kind: "ratchet"}).Eff(); got != 0.9 {
		t.Errorf("unknown kind gave %v, want the fallback 0.9", got)
	}
	if got := (Stage{Kind: ""}).Eff(); got != Efficiency["spur"] {
		t.Errorf("unstated kind gave %v; an unstated mesh is a spur, as it is "+
			"everywhere else in the engine", got)
	}
}

// The thing this package must never do is imply its limits are measurements.
func TestEveryLimitSaysItIsUnverified(t *testing.T) {
	notice := Notice()
	if len(notice) == 0 {
		t.Fatal("no notice at all; the limits would read as measured")
	}
	count := 0
	for _, entries := range Limits {
		for _, l := range entries {
			count++
			if l.Verified {
				t.Errorf("a limit claims to be verified: %+v — if that is now "+
					"true, say where it was measured", l)
			}
			if l.Source == "" {
				t.Errorf("a limit of %v has no source", l.Value)
			}
		}
	}
	if len(notice) != count {
		t.Errorf("%d limits but %d listed in the notice", count, len(notice))
	}
	// And a clean assessment still points at it.
	clean := Assess(Propagate(1, []Stage{{Name: "gentle", DriverTeeth: 24, DrivenTeeth: 24}}))
	if len(clean) != 1 || !strings.Contains(clean[0].Detail, "what the limits are worth") {
		t.Errorf("a passing assessment should still point at the limits: %+v", clean)
	}
}

func TestPitchRadiusFollowsTheMeshingRule(t *testing.T) {
	// Two gears mesh at (t1+t2)/16 studs, so each contributes t/32 studs, and a
	// stud is 8 mm: t/32*8 = t/4 mm... which is half what this returns, because
	// this is in half-stud units the rest of the engine uses. The test that
	// matters is that a pair's radii add up to their center distance.
	for _, c := range [][2]int{{8, 24}, {12, 20}, {16, 16}, {24, 40}} {
		sum := PitchRadiusMM(c[0]) + PitchRadiusMM(c[1])
		if want := float64(c[0]+c[1]) / 2.0; math.Abs(sum-want) > 1e-9 {
			t.Errorf("%dt and %dt radii sum to %v, want %v", c[0], c[1], sum, want)
		}
	}
}
