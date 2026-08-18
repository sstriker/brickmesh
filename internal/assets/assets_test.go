// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"brickmesh/internal/extract"
	"brickmesh/internal/geom"
)

func sample() Catalog {
	return Catalog{Parts: []Part{
		// Deliberately out of order, and out of tier order, so the writer's
		// sorting is doing something rather than being handed the answer.
		{ID: "32316.dat", Title: "Technic Beam  5", Tier: 1, Ports: []Port{
			{X: -40, AZ: 1}, {X: -20, AZ: 1}, {AZ: 1}, {X: 20, AZ: 1},
			{X: 40, AZ: 1},
		}},
		{ID: "9999.dat", Title: "Something Obscure", Tier: 3},
		{ID: "3648b.dat", Title: "Technic Gear 24 Tooth with Single Axle Hole",
			Tier: 1, Ports: []Port{{AZ: 1, Kind: PortCross}}},
		{ID: "4519.dat", Title: "Technic Axle  3", Tier: 2, Ports: []Port{
			{AX: 1, Kind: PortMale | PortCross},
		}},
	}}
}

func TestACatalogSurvivesTheRoundTrip(t *testing.T) {
	raw, err := WriteCatalog(sample())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != 4 {
		t.Fatalf("read %d parts, wrote 4", len(got.Parts))
	}
	byID := map[string]Part{}
	for _, p := range got.Parts {
		byID[p.ID] = p
	}
	for _, want := range sample().Parts {
		back, ok := byID[want.ID]
		if !ok {
			t.Errorf("%s did not come back", want.ID)
			continue
		}
		if back.Title != want.Title || back.Tier != want.Tier {
			t.Errorf("%s came back as %q tier %d, want %q tier %d",
				want.ID, back.Title, back.Tier, want.Title, want.Tier)
		}
		if !reflect.DeepEqual(back.Ports, want.Ports) {
			t.Errorf("%s ports came back as %+v, want %+v",
				want.ID, back.Ports, want.Ports)
		}
	}
}

// The whole reason for sorting: a caller who wants only the common parts takes
// one range off the front of the file. If tier 1 were scattered, that would be
// a gather instead, and the format would be pointless.
func TestTheCommonPartsAreContiguousAtTheFront(t *testing.T) {
	raw, err := WriteCatalog(sample())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}

	end := got.TierEnd(1)
	if end != 2 {
		t.Fatalf("tier 1 ends at %d, want 2", end)
	}
	for i, p := range got.Parts {
		if i < end && p.Tier != 1 {
			t.Errorf("part %d is tier %d, inside the tier 1 range", i, p.Tier)
		}
		if i >= end && p.Tier == 1 {
			t.Errorf("part %d is tier 1, outside the tier 1 range", i)
		}
	}
	// And the tier boundary in the header must agree, since that is what a
	// caller reads before it has the parts.
	if fromHeader := binary.LittleEndian.Uint32(raw[16:]); int(fromHeader) != end {
		t.Errorf("the header says tier 1 ends at %d, the parts say %d",
			fromHeader, end)
	}
}

// Same input, same bytes. A generated file that changes without its input
// changing cannot be reviewed, and shows up as noise in every diff.
func TestTheSameCatalogWritesTheSameBytes(t *testing.T) {
	first, err := WriteCatalog(sample())
	if err != nil {
		t.Fatal(err)
	}
	// Shuffled: the writer sorts, so the order it is handed must not matter.
	shuffled := sample()
	shuffled.Parts[0], shuffled.Parts[3] = shuffled.Parts[3], shuffled.Parts[0]
	second, err := WriteCatalog(shuffled)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Error("the same catalog wrote different bytes depending on the order " +
			"it was handed")
	}
}

// This file arrives over a network. A truncated download has to be an error,
// not a panic in a worker nobody can see.
func TestATruncatedCatalogIsAnError(t *testing.T) {
	raw, err := WriteCatalog(sample())
	if err != nil {
		t.Fatal(err)
	}
	for _, cut := range []int{0, 4, 16, catalogHeader, catalogHeader + 4,
		len(raw) / 2, len(raw) - 1} {
		if _, err := ReadCatalog(raw[:cut]); err == nil {
			t.Errorf("a catalog cut to %d of %d bytes was accepted", cut, len(raw))
		}
	}
	// And a file that is not one at all.
	if _, err := ReadCatalog([]byte("this is not a catalog, it is a sentence")); err == nil {
		t.Error("arbitrary bytes were accepted as a catalog")
	}
	// A version from the future is refused rather than misread.
	future := append([]byte(nil), raw...)
	binary.LittleEndian.PutUint16(future[4:], CatalogVersion+1)
	if _, err := ReadCatalog(future); err == nil {
		t.Error("a catalog from a later version was accepted")
	}
}

