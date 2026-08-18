// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package assets is the two files a browser build downloads.
//
// The split is the one measured in docs/architecture.md and it is not a
// symmetry: the solver considers every part, so it needs the whole port index —
// 81 KB gzipped for the library, one fetch, done before the user has let go of
// the mouse. The meshes are 132 MB, and only the parts actually placed ever get
// drawn, so those are fetched by byte range afterwards.
//
// Hence one format holding everything and one built to be read a slice at a
// time, in the same package because the thing that ties them together is that
// a part has the same index in both.
//
// Written by hand rather than with a schema compiler. A Parquet decoder
// compiled to WebAssembly is one to two megabytes — more than the data it would
// read — and TinyGo has no reflection to build a generic decoder on. Neither
// constraint costs anything here, because what is being stored is flat arrays
// of floats.
package assets

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// The catalog's own header. Little-endian throughout: every target this can
// reach is little-endian, and WebAssembly is defined to be.
const (
	catalogMagic   = "BMCT"
	CatalogVersion = 1
	catalogHeader  = 48
	partEntry      = 20 // bytes per part in the table
	portFloats     = 6  // x y z ax ay az
	portStride     = portFloats * 4
)

// Port kinds, packed into one byte. Bit 0 is the gender, bit 1 the shape, which
// is how the engine's own catalog.Gender and catalog.PortKind are ordered.
const (
	PortMale  = 1 << 0
	PortCross = 1 << 1
)

// Port is a connection point in a part's own frame.
type Port struct {
	X, Y, Z    float32
	AX, AY, AZ float32
	// Kind is the packed byte: PortMale and PortCross, or zero for a round
	// female port, which is what most of them are.
	Kind uint8
}

// Part is one entry in the catalog.
type Part struct {
	ID    string
	Title string
	// Tier grades how likely a part is to be wanted: 1 common, 2 all Technic,
	// 3 the whole library.
	Tier  uint8
	Ports []Port
}

// Catalog is the whole index.
type Catalog struct {
	Parts []Part
}

// WriteCatalog lays the catalog out as bytes.
//
// The parts come back sorted by tier, so tier 1 is contiguous at the front and
// a caller who only wants the common parts can take a single range off the
// front of the file rather than gathering scattered records. Within a tier they
// are sorted by id, so the output is the same every time it is built — a
// generated file that changes without its input changing is a generated file
// nobody can review.
func WriteCatalog(c Catalog) ([]byte, error) {
	parts := append([]Part(nil), c.Parts...)
	sort.SliceStable(parts, func(i, j int) bool {
		if parts[i].Tier != parts[j].Tier {
			return parts[i].Tier < parts[j].Tier
		}
		return parts[i].ID < parts[j].ID
	})

	var (
		pool      []byte
		offsets   = map[string][2]uint32{} // text -> offset, length
		portTotal int
	)
	intern := func(s string) (uint32, uint16, error) {
		if got, ok := offsets[s]; ok {
			return got[0], uint16(got[1]), nil
		}
		if len(s) > math.MaxUint16 {
			return 0, 0, fmt.Errorf("the text %q is too long to store", s[:32])
		}
		at := uint32(len(pool))
		pool = append(pool, s...)
		offsets[s] = [2]uint32{at, uint32(len(s))}
		return at, uint16(len(s)), nil
	}

	table := make([]byte, 0, len(parts)*partEntry)
	for _, p := range parts {
		idOff, idLen, err := intern(p.ID)
		if err != nil {
			return nil, err
		}
		titleOff, titleLen, err := intern(p.Title)
		if err != nil {
			return nil, err
		}
		if len(p.Ports) > math.MaxUint16 {
			return nil, fmt.Errorf("part %s has %d ports, more than can be stored",
				p.ID, len(p.Ports))
		}
		entry := make([]byte, partEntry)
		binary.LittleEndian.PutUint32(entry[0:], idOff)
		binary.LittleEndian.PutUint16(entry[4:], idLen)
		binary.LittleEndian.PutUint32(entry[6:], titleOff)
		binary.LittleEndian.PutUint16(entry[10:], titleLen)
		binary.LittleEndian.PutUint32(entry[12:], uint32(portTotal))
		binary.LittleEndian.PutUint16(entry[16:], uint16(len(p.Ports)))
		entry[18] = p.Tier
		table = append(table, entry...)
		portTotal += len(p.Ports)
	}

	// The floats and the kind bytes go in separate runs rather than interleaved.
	// Interleaving costs nothing in Go and everything in a browser: a run of
	// float32 can be handed to a Float32Array as a view over the same memory,
	// while a stray byte every twenty-four forces a copy and a loop.
	floats := make([]byte, 0, portTotal*portStride)
	kinds := make([]byte, 0, portTotal)
	for _, p := range parts {
		for _, port := range p.Ports {
			var buf [portStride]byte
			for i, v := range [portFloats]float32{
				port.X, port.Y, port.Z, port.AX, port.AY, port.AZ,
			} {
				binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
			}
			floats = append(floats, buf[:]...)
			kinds = append(kinds, port.Kind)
		}
	}

	partsOff := uint32(catalogHeader)
	portsOff := partsOff + uint32(len(table))
	kindsOff := portsOff + uint32(len(floats))
	// The string pool goes last, and every section before it is a multiple of
	// four bytes long, so each starts aligned. Text has no such requirement and
	// would otherwise push whatever followed it off alignment.
	stringsOff := align4(kindsOff + uint32(len(kinds)))

	out := make([]byte, stringsOff, int(stringsOff)+len(pool))
	copy(out, catalogMagic)
	binary.LittleEndian.PutUint16(out[4:], CatalogVersion)
	binary.LittleEndian.PutUint32(out[8:], uint32(len(parts)))
	binary.LittleEndian.PutUint32(out[12:], uint32(portTotal))
	binary.LittleEndian.PutUint32(out[16:], tierEnd(parts, 1))
	binary.LittleEndian.PutUint32(out[20:], tierEnd(parts, 2))
	binary.LittleEndian.PutUint32(out[24:], partsOff)
	binary.LittleEndian.PutUint32(out[28:], portsOff)
	binary.LittleEndian.PutUint32(out[32:], kindsOff)
	binary.LittleEndian.PutUint32(out[36:], stringsOff)
	binary.LittleEndian.PutUint32(out[40:], uint32(len(pool)))

	copy(out[partsOff:], table)
	copy(out[portsOff:], floats)
	copy(out[kindsOff:], kinds)
	return append(out, pool...), nil
}

