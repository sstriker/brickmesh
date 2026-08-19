// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package extract turns the shadow library into the catalog the engine reads.
//
// The shadow library describes where the holes and pins sit, but compresses
// repeating holes into a grid notation. A search that has to build structures
// needs individual coordinates, so the grids are expanded here.
//
// Grid notation: [grid=<countA> <countB> <stepA> <stepB>], where a count
// preceded by C means centered rather than counting up from zero.
package extract

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
	"brickmesh/internal/shadow"
)

// Port is one connection point: x, y, z, ax, ay, az, cross.
type Port [7]float64

// Entry is a part with its ports, before tiering.
type Entry struct {
	Part      string
	Title     string
	Holes     []Port
	Pins      []Port
	AxleHoles int
}

// Record is the interchange format with internal/catalog. The field names are
// the contract: the reader decodes "id", not "part".
type Record struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Tier  uint8  `json:"tier"`
	Holes []Port `json:"holes"`
	Pins  []Port `json:"pins"`
}

// GridAxes reports how many axes a grid spec describes.
//
// Each axis contributes one count, one spacing, and a leading C when it is
// centered, so with t tokens of which c are C the spec has (t-c)/2 axes.
// Counting tokens alone is ambiguous: six tokens is two centered axes, or
// three uncentered ones.
func GridAxes(spec string) int {
	tok := strings.Fields(spec)
	if len(tok) == 0 {
		return 0
	}
	c := 0
	for _, t := range tok {
		if strings.EqualFold(t, "C") {
			c++
		}
	}
	n := len(tok) - c
	if n%2 != 0 {
		return 0
	}
	return n / 2
}

// ParseGrid reads the grid notation, one entry per axis. "C 3 1 20 0" is three
// positions centered on the snap 20 LDU apart, and one position on the other
// axis.
func ParseGrid(spec string) (counts []int, spacings []float64, centered []bool) {
	n := GridAxes(spec)
	if n == 0 {
		return []int{1, 1}, []float64{0, 0}, []bool{false, false}
	}
	tok := strings.Fields(spec)
	i := 0
	for k := 0; k < n; k++ {
		if strings.EqualFold(tok[i], "C") {
			centered = append(centered, true)
			i++
		} else {
			centered = append(centered, false)
		}
		v, err := strconv.Atoi(tok[i])
		if err != nil {
			return []int{1, 1}, []float64{0, 0}, []bool{false, false}
		}
		counts = append(counts, v)
		i++
	}
	for k := 0; k < n && i+k < len(tok); k++ {
		v, err := strconv.ParseFloat(tok[i+k], 64)
		if err != nil {
			return []int{1, 1}, []float64{0, 0}, []bool{false, false}
		}
		spacings = append(spacings, v)
	}
	if len(spacings) != n {
		return []int{1, 1}, []float64{0, 0}, []bool{false, false}
	}
	return counts, spacings, centered
}

func offsets(count int, spacing float64, centered bool) []float64 {
	if count <= 1 {
		return []float64{0}
	}
	out := make([]float64, count)
	for i := range out {
		idx := float64(i)
		if centered {
			idx -= float64(count-1) / 2
		}
		out[i] = idx * spacing
	}
	return out
}

// Expand returns every concrete position of a snap, grid unfolded.
//
// The grid lies in the snap's LOCAL frame, not in some arbitrary perpendicular
// basis. The cylinder points along Y locally, so the two grid directions are
// local X and Z, carried over by the orientation matrix.
//
// Ninety-two grids in the library declare a THIRD axis, and which local
// direction that one is has not been established — see PLAN.md. Rather than
// guess and place ports where there are none, those keep the one position the
// file states outright, the snap's own, and drop the repeats.
func Expand(s shadow.Snap) []geom.Vec3 {
	counts, spacings, centered := ParseGrid(s.Grid)
	if len(counts) > 2 {
		return []geom.Vec3{s.Pos}
	}
	u := s.Ori.Apply(geom.Vec3{X: 1})
	v := s.Ori.Apply(geom.Vec3{Z: 1})
	var out []geom.Vec3
	for _, da := range offsets(counts[0], spacings[0], centered[0]) {
		for _, db := range offsets(counts[1], spacings[1], centered[1]) {
			out = append(out, s.Pos.Add(u.Scale(da)).Add(v.Scale(db)))
		}
	}
	return out
}

// roundTo rounds half to even, matching numpy. The extractor's output is
// compared against the Python one value by value, and half-away-from-zero
// would disagree on any coordinate landing exactly on a half.
func roundTo(v float64, places int) float64 {
	f := math.Pow(10, float64(places))
	return math.RoundToEven(v*f) / f
}

