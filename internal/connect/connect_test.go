// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package connect

import (
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/catalog"
	"brickmesh/internal/geom"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/voxel"
)

// Two beams whose hole planes are 40 LDU apart, and a slim pin long enough to
// bridge them. The part ids match the fixture meshes so the rasterizer can
// answer for them; offline throughout.
const fixtureCatalog = `[
  {"id": "fixbeam", "title": "Technic Beam 5", "tier": 1,
   "holes": [[-40,0,0, 0,1,0, 0], [-20,0,0, 0,1,0, 0], [0,0,0, 0,1,0, 0],
             [20,0,0, 0,1,0, 0], [40,0,0, 0,1,0, 0]],
   "pins": []},
  {"id": "fixpin", "title": "Technic Pin", "tier": 1,
   "holes": [],
   "pins": [[0,-20,0, 0,1,0, 0], [0,20,0, 0,1,0, 0]]}
]`

func setup(t *testing.T) (*catalog.Catalog, *voxel.Rasterizer) {
	t.Helper()
	cat, err := catalog.Load(strings.NewReader(fixtureCatalog))
	if err != nil {
		t.Fatal(err)
	}
	cat.BuildIndex(3)
	lib := ldraw.New(filepath.Join("..", "..", "tests", "fixtures", "ldraw"))
	lib.Offline = true
	return cat, voxel.NewRasterizer(lib)
}

func place(t *testing.T, cat *catalog.Catalog, id string, origin geom.Vec3) catalog.Placement {
	t.Helper()
	p, ok := cat.Parts[id]
	if !ok {
		t.Fatalf("no part %q", id)
	}
	return catalog.Placement{Part: p, Rot: 0, Origin: origin}
}

func TestPortsAreTransformedIntoTheWorld(t *testing.T) {
	cat, _ := setup(t)
	ports := PortsOf(place(t, cat, "fixbeam", geom.Vec3{X: 100}))
	if len(ports) != 5 {
		t.Fatalf("got %d ports, want 5", len(ports))
	}
	seen := map[float64]bool{}
	for _, p := range ports {
		seen[p.Pos.X] = true
		if p.Axis != (geom.Vec3{Y: 1}) {
			t.Errorf("axis = %+v, want +Y", p.Axis)
		}
	}
	for _, want := range []float64{60, 80, 100, 120, 140} {
		if !seen[want] {
			t.Errorf("no port at X=%v", want)
		}
	}
}

// Holes have no direction, so a port along +Y is the same port as along -Y.
func TestPortKeyIsSignFree(t *testing.T) {
	a := keyOf(geom.Vec3{}, geom.Vec3{Y: 1})
	b := keyOf(geom.Vec3{}, geom.Vec3{Y: -1})
	if a != b {
		t.Errorf("%v != %v", a, b)
	}
	if keyOf(geom.Vec3{}, geom.Vec3{X: 1}) == a {
		t.Error("+X and +Y should differ")
	}
}

