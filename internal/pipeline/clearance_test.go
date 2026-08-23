// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/synth"
)

func TestACatchIsSitedByItsBodyNotByLines(t *testing.T) {
	// clearOfOtherShafts consulted shaft LINES only. A catch with a body can be
	// sent down a side no line is near, so it now tests the catch itself
	// against what the layout has already settled. The check being wired in at
	// all is the thing worth pinning: it was written, left uncalled, and only
	// staticcheck noticed.
	deps := requireLibraries(t)
	res := runSpec(t, deps, filepath.Join("..", "..", "examples",
		"gearbox-2-speed.json"))
	if len(res.ringSites) == 0 {
		t.Fatal("a two-speed with no ring site")
	}
	site := res.ringSites[0]
	place, ok := res.Layout.Place[site.station.Shaft]
	if !ok {
		t.Fatal("the ring's shaft is not in the layout")
	}
	d := place.Direction.Unit()
	at := place.Point.Scale(synth.HalfStud).
		Add(d.Scale(site.engaged * synth.HalfStud))

	// Every way out of the shaft, with the catch's own body consulted. At
	// least one has to be free, or the mechanism that was just built could not
	// have been.
	free := 0
	for _, out := range []geom.Vec3{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}, {Z: 1}, {Z: -1}} {
		if math.Abs(out.Dot(d)) > 1e-6 {
			continue
		}
		if !catchFoulsTheGears(context.Background(), deps, res, site.system,
			at, d, out, place) {
			free++
		}
	}
	if free == 0 {
		t.Error("the catch's body fouls every way out of its shaft, yet the " +
			"mechanism it belongs to was placed")
	}
}
