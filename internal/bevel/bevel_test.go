// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package bevel

import (
	"math"
	"os"
	"testing"

	"github.com/sstriker/brickmesh/internal/ldraw"
)

func requireLibraries(t *testing.T) *ldraw.Library {
	t.Helper()
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	return ldraw.New("")
}

// The part of this that both implementations agree on exactly.
//
// The Python samples its points at random and this samples by stride, so the
// two choose different winners from the touching set. They find the same set:
// 444 positions where the surfaces come within 1.2 LDU. That is the gap
// arithmetic, and it is the half of this that is checked rather than assumed.
func TestTheTouchingSetIsTheOneThePythonFinds(t *testing.T) {
	lib := requireLibraries(t)
	got, err := Solve(lib, DefaultDiff, DefaultDriver, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if got.Touching != 444 {
		t.Errorf("%d positions touch, the python finds 444. Same ranges, same "+
			"threshold, so a different count means the gap arithmetic differs",
			got.Touching)
	}
}

// The ring is found by radius, and if that filter is wrong everything after it
// is measuring the housing.
func TestTheRingTeethAreWhereTheRingIs(t *testing.T) {
	lib := requireLibraries(t)
	ring, err := RingTeeth(lib, DefaultDiff, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(ring) == 0 {
		t.Fatal("no ring teeth")
	}
	zlo, zhi := math.Inf(1), math.Inf(-1)
	rlo, rhi := math.Inf(1), math.Inf(-1)
	for _, v := range ring {
		r := math.Hypot(v.X, v.Y)
		zlo, zhi = math.Min(zlo, v.Z), math.Max(zhi, v.Z)
		rlo, rhi = math.Min(rlo, r), math.Max(rhi, r)
	}
	// Measured: the 28t ring inside 62821 sits 20 to 27 LDU along the axis, at
	// a radius of 31 to 36. Worth pinning, because the solver's answer is only
	// meaningful if the driver ends up somewhere near this.
	if zlo < 19 || zhi > 28 {
		t.Errorf("ring teeth run z %.1f..%.1f, want about 20..27", zlo, zhi)
	}
	if rlo < 30 || rhi > 37 {
		t.Errorf("ring teeth run r %.1f..%.1f, want about 31..36", rlo, rhi)
	}
}

// A search range that cannot reach the ring has to say so rather than return
// the least bad position in it.
func TestOutOfRangeIsAnError(t *testing.T) {
	lib := requireLibraries(t)
	opts := Defaults()
	opts.RadialFrom, opts.RadialTo = 200, 210
	if _, err := Solve(lib, DefaultDiff, DefaultDriver, opts); err == nil {
		t.Error("a driver 10 studs away from the ring should not be reported as touching")
	}
}

// The sampling is by stride and not at random, so the same part gives the same
// answer every time. Without that, two runs of the same search disagree.
func TestTheAnswerIsTheSameEveryTime(t *testing.T) {
	lib := requireLibraries(t)
	first, err := Solve(lib, DefaultDiff, DefaultDriver, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	second, err := Solve(lib, DefaultDiff, DefaultDriver, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("two runs disagree:\n %+v\n %+v", first, second)
	}
}
