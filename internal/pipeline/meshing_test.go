// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/layout"
	"github.com/sstriker/brickmesh/internal/ldcad"
	"github.com/sstriker/brickmesh/internal/mech"
	"github.com/sstriker/brickmesh/internal/spec"
)

// The check earns its place by failing on the thing that used to pass.
//
// A three-speed shipped with its first stage diagonal — the two gears of a mesh
// a stud apart along their shafts — and every check passed, because the
// arithmetic never saw a placement and the stations were coplanar by
// construction. Only the finished positions tell.
func TestAPairOutOfPlaneIsCaught(t *testing.T) {
	m := twoShaftMesh(t)
	res := &Result{
		Layout: &layout.Layout{Place: map[string]layout.Placement{
			"a": layout.NewPlacement(geom.Vec3{}, geom.Vec3{X: 1}),
			"b": layout.NewPlacement(geom.Vec3{Z: -4}, geom.Vec3{X: 1}),
		}},
		Stations: []layout.Station{
			{Shaft: "a", Teeth: 16, Axial: 0},
			{Shaft: "b", Teeth: 16, Axial: 8}, // a whole stud out of plane
		},
	}
	checkMeshing(res, m)
	if !hasFail(res, "meshing") {
		t.Error("a pair 80 LDU apart along their shafts was called meshing")
	}
}

func TestAPairInPlaneAtItsPitchDistancePasses(t *testing.T) {
	m := twoShaftMesh(t)
	res := &Result{
		Layout: &layout.Layout{Place: map[string]layout.Placement{
			"a": layout.NewPlacement(geom.Vec3{}, geom.Vec3{X: 1}),
			"b": layout.NewPlacement(geom.Vec3{Z: -4}, geom.Vec3{X: 1}),
		}},
		Stations: []layout.Station{
			{Shaft: "a", Teeth: 16, Axial: 0},
			{Shaft: "b", Teeth: 16, Axial: 0},
		},
	}
	checkMeshing(res, m)
	if hasFail(res, "meshing") {
		t.Errorf("a pair standing correctly was reported: %v", res.Findings)
	}
}

// One shaft can carry two gears of the same size, one per mesh, and nothing in
// a Station says which mesh put it there. Asking about the first two called a
// working two-speed broken.
func TestTwoGearsOfOneSizeOnAShaftAreNotConfused(t *testing.T) {
	m := twoShaftMesh(t)
	res := &Result{
		Layout: &layout.Layout{Place: map[string]layout.Placement{
			"a": layout.NewPlacement(geom.Vec3{}, geom.Vec3{X: 1}),
			"b": layout.NewPlacement(geom.Vec3{Z: -4}, geom.Vec3{X: 1}),
		}},
		Stations: []layout.Station{
			{Shaft: "a", Teeth: 16, Axial: 0},
			{Shaft: "a", Teeth: 16, Axial: 8}, // the one this mesh is about
			{Shaft: "b", Teeth: 16, Axial: 8},
		},
	}
	checkMeshing(res, m)
	if hasFail(res, "meshing") {
		t.Errorf("the second 16t on 'a' is the one that meshes: %v", res.Findings)
	}
}

