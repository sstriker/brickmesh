// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"sort"

	"github.com/sstriker/brickmesh/internal/ldr"
	"github.com/sstriker/brickmesh/internal/mech"
)

// The colour each part actually comes in.
//
// Not a choice. A model coloured by what the engine finds convenient — every
// axle black, every driving ring red — is a model asking a builder for parts
// that were never made: 4158 does not come in black, and an 18947 has never
// been red in its life.
//
// Measured, from the official models this repository already reads: 8448, 8466,
// 8880, 42056, 42083, 42099, 42110 and 42129. What is placed is the colour a
// part wears most often across those, and EVERY colour seen is kept beside it
// with its count.
//
// Those counts are the honest part. Eight sets is not the catalogue, and the
// table shows its own thinness: a 6641 catch turns up red eight times and light
// grey once, a 3648b in two different greys. A part with one sighting has been
// seen once, not settled. If a part is made in a colour none of these sets used
// — and it will be — this table does not know and cannot say so.
//
// The axles are the pleasing part: LEGO codes them by length, and the sets say
// so without being asked. Axle 4 black, axle 7 light bluish grey, axle 9
// yellow, axle 3 light bluish grey, the long pin blue.
var partColour = map[string]int{
	"18946.dat":    4,  // red 35; 4 set(s)
	"18947.dat":    72, // dk bl grey 14; 4 set(s)
	"18948.dat":    15, // white 18; 4 set(s)
	"2780.dat":     0,  // black 1939; 7 set(s)
	"32013.dat":    0,  // black 68, orange 21, dk bl grey 16, lt bl grey 7, red 6, lt grey 2; 7 set(s)
	"32034.dat":    0,  // black 89, lt bl grey 8, lt grey 6, orange 3, red 2, yellow 1; 7 set(s)
	"32073.dat":    14, // yellow 54, black 27, lt bl grey 25; 7 set(s)
	"32269.dat":    7,  // lt grey 4, black 4; 3 set(s)
	"32270.dat":    0,  // black 33, lt grey 4; 7 set(s)
	"32316.dat":    0,  // black 113, dk bl grey 20, lt bl grey 12, 330 12, 321 4, 27 3, white 1, 191 1; 6 set(s)
	"32523.dat":    71, // lt bl grey 71, black 29, white 22, orange 22, dk bl grey 8, 27 4, 191 4; 6 set(s)
	"32524.dat":    0,  // black 70, lt bl grey 29, orange 14, dk bl grey 13, 321 11, 272 10, 330 8, 10 5, red 2, white 2, yellow 1; 6 set(s)
	"32525.dat":    0,  // black 43, dk bl grey 30, 272 20, orange 10, 321 9, lt bl grey 9, 10 9, 330 6, white 5; 5 set(s)
	"35188.dat":    25, // orange 5; 3 set(s)
	"3647.dat":     7,  // lt grey 14; 3 set(s)
	"3648b.dat":    72, // dk bl grey 9, lt grey 6; 5 set(s)
	"3704.dat":     0,  // black 11; 2 set(s)
	"3705.dat":     0,  // black 126, red 20; 8 set(s)
	"3706.dat":     0,  // black 69, red 13; 8 set(s)
	"3707.dat":     0,  // black 35, red 4; 6 set(s)
	"3708.dat":     0,  // black 12, red 11; 6 set(s)
	"3737.dat":     0,  // black 29, red 2; 7 set(s)
	"3749.dat":     7,  // lt grey 64, tan 10; 6 set(s)
	"4019.dat":     7,  // lt grey 26; 3 set(s)
	"40490.dat":    0,  // black 65, lt bl grey 20, dk bl grey 6, 28 1; 6 set(s)
	"41239.dat":    71, // lt bl grey 28, black 27, 330 10, orange 10, dk bl grey 3, 272 2; 5 set(s)
	"41677.dat":    0,  // black 40, dk bl grey 31, blue 20, lt bl grey 15, red 7; 5 set(s)
	"44294.dat":    71, // lt bl grey 26, yellow 10; 5 set(s)
	"4519.dat":     71, // lt bl grey 201, black 100, yellow 33; 8 set(s)
	"60485.dat":    14, // yellow 4, lt bl grey 1; 3 set(s)
	"6536.dat":     0,  // black 110, dk bl grey 88, lt bl grey 51, lt grey 14, orange 8, blue 7, white 3, red 2; 8 set(s)
	"6538a.dat":    7,  // lt grey 16, dk grey 12, black 9; 2 set(s)
	"6539.dat":     7,  // lt grey 8; 3 set(s)
	"65414c01.dat": 71, // lt bl grey 2; 1 set(s)
	"6542a.dat":    8,  // dk grey 16; 3 set(s)
	"6558.dat":     1,  // blue 915, black 145; 7 set(s)
	"6628.dat":     0,  // black 44, red 34; 7 set(s)
	"6641.dat":     4,  // red 8, lt grey 1; 4 set(s)
	"76019.dat":    15, // white 1; 1 set(s)
}

// standIn is what a part gets when no official model to hand has one.
//
// Light bluish grey, because it is the colour most Technic parts are made in
// and the least likely to be a lie. It is still a guess, and standInColours
// says which parts wear it so nobody buys from the picture.
const standIn = 71

// colour is what to place a part in.
func colour(name string) int {
	c, _ := colourFor(name)
	return c
}

// colourFor is the colour to place a part in, and whether that was measured.
func colourFor(name string) (int, bool) {
	if c, ok := partColour[name]; ok {
		return c, true
	}
	return standIn, false
}

// reportStandIns names the parts shown in a colour nobody checked.
//
// Worth saying out loud. The rest of the model is measured, and a reader has no
// way to tell which parts of the picture are and which are not.
func reportStandIns(res *Result) {
	if res.Model == nil {
		return
	}
	seen := map[string]bool{}
	for _, p := range res.Model.Parts {
		if _, ok := partColour[p.Name]; !ok && p.Color != ldr.ColorRed &&
			p.Color != ldr.ColorYellow {
			seen[p.Name] = true
		}
	}
	if len(seen) == 0 {
		return
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	res.Findings = append(res.Findings, mech.Finding{
		Level: "WARN", Check: "parts", Detail: fmt.Sprintf(
			"%d part(s) are shown in a stand-in colour, light bluish grey, "+
				"because no official model to hand has one: %v. Every other "+
				"part wears the colour it is most often made in. Check these "+
				"against a catalogue before buying from the picture",
			len(names), names)})
}

// PlacedColours is every colour code this engine puts in a model, so a renderer
// can be checked against it rather than drifting from it.
func PlacedColours() []int {
	seen := map[int]bool{standIn: true, ldr.ColorRed: true, ldr.ColorYellow: true}
	for _, c := range partColour {
		seen[c] = true
	}
	out := make([]int, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Ints(out)
	return out
}