func sampleMeshes() []Mesh {
	return []Mesh{
		{Positions: []float32{0, 0, 0, 1, 0, 0, 0, 1, 0}, Indices: []uint32{0, 1, 2}},
		{}, // a part with no geometry is ordinary and must not break the table
		{
			Positions: []float32{0, 0, 0, 2, 0, 0, 0, 2, 0, 0, 0, 2},
			Indices:   []uint32{0, 1, 2, 0, 1, 3},
		},
	}
}

func TestAMeshSurvivesTheRoundTrip(t *testing.T) {
	raw, err := WriteMeshes(sampleMeshes())
	if err != nil {
		t.Fatal(err)
	}
	idx, err := ReadMeshIndex(raw)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Count != 3 {
		t.Fatalf("index has %d parts, wrote 3", idx.Count)
	}
	for i, want := range sampleMeshes() {
		off, n, err := idx.Range(i)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ReadMesh(raw[off : off+n])
		if err != nil {
			t.Fatalf("mesh %d: %v", i, err)
		}
		if !reflect.DeepEqual(got.Positions, want.Positions) &&
			!(len(got.Positions) == 0 && len(want.Positions) == 0) {
			t.Errorf("mesh %d positions: %v, want %v", i, got.Positions, want.Positions)
		}
		if got.Triangles() != want.Triangles() {
			t.Errorf("mesh %d has %d triangles, want %d",
				i, got.Triangles(), want.Triangles())
		}
	}
}

// The point of the format: the index is a small prefix, and every mesh
// afterwards is a range whose bounds are already known. This is that fetch
// pattern, with the rest of the file withheld to prove it is not being read.
func TestAMeshCanBeReadFromItsRangeAlone(t *testing.T) {
	raw, err := WriteMeshes(sampleMeshes())
	if err != nil {
		t.Fatal(err)
	}

	// A client fetches only the index first.
	prefix := raw[:IndexSize(3)]
	idx, err := ReadMeshIndex(prefix)
	if err != nil {
		t.Fatalf("the index should be readable from its prefix alone: %v", err)
	}

	off, n, err := idx.Range(2)
	if err != nil {
		t.Fatal(err)
	}
	// Then exactly that range, and nothing else.
	got, err := ReadMesh(raw[off : off+n])
	if err != nil {
		t.Fatal(err)
	}
	if got.Triangles() != 2 {
		t.Errorf("read %d triangles from the range, want 2", got.Triangles())
	}
}

func TestASpanCoversSeveralPartsInOneRequest(t *testing.T) {
	raw, err := WriteMeshes(sampleMeshes())
	if err != nil {
		t.Fatal(err)
	}
	idx, err := ReadMeshIndex(raw)
	if err != nil {
		t.Fatal(err)
	}
	off, n, err := idx.Span([]int{0, 2})
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := idx.Range(0)
	lastOff, lastLen, _ := idx.Range(2)
	if off != first || off+n != lastOff+lastLen {
		t.Errorf("the span is %d..%d, want %d..%d",
			off, off+n, first, lastOff+lastLen)
	}
	if _, _, err := idx.Span(nil); err == nil {
		t.Error("a span of nothing should be an error")
	}
}

// A mesh whose indices point outside its own vertices would be written happily
// and only fail when something tried to draw it.
func TestAMeshThatRefersToVerticesItDoesNotHaveIsRefused(t *testing.T) {
	for _, c := range []struct {
		why  string
		mesh Mesh
	}{
		{"an index past the end", Mesh{
			Positions: []float32{0, 0, 0}, Indices: []uint32{0, 0, 7}}},
		{"positions that are not whole vertices", Mesh{
			Positions: []float32{0, 0}, Indices: nil}},
		{"indices that are not whole triangles", Mesh{
			Positions: []float32{0, 0, 0}, Indices: []uint32{0, 0}}},
	} {
		if _, err := WriteMeshes([]Mesh{c.mesh}); err == nil {
			t.Errorf("%s should have been refused", c.why)
		}
	}
}

func TestATruncatedMeshFileIsAnError(t *testing.T) {
	raw, err := WriteMeshes(sampleMeshes())
	if err != nil {
		t.Fatal(err)
	}
	for _, cut := range []int{0, 4, meshHeader - 1, meshHeader + 4} {
		if _, err := ReadMeshIndex(raw[:cut]); err == nil {
			t.Errorf("a mesh file cut to %d bytes was accepted", cut)
		}
	}
	if _, err := ReadMesh([]byte{1, 0, 0, 0, 3, 0, 0, 0}); err == nil {
		t.Error("a mesh whose payload is missing was accepted")
	}
}