// Pieces that already share a port need nothing added.
func TestAlreadyTouchingPiecesNeedNoChain(t *testing.T) {
	cat, rast := setup(t)
	a := []catalog.Placement{place(t, cat, "fixbeam", geom.Vec3{})}
	b := []catalog.Placement{place(t, cat, "fixbeam", geom.Vec3{X: 80})}
	// A's port at +40 is B's port at -40 from its origin: the same point.
	chain, err := Connect(cat, rast, a, b, Options{MaxParts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 0 {
		t.Errorf("got a chain of %d for pieces that already meet", len(chain))
	}
}

// The real case: two beams on hole planes 40 apart, bridged by one pin.
func TestOnePinBridgesTwoBeams(t *testing.T) {
	cat, rast := setup(t)
	a := []catalog.Placement{place(t, cat, "fixbeam", geom.Vec3{})}
	b := []catalog.Placement{place(t, cat, "fixbeam", geom.Vec3{Y: 40})}

	chain, err := Connect(cat, rast, a, b, Options{MaxParts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 1 {
		t.Fatalf("got a chain of %d, want 1 pin", len(chain))
	}
	if chain[0].Part.ID != "fixpin" {
		t.Errorf("bridged with %q, want the pin", chain[0].Part.ID)
	}

	// And it really touches both: one port on each beam's hole plane.
	var low, high bool
	for _, p := range PortsOf(chain[0]) {
		if p.Pos.Y == 0 {
			low = true
		}
		if p.Pos.Y == 40 {
			high = true
		}
	}
	if !low || !high {
		t.Errorf("the pin does not reach both planes: %+v", PortsOf(chain[0]))
	}
}

func TestNoChainWhenNothingCanReach(t *testing.T) {
	cat, rast := setup(t)
	a := []catalog.Placement{place(t, cat, "fixbeam", geom.Vec3{})}
	// Far out of reach of any part in this catalog.
	b := []catalog.Placement{place(t, cat, "fixbeam", geom.Vec3{Y: 4000})}
	chain, err := Connect(cat, rast, a, b, Options{MaxParts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if chain != nil {
		t.Errorf("got a chain of %d across 4000 LDU", len(chain))
	}
}

func TestAPieceWithoutPortsCannotBeJoined(t *testing.T) {
	cat, rast := setup(t)
	a := []catalog.Placement{place(t, cat, "fixbeam", geom.Vec3{})}
	chain, err := Connect(cat, rast, a, nil, Options{MaxParts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if chain != nil {
		t.Errorf("got %v, want nothing", chain)
	}
}

// Only ports sharing an axis can ever be connected, so the heuristic has to
// prefer an aligned candidate over one that merely sits nearby.
func TestHeuristicPrefersAlignedPorts(t *testing.T) {
	cat, rast := setup(t)
	s := &search{
		cat: cat, rast: rast,
		targets: []Port{{Pos: geom.Vec3{Y: 40}, Axis: geom.Vec3{Y: 1}}},
	}
	aligned := place(t, cat, "fixpin", geom.Vec3{Y: 20}) // ports along Y
	across := place(t, cat, "fixpin", geom.Vec3{Y: 20})  // same, rotated below
	across.Rot = rotationMapping(t, geom.Vec3{Y: 1}, geom.Vec3{X: 1})

	if s.heuristic(aligned) >= s.heuristic(across) {
		t.Errorf("aligned scored %v, crosswise %v; aligned should be cheaper",
			s.heuristic(aligned), s.heuristic(across))
	}
}

// rotationMapping finds a lattice rotation taking from to to.
func rotationMapping(t *testing.T, from, to geom.Vec3) uint8 {
	t.Helper()
	for i, m := range geom.Rotations {
		if m.Apply(from).Sub(to).Len() < 1e-9 {
			return uint8(i)
		}
	}
	t.Fatalf("no rotation maps %v to %v", from, to)
	return 0
}

func TestFrontierGrowsWithEachPart(t *testing.T) {
	cat, _ := setup(t)
	start := []portKey{keyOf(geom.Vec3{}, geom.Vec3{Y: 1})}
	grown := mergeFrontier(start, place(t, cat, "fixpin", geom.Vec3{Y: 20}))
	if len(grown) <= len(start) {
		t.Errorf("frontier did not grow: %d then %d", len(start), len(grown))
	}
	// And it stays sorted, so the signature is stable.
	for i := 1; i < len(grown); i++ {
		if lessKey(grown[i], grown[i-1]) {
			t.Error("frontier is not sorted")
		}
	}
}

func TestSignatureIgnoresOrder(t *testing.T) {
	a := keyOf(geom.Vec3{X: 20}, geom.Vec3{Y: 1})
	b := keyOf(geom.Vec3{X: 40}, geom.Vec3{Y: 1})
	if signature(sortedKeys([]portKey{a, b})) != signature(sortedKeys([]portKey{b, a})) {
		t.Error("the same reachable set should have one signature")
	}
}
