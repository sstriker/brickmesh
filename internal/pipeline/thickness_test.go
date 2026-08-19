// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"math"
	"testing"

	"brickmesh/internal/clutch"
	"brickmesh/internal/layout"
)

// The layout reserves room on a shaft from a table keyed by tooth count, and
// anything not in the table gets two half studs. That is right for every gear
// this engine can place — measured, not recalled: all of them are 20 LDU along
// their own axis, the clutch variants included, and only the 24t differs at
// 19.25 because its rim is chamfered.
//
// The table cannot see the part, only the count, so it cannot notice a part
// that breaks the rule. This can. A gear reserved too little room sits close
// enough to its neighbour to be called clear when it is not, and nothing else
// would say so — the clearance sweep allows gear against gear, because that is
// what meshing is.
func TestEveryGearIsTheThicknessTheLayoutAssumes(t *testing.T) {
	deps := requireLibraries(t)

	check := func(teeth int, name string) {
		t.Helper()
		g, err := deps.Lib.Geometry(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		lo, hi := g.BBox()
		// A gear's rotation axis is its own Z, by the disc convention this
		// engine places them on.
		got := (hi.Z - lo.Z) / 10 // LDU to half studs
		want := layout.GearThickness[teeth]
		if want == 0 {
			want = 2 // the default the layout falls back to
		}
		if math.Abs(got-want) > 0.15 {
			t.Errorf("%s (%dt) is %.2f half studs thick, and the layout reserves "+
				"%.2f. Either the table is wrong or this part does not belong "+
				"in it", name, teeth, got, want)
		}
	}

	for teeth, name := range GearParts {
		check(teeth, name)
	}
	// The clutch variants are the ones most likely to differ, being wider
	// hardware wearing the same tooth count.
	for _, s := range clutch.Systems {
		for teeth, name := range s.Gears {
			check(teeth, name)
		}
	}
}

// And the two rings are not gears: they are the length the clutch package says,
// which is what the shaft allocator leaves room for.
func TestARingIsAsLongAsItsSystemSays(t *testing.T) {
	deps := requireLibraries(t)
	for _, s := range clutch.Systems {
		for name, half := range map[string]float64{
			s.Ring: s.RingHalf, s.Joiner: s.JoinerHalf,
		} {
			g, err := deps.Lib.Geometry(name)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			lo, hi := g.BBox()
			got := (hi.Z - lo.Z) / 10 / 2 // half studs, then half of it
			if math.Abs(got-half) > 0.35 {
				t.Errorf("%s reaches %.2f half studs either side of its centre, "+
					"and %s reserves %.2f", name, got, s.Name, half)
			}
		}
	}
}
