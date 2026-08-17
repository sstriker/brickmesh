// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package voxel

import (
	"path/filepath"
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/ldraw"
)

// Mirrors tests/test_voxel.py, against the same synthetic parts, offline.
func rasterizer(t *testing.T) *Rasterizer {
	t.Helper()
	lib := ldraw.New(filepath.Join("..", "..", "tests", "fixtures", "ldraw"))
	lib.Offline = true
	return NewRasterizer(lib)
}

func cellSet(t *testing.T, r *Rasterizer, part string) map[geom.Cell]bool {
	t.Helper()
	cells, err := r.Voxels(part, 0)
	if err != nil {
		t.Fatalf("%s: %v", part, err)
	}
	out := make(map[geom.Cell]bool, len(cells))
	for _, c := range cells {
		out[c] = true
	}
	return out
}

func TestASolidBoxIsHollowInTheGrid(t *testing.T) {
	// Surface, not volume: the middle of a solid cube is not occupied.
	cells := cellSet(t, rasterizer(t), "fixcube")
	if cells[geom.Cell{}] {
		t.Error("the center of a solid cube should not be occupied")
	}
	// ... while its faces are. The cube spans -20..20, so cell 4 is the +X face.
	if !cells[geom.Cell{X: 4}] {
		t.Error("the +X face should be occupied")
	}
}

func TestTheGridSpansThePart(t *testing.T) {
	cells, err := rasterizer(t).Voxels("fixcube", 0)
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := cells[0], cells[0]
	for _, c := range cells {
		lo = geom.Cell{X: min(lo.X, c.X), Y: min(lo.Y, c.Y), Z: min(lo.Z, c.Z)}
		hi = geom.Cell{X: max(hi.X, c.X), Y: max(hi.Y, c.Y), Z: max(hi.Z, c.Z)}
	}
	if lo != (geom.Cell{X: -4, Y: -4, Z: -4}) || hi != (geom.Cell{X: 4, Y: 4, Z: 4}) {
		t.Errorf("bounds %+v..%+v, want -4..4", lo, hi)
	}
}

// The four cells inside the 12 LDU bore must be empty. If the rasterizer filled
// volumes, every hole would silt up and the search would never find a bearing.
func TestTheBoreStaysOpen(t *testing.T) {
	cells := cellSet(t, rasterizer(t), "fixtube")
	for _, c := range []geom.Cell{{}, {Y: -1}, {Z: -1}, {Y: -1, Z: -1}} {
		if cells[c] {
			t.Errorf("the bore is blocked at %+v", c)
		}
	}
}

func TestTheBoreWallsAreOccupied(t *testing.T) {
	cells := cellSet(t, rasterizer(t), "fixtube")
	// The bore surface sits at +/-6 LDU, which lands in cells 1 and -2.
	if !cells[geom.Cell{Y: 1}] || !cells[geom.Cell{Y: -2}] {
		t.Error("the bore walls should be occupied")
	}
	if !cells[geom.Cell{Y: 4, Z: 4}] {
		t.Error("the outer skin should be occupied")
	}
}

