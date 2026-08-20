// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package synth

import (
	"math"
	"testing"

	"brickmesh/internal/geom"
)

// Part count is a poor measure of a frame and this is why: a pin counts the
// same as a thirteen-hole beam, though one is a fastener and the other is most
// of the structure.
func TestCostChargesForLengthRatherThanForParts(t *testing.T) {
	holes := map[string]int{"long": 13, "short": 3}
	b := DefaultBudget

	oneLong := b.cost([]Placed{{Part: "long"}}, holes, 0)
	threeShort := b.cost([]Placed{{Part: "short"}, {Part: "short"}, {Part: "short"}}, holes, 0)

	if threeShort >= oneLong {
		t.Errorf("three 3-hole beams cost %.2f and one 13-hole beam %.2f; six "+
			"studs of beam should come cheaper than twelve however many parts "+
			"it is split into", threeShort, oneLong)
	}
	// And a part is not free, or the search would happily use a hundred.
	if b.cost([]Placed{{Part: "short"}, {Part: "short"}}, holes, 0) <=
		b.cost([]Placed{{Part: "short"}}, holes, 0)*2-1e-9 {
		t.Error("two parts should cost more than one, by the per-part term")
	}
}

// And the term that lets a compact frame win, which is the one raising the part
// count buys.
func TestCostChargesForTheEnvelope(t *testing.T) {
	holes := map[string]int{"short": 3}
	b := DefaultBudget
	small := b.cost([]Placed{{Part: "short"}}, holes, 10)
	big := b.cost([]Placed{{Part: "short"}}, holes, 40)
	if big <= small {
		t.Errorf("the same parts in a bigger box cost %.2f against %.2f", big, small)
	}
}

// A bound is not a preference: outside it is not a candidate.
func TestTheEnvelopeIsAHardBound(t *testing.T) {
	b := Budget{MaxStuds: geom.Vec3{Z: 3}}
	lo := geom.Vec3{}
	for _, c := range []struct {
		depth float64
		want  bool
	}{{2 * geom.Stud, true}, {3 * geom.Stud, true}, {4 * geom.Stud, false}} {
		got := b.withinEnvelope(lo, geom.Vec3{Z: c.depth})
		if got != c.want {
			t.Errorf("%.0f studs deep against a bound of 3: got %v, want %v",
				c.depth/geom.Stud, got, c.want)
		}
	}
	// An axis with no bound admits anything, so a caller can cap one dimension
	// without having to have an opinion about the others.
	if !b.withinEnvelope(lo, geom.Vec3{X: 500, Z: geom.Stud}) {
		t.Error("x has no bound, so five hundred studs of it is allowed")
	}
	if !(Budget{}).withinEnvelope(lo, geom.Vec3{X: 1e6, Y: 1e6, Z: 1e6}) {
		t.Error("no bounds at all should admit anything")
	}
}

// The default weights have to be comparable, or one term drowns the others.
func TestTheDefaultWeightsAreInTheSameRange(t *testing.T) {
	b := DefaultBudget
	if b.PerStud <= 0 || b.PerCubicStud <= 0 {
		t.Fatal("length and envelope both have to cost something")
	}
	if r := b.PerStud / b.PerCubicStud; r > 10 || r < 0.1 {
		t.Errorf("a stud of beam and a cubic stud of envelope differ by %.0fx; "+
			"one of them will decide every ranking on its own", math.Max(r, 1/r))
	}
	if b.PerPart >= b.PerStud {
		t.Errorf("a bare part costs %.2f and a stud of beam %.2f; a fastener "+
			"should not weigh as much as structure", b.PerPart, b.PerStud)
	}
}
