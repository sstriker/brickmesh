// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/mech"
	"brickmesh/internal/shadow"
	"brickmesh/internal/spec"
	"brickmesh/internal/voxel"
)

// The structural search needs beams with shadow data, so the whole pipeline
// only runs against the real libraries.
func requireLibraries(t *testing.T) Deps {
	t.Helper()
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	return Deps{Lib: lib, Shadow: shadow.Open(root), Rast: voxel.NewRasterizer(lib)}
}

const reduction = `{
  "name": "reduction",
  "shafts": [{"id": "input", "bearings": 2}, {"id": "output", "bearings": 2}],
  "meshes": [{"a": "input", "b": "output", "teeth_a": 8, "teeth_b": 24}],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}`

func build(t *testing.T, doc string) *mech.Mechanism {
	t.Helper()
	s, err := spec.Read(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Build()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAReductionGoesAllTheWayToAModel(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(build(t, reduction), deps, Options{Restarts: 8, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if f.Level == "FAIL" {
			t.Errorf("unexpected failure: %+v", f)
		}
	}
	if res.Layout == nil {
		t.Fatal("no layout")
	}
	if len(res.Stations) != 2 {
		t.Errorf("got %d gear stations, want 2", len(res.Stations))
	}
	if res.Structure == nil || res.Structure.Count == 0 {
		t.Fatal("nothing bears the shafts")
	}
	if res.Model == nil || len(res.Model.Parts) < 3 {
		t.Fatalf("the model has %d parts", len(res.Model.Parts))
	}
}

// The pitch rule, seen from the far end of the pipeline: an 8t and a 24t come
// out (8+24)/16 = 2 studs apart, which is 40 LDU.
func TestTheGearsLandAtTheRightCenterDistance(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(build(t, reduction), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}

	var gears []geom.Vec3
	for _, p := range res.Model.Parts {
		if p.Name == "3647.dat" || p.Name == "3648b.dat" {
			gears = append(gears, p.Pos)
		}
	}
	if len(gears) != 2 {
		t.Fatalf("found %d gears in the model, want 2", len(gears))
	}
	if d := gears[0].Sub(gears[1]).Len(); math.Abs(d-40) > 1e-6 {
		t.Errorf("the gears are %v LDU apart, want 40", d)
	}
}

// What comes out has to go back in: the writer's file is read by our own
// reader, with the referenced parts beside it, and has to resolve to geometry.
// A malformed matrix or a bad reference shows up here rather than in Stud.io.
func TestTheModelReadsBackAsGeometry(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(build(t, reduction), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Read it back through the shared cache. A part is mostly references to
	// primitives, so it can only resolve where those live; the model goes in
	// beside them under a name of its own and comes out again after.
	name := "brickmesh-roundtrip-test.dat"
	path := filepath.Join(deps.Lib.CacheDir, name)
	if err := os.WriteFile(path, []byte(res.Model.Encode()), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	reader := ldraw.New(deps.Lib.CacheDir)
	reader.Offline = true
	g, err := reader.Geometry(strings.TrimSuffix(name, ".dat"))
	if err != nil {
		t.Fatalf("our own reader could not load what we wrote: %v", err)
	}
	if len(g.Verts) == 0 {
		t.Fatal("the model resolved to no geometry")
	}
	// Two shafts 40 LDU apart with gears on them span more than one part.
	if size := g.Size(); size.X < 40 || size.Z < 40 {
		t.Errorf("the model spans %+v, which looks too small to hold both shafts", size)
	}
}

func TestCheckOnlyStopsBeforeTheStructure(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(build(t, reduction), deps, Options{SkipStructure: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Structure != nil {
		t.Error("the structural search should have been skipped")
	}
	if res.Model == nil {
		t.Error("the gears should still be placed")
	}
}

// A mechanism the functional layer rejects should not reach the lattice: the
// second opinion would be about the same problem.
func TestAFailedCheckStopsTheRun(t *testing.T) {
	deps := requireLibraries(t)
	// 8t+12t sums to 20, not a multiple of 8, so the pair is off the lattice.
	res, err := Run(build(t, `{
      "name": "off-lattice",
      "shafts": [{"id": "a", "bearings": 2}, {"id": "b", "bearings": 2}],
      "meshes": [{"a": "a", "b": "b", "teeth_a": 8, "teeth_b": 12}],
      "inputs": [{"shaft": "a", "speed": 1.0}]
    }`), deps, Options{Restarts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed() {
		t.Error("an off-lattice pair should fail")
	}
	if res.Layout != nil {
		t.Error("a failed mechanism should not have been placed")
	}
}

func TestEveryGearPartNumberResolves(t *testing.T) {
	deps := requireLibraries(t)
	for teeth, name := range GearParts {
		g, err := deps.Lib.Geometry(name)
		if err != nil {
			t.Errorf("%dt -> %s: %v", teeth, name, err)
			continue
		}
		// A gear is a disc: its own Z is the short axis, which is the
		// convention the model relies on for orientation.
		if g.ThinAxis() != 2 {
			t.Errorf("%dt (%s): thin axis is %d, not Z; the orientation convention "+
				"would place it wrongly", teeth, name, g.ThinAxis())
		}
	}
}

const gearbox = `{
  "name": "2-speed",
  "states": ["low", "high"],
  "shafts": [
    {"id": "input", "bearings": 2}, {"id": "output", "bearings": 2},
    {"id": "low", "bearings": 2}, {"id": "high", "bearings": 2}
  ],
  "meshes": [
    {"a": "input", "b": "low", "teeth_a": 16, "teeth_b": 24},
    {"a": "input", "b": "high", "teeth_a": 24, "teeth_b": 16}
  ],
  "couplings": [
    {"a": "output", "b": "low", "name": "ring low", "states": ["low"]},
    {"a": "output", "b": "high", "name": "ring high", "states": ["high"]}
  ],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}`

func TestAGearboxGetsItsDrivingRings(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(build(t, gearbox), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}

	var rings, gears []geom.Vec3
	for _, p := range res.Model.Parts {
		switch p.Name {
		case DrivingRing:
			rings = append(rings, p.Pos)
		case "3648b.dat", "4019.dat":
			gears = append(gears, p.Pos)
		}
	}
	if len(rings) != 2 {
		t.Fatalf("got %d driving rings, want one per shift", len(rings))
	}
	// A ring engages the gear beside it, so it must not be inside one.
	for _, r := range rings {
		for _, g := range gears {
			if r.Sub(g).Len() < 1e-6 {
				t.Errorf("a ring landed on top of a gear at %+v", r)
			}
		}
	}
}

func TestTheSelectorPartsAreNamedRatherThanPlaced(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(build(t, gearbox), deps, Options{Restarts: 2, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	var said bool
	for _, f := range res.Findings {
		if f.Check == "parts" && strings.Contains(f.Detail, "6641") {
			said = true
		}
	}
	if !said {
		t.Error("the report should name what moves the rings, since it is not placed")
	}
	for _, p := range res.Model.Parts {
		if strings.HasPrefix(p.Name, "6641") || strings.HasPrefix(p.Name, "6631") {
			t.Errorf("%s was placed; its position is not determined by the mechanism", p.Name)
		}
	}
}

// A mechanism with no shift gets no rings.
func TestNoShiftMeansNoRings(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(build(t, reduction), deps, Options{Restarts: 2, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Model.Parts {
		if p.Name == DrivingRing {
			t.Error("a plain reduction should not get a driving ring")
		}
	}
}
