// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"errors"
	"testing"

	"brickmesh/internal/progress"
)

// A run given a context that is already done stops, and says so, rather than
// working through to the end and handing back a result nobody is waiting for.
func TestACancelledRunStops(t *testing.T) {
	deps := requireLibraries(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Run(ctx, build(t, gearbox), deps, Options{Restarts: 60, Seed: 1})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
}

// The stages report themselves in the order they run, so a caller can say what
// the engine is doing rather than only that it is busy.
func TestTheStagesReportThemselves(t *testing.T) {
	deps := requireLibraries(t)
	var stages []string
	seen := map[string]bool{}
	_, err := Run(context.Background(), build(t, gearbox), deps, Options{
		Restarts: 4, Seed: 1, Animate: true, ScriptName: "x.lua",
		Progress: func(r progress.Report) {
			if !seen[r.Stage] {
				seen[r.Stage] = true
				stages = append(stages, r.Stage)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		progress.StageLayout, progress.StageStructure, progress.StageModel,
		progress.StagePhase, progress.StageClearance, progress.StageAnimation,
	}
	if len(stages) != len(want) {
		t.Fatalf("reported %v, want %v", stages, want)
	}
	for i, s := range want {
		if stages[i] != s {
			t.Errorf("stage %d was %q, want %q — the order is what a caller "+
				"reads as progress", i, stages[i], s)
		}
	}
}

// A run with nowhere to send progress is the ordinary case, and must not be a
// special case in any of the callers.
func TestNoProgressSinkIsFine(t *testing.T) {
	deps := requireLibraries(t)
	if _, err := Run(context.Background(), build(t, gearbox), deps,
		Options{Restarts: 2, Seed: 1}); err != nil {
		t.Fatal(err)
	}
}
