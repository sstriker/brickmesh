// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package catalog reads the parts catalog produced by the Python extractor
// and builds the search index over it.
//
// The index is where the performance lives. The solver keeps asking: which
// part has a hole at THIS point along THIS direction? The answer set does not
// depend on the point, only on the direction. So compute once, for every
// combination of part, port and rotation, which world direction results, group
// on that, and a query becomes a table lookup plus a subtraction.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/sstriker/brickmesh/internal/geom"
)

type PortKind uint8

const (
	Round PortKind = iota // pin hole or pin
	Cross                 // axle hole or axle
)

type Gender uint8

const (
	Female Gender = iota
	Male
)

// Port is a connection point in the part's local frame.
type Port struct {
	Pos    geom.Vec3
	Axis   geom.Vec3
	Kind   PortKind
	Gender Gender
}

// Part is a single part with all its ports.
type Part struct {
	ID    string
	Title string
	Tier  uint8 // 1 common, 2 Technic, 3 everything else
	Ports []Port
	// Occupied cells on the voxel grid, per rotation. Filled lazily.
	cells map[int][]geom.Cell
}

// Catalog is the full inventory.
type Catalog struct {
	Parts map[string]*Part
	index map[indexKey][]Candidate
}

type indexKey struct {
	axis   geom.Vec3 // normalized, sign-free
	gender Gender
	kind   PortKind
}

// Candidate is a pre-rotated port: for a query at point p, the part's origin
// is simply p minus Offset.
type Candidate struct {
	Part   *Part
	Rot    uint8
	Offset geom.Vec3
}

// jsonPart is the interchange format with the Python extractor.
type jsonPart struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Tier  uint8        `json:"tier"`
	Holes [][7]float64 `json:"holes"` // x,y,z,ax,ay,az,cross
	Pins  [][7]float64 `json:"pins"`
}

// Load reads the catalog from the JSON the extractor writes.
func Load(r io.Reader) (*Catalog, error) {
	var raw []jsonPart
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("reading catalog: %w", err)
	}
	c := &Catalog{Parts: make(map[string]*Part, len(raw))}
	for _, jp := range raw {
		p := &Part{ID: jp.ID, Title: jp.Title, Tier: jp.Tier,
			cells: make(map[int][]geom.Cell)}
		for _, h := range jp.Holes {
			p.Ports = append(p.Ports, port(h, Female))
		}
		for _, m := range jp.Pins {
			p.Ports = append(p.Ports, port(m, Male))
		}
		c.Parts[p.ID] = p
	}
	return c, nil
}

func port(r [7]float64, g Gender) Port {
	k := Round
	if r[6] != 0 {
		k = Cross
	}
	return Port{
		Pos:    geom.Vec3{X: r[0], Y: r[1], Z: r[2]},
		Axis:   geom.Vec3{X: r[3], Y: r[4], Z: r[5]}.Unit(),
		Kind:   k,
		Gender: g,
	}
}

// axisKey makes the direction sign-free: a hole along +Y is the same hole as
// along -Y.
func axisKey(a geom.Vec3) geom.Vec3 {
	a = a.Unit()
	for _, v := range [3]float64{a.X, a.Y, a.Z} {
		if math.Abs(v) > 1e-9 {
			if v < 0 {
				a = a.Scale(-1)
			}
			break
		}
	}
	return a.Round(3)
}

// BuildIndex populates the search index. Once per catalog.
func (c *Catalog) BuildIndex(maxTier uint8) {
	c.index = make(map[indexKey][]Candidate)
	for _, p := range c.Parts {
		if p.Tier > maxTier {
			continue
		}
		for _, port := range p.Ports {
			for ri := 0; ri < len(geom.Rotations); ri++ {
				R := geom.Rotations[ri]
				k := indexKey{
					axis:   axisKey(R.Apply(port.Axis)),
					gender: port.Gender,
					kind:   port.Kind,
				}
				c.index[k] = append(c.index[k],
					Candidate{Part: p, Rot: uint8(ri), Offset: R.Apply(port.Pos)})
			}
		}
	}
}

// Lookup returns every placement with a port at `point` along `axis`.
//
// Unlike the Python version, nothing is truncated on a heuristic here. Structs
// in a slice are cheap enough to keep the full set, and truncating can cut
// away the right candidate.
func (c *Catalog) Lookup(point geom.Vec3, axis geom.Vec3, g Gender, k PortKind,
	dst []Placement) []Placement {

	dst = dst[:0]
	for _, cand := range c.index[indexKey{axisKey(axis), g, k}] {
		origin := point.Sub(cand.Offset)
		if !origin.OnLattice(geom.HalfStud) {
			continue
		}
		dst = append(dst, Placement{Part: cand.Part, Rot: cand.Rot, Origin: origin})
	}
	return dst
}

// Placement is a part at a position in an orientation.
type Placement struct {
	Part   *Part
	Rot    uint8
	Origin geom.Vec3
}
