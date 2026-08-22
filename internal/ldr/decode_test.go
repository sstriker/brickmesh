// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package ldr

import (
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
)

func TestAPlainModelReadsBackAsItsParts(t *testing.T) {
	got, err := Decode(strings.NewReader(`0 a model
1 4 10 20 30 1 0 0 0 1 0 0 0 1 3001.dat
1 0 0 0 0 1 0 0 0 1 0 0 0 1 3002.DAT
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d parts, want 2", len(got))
	}
	if got[0].Name != "3001.dat" || got[0].Pos != (geom.Vec3{X: 10, Y: 20, Z: 30}) {
		t.Errorf("first part came back as %+v", got[0])
	}
	// Case is not meaning: LDraw is written by many hands.
	if got[1].Name != "3002.dat" {
		t.Errorf("%q should have been normalized", got[1].Name)
	}
}

// The whole reason for a decoder: a submodel placed twice, at two placements,
// is two sets of parts in two places.
func TestASubmodelPlacedTwiceComesBackTwice(t *testing.T) {
	got, err := Decode(strings.NewReader(`0 FILE main.ldr
1 16 0 0 0 1 0 0 0 1 0 0 0 1 sub.ldr
1 16 100 0 0 1 0 0 0 1 0 0 0 1 sub.ldr
0 FILE sub.ldr
1 4 5 0 0 1 0 0 0 1 0 0 0 1 3001.dat
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d parts, want 2 — one per placement of the submodel", len(got))
	}
	if got[0].Pos.X != 5 || got[1].Pos.X != 105 {
		t.Errorf("the submodel's part landed at x=%g and x=%g, want 5 and 105",
			got[0].Pos.X, got[1].Pos.X)
	}
	if got[0].Depth != 1 {
		t.Errorf("a part inside a submodel is one deep, not %d", got[0].Depth)
	}
}

// Rotation composes, and in the right order: the parent turns the child's whole
// placement, not just its position.
func TestASubmodelsRotationComposes(t *testing.T) {
	// The parent turns a quarter about Y; the child sits 10 along X.
	got, err := Decode(strings.NewReader(`0 FILE main.ldr
1 16 0 0 0 0 0 1 0 1 0 -1 0 0 sub.ldr
0 FILE sub.ldr
1 4 10 0 0 1 0 0 0 1 0 0 0 1 3001.dat
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d parts, want 1", len(got))
	}
	// x=10 turned by that matrix lands on z=-10.
	if got[0].Pos.X != 0 || got[0].Pos.Z != -10 {
		t.Errorf("the child ended at %+v, want the parent's rotation applied",
			got[0].Pos)
	}
	// And the part is turned too, not merely moved.
	if got[0].Rot == geom.Rotations[0] {
		t.Error("the part came back unrotated; a submodel turns what is in it")
	}
}

// A file that references itself must not hang. LDraw does not forbid it; it
// only never means it.
func TestACycleDoesNotHang(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = Decode(strings.NewReader(`0 FILE a.ldr
1 16 0 0 0 1 0 0 0 1 0 0 0 1 b.ldr
0 FILE b.ldr
1 16 0 10 0 1 0 0 0 1 0 0 0 1 a.ldr
`))
	}()
	<-done
}

// OMR embeds some parts as renamed submodels — "42110 - 35188.dat" — and those
// really are submodels, so they resolve; a part by a name the file does not
// declare is a part. Getting this backwards is how a walk of 42110 came back
// missing the catch it was looking for.
func TestARenamedSubmodelResolvesAndAnUndeclaredNameIsAPart(t *testing.T) {
	got, err := Decode(strings.NewReader(`0 FILE 42110 - main.ldr
1 16 0 0 0 1 0 0 0 1 0 0 0 1 42110 - 35188.dat
1 0 50 0 0 1 0 0 0 1 0 0 0 1 3001.dat
0 FILE 42110 - 35188.dat
1 0 0 0 0 1 0 0 0 1 0 0 0 1 35188.dat
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d parts, want 2", len(got))
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if names[0] != "35188.dat" {
		t.Errorf("the renamed submodel should have resolved to the part inside "+
			"it, got %q", names[0])
	}
}

func TestAnEmptyFileIsAnError(t *testing.T) {
	if _, err := Decode(strings.NewReader("0 just a comment\n")); err == nil {
		t.Error("a file with no parts should say so rather than come back empty")
	}
}

// An .mpd may carry a copy of a part file inside it, for a part not in the
// official library yet. It looks exactly like a submodel and is not one: its
// contents are the part's own geometry.
//
// Descending into one loses the part and gains its primitives. Reading LDraw's
// 42110 that way gave 9,297 "parts", 302 distinct, and none of them the catch
// the file was opened for. Not descending gives 3,012 and one catch.
func TestAnInlinedPartIsAPartAndNotASubmodel(t *testing.T) {
	got, err := Decode(strings.NewReader(`0 FILE 42110 - main.ldr
1 16 0 0 0 1 0 0 0 1 0 0 0 1 42110 - 35188.dat
0 FILE 42110 - 35188.dat
0 35188
0 !LDRAW_ORG Unofficial_Part
1 16 0 0 0 1 0 0 0 1 0 0 0 1 s/35188s01.dat
1 16 0 0 0 -1 0 0 0 1 0 0 0 1 s/35188s01.dat
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d parts, want 1 — the inlined file is a part, and its "+
			"insides are not parts of the model", len(got))
	}
	// And under its own number, so the library can be asked about it and two
	// sets that both inline it agree they used the same part.
	if got[0].Name != "35188.dat" {
		t.Errorf("got %q, want the part's own name without the set prefix",
			got[0].Name)
	}
}

// The prefix is only stripped when what is left looks like a part.
func TestOnlyAPartLikeSuffixIsStripped(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"42110 - 35188.dat", "35188.dat"},
		{"3001.dat", "3001.dat"},
		{"gearbox - stage one.ldr", "gearbox - stage one.ldr"},
		{"a - b c.dat", "a - b c.dat"}, // a space: a description, not a number
	} {
		if got := partName(c.in); got != c.want {
			t.Errorf("partName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
