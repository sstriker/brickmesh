// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package bevel

import (
	"math"
	"os"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldraw"
)

// The pitch radius is teeth x 1.25 LDU, measured off the parts.
//
// It mattered because a bevel pair's placement was the last open question in
// PLAN.md, and it rested on a rule taken from documentation: each gear sits at
// the OTHER's pitch radius from where the axes cross. Half of that rule is the
// module, and the module can be measured — a gear's outermost material is its
// pitch circle plus however far the tooth tip stands proud, and that overhang
// is small and consistent.
func TestThePitchRadiusIsTeethTimesFiveQuarters(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")
	for _, c := range []struct {
		name  string
		teeth int
	}{
		{"3648b.dat", 24}, {"4019.dat", 16}, {"6542a.dat", 16},
		{"32270.dat", 12}, {"32269.dat", 20}, {"32498.dat", 36},
		{"18946.dat", 16}, {"81346.dat", 20}, {"3647.dat", 8},
	} {
		g, err := lib.Geometry(c.name)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		outer := 0.0
		for _, v := range g.Verts {
			outer = math.Max(outer, math.Hypot(v.X, v.Y))
		}
		pitch := float64(c.teeth) * 1.25
		proud := outer - pitch
		// A tooth stands proud of the pitch circle by about an addendum. Wider
		// than this and the pitch radius is not what the module says it is.
		if proud < 0.5 || proud > 3.0 {
			t.Errorf("%s %dt: outermost material at %.2f, which is %.2f proud of "+
				"the %.2f the module gives. The module is what fixes where a "+
				"bevel pair sits", c.name, c.teeth, outer, proud, pitch)
		}
	}
}

// The three double bevels agree to a hundredth, which is what says the overhang
// is a designed addendum rather than a coincidence of three shapes.
func TestTheDoubleBevelsShareOneAddendum(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")
	var proud []float64
	for _, c := range []struct {
		name  string
		teeth int
	}{{"32270.dat", 12}, {"32269.dat", 20}, {"32498.dat", 36}} {
		g, err := lib.Geometry(c.name)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		outer := 0.0
		for _, v := range g.Verts {
			outer = math.Max(outer, math.Hypot(v.X, v.Y))
		}
		proud = append(proud, outer-float64(c.teeth)*1.25)
	}
	for _, p := range proud[1:] {
		if math.Abs(p-proud[0]) > 0.02 {
			t.Errorf("the double bevels stand %v proud of their pitch circles; "+
				"they should share one addendum", proud)
		}
	}
}

// And given the module, where the two sit is geometry rather than a rule.
//
// Two pitch circles have to touch. Put the crossing at the origin with A's axis
// along z and B's along x: A's circle is (Ra cos, Ra sin, da) and B's is
// (db, Rb sin, Rb cos). A common point forces db = Ra and da = Rb — each gear
// at the OTHER's pitch radius from the crossing, which is what layout applies.
//
// Stated as a test because it is the half of the answer that needs no library:
// nine measurement approaches disagreed about bevel engagement, and all nine
// were asking where the surfaces touch rather than where the pitch circles do.
func TestEachBevelSitsAtTheOthersPitchRadius(t *testing.T) {
	const module = 1.25
	for _, c := range []struct{ ta, tb int }{{12, 20}, {12, 12}, {20, 36}, {8, 24}} {
		ra, rb := float64(c.ta)*module, float64(c.tb)*module
		// The placement the geometry forces.
		da, db := rb, ra
		a := geom.Vec3{X: ra, Z: da} // a point on A's pitch circle
		b := geom.Vec3{X: db, Z: rb} // and on B's
		if a.Sub(b).Len() > 1e-9 {
			t.Errorf("%dt/%dt: the pitch circles do not meet at %v and %v, so "+
				"placing each at the other's radius is not what makes them mesh",
				c.ta, c.tb, a, b)
		}
	}
}
