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

	// Catch is the part that moves the ring, and CatchReach how far out from
	// the shaft it sits, in LDU. Empty when nothing here knows one.
	//
	// Not measurable from the parts. Every position where a catch reaches a
	// ring's groove collides at some angle, because LDraw models nominal
	// surfaces and a fork that straddles a groove touches it — so a sweep can
	// confirm a placement but cannot find one. These come from an official
	// model that has both parts in it.
	Catch      string
	CatchReach float64
	// CatchAlong and CatchOut name the catch's own axes that point along the
	// shaft and out from it. The two generations differ: the first's catch is
	// an arm that reaches back along its own z, the second's a collar whose
	// face is its own z, so one frame cannot serve both.
	CatchAlong, CatchOut byte

	// A catch does not travel with its ring. Every axle hole in both of them
	// runs across the shaft, not along it, so neither can be threaded onto a
	// shaft-parallel axle and pushed: what moves the ring is the catch turning
	// on a fixed axle. Read from the parts' own axle-hole primitives.
	//
	// CatchTurnAxis is which of the catch's own axes the axle runs along.
	// CatchPivot is where that hole sits along the part's own z, in LDU —
	// which is where both of them put it, whatever else differs.
	CatchTurnAxis byte
	CatchPivot    float64
	// CatchSlides says the catch is threaded on a shaft-parallel axle and
	// pushed along it, rather than swinging or turning on one. A third kind of
	// motion, and the plainest: what moves the ring is the catch's own travel.
	CatchSlides bool
	// Drum is the barrel selector a hand turns, Ball the pin on the catch that
	// rides its groove, and DrumReach how far the drum's axis sits beyond the
	// catch's origin — along the same way out, on an axis parallel to the
	// shaft.
	//
	// DrumSteps is how many detented positions it has round its turn. Not
	// measured from the part: 4158 is catalogued as "Gear Shifter with Groove,
	// 8 Steps, Letter Markings", and the letters A to H are moulded beside the
	// track.
	//
	// DrumPerLDU is how far it turns to carry the fork one LDU, in degrees.
	Drum, Ball string
	DrumReach  float64
	DrumSteps  int
	DrumPerLDU float64
	// CatchSide is how far the catch's origin sits from the point where it
	// grips, along the third axis of its own frame — square to both the shaft
	// and the way out — in LDU. Zero for a catch whose origin is on that line,
	// which the first two are.
	//
	// Signed, and the sign came from the model rather than from a sweep: with
	// the frame CatchAlong and CatchOut build, the fork's origin lands 20 LDU
	// back along that axis from the ring's centre.
	CatchSide float64
	// CatchArm is the distance from that pivot to the end that sits in the
	// groove, in LDU. Set for a lever, whose swing follows from it: turning by
	// asin(travel/arm) moves the tip along the shaft by the ring's travel.
	// Zero for a cam, where the turn is about an axle parallel to the shaft and
	// no arm length relates the two.
	CatchArm float64
	// CatchPerLDU is how far a cam turns to move its ring one LDU, in degrees,
	// and CatchSeat how far apart its seats are, in LDU. Both measured off the
	// part: see the note on Second.
	CatchPerLDU, CatchSeat float64

	// EngageFit is how much nominal overlap this system's ring and clutch gear
	// have when engaged, in LDU — how much a sweep must be told to forgive
	// before it can see the dogs sitting in the recesses rather than a solid
	// block.
	//
	// Per system because the two are modelled differently, which is a fact
	// about the LDraw parts and not about the mechanism. Measured: at their
	// engaged distances a 6539 on a 6542a needs none, and a tenth of an LDU
	// frees it completely; an 18947 on an 18946 needs a quarter, and reads as
	// solid without it.
	EngageFit float64
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

	// From LDraw's official 8448, which has a 6641 and three 6539s in it: the
	// catch sits 60 LDU out from the shaft on a perpendicular, level with the
	// ring. Confirmed by sweeping — at 60 the catch reaches the groove and the
	// ring still turns, at 55 it is buried and at 65 it has let go.
	//
	// The same model confirms the engaged distance above from a second,
	// independent direction: its rings sit exactly 30 LDU from the clutch gears
	// either side of them, which is the 3.0 half studs measured here.
	// None: its dogs meet the gear face to face and any slack at all separates
	// them. 16 windows over 29% of a revolution at nothing forgiven.
	EngageFit: 0,

	Catch: "6641.dat", CatchReach: 60,
	CatchAlong: 'y', CatchOut: 'z',
	// A lever. Its through-hole runs across both the shaft and the way out, at
	// 20 LDU in from the origin along its own z, and the end that reaches the
	// groove is at 46 — so the arm is 26 LDU and the swing follows from the
	// travel rather than being chosen.
	CatchTurnAxis: 'x', CatchPivot: -20, CatchArm: 26,
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

	// Three LDraw sets put a shared ring exactly 40 LDU from each of two clutch
	// gears 80 apart, and two more catch one mid-shift at 30 and 50. So the
	// engaged distance is 30 LDU, and the two engaged positions are 10 either
	// side of the midpoint.
	//
	// The sweep disagrees, and is wrong in the way it is always wrong here: at
	// 30.0 it reads TOO DEEP and only clears at 30.5. Dogs at full engagement
	// touch — that is what engagement is — and LDraw models nominal surfaces,
	// so the instrument reports the fit as a collision. An earlier reading took
	// 3.25 from the middle of the "safe" band and put every ring half a stud
	// out, which is why nothing lined up with the catch's seats.
	Engaged: 3.0, Clear: 4.0,
	RingHalf: 2.8, JoinerHalf: 3.0, JoinerReach: 2.0,

	// From LDraw's official 42110 and 42083, which have 35188s beside 18947s:
	// the catch sits 40 LDU out from the shaft on a perpendicular, level with
	// the ring, its own z along the shaft. Confirmed by sweeping — at 40 it
	// reaches into the groove and the ring still turns, at 35 it is buried and
	// by 55 it no longer reaches.
	//
	// Not the only catch that fits: 42110, 42083 and 42056 all put a 6641
	// against an 18947, at the same 60 LDU and in the same frame it uses on a
	// 6539. An earlier note here said it did not fit, on the strength of a
	// sweep reading "clear" at 60 — but clear is what a working fork reads as.
	// It straddles the channel rather than bottoming in it: its tip comes to
	// 17.2 from the axis, inside the flanges at 18 and well outside the groove
	// floor. 35188 is preferred because 40 LDU of room is easier to find than
	// 60, not because it is the only one.
	//
	// 42083 has two catches whose nearest ring is ambiguous and it does not
	// matter: 35188 measures ±27.60 in both x and y, so it is symmetric across
	// the axis the ambiguity is about and both readings are the same placement.
	// A quarter of an LDU, without which the sweep reads a solid block at the
	// engaged distance the official models put this ring at.
	EngageFit: 0.25,

	Catch: "35188.dat", CatchReach: 40,
	CatchAlong: 'z', CatchOut: 'x',

	// A cam, and the part's name says so: "Changeover Rotary Catch". Its axle
	// hole runs along its own z, 10 LDU in — which this placement puts parallel
	// to the shaft, so turning it cannot swing anything along the shaft the way
	// a lever does. It is a face cam, and the rim says exactly how it works.
	//
	// Its outermost radius, 27.60, comes round four times, and the rim sits at
	// a different height along the axle at each:
	//
	//	  0 deg   a 3.0 LDU tine centred at z = 0.00
	//	 90 deg   a broad lobe centred at z = +9.26
	//	180 deg   the same tine, z = 0.00
	//	270 deg   the mirror lobe, z = -9.26
	//
	// So a quarter turn moves the ring 10 LDU and there are three seats:
	// -90, 0 and +90 degrees for -10, 0 and +10. That is a three-position
	// shifter, which is exactly what a ring between two clutch gears needs,
	// and it matches the -10/0/+10 offsets the official models were caught at.
	// Nothing here is assumed any more.
	CatchTurnAxis: 'z', CatchPivot: -10,
	CatchPerLDU: 9, CatchSeat: 10,
}