// The two files are addressed by the same index. Nothing enforces that at the
// type level, so it is written down here.
func TestAPartHasTheSameIndexInBothFiles(t *testing.T) {
	cat := sample()
	rawCat, err := WriteCatalog(cat)
	if err != nil {
		t.Fatal(err)
	}
	read, err := ReadCatalog(rawCat)
	if err != nil {
		t.Fatal(err)
	}

	// Meshes built in the catalog's order, as the generator must do.
	meshes := make([]Mesh, len(read.Parts))
	for i := range read.Parts {
		meshes[i] = Mesh{
			Positions: []float32{float32(i), 0, 0, 0, 1, 0, 0, 0, 1},
			Indices:   []uint32{0, 1, 2},
		}
	}
	rawMesh, err := WriteMeshes(meshes)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := ReadMeshIndex(rawMesh)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Count != len(read.Parts) {
		t.Fatalf("%d meshes for %d parts", idx.Count, len(read.Parts))
	}
	for i := range read.Parts {
		off, n, err := idx.Range(i)
		if err != nil {
			t.Fatal(err)
		}
		m, err := ReadMesh(rawMesh[off : off+n])
		if err != nil {
			t.Fatal(err)
		}
		if m.Positions[0] != float32(i) {
			t.Errorf("part %d (%s) got mesh %v", i, read.Parts[i].ID, m.Positions[0])
		}
	}
}

func TestTheHeaderSaysWhatItIs(t *testing.T) {
	raw, err := WriteCatalog(sample())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw[:4]); got != catalogMagic {
		t.Errorf("catalog magic is %q, want %q", got, catalogMagic)
	}
	meshes, err := WriteMeshes(sampleMeshes())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(meshes[:4]); got != meshMagic {
		t.Errorf("mesh magic is %q, want %q", got, meshMagic)
	}
	// Reading one as the other has to fail rather than produce nonsense.
	if _, err := ReadCatalog(meshes); err == nil {
		t.Error("the mesh file was accepted as a catalog")
	}
	if _, err := ReadMeshIndex(raw); err == nil {
		t.Error("the catalog was accepted as a mesh file")
	}
}

func ExampleMeshIndex_Range() {
	raw, _ := WriteMeshes(sampleMeshes())
	idx, _ := ReadMeshIndex(raw[:IndexSize(3)])
	off, n, _ := idx.Range(0)
	fmt.Printf("Range: bytes=%d-%d\n", off, off+n-1)
	// Output: Range: bytes=56-111
}

// Sharing corners is most of the reason the mesh file is worth generating: a
// square made of two triangles has six corners and four vertices.
func TestSharedCornersBecomeOneVertex(t *testing.T) {
	square := [][3]geom.Vec3{
		{{X: 0}, {X: 1}, {X: 1, Y: 1}},
		{{X: 0}, {X: 1, Y: 1}, {Y: 1}},
	}
	got := IndexTriangles(square)
	if v := len(got.Positions) / 3; v != 4 {
		t.Errorf("got %d vertices for a square, want 4", v)
	}
	if got.Triangles() != 2 {
		t.Errorf("got %d triangles, want 2", got.Triangles())
	}
	// And the shape has to survive the sharing.
	for i, idx := range got.Indices {
		if int(idx)*3+2 >= len(got.Positions) {
			t.Fatalf("index %d of %d points outside the vertices", i, idx)
		}
	}
	for i, tri := range square {
		for j, want := range tri {
			at := got.Indices[i*3+j] * 3
			if got.Positions[at] != float32(want.X) ||
				got.Positions[at+1] != float32(want.Y) ||
				got.Positions[at+2] != float32(want.Z) {
				t.Errorf("triangle %d corner %d moved", i, j)
			}
		}
	}
}

// Corners that are close but not identical stay separate. Welding them would
// move geometry the collision tests are measured against.
func TestNearlyEqualCornersAreNotWelded(t *testing.T) {
	got := IndexTriangles([][3]geom.Vec3{
		{{X: 0}, {X: 1}, {Y: 1}},
		{{X: 1e-7}, {X: 1}, {Y: 1}},
	})
	if v := len(got.Positions) / 3; v != 4 {
		t.Errorf("got %d vertices, want 4: a corner a ten-millionth away is a "+
			"different corner", v)
	}
}

func TestRecordsBecomePortsWithTheRightKind(t *testing.T) {
	got := FromRecords([]extract.Record{{
		ID: "x.dat", Title: "X", Tier: 1,
		Holes: []extract.Port{{1, 2, 3, 0, 0, 1, 0}, {4, 5, 6, 1, 0, 0, 1}},
		Pins:  []extract.Port{{7, 8, 9, 0, 1, 0, 1}},
	}})
	if len(got.Parts) != 1 || len(got.Parts[0].Ports) != 3 {
		t.Fatalf("got %+v", got)
	}
	want := []uint8{0, PortCross, PortMale | PortCross}
	for i, k := range want {
		if got.Parts[0].Ports[i].Kind != k {
			t.Errorf("port %d kind %d, want %d", i, got.Parts[0].Ports[i].Kind, k)
		}
	}
	if p := got.Parts[0].Ports[0]; p.X != 1 || p.Y != 2 || p.Z != 3 || p.AZ != 1 {
		t.Errorf("the first hole came through as %+v", p)
	}
}
