// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package ldraw

import (
	"errors"
	"math"
	"path/filepath"
	"testing"

	"brickmesh/internal/geom"
)

// Offline against the fixtures the Python suite uses, so a miss shows up as a
// missing fixture rather than as a download.
func fixtures(t *testing.T) *Library {
	t.Helper()
	l := New(filepath.Join("..", "..", "tests", "fixtures", "ldraw"))
	l.Offline = true
	return l
}

func TestTitleIsTheFirstPlainComment(t *testing.T) {
	l := fixtures(t)
	for name, want := range map[string]string{
		"fixcube": "Test Cube 40 LDU",
		"fixbeam": "Test Beam 5 x 1 x 1",
	} {
		g, err := l.Geometry(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if g.Title != want {
			t.Errorf("%s title = %q, want %q", name, g.Title, want)
		}
	}
}

func TestNameGetsTheSuffixAndIsLowercased(t *testing.T) {
	g, err := fixtures(t).Geometry("FixCube")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "fixcube.dat" {
		t.Errorf("name = %q", g.Name)
	}
}

func TestGeometryIsCached(t *testing.T) {
	l := fixtures(t)
	a, err := l.Geometry("fixcube")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := l.Geometry("fixcube")
	if a != b {
		t.Error("expected the same resolved geometry back")
	}
}

func TestBBoxAndSize(t *testing.T) {
	g, err := fixtures(t).Geometry("fixbeam")
	if err != nil {
		t.Fatal(err)
	}
	lo, hi := g.BBox()
	if lo != (geom.Vec3{X: -50, Y: -10, Z: -10}) {
		t.Errorf("lo = %+v", lo)
	}
	if hi != (geom.Vec3{X: 50, Y: 10, Z: 10}) {
		t.Errorf("hi = %+v", hi)
	}
	if g.Size() != (geom.Vec3{X: 100, Y: 20, Z: 20}) {
		t.Errorf("size = %+v", g.Size())
	}
}

func TestQuadsBecomeTwoTriangles(t *testing.T) {
	g, err := fixtures(t).Geometry("fixcube")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Tris) != 12 {
		t.Errorf("got %d triangles, want 12", len(g.Tris))
	}
}

func TestUnknownPartIsNotFound(t *testing.T) {
	_, err := fixtures(t).Geometry("nosuchpart")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// A gear is one tooth primitive instantiated N times at N matrices, so
// collapsing repeated references would shrink the part to a sliver.
func TestRepeatedSubfilesAreNotDeduplicated(t *testing.T) {
	l := fixtures(t)
	pair, err := l.Geometry("fixpair")
	if err != nil {
		t.Fatal(err)
	}
	if pair.Size() != (geom.Vec3{X: 120, Y: 40, Z: 40}) {
		t.Fatalf("size = %+v, want 120 wide: the second cube went missing", pair.Size())
	}
	cube, _ := l.Geometry("fixcube")
	if len(pair.Tris) != 2*len(cube.Tris) {
		t.Errorf("got %d triangles, want twice the cube's %d",
			len(pair.Tris), len(cube.Tris))
	}
}

func TestSubfileTransformsAreApplied(t *testing.T) {
	g, err := fixtures(t).Geometry("fixpair")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[float64]bool{}
	for _, v := range g.Verts {
		seen[math.Round(v.X)] = true
	}
	for _, want := range []float64{-60, -20, 20, 60} {
		if !seen[want] {
			t.Errorf("no vertex at X=%v; got %v", want, seen)
		}
	}
}

func TestThinAxisIsTheShortestDimension(t *testing.T) {
	l := fixtures(t)
	for name, want := range map[string]int{"fixdisc": 2, "fixbeam": 1} {
		g, err := l.Geometry(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := g.ThinAxis(); got != want {
			t.Errorf("%s thin axis = %d, want %d", name, got, want)
		}
	}
}

func TestOfflineRefusesToFetch(t *testing.T) {
	l := New(t.TempDir())
	l.Offline = true
	if _, err := l.Fetch("3001.dat"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
