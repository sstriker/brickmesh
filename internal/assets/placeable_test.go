// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"testing"

	"github.com/sstriker/brickmesh/internal/extract"
)

func TestAPlaceablePartOutsideTheTierIsPulledIn(t *testing.T) {
	records := []extract.Record{
		{ID: "3648b", Title: "Technic Gear 24 Tooth", Tier: 2,
			Holes: []extract.Port{{}}},
		{ID: "32523", Title: "Technic Liftarm 3", Tier: 1},
	}
	got, added := WithPlaceable(records, []string{"3648b.dat", "32523.dat"})
	if len(added) != 0 {
		t.Errorf("added %v, but both were already there", added)
	}
	for _, r := range got {
		if r.Tier != 1 {
			t.Errorf("%s is tier %d after being pulled in, want 1", r.ID, r.Tier)
		}
	}
	// Pulling it in must not cost it the ports it had.
	for _, r := range got {
		if r.ID == "3648b" && len(r.Holes) != 1 {
			t.Errorf("3648b lost its ports: %+v", r)
		}
	}
}

// 3647 and 32270 have no shadow file, so they reach here absent however high
// the tier goes. They still need to ship, because triangles are what the sweep
// and the renderer read.
func TestAPartWithNoShadowFileStillShips(t *testing.T) {
	got, added := WithPlaceable(nil, []string{"3647.dat"})
	if len(added) != 1 || added[0] != "3647" {
		t.Fatalf("added %v, want [3647]", added)
	}
	if len(got) != 1 || got[0].ID != "3647" || got[0].Tier != 1 {
		t.Fatalf("got %+v", got)
	}
	// Empty rather than nil: the writer walks these, and a part with no ports
	// is a fact about the shadow library, not a hole in the record.
	if got[0].Holes == nil || got[0].Pins == nil {
		t.Errorf("ports are nil, not empty: %+v", got[0])
	}
}

// The control: something the engine never places is left exactly as it was.
func TestANonPlaceablePartIsUntouched(t *testing.T) {
	records := []extract.Record{{ID: "3001", Title: "Brick 2 x 4", Tier: 3}}
	got, added := WithPlaceable(records, []string{"3647.dat"})
	if len(added) != 1 {
		t.Fatalf("added %v", added)
	}
	for _, r := range got {
		if r.ID == "3001" && r.Tier != 3 {
			t.Errorf("3001 became tier %d; it is not a part the engine places", r.Tier)
		}
	}
}
