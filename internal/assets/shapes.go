// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"fmt"
	"sync"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
)

// Shapes reads triangles out of the published files rather than out of .dat
// files, which is what lets the engine run somewhere with no parts library and
// no parser — a browser.
//
// A part's index is the same in the catalogue and in the mesh blob, so a name
// is looked up once in the catalogue and everything after that is a byte range.
type Shapes struct {
	catalog *Catalog
	index   *MeshIndex
	// fetch returns the bytes of one mesh, given where it lives. Supplied by
	// the caller because how you get bytes differs everywhere this runs: a file
	// on disk, a range request over HTTP, a slice already in memory.
	fetch func(offset, length int64) ([]byte, error)

	byID map[string]int
	mu   sync.Mutex
	seen map[string]*part.Shape
}

// NewShapes builds a source from a catalogue, a mesh index, and a way to read
// bytes out of the mesh file.
func NewShapes(catalog *Catalog, index *MeshIndex,
	fetch func(offset, length int64) ([]byte, error)) (*Shapes, error) {

	if len(catalog.Parts) != index.Count {
		return nil, fmt.Errorf("the catalogue has %d parts and the mesh file %d: "+
			"they are addressed by the same index, so they have to agree",
			len(catalog.Parts), index.Count)
	}
	byID := make(map[string]int, len(catalog.Parts))
	for i, p := range catalog.Parts {
		byID[p.ID] = i
	}
	return &Shapes{
		catalog: catalog, index: index, fetch: fetch,
		byID: byID, seen: map[string]*part.Shape{},
	}, nil
}

// FromBytes is the simple case: the whole mesh file is already in memory.
func FromBytes(catalog *Catalog, meshes []byte) (*Shapes, error) {
	idx, err := ReadMeshIndex(meshes)
	if err != nil {
		return nil, err
	}
	return NewShapes(catalog, idx, func(offset, length int64) ([]byte, error) {
		if offset < 0 || offset+length > int64(len(meshes)) {
			return nil, fmt.Errorf("mesh at %d..%d is outside the %d bytes given",
				offset, offset+length, len(meshes))
		}
		return meshes[offset : offset+length], nil
	})
}

// Geometry answers the engine's one question about a part.
func (s *Shapes) Geometry(name string) (*part.Shape, error) {
	s.mu.Lock()
	if got, ok := s.seen[name]; ok {
		s.mu.Unlock()
		return got, nil
	}
	s.mu.Unlock()

	i, ok := s.byID[name]
	if !ok {
		return nil, fmt.Errorf("no part %s in the catalogue", name)
	}
	offset, length, err := s.index.Range(i)
	if err != nil {
		return nil, err
	}
	raw, err := s.fetch(offset, length)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	mesh, err := ReadMesh(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(mesh.Indices) == 0 {
		// An empty entry, not a zero-length one: the counts are still written.
		// The generator leaves these for parts whose triangles could not be
		// read, so that the two files stay aligned — see docs/findings.md.
		// Saying so beats drawing nothing.
		return nil, fmt.Errorf("%s has no geometry in this mesh file", name)
	}

	shape := &part.Shape{Name: name, Title: s.catalog.Parts[i].Title}
	shape.Verts = make([]geom.Vec3, len(mesh.Positions)/3)
	for v := range shape.Verts {
		shape.Verts[v] = geom.Vec3{
			X: float64(mesh.Positions[v*3]),
			Y: float64(mesh.Positions[v*3+1]),
			Z: float64(mesh.Positions[v*3+2]),
		}
	}
	shape.Tris = make([][3]geom.Vec3, len(mesh.Indices)/3)
	for t := range shape.Tris {
		shape.Tris[t] = [3]geom.Vec3{
			shape.Verts[mesh.Indices[t*3]],
			shape.Verts[mesh.Indices[t*3+1]],
			shape.Verts[mesh.Indices[t*3+2]],
		}
	}

	s.mu.Lock()
	s.seen[name] = shape
	s.mu.Unlock()
	return shape, nil
}

// Ports is what the shadow library provides elsewhere: where a part's holes and
// pins are and which way they face. The catalogue carries them, so a browser
// has them without the shadow library either.
func (s *Shapes) Ports(name string) ([]Port, bool) {
	i, ok := s.byID[name]
	if !ok {
		return nil, false
	}
	return s.catalog.Parts[i].Ports, true
}

// RotationAxis is the direction a part's holes face, which is what the
// structural search asks of a parts library.
//
// Taken from the ports rather than guessed from the shape. A part with no ports
// has no answer, and saying so is better than returning an axis nobody checked.
func (s *Shapes) RotationAxis(name string) (geom.Vec3, string, bool) {
	ports, ok := s.Ports(name)
	if !ok || len(ports) == 0 {
		return geom.Vec3{}, "", false
	}
	p := ports[0]
	axis := geom.Vec3{X: float64(p.AX), Y: float64(p.AY), Z: float64(p.AZ)}
	if axis.Len() < 1e-9 {
		return geom.Vec3{}, "", false
	}
	return axis.Unit(), "catalog.bin", true
}

// Holes is the part's connection points, from the catalogue.
//
// The same question the shadow library answers on a machine that has one. A
// browser has the catalogue instead, which is the point of publishing it.
func (s *Shapes) Holes(name string) []part.Hole {
	ports, ok := s.Ports(name)
	if !ok {
		return nil
	}
	out := make([]part.Hole, 0, len(ports))
	for _, p := range ports {
		out = append(out, part.Hole{
			Pos:   geom.Vec3{X: float64(p.X), Y: float64(p.Y), Z: float64(p.Z)},
			Axis:  geom.Vec3{X: float64(p.AX), Y: float64(p.AY), Z: float64(p.AZ)}.Unit(),
			Cross: p.Kind&PortCross != 0,
		})
	}
	return out
}
