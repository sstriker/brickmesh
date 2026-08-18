// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package pipeline

import (
	"fmt"
	"math"
	"sort"

	"brickmesh/internal/geom"
	"brickmesh/internal/layout"
	"brickmesh/internal/mech"
	"brickmesh/internal/synth"
)

// checkFraming reports shafts that no beam can bear together.
//
// A beam's holes sit a stud apart, so one beam can bear two parallel shafts
// only if they are a whole number of studs apart. Two studs is fine; two and a
// half is not, and no chain of beams fixes it either — their holes fall on
// sublattices half a stud out of step, so nothing pinned to one ever reaches
// the other.
//
// That is worth saying before the structural search runs rather than after it
// returns a frame in pieces, because the answer is not more searching. It is a
// different spacing. This is the same rule docs/findings.md records from the
// other end: a gear pair whose teeth sum to a multiple of 8 lands on a valid
// centre distance, but only a multiple of 16 lands on one you can frame.
func checkFraming(res *Result) {
	lines := linesOf(res.Layout)
	if len(lines) < 2 {
		return
	}

	// Two lines are in the same piece if a beam can span them.
	parent := make([]int, len(lines))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := range lines {
		for j := i + 1; j < len(lines); j++ {
			if spannable(lines[i], lines[j]) {
				parent[find(i)] = find(j)
			}
		}
	}

	groups := map[int][]string{}
	for i, l := range lines {
		root := find(i)
		groups[root] = append(groups[root], l.name)
	}
	if len(groups) < 2 {
		return
	}

	var pieces [][]string
	for _, names := range groups {
		sort.Strings(names)
		pieces = append(pieces, names)
	}
	sort.Slice(pieces, func(i, j int) bool { return pieces[i][0] < pieces[j][0] })

	res.Findings = append(res.Findings, mech.Finding{
		Level: "WARN", Check: "framing", Detail: fmt.Sprintf(
			"no beam can bear these groups of shafts together: %v. A beam's holes "+
				"are a stud apart, so two parallel shafts share one only when they "+
				"are a whole number of studs apart, and %s. Nothing pinned to one "+
				"group reaches the other, so the frame will come out in pieces "+
				"however long the search runs. Space them a whole number of studs "+
				"apart: for a gear pair that means teeth summing to a multiple of "+
				"16, not merely of 8.",
			pieces, describeGaps(lines))})
}

// spannable reports whether one beam could bear both lines: parallel, and a
// whole number of studs apart.
func spannable(a, b line) bool {
	if math.Abs(math.Abs(a.dir.Dot(b.dir))-1) > 1e-6 {
		return false // not parallel; a beam bears one axis at a time
	}
	d := b.at.Sub(a.at)
	d = d.Sub(a.dir.Scale(d.Dot(a.dir))) // only the gap across the shafts counts
	studs := d.Len() / geom.Stud
	return math.Abs(studs-math.Round(studs)) < 1e-6
}

// describeGaps says which spacing is the awkward one.
func describeGaps(lines []line) string {
	var bad []string
	for i := range lines {
		for j := i + 1; j < len(lines); j++ {
			if spannable(lines[i], lines[j]) {
				continue
			}
			d := lines[j].at.Sub(lines[i].at)
			d = d.Sub(lines[i].dir.Scale(d.Dot(lines[i].dir)))
			bad = append(bad, fmt.Sprintf("%s and %s are %.2f studs apart",
				lines[i].name, lines[j].name, d.Len()/geom.Stud))
		}
	}
	if len(bad) == 0 {
		return "some of them are not"
	}
	sort.Strings(bad)
	return bad[0]
}

// line is one shaft axis, named for whichever shaft reached it first.
type line struct {
	name string
	at   geom.Vec3
	dir  geom.Vec3
}

func linesOf(l *layout.Layout) []line {
	if l == nil {
		return nil
	}
	seen := map[[6]float64]bool{}
	var out []line
	names := make([]string, 0, len(l.Place))
	for id := range l.Place {
		names = append(names, id)
	}
	sort.Strings(names)
	for _, id := range names {
		p := l.Place[id]
		if seen[p.Key()] {
			continue
		}
		seen[p.Key()] = true
		out = append(out, line{
			name: id, at: p.Point.Scale(synth.HalfStud), dir: p.Direction,
		})
	}
	return out
}
