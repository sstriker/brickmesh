// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"os"
	"testing"

	"brickmesh/internal/ldraw"
)

// Real parts, because the sharing this format depends on is a property of how
// LDraw parts are built — primitives meeting edge to edge — and a synthetic
// square proves nothing about how much of it there is.
func TestRealPartsIndexAndComeBack(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")

	// The parts the engine actually places: the beam inventory and the gears.
	ids := []string{
		"32523.dat", "32316.dat", "32524.dat", "40490.dat", "32525.dat",
		"3647.dat", "32270.dat", "4019.dat", "32269.dat", "3648b.dat",
		"32498.dat", "3649.dat", "6539.dat", "6538a.dat",
	}

	var meshes []Mesh
	var wantTris []int
	for _, id := range ids {
		g, err := lib.Geometry(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		m := IndexTriangles(g.Tris)
		if m.Triangles() != len(g.Tris) {
			t.Errorf("%s: indexed to %d triangles from %d",
				id, m.Triangles(), len(g.Tris))
		}
		// Sharing has to actually happen, or the format is doing nothing.
		if vertices := len(m.Positions) / 3; vertices >= len(g.Tris)*3 {
			t.Errorf("%s: %d vertices for %d triangles — no corners shared",
				id, vertices, len(g.Tris))
		}
		meshes = append(meshes, m)
		wantTris = append(wantTris, len(g.Tris))
	}

	raw, err := WriteMeshes(meshes)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := ReadMeshIndex(raw[:IndexSize(len(meshes))])
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		off, n, err := idx.Range(i)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ReadMesh(raw[off : off+n])
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if got.Triangles() != wantTris[i] {
			t.Errorf("%s came back with %d triangles, want %d",
				id, got.Triangles(), wantTris[i])
		}
		// And the geometry itself, corner by corner, since a wrong index is a
		// wrong shape and nothing else would notice.
		g, _ := lib.Geometry(id)
		for k, tri := range g.Tris {
			for c, want := range tri {
				at := got.Indices[k*3+c] * 3
				if got.Positions[at] != float32(want.X) ||
					got.Positions[at+1] != float32(want.Y) ||
					got.Positions[at+2] != float32(want.Z) {
					t.Fatalf("%s triangle %d corner %d came back at the wrong place",
						id, k, c)
				}
			}
		}
	}
}
