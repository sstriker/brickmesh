// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Command brickmesh analyses a mechanism and searches for a structure to hold it.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		catalogPath = flag.String("catalog", "data/catalog.json",
			"catalogue as written by the Python extractor")
		tier = flag.Uint("tier", 1,
			"inventory tier: 1 common, 2 all Technic, 3 the whole library")
		workers = flag.Int("workers", 0, "parallel workers, 0 = number of cores")
	)
	flag.Parse()

	if _, err := os.Stat(*catalogPath); err != nil {
		fmt.Fprintf(os.Stderr,
			"catalogue not found at %s\nrun the extractor first, see extract/README.md\n",
			*catalogPath)
		os.Exit(1)
	}
	fmt.Printf("brickmesh - tier %d, %d workers\n", *tier, *workers)
	fmt.Println("not wired up yet; see README.md for status")
}
