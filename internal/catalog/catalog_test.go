// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/geom"
)

// A liftarm-shaped part: three holes on the 20 LDU pitch, axes along Y, plus
// one axle (cross) hole. Enough to exercise load, index and lookup without
// touching the real catalog.
const sampleJSON = `[
  {"id": "32523", "title": "Technic Beam 3", "tier": 1,
   "holes": [[0,0,0, 0,1,0, 0], [20,0,0, 0,1,0, 0], [40,0,0, 0,1,0, 0]],
   "pins": []},
  {"id": "32270", "title": "Technic Gear 12 Tooth Double Bevel", "tier": 1,
   "holes": [[0,0,0, 0,0,1, 1]],
   "pins": []},
  {"id": "3673", "title": "Technic Pin", "tier": 1,
   "holes": [],
   "pins": [[0,0,0, 1,0,0, 0]]},
  {"id": "9999", "title": "Something exotic", "tier": 3,
   "holes": [[0,0,0, 0,1,0, 0]],
   "pins": []}
]`

func load(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load(strings.NewReader(sampleJSON))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func TestLoadReadsPortsAndGender(t *testing.T) {
	c := load(t)
	if len(c.Parts) != 4 {
		t.Fatalf("got %d parts, want 4", len(c.Parts))
	}
	beam := c.Parts["32523"]
	if beam == nil {
		t.Fatal("part 32523 missing")
	}
	if len(beam.Ports) != 3 {
		t.Fatalf("got %d ports on the beam, want 3", len(beam.Ports))
	}
	for _, p := range beam.Ports {
		if p.Gender != Female {
			t.Error("holes should load as female")
		}
		if p.Kind != Round {
			t.Error("a pin hole should be Round")
		}
	}
	if got := c.Parts["3673"].Ports[0].Gender; got != Male {
		t.Errorf("pins should load as male, got %v", got)
	}
	// The trailing 1 in the record marks a cross (axle) hole.
	if got := c.Parts["32270"].Ports[0].Kind; got != Cross {
		t.Errorf("axle hole should be Cross, got %v", got)
	}
}

func TestLoadNormalizesAxes(t *testing.T) {
	c, err := Load(strings.NewReader(
		`[{"id":"x","title":"t","tier":1,"holes":[[0,0,0, 0,5,0, 0]],"pins":[]}]`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Parts["x"].Ports[0].Axis; got != (geom.Vec3{Y: 1}) {
		t.Errorf("axis = %+v, want a unit vector {0 1 0}", got)
	}
}

func TestLoadRejectsGarbage(t *testing.T) {
	if _, err := Load(strings.NewReader("not json")); err == nil {
		t.Error("expected an error on malformed input")
	}
}

func TestBuildIndexHonorsTier(t *testing.T) {
	c := load(t)
	c.BuildIndex(1)
	// Part 9999 is tier 3: at maxTier 1 it must not be reachable.
	got := c.Lookup(geom.Vec3{}, geom.Vec3{Y: 1}, Female, Round, nil)
	for _, pl := range got {
		if pl.Part.ID == "9999" {
			t.Fatal("tier 3 part surfaced at maxTier 1")
		}
	}
	c.BuildIndex(3)
	found := false
	for _, pl := range c.Lookup(geom.Vec3{}, geom.Vec3{Y: 1}, Female, Round, nil) {
		if pl.Part.ID == "9999" {
			found = true
		}
	}
	if !found {
		t.Error("tier 3 part should be reachable at maxTier 3")
	}
}

// The core promise of the index: origin = point - offset, and every hole of the
// beam can be the one that lands on the query point.
func TestLookupPlacesEveryHoleOnThePoint(t *testing.T) {
	c := load(t)
	c.BuildIndex(1)

	point := geom.Vec3{X: 100, Y: 0, Z: 60}
	got := c.Lookup(point, geom.Vec3{Y: 1}, Female, Round, nil)

	origins := map[geom.Vec3]bool{}
	for _, pl := range got {
		if pl.Part.ID != "32523" {
			continue
		}
		origins[pl.Origin] = true
		if !pl.Origin.OnLattice(geom.HalfStud) {
			t.Errorf("origin %+v is off the half-stud lattice", pl.Origin)
		}
	}
	if len(origins) < 3 {
		t.Errorf("got %d distinct beam origins, want at least 3 (one per hole)", len(origins))
	}
	// Placed so its first hole sits on the point, the beam starts at the point.
	if !origins[point] {
		t.Errorf("expected an origin exactly at the query point; got %v", origins)
	}
}

// A hole along +Y is the same hole as along -Y, so querying either direction
// must return the same placements.
func TestLookupIsSignFree(t *testing.T) {
	c := load(t)
	c.BuildIndex(1)
	plus := c.Lookup(geom.Vec3{}, geom.Vec3{Y: 1}, Female, Round, nil)
	minus := c.Lookup(geom.Vec3{}, geom.Vec3{Y: -1}, Female, Round, nil)
	if len(plus) != len(minus) || len(plus) == 0 {
		t.Fatalf("+Y gave %d placements, -Y gave %d", len(plus), len(minus))
	}
}

func TestLookupSeparatesGenderAndKind(t *testing.T) {
	c := load(t)
	c.BuildIndex(1)
	for _, pl := range c.Lookup(geom.Vec3{}, geom.Vec3{X: 1}, Male, Round, nil) {
		if pl.Part.ID != "3673" {
			t.Errorf("male round query returned %s", pl.Part.ID)
		}
	}
	// Only the gear has a cross hole, and only along Z.
	for _, pl := range c.Lookup(geom.Vec3{}, geom.Vec3{Z: 1}, Female, Cross, nil) {
		if pl.Part.ID != "32270" {
			t.Errorf("cross query returned %s", pl.Part.ID)
		}
	}
}

// Lookup takes a destination slice so the solver can reuse one buffer across
// millions of queries; it must truncate rather than append to what is there.
func TestLookupReusesDestination(t *testing.T) {
	c := load(t)
	c.BuildIndex(1)
	buf := make([]Placement, 0, 64)
	first := len(c.Lookup(geom.Vec3{}, geom.Vec3{Y: 1}, Female, Round, buf))
	second := len(c.Lookup(geom.Vec3{}, geom.Vec3{Y: 1}, Female, Round, buf))
	if first != second {
		t.Errorf("reusing the buffer changed the result: %d then %d", first, second)
	}
}

func TestLookupOffLatticeYieldsNothing(t *testing.T) {
	c := load(t)
	c.BuildIndex(1)
	// 3 LDU is neither a stud nor a half stud, so no beam can reach it.
	if got := c.Lookup(geom.Vec3{X: 3}, geom.Vec3{Y: 1}, Female, Round, nil); len(got) != 0 {
		t.Errorf("got %d placements for an off-lattice point, want 0", len(got))
	}
}

// The extractor and the engine meet at exactly one file, and they describe it
// differently in memory: the Python side keys a dict by part id and calls the
// field `part`, while this side decodes an array whose field is `id`.
// build.to_records is the conversion, and testdata/catalog.json is its output
// for a fixed sample. tests/test_build.py regenerates and compares the same
// fixture, so a format change on either side breaks one of the two suites.
func TestLoadsTheFixtureTheExtractorWrites(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "catalog.json"))
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	c, err := Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(c.Parts))
	}

	beam, ok := c.Parts["32523"]
	if !ok {
		t.Fatal(`no part "32523": the extractor writes the id under "id"`)
	}
	if beam.Title != "Technic Beam 3" {
		t.Errorf("title = %q", beam.Title)
	}
	if beam.Tier != 1 {
		t.Errorf("tier = %d, want 1", beam.Tier)
	}
	if len(beam.Ports) != 2 {
		t.Fatalf("got %d ports, want 2", len(beam.Ports))
	}
	// Holes land as female, pins as male, and the axis is normalized.
	for _, p := range beam.Ports {
		if p.Gender != Female || p.Kind != Round {
			t.Errorf("port = %+v, want a female round hole", p)
		}
		if p.Axis != (geom.Vec3{Y: 1}) {
			t.Errorf("axis = %+v, want {0 1 0}", p.Axis)
		}
	}
	if got := c.Parts["3673"].Ports[0].Gender; got != Male {
		t.Errorf("the pin loaded as gender %v, want male", got)
	}
	if got := c.Parts["3001"].Tier; got != 3 {
		t.Errorf("plain brick tier = %d, want 3", got)
	}

	// And the whole point: it indexes and answers a query.
	c.BuildIndex(1)
	placements := c.Lookup(geom.Vec3{X: 40}, geom.Vec3{Y: 1}, Female, Round, nil)
	if len(placements) == 0 {
		t.Error("no placements from the fixture catalog")
	}
	for _, pl := range placements {
		if pl.Part.ID == "3001" {
			t.Error("tier 3 brick surfaced at maxTier 1")
		}
	}
}
