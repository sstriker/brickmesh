// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package spec reads a mechanism description.
//
// This is the engine's front door: a small JSON document naming shafts and what
// connects them. It is deliberately about function rather than parts — no
// positions, no part numbers — because that is the only level at which a
// mechanism can be stated before it has been worked out.
//
//	{
//	  "name": "subtractor",
//	  "shafts": [{"id": "drive", "bearings": 2}],
//	  "meshes": [{"a": "drive", "b": "output", "teeth_a": 8, "teeth_b": 24}],
//	  "differentials": [{"case": "case", "out_a": "left", "out_b": "right"}],
//	  "inputs": [{"shaft": "drive", "speed": 1.0}],
//	  "outputs": ["left", "right"]
//	}
package spec

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/sstriker/brickmesh/internal/mech"
)

// Shaft is one axis of rotation.
type Shaft struct {
	ID       string `json:"id"`
	Bearings int    `json:"bearings"`
	// Domain names the lattice a shaft lives on. Technic bricks sit at 24 LDU
	// vertically and liftarms at 20, so a transmission spanning both cannot
	// line up; leave it empty for studless.
	Domain string `json:"domain,omitempty"`
}

// Mesh is a gear pair.
type Mesh struct {
	A           string  `json:"a"`
	B           string  `json:"b"`
	TeethA      int     `json:"teeth_a"`
	TeethB      int     `json:"teeth_b"`
	Kind        string  `json:"kind,omitempty"` // spur (default), bevel, worm, chain
	BacklashDeg float64 `json:"backlash_deg,omitempty"`
}

// Differential has three ports; the case runs at the average of the outputs.
type Differential struct {
	Case string `json:"case"`
	OutA string `json:"out_a"`
	OutB string `json:"out_b"`
}

// Coupling locks two coaxial shafts together in the named states.
//
// This is how a gearbox is described: the gears are always meshed and always
// turning, and a shift changes which of them is locked to the output shaft. A
// gear that freewheels is its own shaft, coupled to the one it rides on only in
// the states where its ratio is selected. No states means always locked.
type Coupling struct {
	A      string   `json:"a"`
	B      string   `json:"b"`
	Name   string   `json:"name,omitempty"`
	States []string `json:"states,omitempty"`
}

// Input drives a shaft at a given speed.
type Input struct {
	Shaft string  `json:"shaft"`
	Speed float64 `json:"speed"`
}

// Spec is a whole mechanism.
type Spec struct {
	Name string `json:"name"`
	// Note is for whoever reads the file: why these tooth counts and not
	// others. Ignored by everything here, which is the point — a description
	// with no room for the reasoning behind it collects the reasoning in a
	// commit message nobody finds.
	Note          string         `json:"note,omitempty"`
	Shafts        []Shaft        `json:"shafts"`
	Meshes        []Mesh         `json:"meshes,omitempty"`
	Differentials []Differential `json:"differentials,omitempty"`
	Couplings     []Coupling     `json:"couplings,omitempty"`
	// States a gearbox can be shifted into, in order. Leave it out for a
	// mechanism with only one.
	States []string `json:"states,omitempty"`
	// ShiftPoints make the box change gear on its own. Leave it out for one
	// that is shifted by hand.
	ShiftPoints *ShiftPoints `json:"shift_points,omitempty"`
	// SlipClutches fit a torque limiter to a shaft: a 24-tooth gear with a
	// friction centre that gives way above a force, protecting whatever is
	// downstream. The other kind of clutch entirely from the one a driving ring
	// engages, sharing only the name.
	SlipClutches []SlipClutch `json:"slip_clutches,omitempty"`
	Inputs       []Input      `json:"inputs,omitempty"`
	Outputs      []string     `json:"outputs,omitempty"`
}

// SlipClutch is a torque limiter on a shaft.
//
// It has to sit on a 24-tooth gear, because that is the only size the part is
// made in. AtNcm is what it gives way at; leave it out to take the figure
// internal/torque carries, which is an estimate and marked as one.
type SlipClutch struct {
	Shaft string  `json:"shaft"`
	AtNcm float64 `json:"at_ncm,omitempty"`
}

// ShiftPoints say when the box changes up.
type ShiftPoints struct {
	// Watch is the shaft whose speed decides; the input, if left out.
	Watch string `json:"watch,omitempty"`
	// UpAt is the speed at which each gear gives way to the next, so there is
	// one fewer of them than there are states.
	UpAt []float64 `json:"up_at"`
	// DownAt is where each gear is given up again on the way down.
	DownAt []float64 `json:"down_at,omitempty"`
}

// Read parses a spec.
func Read(r io.Reader) (*Spec, error) {
	var s Spec
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields() // a typo in a key should not be silently ignored
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("reading the spec: %w", err)
	}
	return &s, nil
}

// Build turns a spec into a mechanism, reporting anything that does not refer
// to a declared shaft.
//
// The checks here are about the document being coherent. Whether the mechanism
// is any good is what mech's own checks answer.
func (s *Spec) Build() (*mech.Mechanism, error) {
	if len(s.Shafts) == 0 {
		return nil, fmt.Errorf("a mechanism needs at least one shaft")
	}
	name := s.Name
	if name == "" {
		name = "mechanism"
	}
	m := mech.New(name)

	shafts, err := s.addShafts(m)
	if err != nil {
		return nil, err
	}
	known := func(what, id string) error {
		if !shafts[id] {
			return fmt.Errorf("%s names shaft %q, which is not declared", what, id)
		}
		return nil
	}

	if err := s.addMeshes(m, known); err != nil {
		return nil, err
	}
	if err := s.addDifferentials(m, known); err != nil {
		return nil, err
	}
	states, err := s.addStates(m)
	if err != nil {
		return nil, err
	}
	if err := s.addCouplings(m, known, states); err != nil {
		return nil, err
	}
	if err := s.addDrive(m, known); err != nil {
		return nil, err
	}
	if err := s.addSlipClutches(m, shafts); err != nil {
		return nil, err
	}
	return m, s.addShiftPoints(m, shafts)
}

