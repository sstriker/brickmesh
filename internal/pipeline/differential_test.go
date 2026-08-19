// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/spec"
)

// A differential has to be in the model, not only in the arithmetic.
//
// It was modelled by the functional layer from the beginning and placed by
// nothing, so a subtractor came out as one axle inside a frame — and every
// check passed, because every check was about the kinematics. It took opening
// it in LDCad and seeing a single thing move.
func TestASubtractorHasADifferentialInIt(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "subtractor.json"))

	var housing, left, right int
	for _, p := range res.Model.Parts {
		switch {
		case p.Name == DiffPart:
			housing++
		case strings.Contains(p.Label, "shaft 'left'"):
			left++
		case strings.Contains(p.Label, "shaft 'right'"):
			right++
		}
	}
	if housing != 1 {
		t.Errorf("got %d differential housings, want 1", housing)
	}
	if left == 0 || right == 0 {
		t.Errorf("the two outputs got %d and %d parts; a differential whose "+
			"outputs are not there is a housing on its own", left, right)
	}
}

// And the outputs must be separate shafts, or it is not a differential at all —
// it is one axle with a decoration in the middle.
func TestADifferentialsOutputsTurnSeparately(t *testing.T) {
	deps := requireLibraries(t)
	// Animated, since that is when parts are put into groups — and a group per
	// shaft is exactly the claim being made here.
	res := runSpecAnimated(t, deps,
		filepath.Join("..", "..", "examples", "subtractor.json"))

	groups := map[string]bool{}
	for _, p := range res.Model.Parts {
		if p.Group != "" {
			groups[p.Group] = true
		}
	}
	for _, want := range []string{"shaft_case", "shaft_left", "shaft_right"} {
		if !groups[want] {
			t.Errorf("no %s group; the outputs are not free to turn against "+
				"each other, which is the whole of what a differential does", want)
		}
	}
}

// The axles stop at the housing rather than running through it. Getting the
// sign of that face wrong put a four stud axle straight through the middle,
// which the clearance sweep let past because an axle may be inside anything.
func TestTheOutputAxlesStopAtTheHousing(t *testing.T) {
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples", "subtractor.json"))

	var centre float64
	found := false
	for _, p := range res.Model.Parts {
		if p.Name == DiffPart {
			centre, found = p.Pos.X, true
		}
	}
	if !found {
		t.Fatal("no housing")
	}
	for _, p := range res.Model.Parts {
		if !strings.Contains(p.Label, "axle") {
			continue
		}
		g, err := deps.Lib.Geometry(p.Name)
		if err != nil {
			t.Fatal(err)
		}
		lo, hi := g.BBox()
		half := (hi.X - lo.X) / 2
		near := p.Pos.X - half
		far := p.Pos.X + half
		if near < centre-DiffHalf+1e-6 && far > centre+DiffHalf-1e-6 {
			t.Errorf("%s spans %.0f..%.0f, straight through a housing that "+
				"occupies %.0f..%.0f", p.Name, near, far,
				centre-DiffHalf, centre+DiffHalf)
		}
	}
}

func runSpecAnimated(t *testing.T, deps Deps, path string) *Result {
	t.Helper()
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := spec.Read(strings.NewReader(string(doc)))
	if err != nil {
		t.Fatal(err)
	}
	m, err := sp.Build()
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), m, deps,
		Options{Restarts: 8, Seed: 1, Animate: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model == nil {
		t.Fatal("no model")
	}
	return res
}
