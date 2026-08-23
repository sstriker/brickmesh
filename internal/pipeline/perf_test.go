// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sstriker/brickmesh/internal/extract"
	"github.com/sstriker/brickmesh/internal/ldraw"
	"github.com/sstriker/brickmesh/internal/shadow"
	"github.com/sstriker/brickmesh/internal/spec"
	"github.com/sstriker/brickmesh/internal/voxel"
)

// The engine has to stay quick enough to run in a browser while somebody waits.
//
// This exists because a change that made the whole suite three to four times
// slower got committed and was noticed only because a test timed out. Nothing
// failed. The work it added was real and repeated — a part's subfile tree
// walked again on every lookup, in the search's inner loop — and it would have
// been just as invisible had it been half as bad.
//
// Counted rather than timed, because the counts are what the code does and the
// time is what the machine felt like. A shared CI runner varies by more than a
// real regression often does, so a wall-clock threshold loose enough not to
// flake is too loose to catch anything; these numbers are the same on every
// machine, so the budget can sit close to the truth.
//
// Timing is still reported, and there is a coarse ceiling at the end for
// blow-ups that no counter here is watching.
type budget struct {
	// walks is subfile-tree walks: one per distinct part is all a run needs.
	walks int64
	// reads is part files gone to disk for. Bounded by the library's own
	// caches, so this is a check that they are still there.
	reads int64
}

// What each example costs today, with room above it. The gap is deliberately
// small — a few times the true figure rather than orders of magnitude — because
// these are counts and counts do not wobble. The regression this was written
// for took walks from 23 to 5064.
var budgets = map[string]budget{
	"reduction.json":                {walks: 40, reads: 20000},
	"protected-reduction.json":      {walks: 40, reads: 20000},
	"subtractor.json":               {walks: 40, reads: 20000},
	"gearbox-2-speed.json":          {walks: 60, reads: 24000},
	"gearbox-first-system.json":     {walks: 50, reads: 22000},
	"gearbox-2-speed-auto.json":     {walks: 60, reads: 24000},
	"gearbox-3-speed-compound.json": {walks: 80, reads: 24000},
	"gearbox-early-system.json":     {walks: 50, reads: 22000},
	"gearbox-4-speed-compound.json": {walks: 80, reads: 24000},
}

func TestBuildingAnExampleStaysCheap(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	for _, path := range examples(t) {
		name := filepath.Base(path)
		t.Run(strings.TrimSuffix(name, ".json"), func(t *testing.T) {
			want, ok := budgets[name]
			if !ok {
				t.Fatalf("%s has no budget. A new example needs one, or this "+
					"stops covering what the engine actually does — measure it "+
					"and write the number down with room above it", name)
			}

			// A library of its own, so what is measured is one run from cold
			// rather than whatever an earlier test left warm.
			lib := ldraw.New("")
			root, err := shadow.Ensure("")
			if err != nil {
				t.Fatal(err)
			}
			ports := extract.NewPorts(shadow.Open(root), lib)
			deps := Deps{Lib: lib, Shadow: ports, Rast: voxel.NewRasterizer(lib)}

			start := time.Now()
			res := runSpec(t, deps, path)
			took := time.Since(start)

			walks, reads := ports.Walks(), lib.Reads()
			t.Logf("%d parts, %d walks, %d reads, %s",
				len(res.Model.Parts), walks, reads, took.Round(time.Millisecond))

			if walks > want.walks {
				t.Errorf("%d subfile walks, budget %d. A part's holes are worked "+
					"out once and remembered; this many means something is asking "+
					"again inside a loop", walks, want.walks)
			}
			if reads > want.reads {
				t.Errorf("%d part files read, budget %d. The library caches what "+
					"it has read, so this many means a cache is being missed or "+
					"bypassed", reads, want.reads)
			}
			// Far above anything real — the examples take under two seconds —
			// and there only to catch something that runs away entirely.
			if took > 90*time.Second {
				t.Errorf("took %s, which is past any use in a browser", took)
			}
		})
	}
}

// And the counters have to count, or the budgets above are decoration.
func TestTheCostCountersMove(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	ports := extract.NewPorts(shadow.Open(root), lib)

	if got := ports.Walks(); got != 0 {
		t.Errorf("a fresh Ports has already walked %d times", got)
	}
	if got := ports.Holes("41239.dat"); len(got) == 0 {
		t.Fatal("no holes for a beam")
	}
	if got := ports.Walks(); got != 1 {
		t.Errorf("one lookup, %d walks", got)
	}
	// The second time is remembered, which is the whole point.
	ports.Holes("41239.dat")
	if got := ports.Walks(); got != 1 {
		t.Errorf("the same part twice cost %d walks", got)
	}
	if lib.Reads() == 0 {
		t.Error("walking a beam's subfiles read no part files")
	}
}

// A benchmark, so a slow change shows up as a number to compare rather than a
// pass or a fail. `go test -run xxx -bench Example ./internal/pipeline/`
func BenchmarkExample(b *testing.B) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		b.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	for _, name := range []string{"reduction", "gearbox-3-speed-compound"} {
		b.Run(name, func(b *testing.B) {
			path := filepath.Join("..", "..", "examples", name+".json")
			doc, err := os.ReadFile(path)
			if err != nil {
				b.Fatal(err)
			}
			lib := ldraw.New("")
			root, err := shadow.Ensure("")
			if err != nil {
				b.Fatal(err)
			}
			deps := Deps{Lib: lib,
				Shadow: extract.NewPorts(shadow.Open(root), lib),
				Rast:   voxel.NewRasterizer(lib)}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := buildOnce(deps, doc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func buildOnce(deps Deps, doc []byte) error {
	sp, err := spec.Read(strings.NewReader(string(doc)))
	if err != nil {
		return err
	}
	m, err := sp.Build()
	if err != nil {
		return err
	}
	_, err = Run(context.Background(), m, deps, Options{Restarts: 8, Seed: 1})
	return err
}
