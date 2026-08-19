// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package synth

import (
	"context"
	"errors"
	"os"
	"testing"

	"brickmesh/internal/extract"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/progress"
	"brickmesh/internal/shadow"
	"brickmesh/internal/voxel"
)

func searcherForCancel(t *testing.T) *Searcher {
	t.Helper()
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	lib := ldraw.New("")
	return NewSearcher(voxel.NewRasterizer(lib),
		extract.NewPorts(shadow.Open(root), lib), nil)
}

// A search told to stop before it starts does no restarts and says why.
func TestACancelledSearchStops(t *testing.T) {
	s := searcherForCancel(t)
	l := oneShaft()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var restarts int
	_, err := s.Synthesize(ctx, l, nil, Options{
		Restarts: 200, Seed: 1,
		Progress: func(progress.Report) { restarts++ },
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
	if restarts != 0 {
		t.Errorf("ran %d restarts after being cancelled; the point of the "+
			"context is that it stops", restarts)
	}
}

// Every restart is reported, and the count runs to the number asked for.
func TestEveryRestartIsReported(t *testing.T) {
	s := searcherForCancel(t)
	l := oneShaft()

	const want = 12
	var seen []progress.Report
	if _, err := s.Synthesize(context.Background(), l, nil, Options{
		Restarts: want, Seed: 1,
		Progress: func(r progress.Report) { seen = append(seen, r) },
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != want {
		t.Fatalf("got %d reports for %d restarts", len(seen), want)
	}
	// The count is the point: a bar that jumps about is worse than none.
	last := 0
	for _, r := range seen {
		if r.Stage != progress.StageStructure {
			t.Errorf("reported stage %q, want %q", r.Stage, progress.StageStructure)
		}
		if r.Total != want {
			t.Errorf("reported a total of %d, want %d", r.Total, want)
		}
		if r.Done <= last {
			t.Errorf("the count went %d then %d; it should only rise", last, r.Done)
		}
		last = r.Done
	}
	if last != want {
		t.Errorf("the count stopped at %d of %d", last, want)
	}
}

// A search that finds nothing still reports every restart, or a bar that starts
// on a hopeless layout never finishes.
func TestRestartsAreReportedEvenWhenNothingIsFound(t *testing.T) {
	s := searcherForCancel(t)
	var seen int
	if _, err := s.Synthesize(context.Background(), oneShaft(), nil, Options{
		Restarts: 5, Seed: 1, MaxParts: 1, // too few parts to cover anything
		Progress: func(progress.Report) { seen++ },
	}); err != nil {
		t.Fatal(err)
	}
	if seen != 5 {
		t.Errorf("got %d reports, want one per restart", seen)
	}
}
