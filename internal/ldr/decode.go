// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package ldr

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sstriker/brickmesh/internal/geom"
)

// Placed is a part read out of a file, in the model's own coordinates.
//
// Flat, not a tree. A file may nest submodels many deep and place the same one
// several times over; what anything downstream wants to know is where the
// bricks ended up, so the nesting is composed away here.
type Placed struct {
	Name  string
	Color int
	Rot   geom.Mat3
	Pos   geom.Vec3
	// Depth is how many submodels down it was found, for reporting.
	Depth int
}

// maxDepth stops a file that references itself. A cycle is not a thing LDraw
// forbids, only a thing it never means.
const maxDepth = 32

// Decode reads an .ldr or .mpd and returns every part in world coordinates.
//
// A submodel reference and a part reference look identical — both are type 1
// lines — and which it is depends only on whether the file declares a submodel
// by that name. So the whole file is read first and resolved afterwards.
//
// Names are matched case-insensitively and the suffix is kept: LDraw is written
// by many hands and "3001.DAT", "3001.dat" and "s\3001.dat" all occur.
func Decode(r io.Reader) ([]Placed, error) {
	subs, isPart, order, err := readFiles(r)
	if err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return nil, fmt.Errorf("no content: not an LDraw model")
	}
	var out []Placed
	var walk func(name string, rot geom.Mat3, pos geom.Vec3, depth int)
	walk = func(name string, rot geom.Mat3, pos geom.Vec3, depth int) {
		if depth > maxDepth {
			return
		}
		for _, ref := range subs[name] {
			// Compose: the child's placement, then this one's.
			r2 := rot.Mul(ref.Rot)
			p2 := rot.Apply(ref.Pos).Add(pos)
			if _, isSub := subs[ref.Name]; isSub && !isPart[ref.Name] {
				walk(ref.Name, r2, p2, depth+1)
				continue
			}
			out = append(out, Placed{
				Name: partName(ref.Name), Color: ref.Color,
				Rot: r2, Pos: p2, Depth: depth,
			})
		}
	}
	walk(order[0], geom.Rotations[0], geom.Vec3{}, 0)
	return out, nil
}

// readFiles splits an .mpd into its submodels and reads the type 1 lines of
// each. A file with no "0 FILE" is one model, named for convenience.
//
// It also records which of them are PARTS rather than submodels. An .mpd may
// carry a copy of a part file inside it — OMR does this for parts that are not
// in the official library yet — and such a file looks exactly like a submodel
// except that its contents are the part's own geometry. Descending into one
// loses the part and gains a few thousand primitives: 42110 read back as 9,297
// parts, none of them the catch it was opened for.
func readFiles(r io.Reader) (map[string][]Placed, map[string]bool, []string, error) {
	subs := map[string][]Placed{}
	isPart := map[string]bool{}
	var order []string
	cur := ""

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24) // OMR models run to megabytes
	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		if upper := strings.ToUpper(text); strings.HasPrefix(upper, "0 FILE ") {
			cur = normalize(text[len("0 FILE "):])
			if _, seen := subs[cur]; !seen {
				subs[cur] = nil
				order = append(order, cur)
			}
			continue
		}
		if up := strings.ToUpper(text); strings.HasPrefix(up, "0 !LDRAW_ORG ") && cur != "" {
			// Part, Subpart, Primitive, Unofficial_Part and so on all mean the
			// contents are a shape rather than a build.
			kind := up[len("0 !LDRAW_ORG "):]
			if strings.Contains(kind, "PART") || strings.Contains(kind, "PRIMITIVE") {
				isPart[cur] = true
			}
			continue
		}
		if !strings.HasPrefix(text, "1 ") {
			continue
		}
		p, ok := parseRef(text)
		if !ok {
			continue // a malformed line is skipped, not fatal: real files have them
		}
		if cur == "" {
			cur = "<model>"
			subs[cur] = nil
			order = append(order, cur)
		}
		subs[cur] = append(subs[cur], p)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, nil, err
	}
	return subs, isPart, order, nil
}

// parseRef reads one type 1 line: colour, position, a row-major 3x3, and a name
// which may contain spaces.
func parseRef(text string) (Placed, bool) {
	f := strings.Fields(text)
	if len(f) < 15 {
		return Placed{}, false
	}
	colour, err := strconv.Atoi(f[1])
	if err != nil {
		return Placed{}, false
	}
	var n [12]float64
	for i := 0; i < 12; i++ {
		v, err := strconv.ParseFloat(f[2+i], 64)
		if err != nil {
			return Placed{}, false
		}
		n[i] = v
	}
	return Placed{
		Name:  normalize(strings.Join(f[14:], " ")),
		Color: colour,
		Pos:   geom.Vec3{X: n[0], Y: n[1], Z: n[2]},
		Rot: geom.Mat3{
			{n[3], n[4], n[5]},
			{n[6], n[7], n[8]},
			{n[9], n[10], n[11]},
		},
	}, true
}

// partName strips the prefix OMR puts on a part it carries a copy of.
//
// "42110 - 35188.dat" is the part 35188 under the set that needed it inlined.
// Reported under its own number, the parts library can be asked about it and
// two sets that both inline it agree they used the same part.
func partName(name string) string {
	i := strings.LastIndex(name, " - ")
	if i < 0 {
		return name
	}
	rest := name[i+3:]
	if !strings.HasSuffix(rest, ".dat") || strings.ContainsAny(rest, " /") {
		return name
	}
	return rest
}

// normalize makes a name comparable: lower case, forward slashes, trimmed.
func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "\\", "/")))
}
