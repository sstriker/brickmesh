// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldr"
)

// Nothing is placed in a colour it is not made in.
//
// A driving ring was placed red and a barrel selector black, neither of which
// has ever existed. The colours are read off official models now, and what
// matters is that the two never drift apart again: a part the engine can place
// either has a measured colour or is reported as wearing a stand-in.
func TestEveryPlacedPartWearsAColourItIsMadeIn(t *testing.T) {
	// The rings and catches are the ones that were wrong, so name them.
	for _, c := range []struct {
		part string
		want int
	}{
		{"6539.dat", 7},   // light grey, 8 sightings
		{"18947.dat", 72}, // dark bluish grey, 14
		{"6641.dat", 4},   // red, 8 (and light grey once)
		{"35188.dat", 25}, // orange, 5
		{"18948.dat", 15}, // white, 18
	} {
		got, measured := colourFor(c.part)
		if !measured {
			t.Errorf("%s has no measured colour", c.part)
		}
		if got != c.want {
			t.Errorf("%s placed in colour %d, measured as %d", c.part, got, c.want)
		}
	}

	// And every part the engine can place is either measured or reported. The
	// failure to avoid is a part quietly wearing a colour nobody checked, which
	// is what put a barrel selector in black.
	var quiet []string
	for _, part := range Placeable() {
		if _, measured := colourFor(part); !measured && !standInReported(part) {
			quiet = append(quiet, part)
		}
	}
	if len(quiet) > 0 {
		t.Errorf("%v wear an unmeasured colour and nothing says so", quiet)
	}
}

// standInReported is whether reportStandIns would name this part. It names
// everything not in the table, so the two agree by construction — this is here
// to fail if that ever stops being true.
func standInReported(part string) bool {
	res := &Result{Model: &ldr.Model{}}
	res.Model.Add(part, standIn, geom.Mat3{}, geom.Vec3{}, "")
	reportStandIns(res)
	for _, f := range res.Findings {
		if strings.Contains(f.Detail, part) {
			return true
		}
	}
	return false
}
