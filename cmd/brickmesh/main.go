// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Command brickmesh takes a mechanism description and builds it.
//
// It runs the layers in order — functional checks, shaft placement, gear
// stations, a structure to bear them — and writes the result as LDraw, which
// Stud.io opens directly.
//
//	brickmesh --spec subtractor.json --out subtractor.ldr
//
// Findings go to stderr and the model to the file, so the output is readable
// while the run is still going.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"brickmesh/internal/extract"
	"brickmesh/internal/ldraw"
	"brickmesh/internal/mech"
	"brickmesh/internal/part"
	"brickmesh/internal/pipeline"
	"brickmesh/internal/progress"
	"brickmesh/internal/shadow"
	"brickmesh/internal/spec"
	"brickmesh/internal/voxel"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		specPath  = flag.String("spec", "", "mechanism description to build (required)")
		quiet     = flag.Bool("quiet", false, "do not report progress while searching")
		outPath   = flag.String("out", "", "where to write the model (default <spec>.ldr)")
		checkOnly = flag.Bool("check", false, "run the checks and stop, writing nothing")
		restarts  = flag.Int("restarts", 60, "restarts for the structural search")
		seed      = flag.Int64("seed", 0, "seed for the structural search, for a reproducible run")
		span      = flag.Int("span", 4, "how far along each shaft to search, in half studs")
		force     = flag.Bool("force", false, "write a model even when a check failed")
		animate   = flag.Bool("animate", false,
			"also write an LDCad animation script turning every shaft at its solved ratio")
		seconds = flag.Float64("seconds", 10, "length of the animation")
		turns   = flag.Float64("turns", 4, "turns of the input shaft over the animation")
	)
	flag.Parse()

	if *specPath == "" {
		flag.Usage()
		return fmt.Errorf("a --spec is required")
	}

	f, err := os.Open(*specPath)
	if err != nil {
		return err
	}
	defer f.Close()
	s, err := spec.Read(f)
	if err != nil {
		return err
	}
	m, err := s.Build()
	if err != nil {
		return err
	}

	deps, err := libraries()
	if err != nil {
		return err
	}

	out := *outPath
	if out == "" {
		out = trimExt(*specPath) + ".ldr"
	}
	scriptName := trimExt(filepath.Base(out)) + ".lua"

	// Ctrl-C stops the search rather than killing the process, so a run that is
	// taking too long can be given up on without losing what it printed.
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	res, err := pipeline.Run(ctx, m, deps, pipeline.Options{
		Restarts: *restarts, Seed: *seed, Span: *span,
		Inventory:     part.Beams,
		SkipStructure: *checkOnly,
		Animate:       *animate && !*checkOnly,
		ScriptName:    scriptName,
		Seconds:       *seconds,
		InputTurns:    *turns,
		Progress:      progressTo(os.Stderr, *quiet),
	})
	// What it worked out before it stopped is still worth reading: the
	// functional checks run first and are all done by the time the structural
	// search is anywhere near being interrupted.
	if res != nil {
		report(res.Findings)
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("stopped before the model was finished")
	}
	if err != nil {
		return err
	}

	if *checkOnly {
		if res.Failed() {
			return fmt.Errorf("the mechanism does not check out")
		}
		return nil
	}
	if res.Failed() && !*force {
		return fmt.Errorf("the mechanism does not check out; --force writes a model anyway")
	}
	if res.Model == nil {
		return fmt.Errorf("nothing to write")
	}

	if err := os.WriteFile(out, []byte(res.Model.Encode()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d parts)\n", out, len(res.Model.Parts))

	if res.Script != nil {
		// Beside the model, which is where the SCRIPT line looks for it.
		scriptPath := filepath.Join(filepath.Dir(out), scriptName)
		if err := os.WriteFile(scriptPath, []byte(res.Script.Render()), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s (%d animation(s))\n",
			scriptPath, len(res.Script.Animations))
	}
	return nil
}

// libraries opens the parts libraries, fetching them on first use.
func libraries() (pipeline.Deps, error) {
	lib := ldraw.New("")
	root, err := shadow.Ensure("")
	if err != nil {
		return pipeline.Deps{}, fmt.Errorf("the shadow library: %w", err)
	}
	return pipeline.Deps{
		Lib:    lib,
		Shadow: extract.Ports{Lib: shadow.Open(root), Geom: lib},
		Rast:   voxel.NewRasterizer(lib),
	}, nil
}

// report prints the findings worst first, which is the order they matter in.
func report(findings []mech.Finding) {
	rank := map[string]int{"FAIL": 0, "WARN": 1, "OK": 2}
	sorted := append([]mech.Finding(nil), findings...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && rank[sorted[j].Level] < rank[sorted[j-1].Level]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	for _, f := range sorted {
		fmt.Fprintf(os.Stderr, "  %-5s [%-12s] %s\n", f.Level, f.Check, f.Detail)
	}
}

func trimExt(p string) string {
	return p[:len(p)-len(filepath.Ext(p))]
}

// progressTo writes progress over one line of a terminal, so a long search
// looks like it is doing something without scrolling the findings away.
//
// Only when stderr is a terminal and the run was not asked to be quiet: piped
// into a file, a carriage return every restart is noise.
func progressTo(w *os.File, quiet bool) progress.Func {
	if quiet || !isTerminal(w) {
		return nil
	}
	var last string
	return func(r progress.Report) {
		line := r.String()
		if line == last {
			return
		}
		last = line
		fmt.Fprintf(w, "\r\033[K%s", line)
		if r.Total > 0 && r.Done == r.Total {
			fmt.Fprint(w, "\r\033[K")
		}
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
