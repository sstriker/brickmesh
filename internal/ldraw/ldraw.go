// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package ldraw reads parts from the LDraw parts library.
//
// Coordinate system reminder:
//
//	1 stud  = 20 LDU (X and Z)
//	1 brick = 24 LDU (Y)
//	1 plate =  8 LDU (Y)
//	+Y points DOWN.
//
// The library is fetched whole and extracted once — see library.go for why a
// mirror of part of it was not good enough. Individual parts are also cached
// flat, in the same directory and under the same names the Python extractor
// uses, so the two share a cache rather than each keeping their own.
package ldraw

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/part"
)

const (
	maxDepth     = 24
	fetchTimeout = 30 * time.Second
)

// SearchDirs are tried in order, under the library's root.
//
// The empty one is the root itself, which is how a flat directory of parts
// works — the test fixtures are laid out that way, and it costs one stat.
var SearchDirs = []string{"parts", "p", "parts/s", "p/48", "p/8", ""}

// ErrNotFound reports a part that is in no search directory.
var ErrNotFound = errors.New("part not found")

// Library reads and caches parts.
type Library struct {
	CacheDir string
	Client   *http.Client
	// Offline refuses to fetch, which is what tests want: a miss should
	// surface as a missing fixture, not as a download.
	Offline bool
	// Root is the extracted parts library. Set when one is already in the
	// cache; otherwise the first miss goes and gets it.
	Root string

	// reads counts how many times a part file has been gone to disk for.
	//
	// Here to be asserted on. The engine reads the same few dozen parts over
	// and over, so everything that reads one caches it — and when one of those
	// caches was missing, the only sign was that the tests took twenty times as
	// long, which is a thing a person notices and a build does not. Counting is
	// the version of that a build can check. See internal/pipeline/perf_test.go.
	reads int64

	mu       sync.Mutex
	geoms    map[string]*Geometry
	refs     map[string][]part.Ref
	fetch    sync.Once
	fetchErr error
}

// Reads is how many part files have been read from disk.
func (l *Library) Reads() int64 { return atomic.LoadInt64(&l.reads) }

// DefaultCacheDir matches the Python extractor's ldraw.CACHE.
func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cache/brickmesh-ldraw"
	}
	return filepath.Join(home, ".cache", "brickmesh-ldraw")
}

func New(cacheDir string) *Library {
	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}
	l := &Library{
		CacheDir: cacheDir,
		Client:   &http.Client{Timeout: fetchTimeout},
		geoms:    make(map[string]*Geometry),
	}
	// Use a library that is already there without going to look for one.
	root := filepath.Join(cacheDir, LibraryRoot)
	if fi, err := os.Stat(filepath.Join(root, "parts")); err == nil && fi.IsDir() {
		l.Root = root
	} else if fi, err := os.Stat(cacheDir); err == nil && fi.IsDir() {
		// Or the directory itself, if it is one: that is how the fixtures are
		// laid out, and how anyone pointing this at an existing LDraw
		// installation would expect it to behave.
		l.Root = cacheDir
	}
	return l
}

// Fetch returns the text of a .dat from the extracted library.
//
// One source, deliberately. There used to be a flat per-part cache consulted
// first, left over from sharing a directory with the Python extractor, and it
// outlived its purpose badly: it still held 925 parts fetched from the old
// mirror, and those files are not the same as the official library's. Reading
// them in preference meant measuring old geometry on a machine that had them
// and current geometry on one that did not — which is exactly how the same
// tests passed here and failed in CI.
func (l *Library) Fetch(name string) (string, error) {
	name = strings.ToLower(strings.ReplaceAll(name, "\\", "/"))
	if text, ok := l.findInRoot(name); ok {
		return text, nil
	}
	if l.Offline {
		return "", fmt.Errorf("%w: %s (offline)", ErrNotFound, name)
	}

	// Nothing cached and no library on disk, so go and get one. Once, however
	// many parts are asked for concurrently: it is a single 144 MB archive, and
	// two goroutines racing to download it would be a bad way to spend a
	// morning.
	l.fetch.Do(func() {
		root, err := Ensure(l.CacheDir)
		if err != nil {
			l.fetchErr = err
			return
		}
		l.mu.Lock()
		l.Root = root
		l.mu.Unlock()
	})
	if l.fetchErr != nil {
		return "", fmt.Errorf("%w: %s (%v)", ErrNotFound, name, l.fetchErr)
	}
	if text, ok := l.findInRoot(name); ok {
		return text, nil
	}
	return "", fmt.Errorf("%w: %s (in no search directory of %s)",
		ErrNotFound, name, l.Root)
}

