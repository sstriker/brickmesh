// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package ldr writes LDraw model files.
//
// The format is plain text, one line per part reference:
//
//	1 <color> x y z a b c d e f g h i part.dat
//
// where a..i is the rotation, row by row, and x y z the position, so a point of
// the part lands at M*p + t. Stud.io opens these directly, which is what makes
// it the handover format out of the engine.
//
// Units are LDU and +Y points DOWN, which is worth remembering when a model
// comes out upside down rather than wrong.
package ldr

import (
	"fmt"
	"strings"

	"brickmesh/internal/geom"
)

// Colors used by the writer. LDraw's palette is large; these are the few the
// engine has an opinion about.
const (
	ColorMain      = 16 // inherits from the caller, the usual choice inside a part
	ColorLightGray = 71
	ColorBlack     = 0
	ColorRed       = 4
	ColorYellow    = 14
)

// Part is one placed part in a model.
type Part struct {
	Name  string // e.g. "3648b.dat"
	Color int
	Rot   geom.Mat3
	Pos   geom.Vec3
	Label string // emitted as a comment above the line, when set
}

// Model is a list of placed parts.
type Model struct {
	Name   string
	Author string
	Parts  []Part
}

// New starts a model.
func New(name string) *Model {
	return &Model{Name: name, Author: "brickmesh"}
}

// Add places a part with an explicit orientation.
func (m *Model) Add(name string, color int, rot geom.Mat3, pos geom.Vec3, label string) {
	m.Parts = append(m.Parts, Part{Name: name, Color: color, Rot: rot, Pos: pos, Label: label})
}

// AddLattice places a part in one of the 24 lattice orientations, which is how
// the structural search reports its results.
func (m *Model) AddLattice(name string, color, rot int, pos geom.Vec3, label string) error {
	if rot < 0 || rot >= len(geom.Rotations) {
		return fmt.Errorf("rotation %d is not one of the 24", rot)
	}
	m.Add(name, color, geom.Rotations[rot], pos, label)
	return nil
}

// Encode renders the model.
func (m *Model) Encode() string {
	var b strings.Builder
	name := m.Name
	if name == "" {
		name = "model"
	}
	author := m.Author
	if author == "" {
		author = "brickmesh"
	}

	fmt.Fprintf(&b, "0 %s\n", name)
	fmt.Fprintf(&b, "0 Name: %s.ldr\n", sanitize(name))
	fmt.Fprintf(&b, "0 Author: %s\n", author)
	b.WriteString("0 !LDRAW_ORG Model\n")
	b.WriteString("\n")

	for _, p := range m.Parts {
		if p.Label != "" {
			fmt.Fprintf(&b, "0 // %s\n", p.Label)
		}
		b.WriteString(line(p))
		b.WriteString("\n")
	}
	// LDraw files end with a bare 0; some readers are unhappy without it.
	b.WriteString("0\n")
	return b.String()
}

// line renders one type-1 reference.
func line(p Part) string {
	name := p.Name
	if !strings.HasSuffix(strings.ToLower(name), ".dat") {
		name += ".dat"
	}
	nums := []float64{
		p.Pos.X, p.Pos.Y, p.Pos.Z,
		p.Rot[0][0], p.Rot[0][1], p.Rot[0][2],
		p.Rot[1][0], p.Rot[1][1], p.Rot[1][2],
		p.Rot[2][0], p.Rot[2][1], p.Rot[2][2],
	}
	parts := make([]string, 0, len(nums))
	for _, v := range nums {
		parts = append(parts, trim(v))
	}
	return fmt.Sprintf("1 %d %s %s", p.Color, strings.Join(parts, " "), name)
}

// trim keeps the numbers short without losing precision that matters. LDU
// coordinates are whole or half numbers almost everywhere.
func trim(v float64) string {
	if v == 0 {
		return "0" // never "-0", which some parsers dislike
	}
	return fmt.Sprintf("%g", v)
}

func sanitize(name string) string {
	r := strings.NewReplacer(" ", "_", "/", "_", "\\", "_")
	return r.Replace(name)
}
