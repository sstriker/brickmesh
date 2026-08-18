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
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"brickmesh/internal/ldraw"
	"brickmesh/internal/mech"
	"brickmesh/internal/part"
	"brickmesh/internal/pipeline"
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
		outPath   = flag.String("out", "", "where to write the model (default <spec>.ldr)")
		checkOnly = flag.Bool("check", false, "run the checks and stop, writing nothing")
		restarts  = flag.Int("restarts", 60, "restarts for the structural search")
		seed      = flag.Int64("seed", 0, "seed for the structural search, for a reproducible run")
		span      = flag.Int("span", 4, "how far along each shaft to search, in half studs")
		force     = flag.Bool("force", false, "write a model even when a check failed")
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

	res, err := pipeline.Run(m, deps, pipeline.Options{
		Restarts: *restarts, Seed: *seed, Span: *span,
		Inventory:     part.Beams,
		SkipStructure: *checkOnly,
	})
	if err != nil {
		return err
	}
	report(res.Findings)

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

	out := *outPath
	if out == "" {
		out = trimExt(*specPath) + ".ldr"
	}
	if err := os.WriteFile(out, []byte(res.Model.Encode()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d parts)\n", out, len(res.Model.Parts))
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
		Shadow: shadow.Open(root),
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