// Geometry is a resolved part: every vertex and triangle in the part's own
// frame.
// Geometry is a part's triangles. The type lives in internal/part, because the
// engine reads the same triangles out of a mesh blob in a browser where no .dat
// file and no parser exist.
type Geometry = part.Shape

// Geometry resolves a part by name, caching the result.
func (l *Library) Geometry(name string) (*Geometry, error) {
	name = strings.ToLower(name)
	if !strings.HasSuffix(name, ".dat") {
		name += ".dat"
	}

	l.mu.Lock()
	if g, ok := l.geoms[name]; ok {
		l.mu.Unlock()
		return g, nil
	}
	l.mu.Unlock()

	text, err := l.Fetch(name)
	if err != nil {
		return nil, err
	}
	verts, tris, err := l.resolve(name, 0)
	if err != nil {
		return nil, err
	}
	if len(verts) == 0 {
		return nil, fmt.Errorf("%w: %s resolved to no geometry", ErrNotFound, name)
	}

	g := &Geometry{Name: name, Title: title(text), Verts: verts, Tris: tris}
	l.mu.Lock()
	l.geoms[name] = g
	l.mu.Unlock()
	return g, nil
}

// title is the first plain comment line: not a meta command, not the Name:
// header.
func title(text string) string {
	for _, line := range strings.Split(text, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "0 ") && !strings.HasPrefix(s, "0 !") &&
			!strings.HasPrefix(s, "0 Name:") {
			return strings.TrimSpace(s[2:])
		}
	}
	return ""
}

// resolve expands a part into vertices and triangles in its own frame.
//
// Repeated references to the same subfile must NOT be deduplicated. A gear is
// one tooth primitive instantiated N times at N matrices; dropping the repeats
// collapses the part into a sliver.
func (l *Library) resolve(name string, depth int) ([]geom.Vec3, [][3]geom.Vec3, error) {
	if depth > maxDepth {
		return nil, nil, nil
	}
	text, err := l.Fetch(name)
	if err != nil {
		return nil, nil, err
	}

	var verts []geom.Vec3
	var tris [][3]geom.Vec3

	for _, raw := range strings.Split(text, "\n") {
		tok := strings.Fields(raw)
		if len(tok) == 0 {
			continue
		}
		kind, err := strconv.Atoi(tok[0])
		if err != nil {
			continue
		}

		switch {
		case kind == 1 && len(tok) >= 15:
			sub := strings.ToLower(strings.Join(tok[14:], " "))
			rot, trans, ok := lineMatrix(tok)
			if !ok {
				continue
			}
			childVerts, childTris, err := l.resolve(sub, depth+1)
			if err != nil {
				// A missing subpart is skipped, as in the extractor: plenty of
				// parts reference primitives the mirror does not carry.
				continue
			}
			for _, v := range childVerts {
				verts = append(verts, rot.Apply(v).Add(trans))
			}
			for _, t := range childTris {
				tris = append(tris, [3]geom.Vec3{
					rot.Apply(t[0]).Add(trans),
					rot.Apply(t[1]).Add(trans),
					rot.Apply(t[2]).Add(trans),
				})
			}

		case kind == 2 || kind == 3 || kind == 4:
			n := map[int]int{2: 2, 3: 3, 4: 4}[kind]
			need := 2 + n*3
			if len(tok) < need {
				continue
			}
			pts := make([]geom.Vec3, 0, n)
			bad := false
			for i := 0; i < n; i++ {
				v, ok := vec(tok[2+i*3 : 5+i*3])
				if !ok {
					bad = true
					break
				}
				pts = append(pts, v)
			}
			if bad {
				continue
			}
			verts = append(verts, pts...)
			switch kind {
			case 3:
				tris = append(tris, [3]geom.Vec3{pts[0], pts[1], pts[2]})
			case 4:
				tris = append(tris,
					[3]geom.Vec3{pts[0], pts[1], pts[2]},
					[3]geom.Vec3{pts[0], pts[2], pts[3]})
			}
		}
	}
	return verts, tris, nil
}