// TierEnd is the part index one past the last part of a tier, which is what
// makes "fetch the common parts" a single range rather than a gather.
func (c *Catalog) TierEnd(tier uint8) int { return int(tierEnd(c.Parts, tier)) }

func tierEnd(parts []Part, tier uint8) uint32 {
	for i, p := range parts {
		if p.Tier > tier {
			return uint32(i)
		}
	}
	return uint32(len(parts))
}

func align4(v uint32) uint32 { return (v + 3) &^ 3 }

// ReadCatalog parses what WriteCatalog wrote.
func ReadCatalog(b []byte) (*Catalog, error) {
	if len(b) < catalogHeader {
		return nil, fmt.Errorf("catalog is %d bytes, too short to hold a header", len(b))
	}
	if string(b[:4]) != catalogMagic {
		return nil, fmt.Errorf("not a catalog: magic is %q, want %q",
			b[:4], catalogMagic)
	}
	if v := binary.LittleEndian.Uint16(b[4:]); v != CatalogVersion {
		return nil, fmt.Errorf("catalog is version %d, this build reads %d",
			v, CatalogVersion)
	}

	var (
		nParts     = binary.LittleEndian.Uint32(b[8:])
		nPorts     = binary.LittleEndian.Uint32(b[12:])
		partsOff   = binary.LittleEndian.Uint32(b[24:])
		portsOff   = binary.LittleEndian.Uint32(b[28:])
		kindsOff   = binary.LittleEndian.Uint32(b[32:])
		stringsOff = binary.LittleEndian.Uint32(b[36:])
		stringsLen = binary.LittleEndian.Uint32(b[40:])
	)
	// Every offset is checked before anything is read through it. This file
	// arrives over the network, and a truncated download should be an error
	// rather than a panic in a worker nobody can see.
	for _, s := range []struct {
		what     string
		off, len uint32
	}{
		{"part table", partsOff, nParts * partEntry},
		{"port array", portsOff, nPorts * portStride},
		{"port kinds", kindsOff, nPorts},
		{"string pool", stringsOff, stringsLen},
	} {
		if uint64(s.off)+uint64(s.len) > uint64(len(b)) {
			return nil, fmt.Errorf("the %s runs past the end of the file: "+
				"%d+%d in %d bytes", s.what, s.off, s.len, len(b))
		}
	}
	pool := b[stringsOff : stringsOff+stringsLen]

	c := &Catalog{Parts: make([]Part, nParts)}
	for i := range c.Parts {
		entry := b[partsOff+uint32(i)*partEntry:]
		idOff := binary.LittleEndian.Uint32(entry[0:])
		idLen := binary.LittleEndian.Uint16(entry[4:])
		titleOff := binary.LittleEndian.Uint32(entry[6:])
		titleLen := binary.LittleEndian.Uint16(entry[10:])
		portOff := binary.LittleEndian.Uint32(entry[12:])
		portLen := binary.LittleEndian.Uint16(entry[16:])

		id, err := text(pool, idOff, idLen)
		if err != nil {
			return nil, fmt.Errorf("part %d id: %w", i, err)
		}
		title, err := text(pool, titleOff, titleLen)
		if err != nil {
			return nil, fmt.Errorf("part %d title: %w", i, err)
		}
		if uint64(portOff)+uint64(portLen) > uint64(nPorts) {
			return nil, fmt.Errorf("part %s claims ports %d..%d of %d",
				id, portOff, uint32(portOff)+uint32(portLen), nPorts)
		}

		p := Part{ID: id, Title: title, Tier: entry[18]}
		if portLen > 0 {
			p.Ports = make([]Port, portLen)
		}
		for j := range p.Ports {
			at := portsOff + (portOff+uint32(j))*portStride
			var f [portFloats]float32
			for k := range f {
				f[k] = math.Float32frombits(
					binary.LittleEndian.Uint32(b[at+uint32(k)*4:]))
			}
			p.Ports[j] = Port{
				X: f[0], Y: f[1], Z: f[2], AX: f[3], AY: f[4], AZ: f[5],
				Kind: b[kindsOff+portOff+uint32(j)],
			}
		}
		c.Parts[i] = p
	}
	return c, nil
}

func text(pool []byte, off uint32, length uint16) (string, error) {
	if uint64(off)+uint64(length) > uint64(len(pool)) {
		return "", fmt.Errorf("runs past the end of the string pool")
	}
	return string(pool[off : off+uint32(length)]), nil
}