// addShiftPoints hands the mechanism its schedule, defaulting the watched shaft
// to the input: it is the engine speed a box shifts on.
func (s *Spec) addShiftPoints(m *mech.Mechanism, shafts map[string]bool) error {
	if s.ShiftPoints == nil {
		return nil
	}
	watch := s.ShiftPoints.Watch
	if watch == "" {
		if len(s.Inputs) == 0 {
			return fmt.Errorf("shift points need a shaft to watch, and there is " +
				"no input to fall back on")
		}
		watch = s.Inputs[0].Shaft
	}
	if !shafts[watch] {
		return fmt.Errorf("the shift points watch shaft %q, which is not declared",
			watch)
	}
	m.Shifts(mech.ShiftPoints{Watch: watch, UpAt: s.ShiftPoints.UpAt,
		DownAt: s.ShiftPoints.DownAt})
	return nil
}

func (s *Spec) addShafts(m *mech.Mechanism) (map[string]bool, error) {
	seen := map[string]bool{}
	for _, sh := range s.Shafts {
		if sh.ID == "" {
			return nil, fmt.Errorf("a shaft needs an id")
		}
		if seen[sh.ID] {
			return nil, fmt.Errorf("shaft %q is declared twice", sh.ID)
		}
		seen[sh.ID] = true
		domain := sh.Domain
		if domain == "" {
			domain = "technic-studless"
		}
		m.ShaftIn(sh.ID, sh.Bearings, domain)
	}
	return seen, nil
}

type knownFunc func(what, id string) error

func (s *Spec) addMeshes(m *mech.Mechanism, known knownFunc) error {
	for _, mesh := range s.Meshes {
		if err := known("a mesh", mesh.A); err != nil {
			return err
		}
		if err := known("a mesh", mesh.B); err != nil {
			return err
		}
		if mesh.A == mesh.B {
			return fmt.Errorf("a mesh joins shaft %q to itself", mesh.A)
		}
		if mesh.TeethA <= 0 || mesh.TeethB <= 0 {
			return fmt.Errorf("mesh %s/%s needs a tooth count on both sides",
				mesh.A, mesh.B)
		}
		kind := mesh.Kind
		if kind == "" {
			kind = mech.Spur
		}
		switch kind {
		case mech.Spur, mech.Bevel, mech.Worm, mech.Chain:
		default:
			return fmt.Errorf("mesh %s/%s: unknown kind %q", mesh.A, mesh.B, kind)
		}
		backlash := mesh.BacklashDeg
		if backlash == 0 {
			backlash = 5
		}
		m.MeshOf(mesh.A, mesh.B, mesh.TeethA, mesh.TeethB, kind, backlash)
	}
	return nil
}

func (s *Spec) addDifferentials(m *mech.Mechanism, known knownFunc) error {
	for _, d := range s.Differentials {
		for _, id := range []string{d.Case, d.OutA, d.OutB} {
			if err := known("a differential", id); err != nil {
				return err
			}
		}
		m.Differential(d.Case, d.OutA, d.OutB)
	}
	return nil
}

func (s *Spec) addStates(m *mech.Mechanism) (map[string]bool, error) {
	declared := map[string]bool{}
	for _, st := range s.States {
		if st == "" {
			return nil, fmt.Errorf("a state needs a name")
		}
		if declared[st] {
			return nil, fmt.Errorf("state %q is declared twice", st)
		}
		declared[st] = true
		m.State(st)
	}
	return declared, nil
}

func (s *Spec) addCouplings(m *mech.Mechanism, known knownFunc, states map[string]bool) error {
	for _, c := range s.Couplings {
		if err := known("a coupling", c.A); err != nil {
			return err
		}
		if err := known("a coupling", c.B); err != nil {
			return err
		}
		if c.A == c.B {
			return fmt.Errorf("a coupling joins shaft %q to itself", c.A)
		}
		for _, st := range c.States {
			if !states[st] {
				return fmt.Errorf("coupling %s/%s names state %q, which is not declared",
					c.A, c.B, st)
			}
		}
		m.Couple(c.A, c.B, c.Name, c.States...)
	}
	return nil
}

func (s *Spec) addDrive(m *mech.Mechanism, known knownFunc) error {
	for _, in := range s.Inputs {
		if err := known("an input", in.Shaft); err != nil {
			return err
		}
		m.Drive(in.Shaft, in.Speed)
	}
	for _, out := range s.Outputs {
		if err := known("an output", out); err != nil {
			return err
		}
		m.Output(out)
	}
	return nil
}

// addSlipClutches fits the torque limiters, checking each names a shaft.
func (s *Spec) addSlipClutches(m *mech.Mechanism, shafts map[string]bool) error {
	for _, c := range s.SlipClutches {
		if c.Shaft == "" {
			return fmt.Errorf("a slip clutch needs a shaft to sit on")
		}
		if !shafts[c.Shaft] {
			return fmt.Errorf("slip clutch on unknown shaft %q", c.Shaft)
		}
		if c.AtNcm < 0 {
			return fmt.Errorf("slip clutch on %q gives way at %g Ncm, which is "+
				"not a torque", c.Shaft, c.AtNcm)
		}
		m.Slip(mech.SlipClutch{Shaft: c.Shaft, AtNcm: c.AtNcm})
	}
	return nil
}
