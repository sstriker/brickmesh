// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package synth is the structural layer: finding beams that bear every shaft.
//
// Input is a worked-out layout — shaft lines with direction, gear stations, and
// the free stretches per shaft. Output is a set of placed parts such that every
// shaft has at least two bearing points, nothing intersects anything else, and
// the whole is as small as possible.
//
// What makes it tractable is that nothing here is continuous. A beam sits in
// one of 24 orientations, fewer once symmetry is taken out, and only at lattice
// positions; the bearing point has to lie on the shaft line, which cuts the
// candidates per requirement down to a handful.
package synth

import "github.com/sstriker/brickmesh/internal/part"

// The vocabulary lives in internal/part, which the rigidity check also needs.
// Aliased here so callers of the search read in one package.
type (
	Placed     = part.Placed
	Beam       = part.Beam
	AxisSource = part.AxisSource
)

const Stud = part.Stud

var Beams = part.Beams

var (
	HoleCounts    = part.HoleCounts
	HoleOffsets   = part.HoleOffsets
	LocalHoleAxis = part.LocalHoleAxis
)
