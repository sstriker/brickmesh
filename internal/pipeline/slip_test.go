// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/clutch"
	"github.com/sstriker/brickmesh/internal/spec"
)

const protectedReduction = `{
  "name": "protected",
  "shafts": [{"id": "input", "bearings": 2}, {"id": "output", "bearings": 2}],
  "meshes": [{"a": "input", "b": "output", "teeth_a": 8, "teeth_b": 24}],
  "slip_clutches": [{"shaft": "%s"}],
  "inputs": [{"shaft": "input", "speed": 1.0}],
  "outputs": ["output"]
}`

// A slip clutch is the other kind of clutch, and it shares only the name.
//
// A driving ring's clutch gear has dogs and either grips or does not. This is a
// 24-tooth gear with a friction centre that gives way above a force, so nothing
// downstream of it can be loaded harder than that however hard the input is
// driven. internal/clutch excludes 24 from the shiftable counts for exactly
// that reason, at length; this is the part that reason is about.
func TestASlipClutchPutsTheSlippingGearOnTheShaft(t *testing.T) {
	deps := requireLibraries(t)
	res := runJSON(t, deps, strings.Replace(protectedReduction, "%s", "output", 1))

	var found, plain bool
	for _, p := range res.Model.Parts {
		switch p.Name {
		case SlipPart:
			found = true
		case GearParts[24]:
			plain = true
		}
	}
	if !found {
		t.Errorf("no %s in the model: the 24t on a shaft with a slip clutch is "+
			"the slipping one", SlipPart)
	}
	if plain {
		t.Errorf("a plain %s is in the model as well; the slip clutch replaces "+
			"it rather than joining it", GearParts[24])
	}

	// And the report says what it protects, and owns up to the figure.
	var said bool
	for _, f := range res.Findings {
		if f.Check != "slip clutch" {
			continue
		}
		said = true
		if f.Level != "OK" {
			t.Errorf("%s", f.Detail)
		}
		if !strings.Contains(f.Detail, "unverified") {
			t.Errorf("the slip torque is an estimate and the report does not "+
				"say so: %s", f.Detail)
		}
	}
	if !said {
		t.Error("a slip clutch was fitted and the report never mentioned it")
	}
}

// It is made in 24 teeth and no other size, so a shaft without one is not a
// place it can go. Saying so beats placing a plain gear and leaving the reader
// to notice that nothing slips.
func TestASlipClutchWithNowhereToSitIsRefused(t *testing.T) {
	deps := requireLibraries(t)
	res := runJSON(t, deps, strings.Replace(protectedReduction, "%s", "input", 1))

	var refused bool
	for _, f := range res.Findings {
		if f.Check == "slip clutch" && f.Level == "FAIL" {
			refused = true
		}
	}
	if !refused {
		t.Error("a slip clutch was fitted to a shaft carrying only an 8t and " +
			"nothing objected")
	}
}

// The two clutches must not be confused: 24 teeth cannot be dog-shifted, and
// the slipping gear is not a thing a driving ring can grip.
func TestTheTwoKindsOfClutchStaySeparate(t *testing.T) {
	// Both kinds exist at 24 teeth, which is the whole reason to check. 2471
	// has eight dogs and 2473a engages them; 76019 and 76244 are torque
	// limiters whose centre gives way, and they engage nothing. A 24-tooth
	// station has to get whichever one the mechanism asked for.
	if !clutch.Shiftable(24) {
		t.Error("24 teeth reads as unshiftable, but 2471 has dogs and 2473a " +
			"engages them")
	}
	for _, s := range clutch.Systems {
		for teeth, name := range s.Gears {
			if name == SlipPart {
				t.Errorf("%s is listed as %s's %dt clutch gear, and it is the "+
					"other kind of clutch entirely", name, s.Name, teeth)
			}
		}
	}
}

func runJSON(t *testing.T, deps Deps, doc string) *Result {
	t.Helper()
	sp, err := spec.Read(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	m, err := sp.Build()
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), m, deps, Options{Restarts: 8, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Model == nil {
		t.Fatal("no model")
	}
	return res
}
