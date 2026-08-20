// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/collide"
	"github.com/sstriker/brickmesh/internal/interfere"
	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/spec"
)

// This test exists because the checks did not catch what a picture did.
//
// Every check before it looked at one relationship: does this ring clear its
// gear, does this joiner clear the structure. None of them asked the blunt
// question — is any part inside any other part — and so a model came out with
// beams drawn straight through the gears, and nothing said a word.
//
// The blunt question is cheap. Every pair, every model in examples/.
func TestNoPartIsInsideAnother(t *testing.T) {
	deps := requireLibraries(t)
	for _, path := range examples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			s, err := spec.Read(strings.NewReader(string(doc)))
			if err != nil {
				t.Fatal(err)
			}
			m, err := s.Build()
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

			for i := 0; i < len(res.Model.Parts); i++ {
				for j := i + 1; j < len(res.Model.Parts); j++ {
					a, b := res.Model.Parts[i], res.Model.Parts[j]
					if mayBeInside(a, b) {
						continue
					}
					overlap, err := boxOverlap(deps, a, b)
					if err != nil || overlap < touchTolerance {
						continue // apart, or meeting face to face, which is fine
					}
					if !intersects(t, deps, a, b) {
						continue
					}
					t.Errorf("%s at %+v is inside %s at %+v, by %.1f LDU",
						a.Name, a.Pos, b.Name, b.Pos, overlap)
				}
			}
		})
	}
}

func intersects(t *testing.T, deps Deps, a, b ldr.Part) bool {
	t.Helper()
	ma, err := interfere.MeshFor(deps.Lib, a.Name)
	if err != nil {
		return false
	}
	mb, err := interfere.MeshFor(deps.Lib, b.Name)
	if err != nil {
		return false
	}
	return collide.Intersects(ma, collide.Transform{Rot: a.Rot, Pos: a.Pos},
		mb, collide.Transform{Rot: b.Rot, Pos: b.Pos})
}

func boxOverlap(deps Deps, a, b ldr.Part) (float64, error) {
	alo, ahi, err := placedBox(deps, a)
	if err != nil {
		return 0, err
	}
	blo, bhi, err := placedBox(deps, b)
	if err != nil {
		return 0, err
	}
	return overlapOf(alo, ahi, blo, bhi), nil
}

func examples(t *testing.T) []string {
	t.Helper()
	found, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.json"))
	if err != nil || len(found) == 0 {
		t.Fatalf("no examples found: %v", err)
	}
	return found
}

// Bearing every shaft is not the same as holding together, and holding together
// is not the same as not folding up. All three were separate warnings for a long
// time; this is the assertion that they stay gone.
func TestEveryExampleHoldsTogetherAndDoesNotHinge(t *testing.T) {
	deps := requireLibraries(t)
	for _, path := range examples(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			s, err := spec.Read(strings.NewReader(string(doc)))
			if err != nil {
				t.Fatal(err)
			}
			m, err := s.Build()
			if err != nil {
				t.Fatal(err)
			}
			res, err := Run(context.Background(), m, deps, Options{Restarts: 8, Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			hinged := false
			for _, f := range res.Findings {
				if f.Level == "OK" {
					continue
				}
				switch f.Check {
				case "rigidity":
					// Nothing is allowed to hinge.
					//
					// Twice now this has been true for the wrong reason. It
					// passed while the search was bolting beams twice to one
					// bearing, which lowers Grubler's count and removes no
					// freedom. Fixing that left two examples genuinely hinging,
					// because a straight liftarm reaches one bearing wall or the
					// other and never both — see
					// TestNoStraightBeamTiesTwoWallsOnAShaftLine. It passes now
					// because the inventory has the part that turns a corner.
					if !hingeIsKnown(filepath.Base(path)) {
						t.Errorf("%s: %s", f.Check, f.Detail)
					}
					hinged = true
				case "connectivity", "structure", "framing", "clearance", "turning":
					t.Errorf("%s: %s", f.Check, f.Detail)
				}
			}
			// And the list does not get to go stale: when the inventory grows a
			// part that can close these frames, this fails and says so.
			if hingeIsKnown(filepath.Base(path)) && !hinged {
				t.Errorf("%s no longer hinges. Take it out of knownHinges, and "+
					"see whether TestNoStraightBeamTiesTwoWallsOnAShaftLine "+
					"still holds", filepath.Base(path))
			}
		})
	}
}

// knownHinges are the examples whose frame cannot be closed with the parts the
// structural search has.
//
// Empty, and kept rather than deleted because it was not empty and the reason
// it is now is a part in the inventory, not a fact about the world. See
// PLAN.md M2 and docs/findings.md.
var knownHinges = map[string]bool{}

func hingeIsKnown(name string) bool { return knownHinges[name] }
