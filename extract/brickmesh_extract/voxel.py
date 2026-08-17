# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Voxelaliseren en snelle bezettingstoetsen.

FCL is nauwkeurig maar kost milliseconden per paar. Een zoektocht die
honderdduizenden plaatsingen aanraakt is daarmee onbruikbaar traag. Omdat
alles op een rooster ligt, kan het anders: rasteriseer elk onderdeel een keer
per orientatie naar bezette cellen, en toets botsingen als een bitmasker.

Belangrijk detail: we rasteriseren het OPPERVLAK, niet het volume. Dat is geen
tekortkoming maar precies wat we willen - een Technic-gat blijft dan een gat,
zodat een as er doorheen kan zonder als botsing te tellen. Bij volumevulling
zou elk gat dichtslibben en zou de zoeker nooit een lager vinden.

FCL blijft in gebruik voor de eindkandidaten; dit is het grove filter ervoor.
"""
from __future__ import annotations

import itertools
from dataclasses import dataclass

import numpy as np

from . import ldraw

PITCH = 5.0                # LDU per cel. Een stud = 4 cellen.
STUD = 20.0


def cube_rotations() -> list[np.ndarray]:
    """De 24 rotaties van de kubus. Alleen deze standen liggen op het rooster."""
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
    Bezette cellen van een onderdeel, als integer-coordinaten ten opzichte van
    de eigen oorsprong. Gecachet per onderdeel en orientatie.
    """
    key = (part, rot_index, pitch)
    if key in _vox_cache:
        return _vox_cache[key]

    g = ldraw.geometry(part)
    tris = np.array(g.tris) if g.tris else None
    if tris is None or len(tris) == 0:
        raise ValueError(f"{part}: geen driehoeken")

    R = ROTATIONS[rot_index]
    tris = tris @ R.T

    # elke driehoek bemonsteren met een dichtheid die de celgrootte haalt
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
    """Een begrensde rasterruimte waarin je onderdelen legt en botsingen toetst."""
    lo: np.ndarray            # ondergrens in cellen
    hi: np.ndarray            # bovengrens in cellen
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
        Vlakke indices van een plaatsing. Reken dit een keer uit en hergebruik
        het: de zoeker toetst dezelfde plaatsing vele malen.
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
    """Alle roosterposities binnen een kubus, in LDU."""
    r = np.arange(-extent_studs, extent_studs + 1) * step_ldu
    return np.array(np.meshgrid(r, r, r, indexing="ij")).reshape(3, -1).T


def axis_aligned_rotations(part: str) -> list[int]:
    """
    De rotatie-indices die werkelijk verschillende standen geven. Veel
    onderdelen zijn symmetrisch, dus van de 24 blijven er vaak veel minder
    over - dat scheelt direct in de zoekruimte.
    """
    seen, keep = {}, []
    for i in range(len(ROTATIONS)):
        c = voxels(part, i)
        key = hash(c.tobytes())
        if key not in seen:
            seen[key] = i
            keep.append(i)
    return keep
