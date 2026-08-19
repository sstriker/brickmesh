// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/collide"
	"brickmesh/internal/interfere"
	"brickmesh/internal/ldr"
	"brickmesh/internal/spec"
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
					// Two of the examples are a shaft line with a bearing wall
					// at each end and nothing else to tie the walls together.
					// Nothing in the inventory can: every hole of a straight
					// liftarm faces one way, so a liftarm reaches one wall or
					// the other and never both. That is proved, with a control,
					// by TestNoStraightBeamTiesTwoWallsOnAShaftLine in synth.
					//
					// This used to pass, and it passed falsely. Mobility is a
					// count that does not know which parts a joint is between,
					// so the search satisfied it by bolting a beam twice to one
					// wall — which lowers the number and removes no freedom.
					// Fixing the search to bridge two bodies took the false
					// verdict away and left the true one.
					if !hingeIsKnown(filepath.Base(path)) {
						t.Errorf("%s: %s", f.Check, f.Detail)
					}
					hinged = true
				case "connectivity", "structure", "framing", "clearance":
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
// structural search has. See PLAN.md M2 and docs/findings.md.
var knownHinges = map[string]bool{
	"subtractor.json":               true,
	"gearbox-3-speed-compound.json": true,
}

func hingeIsKnown(name string) bool { return knownHinges[name] }
