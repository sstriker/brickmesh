// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"path/filepath"
	"strings"
	"testing"
)

// The force between meshed teeth has to have somewhere to go.
//
// Meshing gears push their shafts apart, along the line of centres, in
// proportion to the torque. It is the load that decides whether a gearbox holds
// its mesh or spreads and skips, and nothing here had ever asked about it: a
// frame can hold together and refuse to fold while still letting two shafts
// drift apart.
//
// Both shafts borne by one part is what a wall is for — the load is taken
// inside the beam, between two holes, and no pin sees it. This is also the
// regression guard on the bearing planes: go back to a bearing per shaft end
// and the pairs stop sharing a part.
func TestEveryGearPairIsBorneByOnePart(t *testing.T) {
	deps := requireLibraries(t)
	for _, name := range []string{"reduction", "gearbox-2-speed",
		"gearbox-3-speed-compound"} {
		t.Run(name, func(t *testing.T) {
			res := runSpec(t, deps, filepath.Join("..", "..", "examples", name+".json"))
			var said bool
			for _, f := range res.Findings {
				if f.Check != "load path" {
					continue
				}
				said = true
				if f.Level != "OK" {
					t.Errorf("%s", f.Detail)
				}
				if !strings.Contains(f.Detail, "borne by one part") {
					t.Errorf("the separating force crosses joints: %s", f.Detail)
				}
			}
			if !said {
				t.Error("no load path finding at all; the check did not run")
			}
		})
	}
}

// And the measure itself, since the finding above is only as good as it.
func TestShortestHopsCountsJoints(t *testing.T) {
	// 0 - 1 - 2 - 3, and 4 off on its own.
	adj := [][]int{{1}, {0, 2}, {1, 3}, {2}, {}}

	for _, c := range []struct {
		name     string
		from, to []int
		want     int
	}{
		{"one part bears both", []int{1}, []int{1}, 0},
		{"sharing a part among several", []int{0, 2}, []int{2, 3}, 0},
		{"neighbours", []int{0}, []int{1}, 1},
		{"across the chain", []int{0}, []int{3}, 3},
		{"nothing joins them", []int{0}, []int{4}, -1},
	} {
		if got := shortestHops(adj, c.from, c.to); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// A shaft nothing bears is the case worth shouting about: the report has to say
// the force has nowhere to go rather than quietly counting zero pairs.
func TestAnUnborneShaftIsAFailure(t *testing.T) {
	adj := [][]int{{}, {}}
	if got := shortestHops(adj, nil, []int{0}); got != -1 {
		t.Errorf("nothing bears one side, so there is no path; got %d", got)
	}
}
