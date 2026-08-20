// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/layout"
	"brickmesh/internal/ldcad"
	"brickmesh/internal/mech"
	"brickmesh/internal/spec"
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
