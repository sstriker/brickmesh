// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package ldr

import (
	"strconv"
	"strings"
	"testing"

	"github.com/sstriker/brickmesh/internal/geom"
)

func TestHeaderNamesTheModel(t *testing.T) {
	m := New("subtractor")
	out := m.Encode()
	for _, want := range []string{
		"0 subtractor\n", "0 Name: subtractor.ldr\n", "0 Author: brickmesh\n",
		"0 !LDRAW_ORG Model\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.HasSuffix(out, "0\n") {
		t.Error("a model file should end with a bare 0")
	}
}

func TestAPartLineHasTheLDrawShape(t *testing.T) {
	m := New("m")
	m.Add("3648b.dat", ColorLightGray, geom.Rotations[0], geom.Vec3{X: 20, Y: -8, Z: 40}, "")
	line := partLines(t, m.Encode())[0]

	fields := strings.Fields(line)
	// 1, color, x y z, nine matrix entries, name.
	if len(fields) != 15 {
		t.Fatalf("got %d fields, want 15: %q", len(fields), line)
	}
	if fields[0] != "1" {
		t.Errorf("line type %q, want 1", fields[0])
	}
	if fields[1] != strconv.Itoa(ColorLightGray) {
		t.Errorf("color %q", fields[1])
	}
	if fields[14] != "3648b.dat" {
		t.Errorf("part %q", fields[14])
	}
	// Position comes before the matrix.
	if fields[2] != "20" || fields[3] != "-8" || fields[4] != "40" {
		t.Errorf("position %v", fields[2:5])
	}
}

// A point of the part has to land at M*p + t, so the matrix is written row by
// row. Reading it back and applying it is the check.
func TestTheMatrixIsWrittenRowByRow(t *testing.T) {
	rot := geom.Rotations[7]
	m := New("m")
	m.Add("3001.dat", ColorMain, rot, geom.Vec3{X: 5, Y: 6, Z: 7}, "")
	fields := strings.Fields(partLines(t, m.Encode())[0])

	var got geom.Mat3
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			v, err := strconv.ParseFloat(fields[5+r*3+c], 64)
			if err != nil {
				t.Fatal(err)
			}
			got[r][c] = v
		}
	}
	p := geom.Vec3{X: 1, Y: 2, Z: 3}
	if got.Apply(p).Sub(rot.Apply(p)).Len() > 1e-9 {
		t.Errorf("matrix round-tripped wrongly: %v vs %v", got, rot)
	}
}

func TestTheDatSuffixIsAdded(t *testing.T) {
	m := New("m")
	m.Add("3648b", ColorMain, geom.Rotations[0], geom.Vec3{}, "")
	if !strings.HasSuffix(partLines(t, m.Encode())[0], "3648b.dat") {
		t.Error("a part name should end up with .dat")
	}
}

func TestLabelsBecomeComments(t *testing.T) {
	m := New("m")
	m.Add("3648b.dat", ColorMain, geom.Rotations[0], geom.Vec3{}, "24t on the drive shaft")
	if !strings.Contains(m.Encode(), "0 // 24t on the drive shaft\n") {
		t.Error("the label should be emitted as a comment")
	}
}

// Some parsers dislike "-0", which is easy to produce from a rotation.
func TestNegativeZeroIsWrittenPlainly(t *testing.T) {
	m := New("m")
	m.Add("3001.dat", ColorMain, geom.Mat3{{-0, 0, 0}, {0, -0, 0}, {0, 0, -0}},
		geom.Vec3{X: -0}, "")
	if strings.Contains(partLines(t, m.Encode())[0], "-0 ") {
		t.Error("no field should be written as -0")
	}
}

func TestAddLatticeRefusesAnImpossibleRotation(t *testing.T) {
	m := New("m")
	if err := m.AddLattice("3001.dat", ColorMain, 99, geom.Vec3{}, ""); err == nil {
		t.Error("expected an error outside the 24 rotations")
	}
	if err := m.AddLattice("3001.dat", ColorMain, 3, geom.Vec3{}, ""); err != nil {
		t.Errorf("rotation 3 should be fine: %v", err)
	}
	if len(m.Parts) != 1 {
		t.Errorf("the rejected part should not have been added")
	}
}

func TestAnEmptyModelIsStillAValidFile(t *testing.T) {
	out := New("").Encode()
	if !strings.HasPrefix(out, "0 model\n") {
		t.Errorf("an unnamed model should still get a name:\n%s", out)
	}
	if len(partLines(t, out)) != 0 {
		t.Error("no parts expected")
	}
}

