// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/clutch"
	"github.com/sstriker/brickmesh/internal/ldr"
)

// The geometry is checked with the shift thrown, not only where it is parked.
//
// Five faults in a row got past the checks because they only ever looked at the
// drawn model, and a gearbox is not in one state — that is the point of it. The
// parts a shift moves are exactly the parts a shift is about.
func TestEveryGearboxIsCheckedInEveryState(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{
		"gearbox-2-speed", "gearbox-early-system", "gearbox-4-speed-compound",
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := os.ReadFile(filepath.Join("..", "..", "examples", name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			res, err := Run(context.Background(), build(t, string(doc)), deps,
				Options{Restarts: 8, Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			var said string
			for _, f := range res.Findings {
				if f.Check == "clearance" && strings.Contains(f.Detail, "every state") {
					said = f.Detail
				}
				if f.Level == "FAIL" {
					t.Errorf("%s: %s", f.Check, f.Detail)
				}
			}
			if said == "" {
				t.Fatal("nothing checked the shifted positions at all; a check " +
					"that quietly does nothing is the fault it exists to catch")
			}
			// It has to have actually compared something.
			if strings.Contains(said, " 0 pair(s) ") {
				t.Errorf("no pairs were compared: %s", said)
			}
		})
	}
}

// A ball driven into the barrel it rides is not an ordinary fit.
//
// It read as one: the drum shared a class with the catch, so the ball's licence
// to sit in the catch's own hole covered sitting inside the drum as well. With
// the drum given a class of its own, putting it back on an arbitrary phase and
// leaving it unturned reports the ball inside it, in every state, by 8 LDU.
func TestABallInsideTheDrumIsNotExcused(t *testing.T) {
	if mayBeInside(partNamed("6628.dat"), partNamed("4158.dat")) {
		t.Error("a ball inside a barrel selector is excused; it is a collision, " +
			"and excusing it is what let one ride through the drum unremarked")
	}
	if !mayBeInside(partNamed("6628.dat"), partNamed("4159.dat")) {
		t.Error("a ball inside its own catch is refused; that is where its " +
			"shank lives")
	}
}

// partNamed is a part with nothing but its name, which is all classOf reads.
func partNamed(name string) ldr.Part { return ldr.Part{Name: name} }

// Every coupling a state claims is one the ring can actually grip.
//
// The other half of checking a gearbox where it moves. Clearance catches parts
// that overlap; this catches the opposite, which is what a clutch gear turned
// back to front looks like — nothing overlaps anywhere, and a ratio is solved
// through a coupling that in metal would slip.
func TestEveryStateActuallyEngagesWhatItClaims(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{
		"gearbox-2-speed", "gearbox-first-system", "gearbox-early-system",
		"gearbox-4-speed-compound",
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := os.ReadFile(filepath.Join("..", "..", "examples", name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			res, err := Run(context.Background(), build(t, string(doc)), deps,
				Options{Restarts: 8, Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			var said string
			for _, f := range res.Findings {
				if f.Check != "engagement" {
					continue
				}
				if f.Level == "FAIL" {
					t.Errorf("%s", f.Detail)
				}
				said = f.Detail
			}
			if said == "" {
				t.Error("nothing checked that the couplings engage; a check that " +
					"quietly does nothing is the fault it exists to catch")
			}
		})
	}
}

// And it is asked with the fit that system was measured at.
//
// The first system's dogs meet the gear face to face, so any slack at all
// separates them: swept at the blanket quarter-LDU tolerance it engages at no
// distance whatever. Asked that way, this check called a perfectly good
// two-speed broken in both its states.
func TestEngagementIsAskedAtEachSystemsOwnFit(t *testing.T) {
	for _, s := range clutch.Systems {
		if s.Ring == "" {
			continue
		}
		if s.EngageFit < 0 {
			t.Errorf("%s has a negative fit", s.Name)
		}
	}
	if clutch.First.EngageFit != 0 {
		t.Errorf("the first system's fit is %v; it was measured at nothing "+
			"forgiven, and forgiving a quarter LDU loses its engagement "+
			"entirely", clutch.First.EngageFit)
	}
}
