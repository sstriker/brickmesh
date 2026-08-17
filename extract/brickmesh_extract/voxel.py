# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Voxelization and fast occupancy tests.

FCL is accurate but costs milliseconds per pair, which makes a search touching
hundreds of thousands of placements unusably slow. Because everything sits on a
lattice, it can be done differently: rasterize every part once per orientation
into occupied cells, and test collisions as a bit mask.

Important detail: we rasterize the SURFACE, not the volume. That is not a
shortcoming but exactly what we want - a Technic hole then stays a hole, so an
axle can pass through it without counting as a collision. Filling the volume
would silt up every hole and the search would never find a bearing.

FCL stays in use for the final candidates; this is the coarse filter ahead of
it.
"""
from __future__ import annotations

import itertools
from dataclasses import dataclass

import numpy as np

from . import ldraw

PITCH = 5.0                # LDU per cell. One stud = 4 cells.
STUD = 20.0


def cube_rotations() -> list[np.ndarray]:
    """The 24 rotations of the cube. Only these orientations sit on the lattice."""
    out = []
    for perm in itertools.permutations(range(3)):
        for signs in itertools.product((1, -1), repeat=3):
            M = np.zeros((3, 3))
            for i, p in enumerate(perm):
                M[i, p] = signs[i]
            if abs(np.linalg.det(M) - 1.0) < 1e-9:
                out.append(M)
    return out


ROTATIONS = cube_rotations()


_vox_cache: dict[tuple, np.ndarray] = {}


def voxels(part: str, rot_index: int = 0, pitch: float = PITCH) -> np.ndarray:
    """
    Occupied cells of a part, as integer coordinates relative to its own
    origin. Cached per part and orientation.
    """
    key = (part, rot_index, pitch)
    if key in _vox_cache:
        return _vox_cache[key]

    g = ldraw.geometry(part)
    tris = np.array(g.tris) if g.tris else None
    if tris is None or len(tris) == 0:
        raise ValueError(f"{part}: no triangles")

    R = ROTATIONS[rot_index]
    tris = tris @ R.T

    # sample every triangle at a density that reaches the cell size
    pts = []
    for tri in tris:
        e1, e2 = tri[1] - tri[0], tri[2] - tri[0]
        area = 0.5 * np.linalg.norm(np.cross(e1, e2))
        k = max(2, min(24, int(np.sqrt(area) / pitch * 2) + 2))
        a, b = np.meshgrid(np.linspace(0, 1, k), np.linspace(0, 1, k), indexing="ij")
        m = (a + b) <= 1.0
        u = a[m].reshape(-1, 1); v = b[m].reshape(-1, 1)
        pts.append(tri[0] + u * e1 + v * e2)
    P = np.vstack(pts)

    cells = np.unique(np.floor(P / pitch).astype(np.int32), axis=0)
    _vox_cache[key] = cells
    return cells


@dataclass
class Occupancy:
    """A bounded lattice space in which you place parts and test collisions."""
    lo: np.ndarray            # lower bound in cells
    hi: np.ndarray            # upper bound in cells
    grid: np.ndarray = None

    def __post_init__(self):
        self.lo = np.asarray(self.lo, int)
        self.hi = np.asarray(self.hi, int)
        if self.grid is None:
            self.grid = np.zeros(tuple(self.hi - self.lo + 1), dtype=bool)

    @classmethod
    def around(cls, half_extent_ldu: float = 200.0, pitch: float = PITCH):
        n = int(np.ceil(half_extent_ldu / pitch))
        return cls(np.array([-n, -n, -n]), np.array([n, n, n]))

    def _index(self, cells: np.ndarray, offset_ldu: np.ndarray, pitch: float):
        shift = np.round(np.asarray(offset_ldu, float) / pitch).astype(int)
        idx = cells + shift - self.lo
        keep = np.all((idx >= 0) & (idx <= self.hi - self.lo), axis=1)
        return idx[keep]

    def flat(self, cells: np.ndarray, offset_ldu, pitch: float = PITCH) -> np.ndarray:
        """
        Flat indices of a placement. Compute this once and reuse it: the search
        tests the same placement many times over.
        """
        idx = self._index(cells, offset_ldu, pitch)
        shape = self.grid.shape
        return np.unique(np.ravel_multi_index(idx.T, shape)) if len(idx) else np.empty(0, int)

    def collides_flat(self, flat_idx: np.ndarray) -> bool:
        return bool(self.grid.ravel()[flat_idx].any())

    def add_flat(self, flat_idx: np.ndarray):
        self.grid.ravel()[flat_idx] = True

    def remove_flat(self, flat_idx: np.ndarray):
        self.grid.ravel()[flat_idx] = False

    def collides(self, cells: np.ndarray, offset_ldu, pitch: float = PITCH) -> bool:
        idx = self._index(cells, offset_ldu, pitch)
        if len(idx) == 0:
            return False
        return bool(self.grid[idx[:, 0], idx[:, 1], idx[:, 2]].any())

    def add(self, cells: np.ndarray, offset_ldu, pitch: float = PITCH):
        idx = self._index(cells, offset_ldu, pitch)
        self.grid[idx[:, 0], idx[:, 1], idx[:, 2]] = True

    def remove(self, cells: np.ndarray, offset_ldu, pitch: float = PITCH):
        idx = self._index(cells, offset_ldu, pitch)
        self.grid[idx[:, 0], idx[:, 1], idx[:, 2]] = False

    @property
    def filled(self) -> int:
        return int(self.grid.sum())


def lattice_positions(extent_studs: int, step_ldu: float = STUD) -> np.ndarray:
    """All lattice positions within a cube, in LDU."""
    r = np.arange(-extent_studs, extent_studs + 1) * step_ldu
    return np.array(np.meshgrid(r, r, r, indexing="ij")).reshape(3, -1).T


def axis_aligned_rotations(part: str) -> list[int]:
    """
    The rotation indices that genuinely give different orientations. Many parts
    are symmetric, so far fewer than 24 usually remain - which pays off
    directly in the size of the search space.
    """
    seen, keep = {}, []
    for i in range(len(ROTATIONS)):
        c = voxels(part, i)
        key = hash(c.tobytes())
        if key not in seen:
            seen[key] = i
            keep.append(i)
    return keep
