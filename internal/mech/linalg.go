// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker

package mech

import "math"

// The linear algebra the functional layer needs, and no more: the rank of the
// constraint matrix, and a least-squares solve.
//
// numpy does this with LAPACK. These matrices are tiny — one row per
// transmission, one column per shaft, with small integer coefficients — so
// plain elimination is accurate enough and keeps the engine free of a linear
// algebra dependency. The tests hold both implementations to the same answers.

// rank counts the independent rows by Gaussian elimination with partial
// pivoting.
//
// The tolerance is relative to the largest entry, which matters because tooth
// counts run to 40 and a fixed epsilon would call a genuinely singular matrix
// full rank.
func rank(rows [][]float64) int {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0
	}
	a := make([][]float64, len(rows))
	largest := 0.0
	for i, r := range rows {
		a[i] = append([]float64(nil), r...)
		for _, v := range r {
			largest = math.Max(largest, math.Abs(v))
		}
	}
	if largest == 0 {
		return 0
	}
	tol := 1e-12 * largest * float64(max(len(a), len(a[0])))

	n := len(a[0])
	rank, row := 0, 0
	for col := 0; col < n && row < len(a); col++ {
		// Pivot on the largest remaining entry in this column.
		best, bestAbs := -1, tol
		for r := row; r < len(a); r++ {
			if abs := math.Abs(a[r][col]); abs > bestAbs {
				best, bestAbs = r, abs
			}
		}
		if best < 0 {
			continue
		}
		a[row], a[best] = a[best], a[row]
		pivot := a[row][col]
		for r := row + 1; r < len(a); r++ {
			f := a[r][col] / pivot
			if f == 0 {
				continue
			}
			for c := col; c < n; c++ {
				a[r][c] -= f * a[row][c]
			}
		}
		row++
		rank++
	}
	return rank
}

// leastSquares solves A x = b in the least-squares sense through the normal
// equations.
//
// A'A is n by n and positive definite whenever A has full column rank, which
// the caller has already established with rank(). Squaring the condition number
// is the usual objection to normal equations; here A holds small integers and n
// is the number of shafts, so it does not bite.
func leastSquares(rows [][]float64, rhs []float64) ([]float64, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	n := len(rows[0])

	// ata = A'A, atb = A'b
	ata := make([][]float64, n)
	for i := range ata {
		ata[i] = make([]float64, n+1)
	}
	for r, row := range rows {
		for i := 0; i < n; i++ {
			if row[i] == 0 {
				continue
			}
			for j := 0; j < n; j++ {
				ata[i][j] += row[i] * row[j]
			}
			ata[i][n] += row[i] * rhs[r]
		}
	}

	// Gauss-Jordan with partial pivoting.
	for col := 0; col < n; col++ {
		best, bestAbs := -1, 0.0
		for r := col; r < n; r++ {
			if abs := math.Abs(ata[r][col]); abs > bestAbs {
				best, bestAbs = r, abs
			}
		}
		if best < 0 || bestAbs < 1e-12 {
			return nil, false
		}
		ata[col], ata[best] = ata[best], ata[col]
		pivot := ata[col][col]
		for c := col; c <= n; c++ {
			ata[col][c] /= pivot
		}
		for r := 0; r < n; r++ {
			if r == col || ata[r][col] == 0 {
				continue
			}
			f := ata[r][col]
			for c := col; c <= n; c++ {
				ata[r][c] -= f * ata[col][c]
			}
		}
	}

	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = ata[i][n]
	}
	return out, true
}
