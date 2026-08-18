// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package rigidity

import (
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
)

// A stand-in for the shadow library: every beam's holes run along Y, which is
// how a liftarm lying in the XZ plane behaves. No download, no fixture.
type holesAlongY struct{}

func (holesAlongY) RotationAxis(string) (geom.Vec3, string, bool) {
	return geom.Vec3{Y: 1}, "test", true
}

var inventory = []part.Beam{{Part: "beam3", Holes: 3}, {Part: "beam5", Holes: 5}}

// Rotation 0 is the identity, so a beam's holes run along Z, one stud apart,
// centered on the origin.
func beam(name string, origin geom.Vec3) part.Placed {
	return part.Placed{Part: name, Rot: 0, Origin: origin}
}

func TestHoleOffsetsAreCenteredAndOneStudApart(t *testing.T) {
	got := part.HoleOffsets(3)
	want := []float64{-part.Stud, 0, part.Stud}
	if len(got) != 3 {
		t.Fatalf("got %d offsets, want 3", len(got))
	}
	for i, w := range want {
		if got[i].Z != w {
			t.Errorf("offset %d at Z=%v, want %v", i, got[i].Z, w)
		}
	}
}

func TestTwoBeamsSharingOneHoleAreAHinge(t *testing.T) {
	// Offset by two studs along Z: their end holes coincide at exactly one
	// point.
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{Z: 2 * part.Stud}),
	}
	joints, err := FindJoints(holesAlongY{}, parts, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if len(joints) != 1 {
		t.Fatalf("got %d joints, want 1", len(joints))
	}
	// One pin between two parts: 3(2-1) - 2(1) = 1 degree of freedom left.
	m, kind := Mobility(len(parts), joints)
	if m != 1 || kind != "planar" {
		t.Errorf("mobility = %d (%s), want 1 (planar)", m, kind)
	}
	findings, err := Analyze(holesAlongY{}, parts, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if findings[0].Level != "FAIL" || findings[0].Check != "rigidity" {
		t.Errorf("expected a hinge report, got %+v", findings[0])
	}
}

func TestTwoBeamsSharingTwoHolesAreRigid(t *testing.T) {
	// Overlapping by two holes: 3(2-1) - 2(2) = -1, overconstrained and rigid.
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{Z: part.Stud}),
	}
	joints, _ := FindJoints(holesAlongY{}, parts, inventory)
	if len(joints) != 2 {
		t.Fatalf("got %d joints, want 2", len(joints))
	}
	m, _ := Mobility(len(parts), joints)
	if m != -1 {
		t.Errorf("mobility = %d, want -1", m)
	}
	findings, _ := Analyze(holesAlongY{}, parts, inventory)
	if findings[0].Level != "OK" {
		t.Errorf("expected rigid, got %+v", findings[0])
	}
}

func TestPartsThatTouchNowhereFallApart(t *testing.T) {
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{X: 500}),
	}
	joints, _ := FindJoints(holesAlongY{}, parts, inventory)
	if len(joints) != 0 {
		t.Fatalf("got %d joints, want none", len(joints))
	}
	comps := Components(len(parts), joints)
	if len(comps) != 2 {
		t.Fatalf("got %d components, want 2", len(comps))
	}
	findings, _ := Analyze(holesAlongY{}, parts, inventory)
	if findings[0].Level != "FAIL" || findings[0].Check != "connectivity" {
		t.Errorf("expected a connectivity failure, got %+v", findings[0])
	}
	// And it should name the floaters.
	if len(findings) != 3 {
		t.Errorf("expected both loose parts named, got %d findings", len(findings))
	}
}

func TestComponentsMergeThroughAChain(t *testing.T) {
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{Z: 2 * part.Stud}),
		beam("beam3", geom.Vec3{Z: 4 * part.Stud}),
	}
	joints, _ := FindJoints(holesAlongY{}, parts, inventory)
	comps := Components(len(parts), joints)
	if len(comps) != 1 || len(comps[0]) != 3 {
		t.Errorf("got %v, want one component of three", comps)
	}
}

