// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package extract

import (
	"math"
	"os"
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/part"
	"brickmesh/internal/shadow"
)

func realLibraries(t *testing.T) (*shadow.Library, *ldraw.Library) {
	t.Helper()
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	return shadow.Open(root), ldraw.New("")
}

// The check that matters most, because it is the one thing already known to be
// right: the structural search has been synthesising a beam's holes from its
// hole count for as long as there has been a search, and those positions are
// what every model that has ever been built stands on.
//
// So the holes found by walking the part's subfiles have to be exactly those.
// Not close — the same set. If this passes, the walk is reading real geometry
// correctly, and the parts it finds holes on that were never in the inventory
// can be trusted the same way.
func TestWalkedHolesAreTheHolesTheSearchAssumed(t *testing.T) {
	shadowLib, parts := realLibraries(t)
	for _, b := range part.Beams {
		e := EntryForWith(shadowLib, parts, b.Part)
		if e == nil {
			t.Errorf("%s: no ports at all", b.Part)
			continue
		}
		if len(e.Pins) != 0 {
			t.Errorf("%s: a liftarm has no pins, got %d", b.Part, len(e.Pins))
		}
		want := part.HoleOffsets(b.Holes)
		if len(e.Holes) != len(want) {
			t.Errorf("%s: %d holes walked, %d assumed", b.Part, len(e.Holes), len(want))
			continue
		}
		for _, w := range want {
			if !hasHole(e.Holes, w) {
				t.Errorf("%s: nothing walked at %v, which the search assumes is "+
					"a hole", b.Part, w)
			}
		}
		// And one axis for the lot, which is the assumption that made a single
		// axis per part good enough for straight liftarms.
		for _, h := range e.Holes {
			if math.Abs(h[4]) < 0.999 {
				t.Errorf("%s: a hole faces %v %v %v, not along the beam's own "+
					"hole axis", b.Part, h[3], h[4], h[5])
			}
			if h[6] != 0 {
				t.Errorf("%s: a liftarm hole came out as a cross hole", b.Part)
			}
		}
	}
}

// The reason for all of this: a part whose holes do not all face the same way.
//
// 6536 is the axle-and-pin connector perpendicular — a cross hole on one axis
// and a round hole on another, which is what ties two bearing walls together
// and what nothing in the straight-liftarm inventory can do. Reading its own
// shadow file gives one of the two.
func TestAPerpendicularConnectorHasHolesOnTwoAxes(t *testing.T) {
	shadowLib, parts := realLibraries(t)
	e := EntryForWith(shadowLib, parts, "6536.dat")
	if e == nil {
		t.Fatal("6536 has no ports")
	}
	axes := map[geom.Vec3]bool{}
	for _, h := range e.Holes {
		axes[geom.Vec3{X: math.Abs(h[3]), Y: math.Abs(h[4]), Z: math.Abs(h[5])}] = true
	}
	if len(axes) < 2 {
		t.Errorf("6536 came out with holes on %d axis, want 2: %+v", len(axes), e.Holes)
	}
	if own := EntryFor(shadowLib, "6536.dat"); own != nil &&
		len(own.Holes)+len(own.Pins) >= len(e.Holes)+len(e.Pins) {
		t.Error("walking the subfiles found no more than the part's own shadow " +
			"file, so this test is not measuring what it says")
	}
}

// Walking must not invent ports for a part whose shadow file already describes
// it completely. A gear is one hole and four pin sockets and stays that way.
func TestAPartThatDescribesItselfIsUnchanged(t *testing.T) {
	shadowLib, parts := realLibraries(t)
	for _, name := range []string{"3648b.dat", "4019.dat"} {
		own := EntryFor(shadowLib, name)
		walked := EntryForWith(shadowLib, parts, name)
		if own == nil || walked == nil {
			t.Fatalf("%s: %v %v", name, own, walked)
		}
		if len(own.Holes) != len(walked.Holes) || len(own.Pins) != len(walked.Pins) {
			t.Errorf("%s: %d/%d ports became %d/%d",
				name, len(own.Holes), len(own.Pins),
				len(walked.Holes), len(walked.Pins))
		}
	}
}

// Without the parts library it has to behave exactly as it did.
func TestWithoutThePartsLibraryNothingChanges(t *testing.T) {
	shadowLib, _ := realLibraries(t)
	for _, name := range []string{"41239.dat", "6536.dat", "3648b.dat"} {
		own := EntryFor(shadowLib, name)
		walked := EntryForWith(shadowLib, nil, name)
		if (own == nil) != (walked == nil) {
			t.Fatalf("%s: %v vs %v", name, own, walked)
		}
		if own != nil && len(own.Holes) != len(walked.Holes) {
			t.Errorf("%s: %d holes became %d with no library to walk",
				name, len(own.Holes), len(walked.Holes))
		}
	}
}

func hasHole(holes []Port, at geom.Vec3) bool {
	for _, h := range holes {
		if math.Abs(h[0]-at.X) < 1e-6 && math.Abs(h[1]-at.Y) < 1e-6 &&
			math.Abs(h[2]-at.Z) < 1e-6 {
			return true
		}
	}
	return false
}