// Systems in the order they are preferred. The first generation is smaller —
// two studs of shaft against three — so it wins where either would do.
// Early is the 1980s system: a two-stud driving ring with eight dogs, moved by
// a fork that slides rather than swings.
//
// Older than First despite coming last here, and it shares First's gears — a
// 6542a engages either ring. What it adds is a 24-tooth shift, which neither of
// the others can do: 2471 is the only clutch gear at that size.
//
// Measured against the two systems already settled, all three swept the same
// way with nothing forgiven:
//
//	6539  + 6542a   engaged at exactly 30, deep at 29, free at 31
//	18947 + 18946   engaged 30.5 to 34.5
//	2473a + 6542a   engaged 20.5 to 25.5, eight windows, one per dog
//	2473a + 2471    the same band, the same eight windows
//
// So 2.0 half studs, with 20.0 itself reading as interference for the reason
// every exact fit does — LDraw models nominal surfaces. Clear at 3.0 leaves the
// one-stud travel the other two have.
var Early = System{
	Name: "early",
	Ring: "2473a.dat",
	// 18948 rather than First's 6538a, which was the guess and does not fit:
	// 2473a's bore is radius 9.5, nineteen LDU across, and 6538a is twenty.
	// 18948 is 17.6 and goes in. Measured, not assumed the second time.
	Joiner: "18948.dat",
	Gears:  map[int]string{16: "6542a.dat", 24: "2471.dat"},

	Engaged: 2.0, Clear: 3.0,
	RingHalf: 1.8, JoinerHalf: 3.0, JoinerReach: 2.0,
	// The band is five LDU wide and its floor is an exact fit, so half an LDU
	// is what it takes to ask the question from inside the band.
	EngageFit: 0.5,

	// From a model built in Stud.io, which snapped the fork into the ring's
	// groove: a cylinder of radius 11.88 running from z = -5 to +5, ten LDU
	// wide and central. The fork's prong is an arc of inner radius 12.40 with
	// walls at its own z = +/-5 — half an LDU of clearance, and the width
	// exactly. Its arc centre lands 0.587 LDU off the ring's axis, which is
	// the rounding.
	Catch: "4159.dat", CatchReach: 40, CatchSide: -20,
	CatchAlong: 'z', CatchOut: 'y',
	// It slides. Its axle holes run parallel to the shaft, so turning it would
	// carry the prong off the axis and let the ring go; only travel along the
	// axle moves anything.
	//
	// No sweep confirms this placement and none can. A fork that wraps a groove
	// is in contact the whole way round, so the interference test reads TOO
	// DEEP at every tolerance — the model built in Stud.io reads that way too,
	// and it is a model that snapped together and works. This is the same wall
	// the first system hit, which is why its reach came from 8448 rather than
	// from a search. What can be checked is that the prong wraps the axis, and
	// the tests check that.
	CatchSlides: true,

	// The barrel selector, from the second model built in Stud.io: the drum on
	// an axis parallel to the shaft, 40 LDU beyond the fork's own origin and so
	// 80 from the shaft, with a towball in the fork's pin hole exactly 40.000
	// from that axis and reaching in to 22.00 against a groove floor of 22.11.
	//
	// How far it turns comes from the letters, which is better evidence than
	// the model that was measured first.
	//
	// The builder: the drum at D puts a four-speed compound in 1st and at A in
	// 3rd. Those are the two positions of one shared ring, whose travel is 20
	// LDU, and A to D is three of the eight steps — 135 degrees. So 6.75
	// degrees a LDU, detent to detent.
	//
	// It supersedes 11 degrees a LDU, taken from a model with the drum turned
	// by hand to 110 degrees for 10.01 LDU. The builder said at the time that
	// the ball might not sit properly in the groove, and measured it did not:
	// its tip stood 4.33 LDU clear of any surface of the drum against 1.77 in
	// the other position. A hand-turned angle beats nothing and loses to a
	// detent.
	//
	// Corroborated from the part, which is the first time the track has been
	// confirmed by anything outside itself: reading its surface, the two places
	// the groove leaves its flat run are about 130 degrees apart, against the
	// letters' 135, and the angular bins that measurement uses are 5 degrees
	// wide.
	Drum: "4158.dat", Ball: "6628.dat", DrumReach: 40, DrumSteps: 8,
	DrumPerLDU: 6.75,
}

var Systems = []System{First, Second, Early}

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
