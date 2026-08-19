// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package clutch holds what a driving ring can lock to, and where it has to sit
// to do it.
//
// There are two generations of this hardware and they do not mix. The first
// pairs a two-stud driving ring with a two-stud ridged axle joiner and engages
// a 16-tooth gear; the second pairs a three-stud ring with a three-stud joiner
// and engages 16 and 20 tooth gears. A ring of one generation does not grip the
// other's gears, which is measurable and measured.
//
// Every number here comes from the interference sweep rather than from memory,
// and clutch_test.go re-derives them. The measurements were taken again once
// the official parts library replaced a mirror that was missing a sixth of the
// library, including the whole of the second generation.
package clutch

// System is one generation of the shifting hardware.
type System struct {
	Name string
	// Ring is the driving ring; Joiner the ridged axle joiner it slides along,
	// which is what it is actually splined to. A ring cannot grip a bare axle.
	Ring, Joiner string
	// Gears maps a tooth count to the gear this ring can lock to.
	Gears map[int]string

	// Engaged and Clear are where the ring sits, in half studs from the center
	// of the gear. Not always whole half studs: a ring slides freely along its
	// joiner and nothing puts it on the lattice.
	Engaged, Clear float64
	// RingHalf and JoinerHalf are half the length of each, in half studs.
	RingHalf, JoinerHalf float64
	// JoinerReach is how far an axle goes into each end of the joiner. Both
	// holes stop before the middle, so two axles meet there rather than pass.
	JoinerReach float64
}

// Travel is how far the ring slides between engaged and disengaged.
func (s System) Travel() float64 { return s.Clear - s.Engaged }

// Room is the shaft a shifted gear needs beside it: from the gear's own face
// out to the far end of the ring once it has slid clear.
func (s System) Room() float64 { return s.Clear + s.RingHalf - 1 }

// First is the original system, the one in 8466.
//
// The ring meets the gear face to face at three half studs and reads there as
// sixteen free windows in a revolution — one per recess in the clutch gear's
// face, and absent from the plain 16t, which gives four and those are its axle
// hole. It engages over about one LDU, so it is the fussier of the two to
// place.
var First = System{
	Name:   "first",
	Ring:   "6539.dat",
	Joiner: "6538a.dat",
	Gears:  map[int]string{16: "6542a.dat"},

	Engaged: 3.0, Clear: 4.0,
	RingHalf: 2.0, JoinerHalf: 2.0, JoinerReach: 2.0,
}

// Second is the later system, with the three-stud ring.
//
// It engages over four LDU rather than one — from 30.5 to 34.5 from the gear's
// center, four free windows throughout, which is a dog sliding into a recess
// and having depth to do it in. The plain gears stay solid across that whole
// band, which is what says the windows are the clutch and not the gear.
//
// This is the system that gives a 20-tooth shift, and it could not be measured
// at all until the official library arrived: its ring, its joiner and its gears
// were all missing from the mirror.
var Second = System{
	Name:   "second",
	Ring:   "18947.dat",
	Joiner: "18948.dat",
	Gears:  map[int]string{16: "18946.dat", 20: "81346.dat"},

	Engaged: 3.25, Clear: 4.0,
	RingHalf: 2.8, JoinerHalf: 3.0, JoinerReach: 2.0,
}

// Systems in the order they are preferred. The first generation is smaller —
// two studs of shaft against three — so it wins where either would do.
var Systems = []System{First, Second}

// For picks the system that can shift a gear of this many teeth.
func For(teeth int) (System, bool) {
	for _, s := range Systems {
		if _, ok := s.Gears[teeth]; ok {
			return s, true
		}
	}
	return System{}, false
}

// ForBoth is a system that has a clutch gear for both tooth counts.
//
// One driving ring between two clutch gears engages either by sliding, which is
// how a two-speed is really built — but only if both gears belong to the same
// generation, since a ring of one does not grip the other's dogs. Choosing a
// system per gear picks the first that fits each and can land two gears in
// different generations that could otherwise have shared a ring: the 20t exists
// only in the second system, while the 16t exists in both and would be given
// the first.
func ForBoth(a, b int) (System, bool) {
	for _, s := range Systems {
		_, hasA := s.Gears[a]
		_, hasB := s.Gears[b]
		if hasA && hasB {
			return s, true
		}
	}
	return System{}, false
}

// Shiftable reports whether any system has a clutch gear with this many teeth.
//
// 24 is not among them, and that is not an omission in the library. The parts
// called "Technic Gear 24 Tooth Clutch" are the other kind of clutch: a torque
// limiter with a slipping centre, no dogs anywhere on them. Swept against both
// driving rings they stay solid at every distance and every angle, exactly as a
// plain gear does. A 24-tooth gear cannot be dog-shifted; it has to be reached
// through a gear that can.
func Shiftable(teeth int) bool {
	_, ok := For(teeth)
	return ok
}

// ShiftableTeeth is every count that can be shifted, for reporting.
func ShiftableTeeth() []int {
	seen := map[int]bool{}
	var out []int
	for _, s := range Systems {
		for teeth := range s.Gears {
			if !seen[teeth] {
				seen[teeth] = true
				out = append(out, teeth)
			}
		}
	}
	sortInts(out)
	return out
}

func sortInts(v []int) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
