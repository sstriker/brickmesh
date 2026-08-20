// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Command brickmesh-torque follows torque through a gear train.
//
// Reads a train as JSON on standard input and reports the torque and tooth load
// at every stage:
//
//	echo '{"input_ncm": 40, "stages": [
//	        {"name": "8t to 24t", "driver_teeth": 8, "driven_teeth": 24}]}' |
//	  brickmesh-torque
//
// The propagation is exact. The limits it is assessed against are not: they are
// unverified figures, and the notice at the end says which. Replace them with
// your own before trusting a pass.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/sstriker/brickmesh/internal/torque"
)

type input struct {
	InputNcm float64 `json:"input_ncm"`
	Stages   []struct {
		Name        string `json:"name"`
		DriverTeeth int    `json:"driver_teeth"`
		DrivenTeeth int    `json:"driven_teeth"`
		Kind        string `json:"kind,omitempty"`
	} `json:"stages"`
}

func main() {
	asJSON := flag.Bool("json", false, "write the rows as JSON instead of a table")
	flag.Parse()

	var in input
	dec := json.NewDecoder(os.Stdin)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		fmt.Fprintln(os.Stderr, "error: reading the train:", err)
		os.Exit(1)
	}
	if len(in.Stages) == 0 {
		fmt.Fprintln(os.Stderr, "error: no stages to follow the torque through")
		os.Exit(1)
	}

	stages := make([]torque.Stage, len(in.Stages))
	for i, s := range in.Stages {
		stages[i] = torque.Stage{
			Name: s.Name, DriverTeeth: s.DriverTeeth,
			DrivenTeeth: s.DrivenTeeth, Kind: s.Kind,
		}
	}
	rows := torque.Propagate(in.InputNcm, stages)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("%-20s %7s %9s %9s %8s %8s\n",
		"stage", "ratio", "in Ncm", "out Ncm", "drv N", "drvn N")
	for _, r := range rows {
		fmt.Printf("%-20s %7.3f %9.2f %9.2f %8.1f %8.1f\n",
			r.Stage, r.Ratio, r.TorqueInNcm, r.TorqueOutNcm,
			r.ForceDriverN, r.ForceDrivenN)
	}
	fmt.Println()
	for _, a := range torque.Assess(rows) {
		fmt.Printf("  %-5s %s\n", a.Level, a.Detail)
	}
	fmt.Println("\nThe limits these are judged against are not measurements:")
	for _, line := range torque.Notice() {
		fmt.Println(line)
	}
}
