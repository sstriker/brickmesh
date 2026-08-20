// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package voxel rasterizes parts onto the lattice and tests occupancy.
//
// An exact collision test costs milliseconds per pair, which makes a search
// touching hundreds of thousands of placements unusably slow. Because
// everything sits on a lattice it can be done differently: rasterize every part
// once per orientation into occupied cells, and test collisions as bit
// operations.
//
// Important detail: we rasterize the SURFACE, not the volume. That is not a
// shortcoming but exactly what we want — a Technic hole stays a hole, so an
// axle can pass through it without counting as a collision. Filling the volume
// would silt up every hole and the search would never find a bearing.
//
// An exact test still has its place for the final candidates; this is the
// coarse filter ahead of it.
package voxel

import (
	"fmt"
	"math"
	"math/bits"
	"sync"

	"github.com/sstriker/brickmesh/internal/geom"
	"github.com/sstriker/brickmesh/internal/part"
)

const (
	// Pitch in LDU per cell. One stud is four cells.
	Pitch = geom.VoxelPitch
	Stud  = geom.Stud
)

// Cells is a part's occupied cells relative to its own origin.
type Cells []geom.Cell

// Rasterizer turns parts into cells, caching per part and orientation.
type Rasterizer struct {
	Lib part.Shapes

	mu    sync.Mutex
	cache map[cacheKey]Cells
}

type cacheKey struct {
	part string
	rot  int
}

func NewRasterizer(lib part.Shapes) *Rasterizer {
	return &Rasterizer{Lib: lib, cache: map[cacheKey]Cells{}}
}

