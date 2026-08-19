// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package extract

import (
	"sync"
	"sync/atomic"

	"brickmesh/internal/geom"
	"brickmesh/internal/part"
	"brickmesh/internal/shadow"
)

// Ports answers where a part's holes and pins are, from the shadow library.
//
// The same question internal/assets answers from the published catalogue, which
// is what a browser has instead. Both go through part.Holes so nothing above
// them knows which it was handed.
//
// It goes through EntryFor rather than reading snaps directly because reading
// them directly is wrong in two ways that are easy to miss: a beam describes
// its holes by including another file rather than with a cylinder of its own,
// and one snap with a grid on it stands for a row of holes. EntryFor already
// knows both, and the catalogue is built from it, so this cannot drift from the
// data it is meant to match.
type Ports struct {
	Lib *shadow.Library
	// Geom is the parts library. Optional, and worth supplying: without it a
	// beam reports the one hole its own shadow file declares rather than the
	// thirteen it has. See EntryForWith.
	Geom part.Subfiles

	// Answers are cached because working one out means walking a part's whole
	// subfile tree, and the structural search asks for the same dozen parts
	// hundreds of thousands of times. Used through a pointer for that reason:
	// a value copy would each keep their own and share nothing.
	cache sync.Map
	// walks counts the answers actually worked out, as opposed to remembered.
	//
	// Here to be asserted on. When this cache was missing, the read count did
	// not move — the parts library caches the files it has already read — and
	// the only sign was that everything took three or four times as long, which
	// is a thing a person notices and a build does not. A run needs one walk per
	// distinct part and no more. See internal/pipeline/perf_test.go.
	walks int64
}

// Walks is how many times a part's subfile tree has been walked. One per
// distinct part is the whole budget.
func (p *Ports) Walks() int64 { return atomic.LoadInt64(&p.walks) }

// NewPorts is the shadow library and the parts library together.
func NewPorts(lib *shadow.Library, geom part.Subfiles) *Ports {
	return &Ports{Lib: lib, Geom: geom}
}

// Holes is every connection point on a part, in the part's own frame.
func (p *Ports) Holes(name string) []part.Hole {
	if got, ok := p.cache.Load(name); ok {
		return got.([]part.Hole)
	}
	atomic.AddInt64(&p.walks, 1)
	out := p.holes(name)
	p.cache.Store(name, out)
	return out
}

func (p *Ports) holes(name string) []part.Hole {
	e := EntryForWith(p.Lib, p.Geom, name)
	if e == nil {
		return nil
	}
	out := make([]part.Hole, 0, len(e.Holes)+len(e.Pins))
	for _, group := range [][]Port{e.Holes, e.Pins} {
		for _, h := range group {
			out = append(out, part.Hole{
				Pos:   geom.Vec3{X: h[0], Y: h[1], Z: h[2]},
				Axis:  geom.Vec3{X: h[3], Y: h[4], Z: h[5]}.Unit(),
				Cross: h[6] != 0,
			})
		}
	}
	return out
}

// RotationAxis is the direction a part's holes face, which is what the
// structural search asks for.
func (p *Ports) RotationAxis(name string) (geom.Vec3, string, bool) {
	return p.Lib.RotationAxis(name)
}
