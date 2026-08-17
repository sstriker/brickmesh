// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

// Package geom holds the geometric groundwork: LDU units, vectors, the 24
// lattice rotations, and cells for the occupancy grid.
//
// Units are LDU throughout. A stud is 20 LDU horizontally, a brick 24 LDU
// tall, a plate 8. That 20-versus-24 is not a detail: it is why Technic bricks
// and liftarms live on incompatible lattices. See docs/findings.md.
package geom

import "math"

const (
	Stud     = 20.0
	HalfStud = 10.0
	Brick    = 24.0
	Plate    = 8.0
	// VoxelPitch is coarse enough to be fast and fine enough to keep a
	// Technic hole open. See docs/findings.md on why an axle through a hole
	// does not fit this model.
	VoxelPitch = 5.0
)

type Vec3 struct{ X, Y, Z float64 }

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Scale(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Dot(b Vec3) float64   { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

func (a Vec3) Len() float64 { return math.Sqrt(a.Dot(a)) }

func (a Vec3) Unit() Vec3 {
	l := a.Len()
	if l == 0 {
		return a
	}
	return a.Scale(1 / l)
}

func (a Vec3) Round(dp int) Vec3 {
	f := math.Pow(10, float64(dp))
	return Vec3{math.Round(a.X*f) / f, math.Round(a.Y*f) / f, math.Round(a.Z*f) / f}
}

// OnLattice reports whether every coordinate is a multiple of step.
func (a Vec3) OnLattice(step float64) bool {
	for _, v := range [3]float64{a.X, a.Y, a.Z} {
		if math.Abs(v/step-math.Round(v/step)) > 1e-4 {
			return false
		}
	}
	return true
}

// Parallel is true when two directions describe the same line, sign ignored.
// Holes have no direction.
func (a Vec3) Parallel(b Vec3) bool {
	return math.Abs(math.Abs(a.Unit().Dot(b.Unit()))-1) < 1e-9
}

// Mat3 is a rotation matrix, row-major.
type Mat3 [3][3]float64

func (m Mat3) Apply(v Vec3) Vec3 {
	return Vec3{
		m[0][0]*v.X + m[0][1]*v.Y + m[0][2]*v.Z,
		m[1][0]*v.X + m[1][1]*v.Y + m[1][2]*v.Z,
		m[2][0]*v.X + m[2][1]*v.Y + m[2][2]*v.Z,
	}
}

func (m Mat3) Det() float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// Rotations are the 24 rotations of the cube: the only orientations in which
// a part fits the lattice. Many parts are symmetric, so fewer remain in
// practice — 12 for a liftarm, 6 for an axle, 3 for a 24-tooth gear.
var Rotations = buildRotations()

func buildRotations() []Mat3 {
	perms := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	signs := [][3]float64{
		{1, 1, 1}, {1, 1, -1}, {1, -1, 1}, {1, -1, -1},
		{-1, 1, 1}, {-1, 1, -1}, {-1, -1, 1}, {-1, -1, -1},
	}
	var out []Mat3
	for _, p := range perms {
		for _, s := range signs {
			var m Mat3
			for i := 0; i < 3; i++ {
				m[i][p[i]] = s[i]
			}
			if math.Abs(m.Det()-1) < 1e-9 {
				out = append(out, m)
			}
		}
	}
	return out
}

// Cell is one cell of the occupancy grid.
type Cell struct{ X, Y, Z int32 }

func ToCell(v Vec3) Cell {
	return Cell{
		int32(math.Floor(v.X / VoxelPitch)),
		int32(math.Floor(v.Y / VoxelPitch)),
		int32(math.Floor(v.Z / VoxelPitch)),
	}
}

func (c Cell) Add(o Cell) Cell { return Cell{c.X + o.X, c.Y + o.Y, c.Z + o.Z} }

// PitchDistance is the centre distance at which two parallel gears mesh. All
// standard tooth counts are multiples of 4, so this always lands on a whole
// half-stud.
func PitchDistance(t1, t2 int) float64 { return float64(t1+t2) / 16.0 * Stud }

// EffectiveRadius is the radius at which a gear engages.
func EffectiveRadius(teeth int) float64 { return float64(teeth) / 16.0 * Stud }