// Voxels returns the occupied cells of a part in one of the 24 lattice
// orientations.
func (r *Rasterizer) Voxels(part string, rot int) (Cells, error) {
	if rot < 0 || rot >= len(geom.Rotations) {
		return nil, fmt.Errorf("rotation %d is not one of the 24", rot)
	}
	key := cacheKey{part, rot}
	r.mu.Lock()
	if c, ok := r.cache[key]; ok {
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()

	g, err := r.Lib.Geometry(part)
	if err != nil {
		return nil, err
	}
	if len(g.Tris) == 0 {
		return nil, fmt.Errorf("%s: no triangles", part)
	}

	m := geom.Rotations[rot]
	seen := map[geom.Cell]bool{}
	var cells Cells
	for _, tri := range g.Tris {
		a, b, c := m.Apply(tri[0]), m.Apply(tri[1]), m.Apply(tri[2])
		for _, p := range sampleTriangle(a, b, c) {
			cell := geom.ToCell(p)
			if !seen[cell] {
				seen[cell] = true
				cells = append(cells, cell)
			}
		}
	}

	r.mu.Lock()
	r.cache[key] = cells
	r.mu.Unlock()
	return cells, nil
}

// sampleTriangle covers a triangle densely enough that no cell it crosses is
// missed, and no denser.
func sampleTriangle(v0, v1, v2 geom.Vec3) []geom.Vec3 {
	e1, e2 := v1.Sub(v0), v2.Sub(v0)
	area := 0.5 * e1.Cross(e2).Len()

	k := int(math.Sqrt(area)/Pitch*2) + 2
	if k < 2 {
		k = 2
	}
	if k > 24 {
		k = 24
	}

	out := make([]geom.Vec3, 0, k*k/2)
	step := 1.0 / float64(k-1)
	for i := 0; i < k; i++ {
		a := float64(i) * step
		for j := 0; j < k; j++ {
			b := float64(j) * step
			if a+b > 1.0 {
				continue // outside the triangle
			}
			out = append(out, v0.Add(e1.Scale(a)).Add(e2.Scale(b)))
		}
	}
	return out
}

// DistinctRotations returns the rotation indices that genuinely give different
// cells. Many parts are symmetric, so far fewer than 24 usually remain, and
// that pays off directly in the size of the search space.
//
// The reduction is measured on the rasterized cells rather than on exact
// symmetry, so a cube does not collapse all the way to one: sampling on a cell
// boundary differs slightly between orientations. It is a search-space
// optimization, not a claim about the part.
func (r *Rasterizer) DistinctRotations(part string) ([]int, error) {
	seen := map[string]bool{}
	var keep []int
	for i := range geom.Rotations {
		cells, err := r.Voxels(part, i)
		if err != nil {
			return nil, err
		}
		if sig := signature(cells); !seen[sig] {
			seen[sig] = true
			keep = append(keep, i)
		}
	}
	return keep, nil
}

// signature is order-independent, since cell order depends on triangle order.
func signature(cells Cells) string {
	var hi, lo uint64 = 1469598103934665603, 1099511628211
	for _, c := range cells {
		// A commutative mix: the set matters, the order does not.
		h := uint64(uint32(c.X))*0x9E3779B97F4A7C15 ^
			uint64(uint32(c.Y))*0xC2B2AE3D27D4EB4F ^
			uint64(uint32(c.Z))*0x165667B19E3779F9
		hi ^= h
		lo += h | 1
	}
	return fmt.Sprintf("%d:%x:%x", len(cells), hi, lo)
}

// Grid is a bounded lattice of occupied cells, held as a bitset.
//
// The search tests the same placement many times over, so the expensive part is
// not the test but the bookkeeping: a bitset makes occupancy one shift and one
// mask per cell, and clearing a placement is the same operation again.
type Grid struct {
	lo, hi geom.Cell
	dims   [3]int
	words  []uint64
}

// NewGrid covers a cube reaching half_extent LDU from the origin.
func NewGrid(halfExtentLDU float64) *Grid {
	n := int32(math.Ceil(halfExtentLDU / Pitch))
	return NewGridBounds(geom.Cell{X: -n, Y: -n, Z: -n}, geom.Cell{X: n, Y: n, Z: n})
}

func NewGridBounds(lo, hi geom.Cell) *Grid {
	dims := [3]int{int(hi.X-lo.X) + 1, int(hi.Y-lo.Y) + 1, int(hi.Z-lo.Z) + 1}
	total := dims[0] * dims[1] * dims[2]
	return &Grid{lo: lo, hi: hi, dims: dims, words: make([]uint64, (total+63)/64)}
}

// Index converts a part's cells to flat indices at a placement offset, dropping
// anything outside the grid.
//
// Compute this once per placement and reuse it: the search asks about the same
// placement again and again.
func (g *Grid) Index(cells Cells, offsetLDU geom.Vec3, dst []uint32) []uint32 {
	dst = dst[:0]
	shift := geom.Cell{
		X: int32(math.Round(offsetLDU.X / Pitch)),
		Y: int32(math.Round(offsetLDU.Y / Pitch)),
		Z: int32(math.Round(offsetLDU.Z / Pitch)),
	}
	for _, c := range cells {
		x, y, z := c.X+shift.X-g.lo.X, c.Y+shift.Y-g.lo.Y, c.Z+shift.Z-g.lo.Z
		if x < 0 || y < 0 || z < 0 ||
			int(x) >= g.dims[0] || int(y) >= g.dims[1] || int(z) >= g.dims[2] {
			continue
		}
		dst = append(dst, uint32((int(x)*g.dims[1]+int(y))*g.dims[2]+int(z)))
	}
	return dst
}

func (g *Grid) Collides(flat []uint32) bool {
	for _, i := range flat {
		if g.words[i>>6]&(1<<(i&63)) != 0 {
			return true
		}
	}
	return false
}

func (g *Grid) Add(flat []uint32) {
	for _, i := range flat {
		g.words[i>>6] |= 1 << (i & 63)
	}
}

func (g *Grid) Remove(flat []uint32) {
	for _, i := range flat {
		g.words[i>>6] &^= 1 << (i & 63)
	}
}

// Filled counts occupied cells.
func (g *Grid) Filled() int {
	n := 0
	for _, w := range g.words {
		n += bits.OnesCount64(w)
	}
	return n
}

// Clear empties the grid without reallocating.
func (g *Grid) Clear() {
	for i := range g.words {
		g.words[i] = 0
	}
}

// LatticePositions are all stud-grid positions within a cube, in LDU.
func LatticePositions(extentStuds int) []geom.Vec3 {
	var out []geom.Vec3
	for x := -extentStuds; x <= extentStuds; x++ {
		for y := -extentStuds; y <= extentStuds; y++ {
			for z := -extentStuds; z <= extentStuds; z++ {
				out = append(out, geom.Vec3{
					X: float64(x) * Stud, Y: float64(y) * Stud, Z: float64(z) * Stud})
			}
		}
	}
	return out
}