func TestASinglePartIsTriviallyRigid(t *testing.T) {
	if m, kind := Mobility(1, nil); m != 0 || kind != "single part" {
		t.Errorf("got %d (%s)", m, kind)
	}
}

// Computed spatially a square of four beams looks rigid; it is really a
// four-bar linkage with one degree of freedom. That is why the planar form
// exists.
func TestPlanarAndSpatialFormsDisagreeOnAFourBarLinkage(t *testing.T) {
	oneAxis := []Joint{
		{A: 0, B: 1, Axis: geom.Vec3{Y: 1}},
		{A: 1, B: 2, Axis: geom.Vec3{Y: 1}},
		{A: 2, B: 3, Axis: geom.Vec3{Y: 1}},
		{A: 3, B: 0, Axis: geom.Vec3{Y: 1}},
	}
	if m, kind := Mobility(4, oneAxis); m != 1 || kind != "planar" {
		t.Errorf("planar: got %d (%s), want 1", m, kind)
	}
	// The same four joints on mixed axes take the spatial form, which calls it
	// rigid: 6(3) - 5(4) = -2.
	mixed := append([]Joint(nil), oneAxis...)
	mixed[0].Axis = geom.Vec3{X: 1}
	if m, kind := Mobility(4, mixed); m != -2 || kind != "spatial" {
		t.Errorf("spatial: got %d (%s), want -2", m, kind)
	}
}

func TestSummarySeparatesHingesFromRigidPairs(t *testing.T) {
	// Holes run -20/0/+20 from each origin. Part 1 overlaps part 0 by two
	// holes; part 2 starts one stud further on and meets part 1 at exactly one.
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{Z: part.Stud}),
		beam("beam3", geom.Vec3{Z: 3 * part.Stud}),
	}
	s, err := Summarize(holesAlongY{}, parts, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.RigidPairs) != 1 || s.RigidPairs[0] != [2]int{0, 1} {
		t.Errorf("rigid pairs = %v, want [0 1]", s.RigidPairs)
	}
	if len(s.Hinges) != 1 || s.Hinges[0] != [2]int{1, 2} {
		t.Errorf("hinges = %v, want [1 2]", s.Hinges)
	}
}

func TestUnknownPartIsAnError(t *testing.T) {
	_, err := FindJoints(holesAlongY{}, []part.Placed{beam("nosuch", geom.Vec3{})}, inventory)
	if err == nil {
		t.Error("expected an error for a part outside the inventory")
	}
}

type noAxis struct{}

func (noAxis) RotationAxis(string) (geom.Vec3, string, bool) {
	return geom.Vec3{}, "", false
}

func TestMissingHoleAxisIsAnError(t *testing.T) {
	_, err := FindJoints(noAxis{}, []part.Placed{beam("beam3", geom.Vec3{})}, inventory)
	if err == nil {
		t.Error("expected an error when the hole axis is unknown")
	}
}

// A pin joins two holes that face the same way and sit on one axis line. That
// covers both cases: holes at the very same point, and two parts lying against
// each other with their hole planes a part's width apart.
func TestPartsLyingAgainstEachOtherAreJoined(t *testing.T) {
	// Beams offset along their hole axis by 20 LDU: side by side, pinned.
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{Y: 20}),
	}
	joints, err := FindJoints(holesAlongY{}, parts, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if len(joints) != 3 {
		t.Errorf("got %d joints, want 3 — every hole lines up with its neighbour",
			len(joints))
	}
}

func TestHolesTooFarApartAlongTheAxisAreNotJoined(t *testing.T) {
	// Further than one pin can span.
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{Y: PinReach + 10}),
	}
	joints, _ := FindJoints(holesAlongY{}, parts, inventory)
	if len(joints) != 0 {
		t.Errorf("got %d joints across %v LDU; a pin reaches %v",
			len(joints), PinReach+10, PinReach)
	}
}

