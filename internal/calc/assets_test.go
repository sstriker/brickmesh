// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package calc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"brickmesh/internal/extract"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/pipeline"
	"brickmesh/internal/shadow"
)

// The site ships tier 1. Every part the engine can place has to have geometry
// in it, or the page draws a model with pieces missing and — far worse — the
// clearance sweep skips those pieces and reports that nothing collides.
//
// This is checked rather than assumed because it was wrong: gears are titled
// "Technic Gear ..." and grade as tier 2, so the deployed site had geometry for
// no gear at all, and 3647 and 32270 have no shadow file so they were dropped
// at every tier. Both were invisible.
func TestTheShippedTierHasEveryPartTheEnginePlaces(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	lib := ldraw.New("")
	parts := publish(t, lib, extract.Ports{Lib: shadow.Open(root), Geom: lib}, "")

	for _, name := range pipeline.Placeable() {
		if _, err := parts.shapes.Geometry(name); err != nil {
			t.Errorf("%s is a part the engine places, and the browser has no "+
				"geometry for it: %v", name, err)
		}
	}
}

// And the same thing from the other end: build every example the way the site
// does, and check that nothing was placed that cannot be drawn or swept.
func TestNoExampleUsesAPartTheBrowserCannotSee(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	lib := ldraw.New("")
	parts := publish(t, lib, extract.Ports{Lib: shadow.Open(root), Geom: lib}, "")

	specs, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.json"))
	if err != nil || len(specs) == 0 {
		t.Fatalf("no examples to check: %v", err)
	}
	for _, spec := range specs {
		t.Run(strings.TrimSuffix(filepath.Base(spec), ".json"), func(t *testing.T) {
			doc, err := os.ReadFile(spec)
			if err != nil {
				t.Fatal(err)
			}
			built := parts.Build(context.Background(), doc, false)
			if built.Error != "" {
				t.Fatal(built.Error)
			}
			for _, f := range built.Findings {
				if f.Check == "clearance" && strings.Contains(f.Detail, "no geometry for") {
					t.Errorf("%s", f.Detail)
				}
			}
			// The renderer sees the same parts the sweep does, so an empty
			// buffer here means the same gap by another route.
			if len(parts.Draw()) == 0 {
				t.Error("nothing to draw")
			}
		})
	}
}

// What the site did before this was fixed, reproduced deliberately: publish
// everything except the gears and build a gearbox against it.
//
// The point is not that the answer is worse. It is that the answer was
// confident. The sweep skipped every part it could not measure and reported
// that nothing shared space, which reads exactly like a model that was checked
// and found clear.
func TestAMissingMeshIsReportedRatherThanSkipped(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	lib := ldraw.New("")
	ports := extract.Ports{Lib: shadow.Open(root), Geom: lib}

	var gearless []string
	gears := map[string]bool{}
	for _, name := range pipeline.GearParts {
		gears[name] = true
	}
	for _, name := range pipeline.Placeable() {
		if !gears[name] {
			gearless = append(gearless, name)
		}
	}
	rawCatalog, rawMeshes := publishedFrom(t, lib, ports, gearless)
	parts, err := Load(rawCatalog, rawMeshes)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := os.ReadFile(filepath.Join("..", "..", "examples", "reduction.json"))
	if err != nil {
		t.Fatal(err)
	}
	built := parts.Build(context.Background(), doc, false)
	if built.Error != "" {
		t.Fatal(built.Error)
	}

	var said, cleared bool
	for _, f := range built.Findings {
		if f.Check != "clearance" {
			continue
		}
		if strings.Contains(f.Detail, "no geometry for") {
			said = true
			if !strings.Contains(f.Detail, "3648b.dat") {
				t.Errorf("the report does not name the missing gear: %s", f.Detail)
			}
		}
		if strings.Contains(f.Detail, "share space") && f.Level == "OK" {
			cleared = true
		}
	}
	if !said {
		t.Error("a model was built with no gear geometry and the clearance " +
			"check did not say so")
	}
	if cleared {
		t.Error("the clearance check gave a clear verdict on a model it could " +
			"not fully measure")
	}
}
