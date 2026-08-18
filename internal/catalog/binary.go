// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package catalog

import (
	"fmt"

	"brickmesh/internal/assets"
	"brickmesh/internal/geom"
)

// LoadBinary reads the catalog from the format a browser downloads.
//
// The same catalog the JSON loader produces, from the file that is actually
// published. Having both is the point rather than a transition: JSON stays
// readable in a diff and is what the extractor's tests compare, while the
// binary is a fifth the size and needs no reflection to parse — which is what
// makes it the one a TinyGo build can read. See docs/architecture.md.
func LoadBinary(b []byte) (*Catalog, error) {
	raw, err := assets.ReadCatalog(b)
	if err != nil {
		return nil, fmt.Errorf("reading catalog: %w", err)
	}
	c := &Catalog{Parts: make(map[string]*Part, len(raw.Parts))}
	for _, rp := range raw.Parts {
		p := &Part{
			ID: rp.ID, Title: rp.Title, Tier: rp.Tier,
			cells: make(map[int][]geom.Cell),
		}
		for _, port := range rp.Ports {
			p.Ports = append(p.Ports, fromBinary(port))
		}
		c.Parts[p.ID] = p
	}
	return c, nil
}

// fromBinary unpacks the kind byte.
//
// The two bits are laid out to match Gender and PortKind, so this stays an
// unpacking rather than a translation. The test that a part survives the round
// trip through both loaders is what keeps them from drifting apart.
func fromBinary(p assets.Port) Port {
	kind := Round
	if p.Kind&assets.PortCross != 0 {
		kind = Cross
	}
	gender := Female
	if p.Kind&assets.PortMale != 0 {
		gender = Male
	}
	return Port{
		Pos:  geom.Vec3{X: float64(p.X), Y: float64(p.Y), Z: float64(p.Z)},
		Axis: geom.Vec3{X: float64(p.AX), Y: float64(p.AY), Z: float64(p.AZ)},
		Kind: kind, Gender: gender,
	}
}
