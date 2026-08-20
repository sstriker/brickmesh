// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"testing"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
	"brickmesh/internal/synth"
)

// Rigidity is a condition of choosing a structure now, not a remark about the
// one already chosen.
//
// It used to be a post-check: take the first candidate that fits, brace it, and
// tell the reader off in the report if it still folded. That is advice arriving
// after the decision. The search returns solutions smallest first and there are
// usually many, so one that folds can be passed over.
//
// This is the predicate that decides. Two liftarms overlapping by one hole are
// a hinge and two overlapping by two are not, which is the smallest case where
// the distinction exists at all.
func TestHoldsRigidTellsAHingeFromAJoint(t *testing.T) {
	deps := requireLibraries(t)
	res := &Result{}

	hinge := []synth.Placed{
		{Part: "32523.dat", Rot: 0, Origin: geom.Vec3{}},
		{Part: "32523.dat", Rot: 0, Origin: geom.Vec3{Z: 2 * part.Stud}},
	}
	if holdsRigid(deps, hinge, res) {
		t.Error("two liftarms meeting at one hole fold about it, and this " +
			"called them rigid")
	}

	rigid := []synth.Placed{
		{Part: "32523.dat", Rot: 0, Origin: geom.Vec3{}},
		{Part: "32523.dat", Rot: 0, Origin: geom.Vec3{Z: part.Stud}},
	}
	if !holdsRigid(deps, rigid, res) {
		t.Error("two liftarms overlapping by two holes cannot fold, and this " +
			"called them loose")
	}
}

// And a structure in loose pieces is not rigid either, however many joints the
// pieces have among themselves. Connectivity is checked before mobility, and
// this is what says so.
func TestHoldsRigidRejectsLoosePieces(t *testing.T) {
	deps := requireLibraries(t)
	apart := []synth.Placed{
		{Part: "32523.dat", Rot: 0, Origin: geom.Vec3{}},
		{Part: "32523.dat", Rot: 0, Origin: geom.Vec3{Z: part.Stud}},
		{Part: "32523.dat", Rot: 0, Origin: geom.Vec3{X: 500}},
	}
	if holdsRigid(deps, apart, &Result{}) {
		t.Error("a beam five hundred LDU away is not part of the structure, " +
			"and this counted the whole as rigid anyway")
	}
}
