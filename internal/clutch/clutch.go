// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package clutch holds what a driving ring can lock to, and where it has to sit
// to do it.
//
// Every number here was measured with the interference sweep rather than
// recalled, and clutch_test.go re-derives them from the library. The ring runs
// 40 LDU along its shaft and a gear is 20 LDU thick, so their faces meet at 30
// LDU between centers. That is not merely where they stop overlapping: at
// exactly 30 the sweep finds the 16-tooth clutch gear blocked for most of a
// turn with sixteen free windows 22.5 degrees apart — one per clutch tooth,
// which is the ring's dogs sitting in the gear's recesses. One LDU further out
// and every gear is free at every angle.
//
// The plain gears show nothing of the sort, which is the point: a shift needs
// the clutch variant. A driving ring next to a plain 16t gear is scenery.
package clutch

// Ring is the driving ring of the first switching system, the one in 8466.
const Ring = "6539.dat"

// Gears maps a tooth count to the gear a ring can actually lock to.
//
// Only the 16t is here, and that is the library's answer rather than a
// shortcut: a search of every part title turns up exactly two clutch gears,
// 6542a and 6542b, which are the same 16t gear with and without a smooth face.
// Real 20t and 24t shifts reach their gears through a driving ring extension
// (32187, or 35186 with its eight clutch teeth), which stacks onto the ring and
// is not modelled here.
var Gears = map[int]string{16: "6542a.dat"}

// Where the ring sits, in half studs from the center of the gear it engages.
const (
	// Engaged is 30 LDU: faces flush, dogs in the recesses.
	Engaged = 3.0
	// Clear is 40 LDU, the next stop on the half-stud lattice past the 31 LDU
	// at which the sweep first reports every gear free at every angle. The
	// difference between the two is the ring's travel, and it is what a shift
	// looks like.
	Clear = 4.0
	// Room is what a shifted gear needs beside it: from the gear's own face out
	// to the far end of the ring once it has slid clear.
	Room = 5.0
)

// Travel is how far the ring slides between engaged and disengaged.
const Travel = Clear - Engaged
