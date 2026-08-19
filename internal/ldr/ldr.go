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
	Group string // the group it belongs to, for animation
}

// Group is a set of parts LDCad can move as one.
//
// A script reaches a group by name, so the names are the interface between the
// model and the animation.
//
// Center is what the group turns about, and it is written out as 0 0 0 whatever
// is put here, because LDCad reads it RELATIVE TO THE GROUP'S MAIN ITEM and not
// as a point in the model. Its own meta reference says so in four words —
// "Relative center to use for this group" — and this engine read it as a model
// coordinate and wrote the point on the shaft. LDCad then added that to the main
// item's own position and turned every group about somewhere off in space.
//
// Measured, in the end, rather than argued: a group whose parts sit on a shaft
// at z=-40, declared [center=0 0 -40], reported its centre at z=-80.
//
// Zero is the right answer here and not merely a safe one. Every part in one of
// these groups sits on the shaft it turns about, so the main item's own origin
// is a point on the axis — which is exactly what the centre needs to be.
type Group struct {
	Name string
	// Center is kept because callers compute it and tests read it, and because
	// a group whose members are not all on its axis would need it. Nothing is
	// emitted from it today; see above.
	Center geom.Vec3
}

// Model is a list of placed parts.
type Model struct {
	Name   string
	Author string
	Parts  []Part
	// Groups declared in the file. Order fixes the ids the part lines refer to.
	Groups []Group
	// Script is a Lua file LDCad runs with this model, if any.
	Script string
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
	if m.Script != "" {
		fmt.Fprintf(&b, "0 !LDCAD SCRIPT [source=%s]\n", m.Script)
	}
	b.WriteString("\n")

	ids := make(map[string]int, len(m.Groups))
	for i, g := range m.Groups {
		ids[g.Name] = i
		// center=0 0 0: the main item's own origin, which is on the shaft.
		// See the note on Group.Center for why this is not g.Center.
		fmt.Fprintf(&b, "0 !LDCAD GROUP_DEF [topLevel=true] [LID=%d] [GID=%s] "+
			"[name=%s] [center=0 0 0]\n",
			i, groupID(name+"/"+g.Name), g.Name)
	}
	if len(m.Groups) > 0 {
		b.WriteString("\n")
	}

	for _, p := range m.Parts {
		if p.Label != "" {
			fmt.Fprintf(&b, "0 // %s\n", p.Label)
		}
		// A GROUP_NXT applies to the line that follows it.
		if id, ok := ids[p.Group]; ok && p.Group != "" {
			fmt.Fprintf(&b, "0 !LDCAD GROUP_NXT [ids=%d] [nrs=0]\n", id)
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

// groupID is the globally unique id LDCad wants for a group.
//
// Globally, and it means it. Derived from the group's name alone, every model
// this engine wrote called its input shaft the same thing and so handed LDCad
// the same id — and with two of them open at once the second one's groups had
// no link to anything, so the script failed on its first getOri with "Active
// group link needed". A reduction and a gearbox are not often opened together
// by accident, but they are exactly what gets opened together when someone is
// looking at a set of examples.
//
// So the model's name goes into it as well. Still derived rather than random,
// because exporting the same model twice should give the same file rather than
// a fresh id every time.
func groupID(name string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	var h uint64 = 1469598103934665603
	for i := 0; i < len(name); i++ {
		h ^= uint64(name[i])
		h *= 1099511628211
	}
	out := make([]byte, 11)
	for i := range out {
		// Re-mix each round, or the tail of the id degenerates into one or two
		// repeated characters once the value gets small.
		h ^= h >> 33
		h *= 0xff51afd7ed558ccd
		h ^= h >> 29
		out[i] = alphabet[h%uint64(len(alphabet))]
	}
	return string(out)
}

func sanitize(name string) string {
	r := strings.NewReplacer(" ", "_", "/", "_", "\\", "_")
	return r.Replace(name)
}