// partLines pulls the type-1 lines out of an encoded model.
func partLines(t *testing.T, out string) []string {
	t.Helper()
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "1 ") {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestGroupsAreDeclaredAndReferenced(t *testing.T) {
	m := New("gearbox")
	m.Groups = []Group{
		{Name: "shaft_input", Center: geom.Vec3{}},
		{Name: "shaft_output", Center: geom.Vec3{Z: -40}},
	}
	m.Add("3647.dat", ColorMain, geom.Rotations[0], geom.Vec3{}, "")
	m.Parts[0].Group = "shaft_input"
	m.Add("3648b.dat", ColorMain, geom.Rotations[0], geom.Vec3{Z: -40}, "")
	m.Parts[1].Group = "shaft_output"
	m.Add("3705.dat", ColorMain, geom.Rotations[0], geom.Vec3{}, "") // ungrouped
	out := m.Encode()

	if !strings.Contains(out, "[LID=0]") || !strings.Contains(out, "[LID=1]") {
		t.Error("both groups should be declared, with ids in order")
	}
	// Zero, not the -40 the caller asked for: LDCad reads a centre relative to
	// the group's main item. See TestAGroupCentreIsWrittenRelativeAndSoIsAlwaysZero.
	if !strings.Contains(out, "[name=shaft_output] [center=0 0 0]") {
		t.Errorf("the group's centre should be written relative, as zero:\n%s", out)
	}

	// A GROUP_NXT applies to the line after it, so it has to sit directly
	// above the part it claims and nowhere else.
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, "0 !LDCAD GROUP_NXT") {
			continue
		}
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "1 ") {
			t.Errorf("a GROUP_NXT is not followed by a part line:\n%s", out)
		}
	}
	if got := strings.Count(out, "GROUP_NXT"); got != 2 {
		t.Errorf("got %d GROUP_NXT lines, want 2: the third part has no group", got)
	}
}

func TestGroupIDsAreDistinctAndStable(t *testing.T) {
	seen := map[string]string{}
	for _, name := range []string{
		"shaft_input", "shaft_output", "shaft_first", "shaft_second", "shaft_third",
		"a", "b", "ab", "ba",
	} {
		id := groupID(name)
		if len(id) != 11 {
			t.Errorf("%s: id %q is %d characters", name, id, len(id))
		}
		if other, clash := seen[id]; clash {
			t.Errorf("%s and %s share the id %q", name, other, id)
		}
		seen[id] = name
		if groupID(name) != id {
			t.Errorf("%s: the id changed between calls", name)
		}
	}
}

func TestTheScriptIsReferencedWhenSet(t *testing.T) {
	m := New("m")
	if strings.Contains(m.Encode(), "SCRIPT") {
		t.Error("no script, no SCRIPT line")
	}
	m.Script = "m.lua"
	if !strings.Contains(m.Encode(), "0 !LDCAD SCRIPT [source=m.lua]") {
		t.Error("the script should be referenced from the header")
	}
}

// LDCad reads a group's centre relative to the group's main item, not as a
// point in the model. Its meta reference says so in four words — "Relative
// center to use for this group" — and this engine read it as a model
// coordinate, wrote the point on the shaft, and had LDCad add that to the main
// item's own position and turn every group about somewhere off in space.
//
// Measured rather than argued: a group whose parts sit on a shaft at z=-40,
// declared [center=0 0 -40], reported its centre at z=-80.
//
// Zero is right rather than merely safe. Every part in one of these groups sits
// on the shaft it turns about, so the main item's own origin is a point on the
// axis, which is what the centre has to be.
func TestAGroupCentreIsWrittenRelativeAndSoIsAlwaysZero(t *testing.T) {
	m := New("centres")
	m.Groups = []Group{
		{Name: "on_the_origin", Center: geom.Vec3{}},
		{Name: "off_somewhere", Center: geom.Vec3{X: 140, Z: -40}},
	}
	m.Parts = []Part{{Name: "3737.dat", Rot: geom.Rotations[0],
		Pos: geom.Vec3{X: 140, Z: -40}, Group: "off_somewhere"}}

	out := m.Encode()
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "GROUP_DEF") {
			continue
		}
		if !strings.Contains(line, "[center=0 0 0]") {
			t.Errorf("a group centre reached the file as something other than "+
				"zero, so LDCad will add it to the main item's position and "+
				"turn the group about the wrong point:\n  %s", line)
		}
	}
}

// Two models must not hand LDCad the same group id.
//
// LDCad's GID is globally unique, and it holds it to that: open two models
// whose groups share an id and the second one's groups link to nothing, so the
// first getOri fails with "Active group link needed". Every model this engine
// wrote called its input shaft "shaft_input", so every model collided with
// every other — and opening a reduction beside a gearbox is not a corner case,
// it is what looking at the examples means.
func TestTwoModelsDoNotShareGroupIDs(t *testing.T) {
	ids := func(modelName string) map[string]string {
		m := New(modelName)
		m.Groups = []Group{{Name: "shaft_input"}, {Name: "shaft_output"}}
		out := map[string]string{}
		for _, line := range strings.Split(m.Encode(), "\n") {
			if !strings.Contains(line, "GROUP_DEF") {
				continue
			}
			gid := between(line, "[GID=", "]")
			name := between(line, "[name=", "]")
			out[name] = gid
		}
		return out
	}

	a, b := ids("reduction"), ids("2-speed gearbox")
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("expected two groups each, got %v and %v", a, b)
	}
	for name, gid := range a {
		if b[name] == gid {
			t.Errorf("%q has the same id %q in both models, so opening them "+
				"together leaves one of them linked to nothing", name, gid)
		}
	}

	// And still the same file twice for the same model, or every export churns.
	if again := ids("reduction"); again["shaft_input"] != a["shaft_input"] {
		t.Error("the same model gave a different id the second time")
	}
}

func between(s, from, to string) string {
	i := strings.Index(s, from)
	if i < 0 {
		return ""
	}
	s = s[i+len(from):]
	j := strings.Index(s, to)
	if j < 0 {
		return ""
	}
	return s[:j]
}