// EntryFor builds the ports of one part, or nil when the library says nothing
// about it.
func EntryFor(lib *shadow.Library, part string) *Entry {
	snaps := lib.Snaps(part)
	if len(snaps) == 0 {
		return nil
	}
	e := &Entry{Part: part}
	for _, s := range snaps {
		if s.Kind != "SNAP_CYL" && s.Kind != "SNAP_INCL" {
			continue
		}
		// A crane-arm slot, a door hinge or a plug is not a pin.
		if !s.Generic() {
			continue
		}
		female := s.Gender != "M"
		axis := s.Axis()
		cross := 0.0
		if s.Axle() {
			cross = 1
		}
		for _, p := range Expand(s) {
			port := Port{
				roundTo(p.X, 2), roundTo(p.Y, 2), roundTo(p.Z, 2),
				roundTo(axis.X, 3), roundTo(axis.Y, 3), roundTo(axis.Z, 3),
				cross,
			}
			if female {
				e.Holes = append(e.Holes, port)
				if s.Axle() {
					e.AxleHoles++
				}
			} else {
				e.Pins = append(e.Pins, port)
			}
		}
	}
	return e
}

// Preference tiers. The search starts at tier 1 and widens only when it fails
// there, so the larger inventory is paid for only when it is needed.
var tiers = []*regexp.Regexp{
	regexp.MustCompile(`^Technic (Beam|Liftarm)|^Technic Pin\b|Technic Pin with Friction|` +
		`Technic Axle Joiner|Angle Connector|Technic Bush|^Technic Axle\b`),
	regexp.MustCompile(`^Technic `),
	regexp.MustCompile(`.`),
}

// TierOf grades a part by its title: 1 common, 2 all Technic, 3 everything.
func TierOf(title string) uint8 {
	for i, re := range tiers {
		if re.MatchString(title) {
			return uint8(i + 1)
		}
	}
	return 3
}

// Usable drops everything that is not a real orderable part.
//
// LDraw marks subparts with a ~ before the title: the front of a motor housing,
// the body of a horse. Those exist only as a building block inside another file
// and cannot be ordered on their own. Leave them in and the search invents
// structures out of parts that do not exist.
func Usable(entries map[string]*Entry, titles map[string]string) map[string]*Entry {
	out := make(map[string]*Entry, len(entries))
	for id, e := range entries {
		t := titles[id]
		lower := strings.ToLower(t)
		if strings.HasPrefix(t, "~") || strings.HasPrefix(t, "=") ||
			strings.HasPrefix(lower, "moved") || strings.Contains(lower, "obsolete") {
			continue
		}
		clone := *e
		clone.Title = t
		out[id] = &clone
	}
	return out
}

// Records converts to the engine's format, keeping parts up to maxTier that
// have at least one port. Sorted, so the output is reproducible.
func Records(entries map[string]*Entry, maxTier uint8) []Record {
	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]Record, 0, len(ids))
	for _, id := range ids {
		e := entries[id]
		tier := TierOf(e.Title)
		if tier > maxTier {
			continue
		}
		if len(e.Holes) == 0 && len(e.Pins) == 0 {
			continue
		}
		holes, pins := e.Holes, e.Pins
		if holes == nil {
			holes = []Port{}
		}
		if pins == nil {
			pins = []Port{}
		}
		out = append(out, Record{ID: id, Title: e.Title, Tier: tier,
			Holes: holes, Pins: pins})
	}
	return out
}

// Options controls a build.
type Options struct {
	MaxTier uint8
	Limit   int // 0 for everything
	Log     func(string)
	// Geom is the parts library, used to find the hole primitives a part
	// places. Without it a thirteen-hole beam comes out with one hole, because
	// that is all its own shadow file declares. See EntryForWith.
	Geom part.Subfiles
}

// Build runs the whole extraction against an already-extracted shadow library.
func Build(lib *shadow.Library, opts Options) ([]Record, error) {
	logf := opts.Log
	if logf == nil {
		logf = func(string) {}
	}
	if opts.MaxTier == 0 {
		opts.MaxTier = 3
	}

	names, err := lib.Parts()
	if err != nil {
		return nil, err
	}
	if opts.Limit > 0 && opts.Limit < len(names) {
		names = names[:opts.Limit]
	}

	logf("expanding port grids ...")
	entries := make(map[string]*Entry, len(names))
	declared, walked := 0, 0
	for _, name := range names {
		own := EntryFor(lib, name)
		e := EntryForWith(lib, opts.Geom, name)
		if e == nil || (len(e.Holes) == 0 && len(e.Pins) == 0) {
			continue
		}
		if own != nil {
			declared += len(own.Holes) + len(own.Pins)
		}
		walked += len(e.Holes) + len(e.Pins)
		entries[name] = e
	}
	logf("  " + strconv.Itoa(len(entries)) + " parts with port data")
	// Reported because the difference between these two numbers is the whole
	// point of walking the subfiles, and when the walk quietly found nothing —
	// a part id passed where a filename was wanted — the totals were the only
	// place it showed.
	logf("  " + strconv.Itoa(declared) + " ports declared by the parts themselves, " +
		strconv.Itoa(walked) + " after following the primitives they place")

	titles, err := lib.Titles()
	if err != nil {
		return nil, err
	}
	logf("dropping subparts and obsolete entries ...")
	entries = Usable(entries, titles)
	logf("  " + strconv.Itoa(len(entries)) + " usable parts")

	records := Records(entries, opts.MaxTier)
	ports := 0
	for _, r := range records {
		ports += len(r.Holes) + len(r.Pins)
	}
	logf("  " + strconv.Itoa(len(records)) + " parts at tier <= " +
		strconv.Itoa(int(opts.MaxTier)) + ", " + strconv.Itoa(ports) + " ports")
	return records, nil
}