func twoShaftMesh(t *testing.T) *mech.Mechanism {
	t.Helper()
	sp := spec.Spec{
		Name:   "pair",
		Shafts: []spec.Shaft{{ID: "a"}, {ID: "b"}},
		Meshes: []spec.Mesh{{A: "a", B: "b", TeethA: 16, TeethB: 16}},
		Inputs: []spec.Input{{Shaft: "a", Speed: 1}},
	}
	m, err := sp.Build()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func hasFail(res *Result, check string) bool {
	for _, f := range res.Findings {
		if f.Check == check && f.Level == "FAIL" {
			return true
		}
	}
	return false
}

// The catch turns on its axle; it never travels with its ring.
func TestNoCatchIsAnimatedSliding(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpecAnimated(t, deps, filepath.Join("..", "..", "examples",
		"gearbox-3-speed-compound.json"))
	if res.Script == nil {
		t.Fatal("no animation")
	}
	out := res.Script.Render()
	if !strings.Contains(out, "orbit(") {
		t.Error("no catch turns on its axle; every axle hole in both catches " +
			"runs across the shaft, so none of them can be pushed along it")
	}
	for _, ani := range res.Script.Animations {
		for _, sl := range ani.Sliding {
			if strings.HasPrefix(sl.Group, "catch_") {
				t.Errorf("%s is animated as a slide", sl.Group)
			}
		}
	}
}

// The middle shaft of a compound gearbox holds only at the shift that moves its
// own ring, not at the one that moves the output's.
func TestAShiftOnlyStopsWhatItActuallyDisconnects(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpecAnimated(t, deps, filepath.Join("..", "..", "examples",
		"gearbox-3-speed-compound.json"))
	if res.Script == nil {
		t.Fatal("no animation")
	}
	var shift *ldcad.Animation
	for i := range res.Script.Animations {
		if res.Script.Animations[i].Name == "shift" {
			shift = &res.Script.Animations[i]
		}
	}
	if shift == nil {
		t.Fatal("no shift animation")
	}
	holds := map[string][]bool{}
	for _, tn := range shift.Turning {
		holds[tn.Group] = tn.Holds
	}
	// 1st -> 2nd moves the output's ring; 2nd -> 3rd moves the middle one.
	for group, want := range map[string][]bool{
		"shaft_input":  {false, false, false},
		"shaft_mid":    {false, true, false},
		"shaft_output": {true, true, false},
	} {
		got := holds[group]
		if len(got) != len(want) {
			t.Errorf("%s: %d hold flags, want %d", group, len(got), len(want))
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s holds %v, want %v — a shift that moves another "+
					"ring must not stop it", group, got, want)
				break
			}
		}
	}
}

// The kinds nothing in examples/ exercises. Before this they were skipped in
// silence, and the report still said every pair stood correctly — which is the
// same mistake as not checking, dressed as cover.

func place(pt geom.Vec3, dir geom.Vec3) layout.Placement {
	return layout.NewPlacement(pt, dir)
}

// A bevel pair: square shafts whose axes meet, each gear at the other's
// effective radius from the meeting point. 12t and 20t, so the radii are 15 and
// 25 LDU and each sits at the OTHER's.
func bevelAt(aAxial, bAxial float64) *Result {
	return &Result{
		Layout: &layout.Layout{Place: map[string]layout.Placement{
			"a": place(geom.Vec3{}, geom.Vec3{X: 1}),
			"b": place(geom.Vec3{}, geom.Vec3{Y: 1}),
		}},
		Stations: []layout.Station{
			{Shaft: "a", Teeth: 12, Axial: aAxial},
			{Shaft: "b", Teeth: 20, Axial: bAxial},
		},
	}
}

