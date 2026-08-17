// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Command brickmesh-extract builds the parts catalog the engine reads.
//
// The Go port of extract/brickmesh_extract/build.py, sharing its cache
// directories and writing the same schema. Both exist while the port is being
// verified: tests/test_go_parity.py runs the two over the same library and
// compares the output part by part.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"brickmesh/internal/extract"
	"brickmesh/internal/shadow"
)

func main() {
	var (
		out      = flag.String("out", "data/catalog.json", "where to write the catalog")
		tier     = flag.Uint("tier", 3, "1 common, 2 all Technic, 3 the whole library")
		limit    = flag.Int("limit", 0, "stop after this many parts; 0 for all")
		cacheDir = flag.String("shadow-cache", "",
			"where the shadow library lives (default ~/.cache/brickmesh-shadow)")
	)
	flag.Parse()

	if *tier < 1 || *tier > 3 {
		fmt.Fprintln(os.Stderr, "tier must be 1, 2 or 3")
		os.Exit(2)
	}

	logf := func(m string) { fmt.Fprintln(os.Stderr, m) }
	logf("fetching the LDCad shadow library ...")
	root, err := shadow.Ensure(*cacheDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	records, err := extract.Build(shadow.Open(root), extract.Options{
		MaxTier: uint8(*tier), Limit: *limit, Log: logf,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(records) == 0 {
		fmt.Fprintln(os.Stderr,
			"no parts survived filtering; refusing to write an empty catalog")
		os.Exit(1)
	}

	if dir := filepath.Dir(*out); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(records); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logf("wrote " + *out)
}
