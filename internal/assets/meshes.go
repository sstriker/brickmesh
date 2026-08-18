// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	meshMagic     = "BMMS"
	MeshesVersion = 1
	meshHeader    = 32
	meshEntry     = 8 // byte offset and length per part
)

// Mesh is one part's triangles, with the vertices shared between them.
//
// Indexed rather than a flat run of triangles because that is what a renderer
// wants and because the parts are full of repeated corners: an LDraw part is
// built from primitives that meet edge to edge.
type Mesh struct {
	// Positions is three float32 per vertex.
	Positions []float32
	// Indices is three per triangle, into Positions.
	Indices []uint32
}

// Triangles is how many the mesh has.
func (m Mesh) Triangles() int { return len(m.Indices) / 3 }

// WriteMeshes lays out one mesh per part, in the catalog's part order.
//
// The order is the contract between the two files: part i in the catalog is
// mesh i here, so a solver that has chosen part 412 asks for entry 412 without
// a name lookup or a second index. A caller passing these in any other order
// gets a file that is wrong in a way nothing will notice until something is
// drawn with the wrong shape.
func WriteMeshes(meshes []Mesh) ([]byte, error) {
	tableOff := uint32(meshHeader)
	payloadOff := tableOff + uint32(len(meshes))*meshEntry

	table := make([]byte, len(meshes)*meshEntry)
	payload := make([]byte, 0, 1<<20)
	for i, m := range meshes {
		if len(m.Positions)%3 != 0 {
			return nil, fmt.Errorf("mesh %d has %d position floats, not a multiple of three",
				i, len(m.Positions))
		}
		if len(m.Indices)%3 != 0 {
			return nil, fmt.Errorf("mesh %d has %d indices, not a multiple of three",
				i, len(m.Indices))
		}
		vertices := len(m.Positions) / 3
		for _, idx := range m.Indices {
			if int(idx) >= vertices {
				return nil, fmt.Errorf("mesh %d refers to vertex %d of %d",
					i, idx, vertices)
			}
		}

		at := len(payload)
		var counts [8]byte
		binary.LittleEndian.PutUint32(counts[0:], uint32(vertices))
		binary.LittleEndian.PutUint32(counts[4:], uint32(len(m.Indices)))
		payload = append(payload, counts[:]...)
		for _, v := range m.Positions {
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], math.Float32bits(v))
			payload = append(payload, buf[:]...)
		}
		for _, idx := range m.Indices {
			var buf [4]byte
			binary.LittleEndian.PutUint32(buf[:], idx)
			payload = append(payload, buf[:]...)
		}

		binary.LittleEndian.PutUint32(table[i*meshEntry:], payloadOff+uint32(at))
		binary.LittleEndian.PutUint32(table[i*meshEntry+4:], uint32(len(payload)-at))
	}

	out := make([]byte, meshHeader, int(payloadOff)+len(payload))
	copy(out, meshMagic)
	binary.LittleEndian.PutUint16(out[4:], MeshesVersion)
	binary.LittleEndian.PutUint32(out[8:], uint32(len(meshes)))
	binary.LittleEndian.PutUint32(out[12:], tableOff)
	binary.LittleEndian.PutUint32(out[16:], payloadOff)
	out = append(out, table...)
	return append(out, payload...), nil
}

// MeshIndex is the header and offset table, which is all a caller needs before
// deciding what to fetch.
//
// Small on purpose: eight bytes a part, so twenty-odd kilobytes for the whole
// library against a hundred and thirty megabytes of triangles. Fetch this once
// and every mesh afterwards is a range request whose bounds are already known.
type MeshIndex struct {
	Count   int
	offsets []uint32
	lengths []uint32
}

// IndexSize is how many bytes of the file the index occupies, so a caller can
// ask for exactly that many and no more.
func IndexSize(parts int) int64 { return int64(meshHeader) + int64(parts)*meshEntry }

// ReadMeshIndex parses the front of the file. It needs the header and the
// table, and nothing after them — a prefix is enough.
func ReadMeshIndex(b []byte) (*MeshIndex, error) {
	if len(b) < meshHeader {
		return nil, fmt.Errorf("mesh file is %d bytes, too short to hold a header", len(b))
	}
	if string(b[:4]) != meshMagic {
		return nil, fmt.Errorf("not a mesh file: magic is %q, want %q", b[:4], meshMagic)
	}
	if v := binary.LittleEndian.Uint16(b[4:]); v != MeshesVersion {
		return nil, fmt.Errorf("mesh file is version %d, this build reads %d",
			v, MeshesVersion)
	}
	count := binary.LittleEndian.Uint32(b[8:])
	tableOff := binary.LittleEndian.Uint32(b[12:])
	if uint64(tableOff)+uint64(count)*meshEntry > uint64(len(b)) {
		return nil, fmt.Errorf("the offset table needs %d bytes and there are %d",
			uint64(tableOff)+uint64(count)*meshEntry, len(b))
	}

	idx := &MeshIndex{
		Count:   int(count),
		offsets: make([]uint32, count),
		lengths: make([]uint32, count),
	}
	for i := range idx.offsets {
		at := tableOff + uint32(i)*meshEntry
		idx.offsets[i] = binary.LittleEndian.Uint32(b[at:])
		idx.lengths[i] = binary.LittleEndian.Uint32(b[at+4:])
	}
	return idx, nil
}

// Range is where part i's mesh lives, as a byte offset and length — the two
// numbers an HTTP Range request is made of.
func (m *MeshIndex) Range(part int) (offset, length int64, err error) {
	if part < 0 || part >= m.Count {
		return 0, 0, fmt.Errorf("no part %d in a file of %d", part, m.Count)
	}
	return int64(m.offsets[part]), int64(m.lengths[part]), nil
}

// Span covers several parts at once.
//
// Worth having because parts placed together are often stored together — the
// gears are consecutive in the catalog — and one request for a run of them
// beats a request each. The caller gets back a range that may include meshes it
// did not ask for, which ReadMesh will happily skip over.
func (m *MeshIndex) Span(parts []int) (offset, length int64, err error) {
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("no parts asked for")
	}
	lo, hi := int64(math.MaxInt64), int64(0)
	for _, p := range parts {
		off, n, err := m.Range(p)
		if err != nil {
			return 0, 0, err
		}
		if off < lo {
			lo = off
		}
		if off+n > hi {
			hi = off + n
		}
	}
	return lo, hi - lo, nil
}

// ReadMesh decodes one mesh from bytes that start at its own offset, which is
// what comes back from a range request for exactly that mesh.
func ReadMesh(b []byte) (Mesh, error) {
	if len(b) < 8 {
		return Mesh{}, fmt.Errorf("mesh is %d bytes, too short for its counts", len(b))
	}
	vertices := binary.LittleEndian.Uint32(b[0:])
	indices := binary.LittleEndian.Uint32(b[4:])
	need := 8 + uint64(vertices)*12 + uint64(indices)*4
	if need > uint64(len(b)) {
		return Mesh{}, fmt.Errorf("mesh needs %d bytes and there are %d", need, len(b))
	}

	m := Mesh{
		Positions: make([]float32, vertices*3),
		Indices:   make([]uint32, indices),
	}
	at := uint32(8)
	for i := range m.Positions {
		m.Positions[i] = math.Float32frombits(
			binary.LittleEndian.Uint32(b[at+uint32(i)*4:]))
	}
	at += vertices * 12
	for i := range m.Indices {
		m.Indices[i] = binary.LittleEndian.Uint32(b[at+uint32(i)*4:])
	}
	return m, nil
}