// lineMatrix reads the translation and rotation of a type-1 line.
func lineMatrix(tok []string) (geom.Mat3, geom.Vec3, bool) {
	var m geom.Mat3
	nums := make([]float64, 0, 12)
	for _, s := range tok[2:14] {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return m, geom.Vec3{}, false
		}
		nums = append(nums, f)
	}
	trans := geom.Vec3{X: nums[0], Y: nums[1], Z: nums[2]}
	for r := 0; r < 3; r++ {
		for c := 0; c < 3; c++ {
			m[r][c] = nums[3+r*3+c]
		}
	}
	return m, trans, true
}

func vec(tok []string) (geom.Vec3, bool) {
	var out [3]float64
	for i, s := range tok {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return geom.Vec3{}, false
		}
		out[i] = f
	}
	return geom.Vec3{X: out[0], Y: out[1], Z: out[2]}, true
}

// Ref is one subfile reference: a part or primitive placed inside another.
type Ref = part.Ref

// Refs are the subfiles a part places directly, in file order.
//
// Geometry flattens this tree into triangles and throws the structure away,
// which is right for drawing and wrong for anything that asks what a part is
// made of. The shadow library describes a beam's holes by describing one hole
// primitive and letting the part place thirteen of them, so finding a beam's
// holes means walking this rather than reading the beam's own shadow file —
// which declares one hole, at the end, and says nothing about the other twelve.
// Title is a part's first line, which is what it calls itself.
//
// Worth having from the library rather than a table: a gear's own title says
// how many teeth it has, and reading it there means a model full of parts this
// engine never places can still be understood.
func (l *Library) Title(name string) (string, error) {
	body, err := l.Fetch(name)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "0 ") {
			return strings.TrimSpace(line[2:]), nil
		}
		break // the first line was not a comment: no title
	}
	return "", nil
}

func (l *Library) Refs(name string) ([]part.Ref, error) {
	// Cached like geometry is. Fetch reads from disk every time, and the port
	// extractor walks a part's whole subfile tree — twenty-five files for a
	// beam — which is fine once and ruinous in the inner loop of a search.
	l.mu.Lock()
	if r, ok := l.refs[name]; ok {
		l.mu.Unlock()
		return r, nil
	}
	l.mu.Unlock()

	text, err := l.Fetch(name)
	if err != nil {
		return nil, err
	}
	var out []Ref
	for _, raw := range strings.Split(text, "\n") {
		tok := strings.Fields(raw)
		if len(tok) < 15 || tok[0] != "1" {
			continue
		}
		rot, trans, ok := lineMatrix(tok)
		if !ok {
			continue
		}
		out = append(out, part.Ref{
			Name: strings.ToLower(strings.Join(tok[14:], " ")),
			Rot:  rot, Pos: trans,
		})
	}
	l.mu.Lock()
	if l.refs == nil {
		l.refs = map[string][]part.Ref{}
	}
	l.refs[name] = out
	l.mu.Unlock()
	return out, nil
}