// The Python rasterizer produces these exact counts. Nothing forces the two to
// agree — no file passes between them — but a silent divergence in the coarse
// collision filter would be worth knowing about.
func TestCellCountsMatchThePythonRasterizer(t *testing.T) {
	r := rasterizer(t)
	for part, want := range map[string]int{"fixcube": 378, "fixtube": 606} {
		cells, err := r.Voxels(part, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(cells) != want {
			t.Errorf("%s: %d cells, python gets %d", part, len(cells), want)
		}
	}
}

func TestPitchIsFourCellsToTheStud(t *testing.T) {
	if Stud/Pitch != 4 {
		t.Errorf("stud/pitch = %v, want 4", Stud/Pitch)
	}
}

func TestSymmetryReducesTheOrientationsToTry(t *testing.T) {
	r := rasterizer(t)
	for _, part := range []string{"fixcube", "fixbeam", "fixdisc", "fixtube"} {
		keep, err := r.DistinctRotations(part)
		if err != nil {
			t.Fatal(err)
		}
		if len(keep) < 1 || len(keep) >= len(geom.Rotations) {
			t.Errorf("%s kept %d of 24", part, len(keep))
		}
		seen := map[int]bool{}
		for _, i := range keep {
			if seen[i] {
				t.Errorf("%s: duplicate rotation %d", part, i)
			}
			seen[i] = true
		}
	}
}

func TestALongPartHasFewerDistinctOrientationsThanAFlatOne(t *testing.T) {
	r := rasterizer(t)
	beam, _ := r.DistinctRotations("fixbeam")
	disc, _ := r.DistinctRotations("fixdisc")
	if len(beam) >= len(disc) {
		t.Errorf("beam kept %d, disc kept %d; a beam is the more symmetric",
			len(beam), len(disc))
	}
}

func TestGridAddCollideAndRemove(t *testing.T) {
	r := rasterizer(t)
	cells, err := r.Voxels("fixcube", 0)
	if err != nil {
		t.Fatal(err)
	}
	g := NewGrid(120)

	flat := g.Index(cells, geom.Vec3{}, nil)
	if len(flat) == 0 {
		t.Fatal("no cells landed in the grid")
	}
	if g.Collides(flat) {
		t.Error("an empty grid collides with nothing")
	}
	g.Add(flat)
	if g.Filled() == 0 {
		t.Error("nothing was filled")
	}
	if !g.Collides(flat) {
		t.Error("a part collides with itself once placed")
	}

	// Far enough away to clear it.
	away := g.Index(cells, geom.Vec3{X: 100}, nil)
	if g.Collides(away) {
		t.Error("100 LDU away should be clear")
	}

	g.Remove(flat)
	if g.Filled() != 0 {
		t.Errorf("%d cells left after removing", g.Filled())
	}
	if g.Collides(flat) {
		t.Error("removed and still colliding")
	}
}

func TestIndexReusesItsDestination(t *testing.T) {
	r := rasterizer(t)
	cells, _ := r.Voxels("fixcube", 0)
	g := NewGrid(120)
	buf := make([]uint32, 0, 512)
	first := len(g.Index(cells, geom.Vec3{}, buf))
	second := len(g.Index(cells, geom.Vec3{}, buf))
	if first != second {
		t.Errorf("reusing the buffer changed the result: %d then %d", first, second)
	}
}

func TestCellsOutsideTheGridAreDropped(t *testing.T) {
	r := rasterizer(t)
	cells, _ := r.Voxels("fixcube", 0)
	g := NewGrid(30) // smaller than the placement offset
	if got := g.Index(cells, geom.Vec3{X: 1000}, nil); len(got) != 0 {
		t.Errorf("got %d cells, want none", len(got))
	}
}

// The point of all of it: a thin part laid through the tube's bore must not
// register as a collision, or no bearing is ever found.
func TestAnAxleThroughTheBoreDoesNotCollide(t *testing.T) {
	r := rasterizer(t)
	tube, err := r.Voxels("fixtube", 0)
	if err != nil {
		t.Fatal(err)
	}
	cube, _ := r.Voxels("fixcube", 0)

	g := NewGrid(200)
	g.Add(g.Index(tube, geom.Vec3{}, nil))

	// A cube is 40 LDU wide and cannot fit through a 12 LDU bore.
	if !g.Collides(g.Index(cube, geom.Vec3{}, nil)) {
		t.Error("a 40 LDU cube should not fit through a 12 LDU bore")
	}
	// But the bore itself is free.
	var bore Cells
	for x := int32(-8); x <= 8; x++ {
		bore = append(bore, geom.Cell{X: x})
	}
	if g.Collides(g.Index(bore, geom.Vec3{}, nil)) {
		t.Error("the bore should be clear along its length")
	}
}

func TestClearEmptiesTheGrid(t *testing.T) {
	r := rasterizer(t)
	cells, _ := r.Voxels("fixcube", 0)
	g := NewGrid(120)
	g.Add(g.Index(cells, geom.Vec3{}, nil))
	g.Clear()
	if g.Filled() != 0 {
		t.Errorf("%d cells left after Clear", g.Filled())
	}
}

func TestLatticePositionsAreOnTheStudGrid(t *testing.T) {
	pts := LatticePositions(1)
	if len(pts) != 27 {
		t.Fatalf("got %d positions, want 27", len(pts))
	}
	for _, p := range pts {
		for _, v := range [3]float64{p.X, p.Y, p.Z} {
			if v != 0 && v != Stud && v != -Stud {
				t.Errorf("%v is off the stud grid", p)
			}
		}
	}
}

func TestVoxelsRejectsAnImpossibleRotation(t *testing.T) {
	if _, err := rasterizer(t).Voxels("fixcube", 99); err == nil {
		t.Error("expected an error for a rotation outside the 24")
	}
}
