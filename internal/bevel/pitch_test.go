// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package bevel

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/ldraw"
)

// pitchRadius is a gear's outermost material, taken as the median over azimuth.
//
// The median rather than the maximum, because a gear is round and the things
// bolted to it are not: 24014 is a 12-tooth double bevel with an axle extension
// reaching to 49.5, and the maximum reads that instead of its teeth. Every
// azimuth of a gear sees the same tooth radius; only a few see an extension.
func pitchRadius(g *ldraw.Geometry) float64 {
	const bins = 36
	var best [bins]float64
	for _, v := range g.Verts {
		r := math.Hypot(v.X, v.Y)
		a := math.Atan2(v.Y, v.X)
		if a < 0 {
			a += 2 * math.Pi
		}
		i := int(a / (2 * math.Pi) * bins)
		if i >= bins {
			i = bins - 1
		}
		if r > best[i] {
			best[i] = r
		}
	}
	out := best[:]
	sort.Float64s(out)
	return out[bins/2]
}

// The three double bevels agree to a hundredth, which is what says the overhang
// is a designed addendum rather than a coincidence of three shapes.
func TestTheDoubleBevelsShareOneAddendum(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")
	var proud []float64
	for _, c := range []struct {
		name  string
		teeth int
	}{{"32270.dat", 12}, {"32269.dat", 20}, {"32498.dat", 36}} {
		g, err := lib.Geometry(c.name)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		proud = append(proud, pitchRadius(g)-float64(c.teeth)*1.25)
	}
	for _, p := range proud[1:] {
		if math.Abs(p-proud[0]) > 0.02 {
			t.Errorf("the double bevels stand %v proud of their pitch circles; "+
				"they should share one addendum", proud)
		}
	}
}

// And given the module, where the two sit is geometry rather than a rule.
//
// Two pitch circles have to touch. Put the crossing at the origin with A's axis
// along z and B's along x: A's circle is (Ra cos, Ra sin, da) and B's is
// (db, Rb sin, Rb cos). A common point forces db = Ra and da = Rb — each gear
// at the OTHER's pitch radius from the crossing, which is what layout applies.
//
// Stated as a test because it is the half of the answer that needs no library:
// nine measurement approaches disagreed about bevel engagement, and all nine
// were asking where the surfaces touch rather than where the pitch circles do.
func TestEachBevelSitsAtTheOthersPitchRadius(t *testing.T) {
	const module = 1.25
	for _, c := range []struct{ ta, tb int }{{12, 20}, {12, 12}, {20, 36}, {8, 24}} {
		ra, rb := float64(c.ta)*module, float64(c.tb)*module
		// The placement the geometry forces.
		da, db := rb, ra
		a := geom.Vec3{X: ra, Z: da} // a point on A's pitch circle
		b := geom.Vec3{X: db, Z: rb} // and on B's
		if a.Sub(b).Len() > 1e-9 {
			t.Errorf("%dt/%dt: the pitch circles do not meet at %v and %v, so "+
				"placing each at the other's radius is not what makes them mesh",
				c.ta, c.tb, a, b)
		}
	}
}

// toothRe reads the tooth count out of a part's own title.
var toothRe = regexp.MustCompile(`^0\s+Technic Gear\s+(\d+)\s+Tooth\b`)

// notOnTheModule are the parts whose title says "Technic Gear N Tooth" and
// whose teeth are not on the 1.25 LDU module. Named rather than tolerated
// silently, because each is a different reason.
var notOnTheModule = map[string]string{
	// An old tooth system: 14 teeth on a radius of 46, which is 3.29 a tooth.
	"641.dat": "a 1970s gear, not the Technic module",
	// True bevels, whose outermost material is the large end of a cone rather
	// than a tooth tip on the reference plane, so this measure does not apply.
	"69761.dat": "a true bevel; its outer radius is the big end of a cone",
	"69762.dat": "a true bevel; its outer radius is the big end of a cone",
	// Not a gear at all.
	"32060.dat": "a timing wheel, which meshes with nothing",
}

// Every gear in the library, not nine of them.
//
// The module is what fixes where a bevel pair sits — see the tests above — so
// it is worth knowing it holds generally rather than for a list somebody typed.
// Typing the list was in fact the near-miss: an earlier draft had 94925 down as
// 12 tooth when its own title says 16, and it looked like a wild outlier until
// the title was read. The count comes from the part now.
func TestEveryGearInTheLibraryIsOnTheModule(t *testing.T) {
	if os.Getenv("BRICKMESH_LIBRARIES") != "1" {
		t.Skip("set BRICKMESH_LIBRARIES=1 to run against the real libraries")
	}
	lib := ldraw.New("")
	if _, err := lib.Geometry("3001.dat"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(lib.Root, "parts"))
	if err != nil {
		t.Fatal(err)
	}
	tested := 0
	seenException := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".dat" {
			continue
		}
		f, err := os.Open(filepath.Join(lib.Root, "parts", e.Name()))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		var title string
		if sc.Scan() {
			title = sc.Text()
		}
		f.Close()
		m := toothRe.FindStringSubmatch(title)
		if m == nil {
			continue
		}
		if why, skip := notOnTheModule[e.Name()]; skip {
			seenException[e.Name()] = true
			_ = why
			continue
		}
		teeth, _ := strconv.Atoi(m[1])
		g, err := lib.Geometry(e.Name())
		if err != nil || len(g.Verts) == 0 {
			continue
		}
		tested++
		proud := pitchRadius(g) - float64(teeth)*1.25
		if proud < 0.5 || proud > 3.0 {
			t.Errorf("%s (%s): %d teeth put its pitch circle at %.2f, and its "+
				"outermost material is %.2f past that. Either it is not on the "+
				"module or it belongs in notOnTheModule with a reason",
				e.Name(), title, teeth, float64(teeth)*1.25, proud)
		}
	}
	if tested < 30 {
		t.Fatalf("only %d gear(s) found; the title match has stopped working "+
			"and this test is checking almost nothing", tested)
	}
	// The exception list does not get to go stale.
	for name := range notOnTheModule {
		if !seenException[name] {
			t.Errorf("%s is listed as off the module and is no longer in the "+
				"library under that name; take it out", name)
		}
	}
	t.Logf("%d gear(s) on the module, %d named exceptions", tested, len(notOnTheModule))
}