// The condition that catches wishful bridges: a gap ACROSS the hole axis
// cannot be pinned, however small.
func TestHolesOffsetAcrossTheAxisAreNotJoined(t *testing.T) {
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{X: 20}), // holes face Y, offset along X
	}
	joints, _ := FindJoints(holesAlongY{}, parts, inventory)
	if len(joints) != 0 {
		t.Errorf("got %d joints; the holes are on parallel lines, not one line",
			len(joints))
	}
}

// An axle through two bearings is what actually holds them together. A
// bearing's hole faces along its shaft and the two sit apart along that same
// direction, so no beam laid between them can be pinned to either — leaving the
// shaft out is why a sound structure can read as loose pieces.
func TestAnAxleThreadsItsBearingsTogether(t *testing.T) {
	// Two beams whose holes face Y, 80 LDU apart along Y: nothing pins them.
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{Y: 80}),
	}
	if joints, _ := FindJoints(holesAlongY{}, parts, inventory); len(joints) != 0 {
		t.Fatalf("got %d pin joints, want none at that distance", len(joints))
	}

	axle := Axle{Point: geom.Vec3{}, Dir: geom.Vec3{Y: 1}, From: -20, To: 100}
	joints, err := FindJointsWith(holesAlongY{}, parts, inventory, []Axle{axle})
	if err != nil {
		t.Fatal(err)
	}
	if len(joints) != 1 {
		t.Fatalf("got %d joints, want 1 through the axle", len(joints))
	}
	if comps := Components(len(parts), joints); len(comps) != 1 {
		t.Errorf("got %d pieces, want one: the axle joins them", len(comps))
	}
}

// Parts on one axle are a chain of constraints, not every pair: five on a shaft
// is four joints, and the mobility formula counts what it is given.
func TestPartsOnAnAxleAreJoinedInAChain(t *testing.T) {
	// Spaced beyond a pin's reach, so only the axle can be joining them.
	var parts []part.Placed
	for i := 0; i < 4; i++ {
		parts = append(parts, beam("beam3", geom.Vec3{Y: float64(i) * (PinReach + 20)}))
	}
	axle := Axle{Point: geom.Vec3{}, Dir: geom.Vec3{Y: 1}, From: -20, To: 200}
	joints, _ := FindJointsWith(holesAlongY{}, parts, inventory, []Axle{axle})
	if len(joints) != len(parts)-1 {
		t.Errorf("got %d joints for %d parts, want %d", len(joints), len(parts), len(parts)-1)
	}
}

func TestAnAxleDoesNotReachBeyondItsEnds(t *testing.T) {
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{Y: 200}),
	}
	axle := Axle{Point: geom.Vec3{}, Dir: geom.Vec3{Y: 1}, From: -20, To: 60}
	joints, _ := FindJointsWith(holesAlongY{}, parts, inventory, []Axle{axle})
	if len(joints) != 0 {
		t.Errorf("got %d joints; the second beam is past the end of the axle", len(joints))
	}
}

func TestAnAxleIgnoresHolesOnAParallelLine(t *testing.T) {
	parts := []part.Placed{
		beam("beam3", geom.Vec3{}),
		beam("beam3", geom.Vec3{X: 100, Y: 40}), // same direction, another line
	}
	axle := Axle{Point: geom.Vec3{}, Dir: geom.Vec3{Y: 1}, From: -20, To: 100}
	joints, _ := FindJointsWith(holesAlongY{}, parts, inventory, []Axle{axle})
	if len(joints) != 0 {
		t.Errorf("got %d joints; the other beam is not on this axle", len(joints))
	}
}

func TestCoversNeedsTheHoleToFaceAlongTheAxle(t *testing.T) {
	axle := Axle{Point: geom.Vec3{}, Dir: geom.Vec3{Y: 1}, From: -20, To: 100}
	if !axle.Covers(geom.Vec3{Y: 40}, geom.Vec3{Y: 1}) {
		t.Error("a hole on the axle facing along it is covered")
	}
	if axle.Covers(geom.Vec3{Y: 40}, geom.Vec3{X: 1}) {
		t.Error("a hole facing across the axle is not on it")
	}
}
