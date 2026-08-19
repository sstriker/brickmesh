// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/clutch"
	"brickmesh/internal/collide"
	"brickmesh/internal/extract"
	"brickmesh/internal/geom"
	"brickmesh/internal/interfere"
	"brickmesh/internal/ldr"
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
	return Deps{Lib: lib, Shadow: extract.Ports{Lib: shadow.Open(root)},
		Rast: voxel.NewRasterizer(lib)}
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
	res, err := Run(context.Background(), build(t, reduction), deps, Options{Restarts: 8, Seed: 1})
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
	res, err := Run(context.Background(), build(t, reduction), deps, Options{Restarts: 4, Seed: 1})
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
	res, err := Run(context.Background(), build(t, reduction), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Read it back through the shared cache. A part is mostly references to
	// primitives, so it can only resolve where those live; the model goes in
	// beside them under a name of its own and comes out again after.
	// Its own handle on the library, since Deps carries the interface now and
	// this test is about where files sit on disk.
	lib := ldraw.New("")
	if lib.Root == "" {
		t.Skip("no extracted library to put the model beside")
	}
	name := "brickmesh-roundtrip-test.dat"
	path := filepath.Join(lib.Root, name)
	if err := os.WriteFile(path, []byte(res.Model.Encode()), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	reader := ldraw.New(lib.CacheDir)
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
	res, err := Run(context.Background(), build(t, reduction), deps, Options{SkipStructure: true})
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
	res, err := Run(context.Background(), build(t, `{
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
    {"a": "input", "b": "low", "teeth_a": 12, "teeth_b": 20},
    {"a": "input", "b": "high", "teeth_a": 16, "teeth_b": 16}
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
	res, err := Run(context.Background(), build(t, gearbox), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}

	var rings []ldr.Part
	var gears []ldr.Part
	for _, p := range res.Model.Parts {
		if isRing(p.Name) {
			rings = append(rings, p)
			continue
		}
		if _, _, ok := gearFromLabel(p.Label); ok {
			gears = append(gears, p)
		}
	}
	if len(rings) != 2 {
		t.Fatalf("got %d driving rings, want one per shift", len(rings))
	}

	// Not "is it on top of a gear" but "can it be built": every ring is turned
	// a full revolution against every gear, and anything a ring passes through
	// is a model that cannot be assembled. Checking only for a shared position
	// is what let the rings sit inside their gears for as long as they did.
	ring, err := interfere.MeshFor(deps.Lib, rings[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rings {
		for _, g := range gears {
			gear, err := interfere.MeshFor(deps.Lib, g.Name)
			if err != nil {
				t.Fatal(err)
			}
			got, err := interfere.MeshLock(context.Background(),
				gear, collide.Transform{Rot: g.Rot, Pos: g.Pos},
				ring, collide.Transform{Rot: r.Rot, Pos: r.Pos},
				16, interfere.Options{Steps: 72})
			if err != nil {
				t.Fatal(err)
			}
			if got.Verdict == interfere.TooDeep {
				t.Errorf("the ring at %+v is inside the %s at %+v: no rotation "+
					"of it is free", r.Pos, g.Name, g.Pos)
			}
		}
	}
}

// A ring beside a plain gear has nothing to grip. Where the library has the
// clutch variant, the shifted station gets it.
func TestAShiftedSixteenBecomesAClutchGear(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, gearbox), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := clutch.First.Gears[16]
	var found bool
	for _, p := range res.Model.Parts {
		if p.Name == want {
			found = true
		}
	}
	if !found {
		t.Errorf("no %s in the model: the shifted 16t should be the clutch variant", want)
	}
}

func TestTheSelectorPartsAreNamedRatherThanPlaced(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, gearbox), deps, Options{Restarts: 2, Seed: 1})
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
	res, err := Run(context.Background(), build(t, reduction), deps, Options{Restarts: 2, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Model.Parts {
		if isRing(p.Name) {
			t.Error("a plain reduction should not get a driving ring")
		}
	}
}

// The shafts are real parts, and they are what ties the bearings of one shaft
// together — leaving them out is why a sound structure read as loose pieces.
func TestTheShaftsAreInTheModelAndHoldItTogether(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, reduction), deps, Options{Restarts: 8, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}

	axles := 0
	for _, p := range res.Model.Parts {
		for _, name := range AxleParts {
			if p.Name == name {
				axles++
			}
		}
	}
	if axles != 2 {
		t.Errorf("got %d axles in the model, want one per shaft", axles)
	}

	var rigid bool
	for _, f := range res.Findings {
		if f.Check == "rigidity" && f.Level == "OK" {
			rigid = true
		}
		if f.Check == "connectivity" {
			t.Errorf("still in pieces: %s", f.Detail)
		}
	}
	if !rigid {
		t.Error("the structure should hold together once the shafts are counted")
	}
}

func TestAnAxleIsLongEnoughForItsShaft(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, reduction), deps, Options{Restarts: 4, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Axles) == 0 {
		t.Fatal("no axles")
	}
	// Every gear on a line has to fall within its axle.
	for _, st := range res.Stations {
		place := res.Layout.Place[st.Shaft]
		at := place.Point.Scale(synthHalfStud).Add(place.Direction.Scale(st.Axial * synthHalfStud))
		covered := false
		for _, a := range res.Axles {
			if a.Covers(at, place.Direction) {
				covered = true
			}
		}
		if !covered {
			t.Errorf("the %dt on '%s' has no axle through it", st.Teeth, st.Shaft)
		}
	}
}

const synthHalfStud = 10.0

// Three states, for the animation tests, which do not care what grips what.
// Not a buildable shift: the 24t cannot be dog-shifted, and no three ratios can
// be, on one pair of shafts. See docs/findings.md.
const gearboxSpec = `{
  "name": "3-speed",
  "states": ["1st", "2nd", "3rd"],
  "shafts": [
    {"id": "input", "bearings": 2}, {"id": "output", "bearings": 2},
    {"id": "g1", "bearings": 2}, {"id": "g2", "bearings": 2}, {"id": "g3", "bearings": 2}
  ],
  "meshes": [
    {"a": "input", "b": "g1", "teeth_a": 8, "teeth_b": 24},
    {"a": "input", "b": "g2", "teeth_a": 12, "teeth_b": 20},
    {"a": "input", "b": "g3", "teeth_a": 16, "teeth_b": 16}
  ],
  "couplings": [
    {"a": "output", "b": "g1", "states": ["1st"]},
    {"a": "output", "b": "g2", "states": ["2nd"]},
    {"a": "output", "b": "g3", "states": ["3rd"]}
  ],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}`

func TestAGearboxAnimatesOncePerState(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, gearboxSpec), deps, Options{
		Restarts: 4, Seed: 1, Animate: true, ScriptName: "gb.lua",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Script == nil {
		t.Fatal("no script")
	}
	// One per state, plus the one that walks through them.
	var names []string
	for _, a := range res.Script.Animations {
		names = append(names, a.Name)
	}
	if len(res.Script.Animations) != 4 {
		t.Fatalf("got %v, want one per state and a shift", names)
	}
	last := res.Script.Animations[3]
	if last.Name != "shift" || len(last.Segments) != 3 {
		t.Errorf("got %v with %d segments, want a shift walking all three states",
			last.Name, len(last.Segments))
	}
	if res.Model.Script != "gb.lua" {
		t.Errorf("the model should reference the script, got %q", res.Model.Script)
	}
}

// The point of the export: what turns on screen is the ratio the mechanism
// solved for, not an approximation of it.
func TestTheAnimatedOutputTurnsAtTheSolvedRatio(t *testing.T) {
	deps := requireLibraries(t)
	m := build(t, gearboxSpec)
	res, err := Run(context.Background(), m, deps, Options{Restarts: 4, Seed: 1, Animate: true, ScriptName: "gb.lua"})
	if err != nil {
		t.Fatal(err)
	}

	for _, ani := range res.Script.Animations {
		if len(ani.Segments) > 0 {
			continue // the walk through the states, checked state by state above
		}
		speeds, ok := m.Solve(ani.Name)
		if !ok {
			t.Fatalf("%s does not solve", ani.Name)
		}
		var found bool
		for _, turn := range ani.Turning {
			if turn.Group != "shaft_output" {
				continue
			}
			found = true
			if math.Abs(turn.Speed-speeds["output"]) > 1e-9 {
				t.Errorf("%s: animating the output at %v, solved %v",
					ani.Name, turn.Speed, speeds["output"])
			}
		}
		if !found {
			t.Errorf("%s: the output shaft is not animated", ani.Name)
		}
	}
}

// The freewheeling gears keep turning in every state — they are always meshed.
func TestTheIdleGearsKeepTurning(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, gearboxSpec), deps, Options{
		Restarts: 4, Seed: 1, Animate: true, ScriptName: "gb.lua",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ani := range res.Script.Animations {
		for _, turn := range ani.Turning {
			if strings.HasPrefix(turn.Group, "shaft_g") && turn.Speed == 0 {
				t.Errorf("%s: %s stands still, but it is always meshed",
					ani.Name, turn.Group)
			}
		}
	}
}

func TestEveryTurningGroupIsDeclaredInTheModel(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, gearboxSpec), deps, Options{
		Restarts: 4, Seed: 1, Animate: true, ScriptName: "gb.lua",
	})
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	for _, g := range res.Model.Groups {
		declared[g.Name] = true
	}
	for _, ani := range res.Script.Animations {
		for _, turn := range ani.Turning {
			if !declared[turn.Group] {
				t.Errorf("the script turns %q, which the model never declares", turn.Group)
			}
		}
	}
	// And every declared group has at least one part in it, or it turns nothing.
	inUse := map[string]bool{}
	for _, p := range res.Model.Parts {
		if p.Group != "" {
			inUse[p.Group] = true
		}
	}
	for name := range declared {
		if !inUse[name] {
			t.Errorf("group %q is declared but holds no parts", name)
		}
	}
}

func TestNoAnimationUnlessAsked(t *testing.T) {
	deps := requireLibraries(t)
	res, err := Run(context.Background(), build(t, reduction), deps, Options{Restarts: 2, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Script != nil || res.Model.Script != "" || len(res.Model.Groups) != 0 {
		t.Error("the model should stay plain LDraw unless an animation was asked for")
	}
}
