// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package extract

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldraw"
	"github.com/sstriker/brickmesh/internal/shadow"
)

// holeScore is how much of a ring of pin-hole radius about an axis through pos
// is covered by mesh vertices, in twelfths.
//
// Counting vertices at that radius was the first try and it was not specific:
// a beam's own rounded outside puts plenty there, and one part scored HIGHER at
// a position deliberately moved off its holes than at the holes themselves. A
// hole's wall is a full cylinder, so what distinguishes it is that the
// vertices go all the way round.
func holeScore(g *ldraw.Geometry, pos, axis geom.Vec3, radius float64) int {
	a := axis.Unit()
	u, v := perpBasis(a)
	var seen [12]bool
	for _, p := range g.Verts {
		d := p.Sub(pos)
		along := d.Dot(a)
		if math.Abs(along) > 6 {
			continue
		}
		across := d.Sub(a.Scale(along))
		if math.Abs(across.Len()-radius) > 1.0 {
			continue
		}
		ang := math.Atan2(across.Dot(v), across.Dot(u))
		if ang < 0 {
			ang += 2 * math.Pi
		}
		seen[int(ang/(2*math.Pi)*12)%12] = true
	}
	n := 0
	for _, b := range seen {
		if b {
			n++
		}
	}
	return n
}

func perpBasis(a geom.Vec3) (geom.Vec3, geom.Vec3) {
	seed := geom.Vec3{X: 1}
	if math.Abs(a.X) > 0.9 {
		seed = geom.Vec3{Y: 1}
	}
	u := seed.Sub(a.Scale(seed.Dot(a))).Unit()
	return u, a.Cross(u).Unit()
}

// Now the question: which local directions are a three-axis grid's axes?
//
// Every ordering of X, Y and Z is tried, and each is scored by whether the
// positions it produces are real holes. A wrong ordering puts them in solid
// material, which the ring test above tells apart from a bore.
func TestNoOtherAxisOrderingFitsThePartsBetter(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("needs the libraries")
	}
	root, err := shadow.Ensure("")
	if err != nil {
		t.Fatal(err)
	}
	sh := shadow.Open(root)
	lib := ldraw.New("")
	parts, err := sh.Parts()
	if err != nil {
		t.Fatal(err)
	}

	axesOf := map[string][3]geom.Vec3{
		"XYZ": {{X: 1}, {Y: 1}, {Z: 1}},
		"XZY": {{X: 1}, {Z: 1}, {Y: 1}},
		"YXZ": {{Y: 1}, {X: 1}, {Z: 1}},
		"YZX": {{Y: 1}, {Z: 1}, {X: 1}},
		"ZXY": {{Z: 1}, {X: 1}, {Y: 1}},
		"ZYX": {{Z: 1}, {Y: 1}, {X: 1}},
	}
	order := []string{"XYZ", "XZY", "YXZ", "YZX", "ZXY", "ZYX"}
	good := map[string]int{}
	bad := map[string]int{}
	byGender := map[string]int{}
	okByGender := map[string]int{}
	var failed []string
	judged := 0

	for _, p := range parts {
		for _, s := range sh.Snaps(p) {
			if s.Grid == "" || !s.Generic() {
				continue
			}
			counts, spacings, centered := ParseGrid(s.Grid)
			if len(counts) != 3 {
				continue
			}
			nd := 0
			for i := range counts {
				if counts[i] > 1 && spacings[i] != 0 {
					nd++
				}
			}
			if nd < 2 {
				continue // the ordering changes nothing here
			}
			radius, ok := secsRadius(s.Secs)
			if !ok {
				continue
			}
			g, err := lib.Geometry(p + ".dat")
			if err != nil {
				continue
			}
			axis := s.Axis()
			judged++
			byGender[s.Gender]++
			for _, name := range order {
				dirs := axesOf[name]
				all := true
				for _, da := range offsets(counts[0], spacings[0], centered[0]) {
					for _, db := range offsets(counts[1], spacings[1], centered[1]) {
						for _, dc := range offsets(counts[2], spacings[2], centered[2]) {
							at := s.Pos.
								Add(s.Ori.Apply(dirs[0]).Scale(da)).
								Add(s.Ori.Apply(dirs[1]).Scale(db)).
								Add(s.Ori.Apply(dirs[2]).Scale(dc))
							if holeScore(g, at, axis, radius) < 12 {
								all = false
							}
						}
					}
				}
				if all {
					good[name]++
					if name == "XYZ" {
						okByGender[s.Gender]++
					}
				} else {
					bad[name]++
					if name == "XYZ" {
						failed = append(failed, fmt.Sprintf("%s %s %q", p, s.Gender, s.Grid))
					}
				}
			}
		}
	}
	if judged < 20 {
		t.Fatalf("only %d snap(s) could judge; the measure has stopped applying "+
			"and this test is no longer evidence of anything", judged)
	}
	fmt.Printf("  %d snap(s) could judge\n", judged)
	for _, name := range order {
		fmt.Printf("   %-4s every position a real hole in %d, not in %d\n",
			name, good[name], bad[name])
	}
	for _, name := range order {
		if name == "XYZ" {
			continue
		}
		if good[name] >= good["XYZ"] {
			t.Errorf("%s fits the parts as well as XYZ (%d against %d). Expand "+
				"assumes X then Y then Z, and that is no longer the only reading",
				name, good[name], good["XYZ"])
		}
	}
	// The measure only sees bores, so a male snap it cannot judge is not
	// evidence against the ordering; a female one is.
	if okByGender["F"]*2 < byGender["F"] {
		t.Errorf("XYZ puts holes where holes are in only %d of %d female snaps; "+
			"the ordering Expand uses is not carrying its own weight",
			okByGender["F"], byGender["F"])
	}
	fmt.Println("  XYZ by gender:")
	for g, n := range byGender {
		fmt.Printf("    gender %-2q %d of %d\n", g, okByGender[g], n)
	}
	fmt.Println("  XYZ failures:")
	for i, f := range failed {
		if i >= 12 {
			break
		}
		fmt.Println("   ", f)
	}
}

// secsRadius is the bore a snap declares. LDCad writes sections as triples — a
// shape letter, a radius and a length — repeated, so a pin hole reads
// "R 8 2  R 6 16  R 8 2": a chamfer, the bore, a chamfer.
//
// The radius of the LONGEST section, not the widest. Taking the widest picked
// the two-unit chamfer, looked for a ring where there is only a lip, and scored
// zero on every part built that way — which read as "no ordering works" and was
// very nearly taken for an answer.
func secsRadius(secs string) (float64, bool) {
	f := strings.Fields(secs)
	best, longest, ok := 0.0, 0.0, false
	for i := 0; i+2 < len(f); i++ {
		if len(f[i]) != 1 || (f[i][0] >= '0' && f[i][0] <= '9') {
			continue
		}
		r, err1 := strconv.ParseFloat(f[i+1], 64)
		l, err2 := strconv.ParseFloat(f[i+2], 64)
		if err1 != nil || err2 != nil || r <= 0 {
			continue
		}
		if l > longest {
			best, longest, ok = r, l, true
		}
	}
	return best, ok
}
