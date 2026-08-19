// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"math"
	"os"
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/ldraw"
)

// The point of the whole format: a part read out of the published files is the
// same part the engine reads out of the library. If that is not true then a
// browser draws and measures something else, and nothing downstream would
// notice.
func TestAPartFromTheBlobIsThePartFromTheLibrary(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")

	ids := []string{
		"32523.dat", "32316.dat", "3647.dat", "3648b.dat", "4019.dat",
		"6539.dat", "6538a.dat", "6542a.dat", "3707.dat",
	}
	catalog := Catalog{}
	var meshes []Mesh
	for _, id := range ids {
		g, err := lib.Geometry(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		catalog.Parts = append(catalog.Parts, Part{ID: id, Title: g.Title, Tier: 1})
		meshes = append(meshes, IndexTriangles(g.Tris))
	}

	// Written in the catalogue's own order, as the generator does.
	sorted := Sorted(catalog)
	order := map[string]int{}
	for i, p := range sorted.Parts {
		order[p.ID] = i
	}
	inOrder := make([]Mesh, len(meshes))
	for i, id := range ids {
		inOrder[order[id]] = meshes[i]
	}
	rawMeshes, err := WriteMeshes(inOrder)
	if err != nil {
		t.Fatal(err)
	}
	rawCatalog, err := WriteCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	readCatalog, err := ReadCatalog(rawCatalog)
	if err != nil {
		t.Fatal(err)
	}

	shapes, err := FromBytes(readCatalog, rawMeshes)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		want, err := lib.Geometry(id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := shapes.Geometry(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if len(got.Tris) != len(want.Tris) {
			t.Errorf("%s: %d triangles from the blob, %d from the library",
				id, len(got.Tris), len(want.Tris))
			continue
		}
		// Corner by corner. float32 is what the blob stores, so the comparison
		// is to that precision and not to the double the parser produced.
		for i := range want.Tris {
			for c := 0; c < 3; c++ {
				if !sameCorner(got.Tris[i][c], want.Tris[i][c]) {
					t.Fatalf("%s triangle %d corner %d: %+v from the blob, %+v "+
						"from the library", id, i, c, got.Tris[i][c], want.Tris[i][c])
				}
			}
		}
		if got.Title != want.Title {
			t.Errorf("%s: title %q from the blob, %q from the library",
				id, got.Title, want.Title)
		}
	}
}

func sameCorner(a, b geom.Vec3) bool {
	return float32(a.X) == float32(b.X) &&
		float32(a.Y) == float32(b.Y) &&
		float32(a.Z) == float32(b.Z)
}

// A part the blob has no geometry for has to say so. The generator writes empty
// entries to keep the two files aligned, and silently drawing nothing would be
// the wrong answer twice over.
func TestAPartWithNoGeometrySaysSo(t *testing.T) {
	catalog := Catalog{Parts: []Part{{ID: "empty.dat", Tier: 1}}}
	raw, err := WriteCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	read, err := ReadCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	meshes, err := WriteMeshes([]Mesh{{}})
	if err != nil {
		t.Fatal(err)
	}
	shapes, err := FromBytes(read, meshes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shapes.Geometry("empty.dat"); err == nil {
		t.Error("a part with no triangles should be an error, not an empty shape")
	}
	if _, err := shapes.Geometry("nosuch.dat"); err == nil {
		t.Error("a part that is not in the catalogue should be an error")
	}
}

// The two files are addressed by one index, so a mismatched pair is refused
// rather than silently drawing one part's shape for another's name.
func TestAMismatchedPairIsRefused(t *testing.T) {
	catalog := Catalog{Parts: []Part{{ID: "a.dat"}, {ID: "b.dat"}}}
	raw, _ := WriteCatalog(catalog)
	read, _ := ReadCatalog(raw)
	meshes, _ := WriteMeshes([]Mesh{{}}) // one mesh for two parts
	if _, err := FromBytes(read, meshes); err == nil {
		t.Error("a catalogue and a mesh file of different lengths cannot be paired")
	}
}

// The ports come from the catalogue, which is what lets the structural search
// run with no shadow library.
func TestTheAxisComesFromThePorts(t *testing.T) {
	catalog := Catalog{Parts: []Part{{
		ID: "beam.dat", Tier: 1,
		Ports: []Port{{X: -20, AZ: 1}, {X: 20, AZ: 1}},
	}, {ID: "blank.dat", Tier: 1}}}
	raw, _ := WriteCatalog(catalog)
	read, _ := ReadCatalog(raw)
	shapes, err := FromBytes(read, mustMeshes(len(read.Parts)))
	if err != nil {
		t.Fatal(err)
	}

	axis, source, ok := shapes.RotationAxis("beam.dat")
	if !ok {
		t.Fatal("a part with ports has an axis")
	}
	if math.Abs(axis.Z-1) > 1e-9 {
		t.Errorf("axis %+v, want the port's own direction", axis)
	}
	if source != "catalog.bin" {
		t.Errorf("source %q; a reader should be able to see where it came from", source)
	}
	if _, _, ok := shapes.RotationAxis("blank.dat"); ok {
		t.Error("a part with no ports has no axis, and guessing one would be worse")
	}
}

func mustMeshes(n int) []byte {
	raw, err := WriteMeshes(make([]Mesh, n))
	if err != nil {
		panic(err)
	}
	return raw
}