func bevelMech(t *testing.T) *mech.Mechanism {
	t.Helper()
	sp := spec.Spec{
		Name:   "bevel",
		Shafts: []spec.Shaft{{ID: "a"}, {ID: "b"}},
		Meshes: []spec.Mesh{{A: "a", B: "b", TeethA: 12, TeethB: 20, Kind: "bevel"}},
		Inputs: []spec.Input{{Shaft: "a", Speed: 1}},
	}
	m, err := sp.Build()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestABevelPairStandingByTheRulePasses(t *testing.T) {
	// 12t sits at the 20t's radius (25 LDU = 2.5 half studs) and vice versa.
	res := bevelAt(2.5, 1.5)
	checkMeshing(res, bevelMech(t))
	if hasFail(res, "meshing") {
		t.Errorf("a bevel pair placed by the layout's own rule was reported: %v",
			res.Findings)
	}
}

func TestABevelGearAtTheWrongRadiusIsCaught(t *testing.T) {
	res := bevelAt(2.5, 4.0) // the 20t a stud and a half too far out
	checkMeshing(res, bevelMech(t))
	if !hasFail(res, "meshing") {
		t.Error("a bevel gear off its radius was called correct")
	}
}

func TestBevelShaftsThatNeverMeetAreCaught(t *testing.T) {
	res := bevelAt(2.5, 1.5)
	// Square, but offset so the axes are skew rather than crossing.
	res.Layout.Place["b"] = place(geom.Vec3{Z: 4}, geom.Vec3{Y: 1})
	checkMeshing(res, bevelMech(t))
	if !hasFail(res, "meshing") {
		t.Error("bevel shafts whose axes never meet were called a mesh")
	}
}

// A differential holds three shafts on one line. Nothing said so before.
func TestADifferentialOffItsLineIsCaught(t *testing.T) {
	sp := spec.Spec{
		Name:   "diff",
		Shafts: []spec.Shaft{{ID: "case"}, {ID: "l"}, {ID: "r"}},
		Differentials: []spec.Differential{
			{Case: "case", OutA: "l", OutB: "r"},
		},
		Inputs: []spec.Input{{Shaft: "case", Speed: 1}},
	}
	m, err := sp.Build()
	if err != nil {
		t.Fatal(err)
	}
	res := &Result{Layout: &layout.Layout{Place: map[string]layout.Placement{
		"case": place(geom.Vec3{}, geom.Vec3{X: 1}),
		"l":    place(geom.Vec3{}, geom.Vec3{X: 1}),
		"r":    place(geom.Vec3{Z: 2}, geom.Vec3{X: 1}), // a stud off the line
	}}}
	checkMeshing(res, m)
	if !hasFail(res, "meshing") {
		t.Error("a differential output on its own line was not reported")
	}
	res.Findings = nil
	res.Layout.Place["r"] = place(geom.Vec3{}, geom.Vec3{X: 1})
	checkMeshing(res, m)
	if hasFail(res, "meshing") {
		t.Errorf("three coaxial shafts were reported: %v", res.Findings)
	}
}

// And the report has to say what it did NOT look at, or it reads as cover.
func TestTheReportNamesWhatItCouldNotCheck(t *testing.T) {
	res := bevelAt(2.5, 1.5)
	checkMeshing(res, bevelMech(t))
	var detail string
	for _, f := range res.Findings {
		if f.Check == "meshing" {
			detail = f.Detail
		}
	}
	if !strings.Contains(detail, "Not checked") {
		t.Errorf("a bevel's engagement rule is not settled and the report "+
			"should say so; it said: %s", detail)
	}
}

// The axle a catch turns on is placed, and the run says whether anything holds
// it. Both halves matter: the placement because the catch's own hole fixes it,
// and the answer because today's frames do not reach it.
func TestTheCatchGetsAnAxleAndTheRunSaysIfNothingHoldsIt(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples",
		"gearbox-2-speed.json"))

	var placed int
	for _, p := range res.Model.Parts {
		if strings.Contains(p.Label, "control axle") {
			placed++
		}
	}
	if placed == 0 {
		t.Fatal("a ring with a catch got no axle for the catch to turn on")
	}

	// It has to run through the catch's own hole, or it is an axle beside a
	// catch rather than under one.
	for _, site := range res.ringSites {
		if site.catchRot == (geom.Mat3{}) {
			continue
		}
		axes := controlAxles(res)
		if len(axes) == 0 {
			t.Fatal("no control axle worked out for a placed catch")
		}
		c := axes[0]
		catch := catchPos(t, res)
		// The pivot is on the axle's line.
		d := c.at.Sub(catch)
		along := c.dir.Scale(d.Dot(c.dir))
		if d.Sub(along).Len() > 1e-6 {
			t.Errorf("the control axle misses the catch: %v is not on the line "+
				"through %v along %v", catch, c.at, c.dir)
		}
		break
	}

	var said bool
	for _, f := range res.Findings {
		if f.Check == "bearings" && strings.Contains(f.Detail, "control axle") {
			said = true
		}
	}
	if !said {
		t.Error("the run should say whether the frame holds the control axle; " +
			"an axle nothing holds is worth knowing about before building")
	}
}

func catchPos(t *testing.T, res *Result) geom.Vec3 {
	t.Helper()
	for _, p := range res.Model.Parts {
		if strings.HasPrefix(p.Label, "catch for") {
			return p.Pos
		}
	}
	t.Fatal("no catch in the model")
	return geom.Vec3{}
}
