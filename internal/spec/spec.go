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

	"brickmesh/internal/mech"
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

// Input drives a shaft at a given speed.
type Input struct {
	Shaft string  `json:"shaft"`
	Speed float64 `json:"speed"`
}

// Spec is a whole mechanism.
type Spec struct {
	Name          string         `json:"name"`
	Shafts        []Shaft        `json:"shafts"`
	Meshes        []Mesh         `json:"meshes,omitempty"`
	Differentials []Differential `json:"differentials,omitempty"`
	Inputs        []Input        `json:"inputs,omitempty"`
	Outputs       []string       `json:"outputs,omitempty"`
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

	known := func(what, id string) error {
		if !seen[id] {
			return fmt.Errorf("%s names shaft %q, which is not declared", what, id)
		}
		return nil
	}

	for _, mesh0 := range s.Meshes {
		if err := known("a mesh", mesh0.A); err != nil {
			return nil, err
		}
		if err := known("a mesh", mesh0.B); err != nil {
			return nil, err
		}
		if mesh0.A == mesh0.B {
			return nil, fmt.Errorf("a mesh joins shaft %q to itself", mesh0.A)
		}
		if mesh0.TeethA <= 0 || mesh0.TeethB <= 0 {
			return nil, fmt.Errorf("mesh %s/%s needs a tooth count on both sides",
				mesh0.A, mesh0.B)
		}
		kind := mesh0.Kind
		if kind == "" {
			kind = mech.Spur
		}
		switch kind {
		case mech.Spur, mech.Bevel, mech.Worm, mech.Chain:
		default:
			return nil, fmt.Errorf("mesh %s/%s: unknown kind %q", mesh0.A, mesh0.B, kind)
		}
		backlash := mesh0.BacklashDeg
		if backlash == 0 {
			backlash = 5
		}
		m.MeshOf(mesh0.A, mesh0.B, mesh0.TeethA, mesh0.TeethB, kind, backlash)
	}

	for _, d := range s.Differentials {
		for _, id := range []string{d.Case, d.OutA, d.OutB} {
			if err := known("a differential", id); err != nil {
				return nil, err
			}
		}
		m.Differential(d.Case, d.OutA, d.OutB)
	}

	for _, in := range s.Inputs {
		if err := known("an input", in.Shaft); err != nil {
			return nil, err
		}
		m.Drive(in.Shaft, in.Speed)
	}
	for _, out := range s.Outputs {
		if err := known("an output", out); err != nil {
			return nil, err
		}
		m.Output(out)
	}
	return m, nil
}
