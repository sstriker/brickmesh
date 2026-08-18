// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package spec

import (
	"strings"
	"testing"

	"brickmesh/internal/mech"
)

const subtractor = `{
  "name": "subtractor",
  "shafts": [
    {"id": "drive", "bearings": 2},
    {"id": "left", "bearings": 2},
    {"id": "right", "bearings": 2}
  ],
  "differentials": [{"case": "drive", "out_a": "left", "out_b": "right"}],
  "inputs": [{"shaft": "left", "speed": 1.0}, {"shaft": "right", "speed": 1.0}],
  "outputs": ["drive"]
}`

func build(t *testing.T, doc string) *mech.Mechanism {
	t.Helper()
	s, err := Read(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	m, err := s.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return m
}

func TestASpecBecomesAWorkingMechanism(t *testing.T) {
	m := build(t, subtractor)
	if m.Name != "subtractor" {
		t.Errorf("name %q", m.Name)
	}
	if got := m.DOF(""); got != 2 {
		t.Errorf("DOF = %d, want 2 for a differential", got)
	}
	sol, ok := m.Solve("")
	if !ok {
		t.Fatal("not solvable")
	}
	// Both tracks the same way: the case runs with them.
	if sol["drive"] != 1 {
		t.Errorf("case speed %v, want 1", sol["drive"])
	}
}

func TestShaftOrderIsKept(t *testing.T) {
	// The order fixes the matrix columns, so it should follow the document.
	got := build(t, subtractor).Order()
	want := []string{"drive", "left", "right"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
}

func TestMeshDefaults(t *testing.T) {
	m := build(t, `{
      "name": "pair",
      "shafts": [{"id": "a", "bearings": 2}, {"id": "b", "bearings": 2}],
      "meshes": [{"a": "a", "b": "b", "teeth_a": 8, "teeth_b": 24}]
    }`)
	mesh0, ok := m.Links[0].(mech.Mesh)
	if !ok {
		t.Fatal("the link is not a mesh")
	}
	if mesh0.Kind != mech.Spur {
		t.Errorf("kind %q, want spur by default", mesh0.Kind)
	}
	if mesh0.BacklashDeg != 5 {
		t.Errorf("backlash %v, want the 5 degree default", mesh0.BacklashDeg)
	}
}

func TestStudlessIsTheDefaultDomain(t *testing.T) {
	m := build(t, `{"name":"x","shafts":[{"id":"a","bearings":2}]}`)
	s, _ := m.Get("a")
	if s.Domain != "technic-studless" {
		t.Errorf("domain %q", s.Domain)
	}
}

func TestUnknownFieldsAreRejected(t *testing.T) {
	// A typo in a key should not be quietly ignored: the mechanism it describes
	// would not be the one that gets built.
	_, err := Read(strings.NewReader(
		`{"name":"x","shafts":[{"id":"a","bearings":2}],"meshes_typo":[]}`))
	if err == nil {
		t.Error("expected an unknown field to be reported")
	}
}

func TestReferencesMustResolve(t *testing.T) {
	cases := map[string]string{
		"mesh": `{"name":"x","shafts":[{"id":"a","bearings":2}],
                 "meshes":[{"a":"a","b":"ghost","teeth_a":8,"teeth_b":24}]}`,
		"differential": `{"name":"x","shafts":[{"id":"a","bearings":2}],
                 "differentials":[{"case":"a","out_a":"ghost","out_b":"a"}]}`,
		"input": `{"name":"x","shafts":[{"id":"a","bearings":2}],
                 "inputs":[{"shaft":"ghost","speed":1}]}`,
		"output": `{"name":"x","shafts":[{"id":"a","bearings":2}],
                 "outputs":["ghost"]}`,
	}
	for what, doc := range cases {
		s, err := Read(strings.NewReader(doc))
		if err != nil {
			t.Fatalf("%s: read: %v", what, err)
		}
		if _, err := s.Build(); err == nil {
			t.Errorf("%s: a reference to an undeclared shaft should be reported", what)
		}
	}
}

func TestMalformedSpecsAreRejected(t *testing.T) {
	cases := map[string]string{
		"no shafts":       `{"name":"x","shafts":[]}`,
		"nameless shaft":  `{"name":"x","shafts":[{"bearings":2}]}`,
		"duplicate shaft": `{"name":"x","shafts":[{"id":"a"},{"id":"a"}]}`,
		"self mesh": `{"name":"x","shafts":[{"id":"a"}],
                      "meshes":[{"a":"a","b":"a","teeth_a":8,"teeth_b":8}]}`,
		"no teeth": `{"name":"x","shafts":[{"id":"a"},{"id":"b"}],
                     "meshes":[{"a":"a","b":"b"}]}`,
		"unknown kind": `{"name":"x","shafts":[{"id":"a"},{"id":"b"}],
                     "meshes":[{"a":"a","b":"b","teeth_a":8,"teeth_b":24,"kind":"rubber band"}]}`,
	}
	for what, doc := range cases {
		s, err := Read(strings.NewReader(doc))
		if err != nil {
			continue // rejected at parse time, which is also fine
		}
		if _, err := s.Build(); err == nil {
			t.Errorf("%s: should have been rejected", what)
		}
	}
}

func TestGarbageIsNotASpec(t *testing.T) {
	if _, err := Read(strings.NewReader("not json")); err == nil {
		t.Error("expected an error")
	}
}

const gearbox = `{
  "name": "2-speed",
  "states": ["low", "high"],
  "shafts": [
    {"id": "input", "bearings": 2},
    {"id": "output", "bearings": 2},
    {"id": "g1", "bearings": 2},
    {"id": "g2", "bearings": 2}
  ],
  "meshes": [
    {"a": "input", "b": "g1", "teeth_a": 8, "teeth_b": 24},
    {"a": "input", "b": "g2", "teeth_a": 24, "teeth_b": 8}
  ],
  "couplings": [
    {"a": "output", "b": "g1", "name": "dog low", "states": ["low"]},
    {"a": "output", "b": "g2", "name": "dog high", "states": ["high"]}
  ],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}`

func TestAGearboxSpecSelectsARatioPerState(t *testing.T) {
	m := build(t, gearbox)
	if got := m.States(); len(got) != 2 || got[0] != "low" || got[1] != "high" {
		t.Fatalf("states %v, want low then high in order", got)
	}
	for state, want := range map[string]float64{"low": -1.0 / 3.0, "high": -3} {
		sol, ok := m.Solve(state)
		if !ok {
			t.Fatalf("%s: not solvable", state)
		}
		if diff := sol["output"] - want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s: ratio %v, want %v", state, sol["output"], want)
		}
	}
}

func TestACouplingMustNameDeclaredStates(t *testing.T) {
	s, err := Read(strings.NewReader(`{
      "name": "x", "states": ["low"],
      "shafts": [{"id": "a"}, {"id": "b"}],
      "couplings": [{"a": "a", "b": "b", "states": ["ludicrous"]}]
    }`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Build(); err == nil {
		t.Error("a coupling naming an undeclared state should be reported")
	}
}

func TestMalformedStatesAreRejected(t *testing.T) {
	cases := map[string]string{
		"empty name": `{"name":"x","states":[""],"shafts":[{"id":"a"}]}`,
		"duplicate":  `{"name":"x","states":["a","a"],"shafts":[{"id":"a"}]}`,
		"self coupling": `{"name":"x","shafts":[{"id":"a"}],
                          "couplings":[{"a":"a","b":"a"}]}`,
		"unknown shaft": `{"name":"x","shafts":[{"id":"a"}],
                          "couplings":[{"a":"a","b":"ghost"}]}`,
	}
	for what, doc := range cases {
		s, err := Read(strings.NewReader(doc))
		if err != nil {
			continue
		}
		if _, err := s.Build(); err == nil {
			t.Errorf("%s: should have been rejected", what)
		}
	}
}
