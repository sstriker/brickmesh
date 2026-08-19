// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package synth

import (
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
)

// Two bearing walls on one shaft line cannot be tied together by any straight
// liftarm, and this is why the subtractor is reported as hinging.
//
// The argument is short. A shaft passes through a bearing, so the bearing's
// holes face along the shaft. Every hole of a straight liftarm faces the same
// way, so the liftarm lies across the shaft and all its holes share one
// coordinate along it. A pin joins two holes only if they are on one line and
// within two studs of each other. So a liftarm can reach one wall or the other
// and never both, whatever its length and wherever it is put.
//
// Written as a test rather than left as prose because it is the reason a
// warning stands, and a reason for a warning should fail loudly on the day it
// stops being true — which is the day a part with holes on two axes joins the
// inventory.
func TestNoStraightBeamTiesTwoWallsOnAShaftLine(t *testing.T) {
	s := searcher(t)

	// The fixture beam's holes run along Y, so a wall carrying a Y shaft is one
	// of these placed anywhere; the two walls sit at either end of it.
	left := map[geom.Vec3]bool{}
	right := map[geom.Vec3]bool{}
	for _, z := range []float64{-part.Stud, 0, part.Stud} {
		left[geom.Vec3{X: 0, Y: -4 * part.Stud, Z: z}] = true
		right[geom.Vec3{X: 0, Y: 4 * part.Stud, Z: z}] = true
	}

	got, err := s.ConnectorsBetween(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("%d straight beams span two walls 8 studs apart on their own "+
			"hole axis; the inventory has grown a part that can do this and "+
			"the hinge warnings should be revisited", len(got))
	}

	// The control, so this is not passing because ConnectorsBetween never
	// returns anything: the same two hole sets a pin's reach apart do connect.
	near := map[geom.Vec3]bool{{X: 0, Y: 0, Z: 0}: true}
	far := map[geom.Vec3]bool{{X: 0, Y: 0, Z: 2 * part.Stud}: true}
	if got, err := s.ConnectorsBetween(near, far); err != nil {
		t.Fatal(err)
	} else if len(got) == 0 {
		t.Fatal("nothing spans two holes two studs apart, so the test above " +
			"proves nothing")
	}
}
