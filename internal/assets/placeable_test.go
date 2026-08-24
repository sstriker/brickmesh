// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package assets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/extract"
)

func TestAPlaceablePartOutsideTheTierIsPulledIn(t *testing.T) {
	records := []extract.Record{
		{ID: "3648b", Title: "Technic Gear 24 Tooth", Tier: 2,
			Holes: []extract.Port{{}}},
		{ID: "32523", Title: "Technic Liftarm 3", Tier: 1},
	}
	got, added := WithPlaceable(records, []string{"3648b.dat", "32523.dat"}, nil)
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
	got, added := WithPlaceable(nil, []string{"3647.dat"}, nil)
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
	got, added := WithPlaceable(records, []string{"3647.dat"}, nil)
	if len(added) != 1 {
		t.Fatalf("added %v", added)
	}
	for _, r := range got {
		if r.ID == "3001" && r.Tier != 3 {
			t.Errorf("3001 became tier %d; it is not a part the engine places", r.Tier)
		}
	}
}

// A part the engine places gets its real name when the library has one.
//
// It used to get a stub, and the stub cost more than tidiness: tooth counts are
// read out of titles, so a 24-tooth gear published as "Technic (placed by
// brickmesh)" reads as having no teeth. Five of the twenty-two parts arriving
// this way are gears.
func TestAPlacedPartKeepsItsName(t *testing.T) {
	titles := func(name string) (string, error) {
		if name == "2471.dat" {
			return "Technic Gear 24 Tooth with Clutch on Both Sides", nil
		}
		return "", errNoSuchPart
	}
	got, added := WithPlaceable(nil, []string{"2471.dat", "9999.dat"}, titles)
	if len(added) != 2 {
		t.Fatalf("added %v, want both", added)
	}
	var gear, unknown string
	for _, r := range got {
		switch r.ID {
		case "2471":
			gear = r.Title
		case "9999":
			unknown = r.Title
		}
	}
	if !strings.Contains(gear, "24 Tooth") {
		t.Errorf("2471 is published as %q; its tooth count has to be in there, "+
			"because that is where a tooth count is read from", gear)
	}
	if unknown == "" || strings.Contains(unknown, "Tooth") {
		t.Errorf("a part the library does not have is published as %q; it should "+
			"fall back to the stub rather than invent something", unknown)
	}
}

var errNoSuchPart = fmt.Errorf("no such part")
