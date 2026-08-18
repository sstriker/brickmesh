// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package calc is the functional layer behind one call.
//
// It answers the questions that need no parts library and no search: will this
// gear train turn, what does it turn at, does the loop close, does the box
// hunt. Those compute in under a millisecond from the description alone, which
// is why they can ship as a page long before the structural solver is finished
// — see docs/architecture.md.
//
// JSON in, JSON out, and nothing else in the signature. That is what lets the
// same function serve a command line, a test, and a WebAssembly export without
// any of them knowing about the others.
package calc

import (
	"bytes"
	"encoding/json"
	"fmt"

	"brickmesh/internal/mech"
	"brickmesh/internal/spec"
)

// Finding is one thing the checks have to say.
type Finding struct {
	Level  string `json:"level"` // OK, WARN or FAIL
	Check  string `json:"check"`
	Detail string `json:"detail"`
}

// State is what the mechanism does in one of its states.
type State struct {
	// Name is empty for a mechanism with only one state, which is most of them.
	Name string `json:"name"`
	// Speeds is every shaft's turns per turn of the input, signed. Absent when
	// the state does not resolve to definite speeds.
	Speeds map[string]float64 `json:"speeds,omitempty"`
	// Determined is whether it resolved at all.
	Determined bool `json:"determined"`
}

// Result is the whole answer.
type Result struct {
	Name string `json:"name"`
	// OK is false when any check failed. A mechanism with warnings is still OK:
	// a warning is something to know, not something that stops it turning.
	OK       bool      `json:"ok"`
	Findings []Finding `json:"findings"`
	States   []State   `json:"states"`
	Outputs  []string  `json:"outputs"`
	// Error is set when the description itself could not be read, in which case
	// nothing else is.
	Error string `json:"error,omitempty"`
}

// Check reads a mechanism description and runs every check that needs no parts
// library.
//
// A description that will not parse is an ordinary answer rather than an error:
// the caller is a text box, and someone halfway through typing should be told
// what is wrong with it, not handed a failure.
func Check(description []byte) Result {
	s, err := spec.Read(bytes.NewReader(description))
	if err != nil {
		return Result{Error: err.Error()}
	}
	m, err := s.Build()
	if err != nil {
		return Result{Name: s.Name, Error: err.Error()}
	}

	res := Result{Name: m.Name, OK: true, Outputs: m.Outputs}
	for _, f := range m.RunChecks() {
		if f.Level == "FAIL" {
			res.OK = false
		}
		res.Findings = append(res.Findings, Finding(f))
	}
	if res.Findings == nil {
		res.Findings = []Finding{}
	}

	states := m.States()
	if len(states) == 0 {
		states = []string{""}
	}
	for _, name := range states {
		speeds, ok := m.Solve(name)
		res.States = append(res.States, State{
			Name: name, Speeds: speeds, Determined: ok,
		})
	}
	return res
}

// CheckJSON is Check with the answer already encoded, which is the shape a
// WebAssembly export wants: strings across the boundary, no reflection on the
// far side.
func CheckJSON(description []byte) []byte {
	out, err := json.Marshal(Check(description))
	if err != nil {
		// Result is plain data, so this cannot happen — but a caller across a
		// language boundary still deserves valid JSON rather than nothing.
		return []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return out
}

// Ensure the two Finding types stay assignable: Check converts between them
// directly, and a field added to one without the other would stop compiling
// here rather than silently dropping out of the JSON.
var _ = func() mech.Finding { return mech.Finding(Finding{}) }
