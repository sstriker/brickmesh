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
// Parts are fetched one file at a time from a mirror and cached on disk, in the
// same directory and under the same names the Python extractor uses, so the two
// share a cache rather than each keeping their own.
package ldraw

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"brickmesh/internal/geom"
)

const (
	Mirror       = "https://raw.githubusercontent.com/mpetrov/ldraw-parts/master"
	maxDepth     = 24
	fetchTimeout = 30 * time.Second
)

// SearchDirs are tried in order, matching the extractor.
var SearchDirs = []string{"parts", "p", "parts/s", "p/48", "p/8"}

// ErrNotFound reports a part that is in no search directory.
var ErrNotFound = errors.New("part not found")

// Library reads and caches parts.
type Library struct {
	CacheDir string
	Client   *http.Client
	// Offline refuses to fetch, which is what tests want: a miss should
	// surface as a missing fixture, not as a download.
	Offline bool

	mu    sync.Mutex
	geoms map[string]*Geometry
}

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
	return &Library{
		CacheDir: cacheDir,
		Client:   &http.Client{Timeout: fetchTimeout},
		geoms:    make(map[string]*Geometry),
	}
}

// cachePath flattens the search-directory separators the same way the Python
// side does, so both find the same file.
func (l *Library) cachePath(name string) string {
	flat := strings.NewReplacer("/", "__", "\\", "__").Replace(name)
	return filepath.Join(l.CacheDir, flat)
}

// Fetch returns the text of a .dat, from cache when possible.
func (l *Library) Fetch(name string) (string, error) {
	name = strings.ToLower(strings.ReplaceAll(name, "\\", "/"))
	cp := l.cachePath(name)
	if b, err := os.ReadFile(cp); err == nil {
		return string(b), nil
	}
	if l.Offline {
		return "", fmt.Errorf("%w: %s (offline)", ErrNotFound, name)
	}
	if err := os.MkdirAll(l.CacheDir, 0o755); err != nil {
		return "", err
	}

	var last error
	for _, dir := range SearchDirs {
		url := fmt.Sprintf("%s/%s/%s", Mirror, dir, name)
		resp, err := l.Client.Get(url)
		if err != nil {
			last = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			last = fmt.Errorf("%s: %s", url, resp.Status)
			continue
		}
		if err := os.WriteFile(cp, body, 0o644); err != nil {
			return "", err
		}
		return string(body), nil
	}
	return "", fmt.Errorf("%w: %s (%v)", ErrNotFound, name, last)
}

// Geometry is a resolved part: every vertex and triangle in the part's own
// frame.
type Geometry struct {
	Name  string
	Title string
	Verts []geom.Vec3
	Tris  [][3]geom.Vec3
}

// BBox returns the low and high corners.
func (g *Geometry) BBox() (lo, hi geom.Vec3) {
	if len(g.Verts) == 0 {
		return
	}
	lo, hi = g.Verts[0], g.Verts[0]
	for _, v := range g.Verts[1:] {
		lo = geom.Vec3{X: min(lo.X, v.X), Y: min(lo.Y, v.Y), Z: min(lo.Z, v.Z)}
		hi = geom.Vec3{X: max(hi.X, v.X), Y: max(hi.Y, v.Y), Z: max(hi.Z, v.Z)}
	}
	return
}

func (g *Geometry) Size() geom.Vec3 {
	lo, hi := g.BBox()
	return hi.Sub(lo)
}

// ThinAxis is the index of the shortest bbox dimension. For disc-shaped parts
// that is the rotation axis in the part's default orientation. Reported so it
// can be eyeballed, never trusted blindly — the shadow library knows better.
func (g *Geometry) ThinAxis() int {
	s := g.Size()
	idx, best := 0, s.X
	if s.Y < best {
		idx, best = 1, s.Y
	}
	if s.Z < best {
		idx = 2
	}
	return idx
}

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
