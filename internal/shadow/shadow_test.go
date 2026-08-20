// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package shadow

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
)

func fixtures(t *testing.T) *Library {
	t.Helper()
	return Open(filepath.Join("..", "..", "tests", "fixtures", "shadow",
		"LDCadShadowLibrary-main"))
}

func TestSnapsAreParsed(t *testing.T) {
	snaps := fixtures(t).Snaps("fixbeam")
	if len(snaps) != 4 {
		t.Fatalf("got %d snaps, want 4", len(snaps))
	}
	for _, s := range snaps {
		if s.Kind != "SNAP_CYL" {
			t.Errorf("kind = %q", s.Kind)
		}
	}
}

func TestGenderAndSectionAreRead(t *testing.T) {
	var axle, round, male int
	for _, s := range fixtures(t).Snaps("fixbeam") {
		if s.Axle() {
			axle++
		} else {
			round++
		}
		if s.Gender == "M" {
			male++
		}
	}
	if axle != 1 {
		t.Errorf("got %d axle snaps, want 1", axle)
	}
	if round != 3 {
		t.Errorf("got %d round snaps, want 3", round)
	}
	if male != 1 {
		t.Errorf("got %d male snaps, want 1", male)
	}
}

func TestAxisComesFromTheOrientation(t *testing.T) {
	// Snap cylinders point along +Y locally; the ori matrix turns them.
	for _, s := range fixtures(t).Snaps("fixbeam") {
		if s.Grid == "" {
			continue
		}
		axis := s.Axis()
		if math.Abs(axis.Z-1) > 1e-9 {
			t.Errorf("axis = %+v, want +Z", axis)
		}
	}
}

func TestGroupedSnapsAreNotGeneric(t *testing.T) {
	var grouped []Snap
	for _, s := range fixtures(t).Snaps("fixbeam") {
		if !s.Generic() {
			grouped = append(grouped, s)
		}
	}
	if len(grouped) != 1 {
		t.Fatalf("got %d grouped snaps, want 1", len(grouped))
	}
	if grouped[0].Group != "craneArm" {
		t.Errorf("group = %q", grouped[0].Group)
	}
}

func TestRotationAxisPrefersTheAxleHole(t *testing.T) {
	axis, source, ok := fixtures(t).RotationAxis("fixbeam")
	if !ok {
		t.Fatal("no rotation axis")
	}
	if math.Abs(axis.Z-1) > 1e-9 {
		t.Errorf("axis = %+v, want +Z", axis)
	}
	if source != "LDCad shadow library, axle hole" {
		t.Errorf("source = %q", source)
	}
}

func TestRotationAxisFallsBackToAnInclude(t *testing.T) {
	_, source, ok := fixtures(t).RotationAxis("fixincl")
	if !ok {
		t.Fatal("no rotation axis from the include")
	}
	if source != "LDCad shadow library, include confh-pinhole" {
		t.Errorf("source = %q", source)
	}
}

func TestUnknownPartHasNoSnaps(t *testing.T) {
	lib := fixtures(t)
	if got := lib.Snaps("nosuchpart"); got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if _, _, ok := lib.RotationAxis("nosuchpart"); ok {
		t.Error("expected no axis")
	}
}

// The header quotes the LDraw title back, which is the only copy of it on disk
// once the library is extracted. Titles drive the tier and the subpart filter,
// so reading the wrong thing here is quiet and total: nothing matches, nothing
// is filtered, and every part lands in the last tier.
func TestTitlesComeFromTheShadowHeader(t *testing.T) {
	titles, err := fixtures(t).Titles()
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		"fixbeam": "Test Beam 5 x 1 x 1",
		"fixcube": "Test Cube 40 LDU",
		"fixsub":  "~Test Beam Subpart", // the ~ has to survive
	} {
		if titles[id] != want {
			t.Errorf("%s title = %q, want %q", id, titles[id], want)
		}
	}
}

func TestPartsAreListedSorted(t *testing.T) {
	parts, err := fixtures(t).Parts()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fixbeam", "fixcube", "fixincl", "fixsub"}
	if len(parts) != len(want) {
		t.Fatalf("got %v, want %v", parts, want)
	}
	for i := range want {
		if parts[i] != want[i] {
			t.Fatalf("got %v, want %v", parts, want)
		}
	}
}

func TestParseMatDefaultsToIdentity(t *testing.T) {
	if got := parseMat(""); got != (geom.Mat3{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}) {
		t.Errorf("got %v, want the identity", got)
	}
}

func TestEnsureReturnsAnAlreadyExtractedLibrary(t *testing.T) {
	// A directory that is already there must not trigger a download; the test
	// would hang or fail offline if it did.
	dest := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dest, rootDir), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Ensure(dest)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dest, rootDir) {
		t.Errorf("got %q", got)
	}
}
