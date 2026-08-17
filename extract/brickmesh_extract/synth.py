# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
The structural layer: find beams that bear every shaft.

Input is a worked-out layout - shaft lines with direction, gear stations, and
the free stretches per shaft. Output is a set of placed parts such that every
shaft has at least two bearing points, nothing touches anything else, and the
whole is as small as possible.

What makes it tractable is that nothing here is continuous. A beam can sit in
only 24 orientations (fewer once symmetry is taken out) and only at lattice
positions. And the bearing point has to lie on the shaft line, which cuts the
candidates per requirement down to a handful.
"""
from __future__ import annotations

from dataclasses import dataclass

import numpy as np

from . import ldraw, snap
from .layout import Layout, Station, free_intervals
from .voxel import ROTATIONS, axis_aligned_rotations, voxels

HALF_STUD = 10.0
STUD = 20.0

# Inventory of load-bearing parts: (name, number of holes)
BEAMS = [("32523.dat", 3), ("32316.dat", 5), ("32524.dat", 7),
         ("40490.dat", 9), ("32525.dat", 11), ("41239.dat", 13)]


def hole_offsets(n_holes: int) -> np.ndarray:
    """Local hole positions along the length direction (Z), in LDU."""
    k = np.arange(n_holes) - (n_holes - 1) / 2.0
    return np.stack([np.zeros(n_holes), np.zeros(n_holes), k * STUD], axis=1)


CONTACT_FRACTION = 0.12
"""
Two parts joined by a pin lie flat against each other; their surfaces coincide.
On a 5 LDU grid that cannot be told apart from intersecting. Hence no absolute
requirement of zero shared cells, but a threshold: touching shares an edge,
intersecting shares a substantial part of the smaller part.
"""


def _compatible(a: frozenset, b: frozenset) -> bool:
    shared = len(a & b)
    if shared == 0:
        return True
    return shared <= CONTACT_FRACTION * min(len(a), len(b))


@dataclass(frozen=True, order=True)
class Placed:
    part: str
    rot: int
    origin: tuple            # LDU

    def holes(self, n_holes: int) -> np.ndarray:
        R = ROTATIONS[self.rot]
        return hole_offsets(n_holes) @ R.T + np.array(self.origin)

    def hole_axis(self, local_axis: np.ndarray) -> np.ndarray:
        return ROTATIONS[self.rot] @ local_axis


_axis_cache: dict[str, np.ndarray] = {}
_rot_cache: dict[str, list] = {}


def _rots(part: str) -> list:
    if part not in _rot_cache:
        _rot_cache[part] = axis_aligned_rotations(part)
    return _rot_cache[part]


def _local_hole_axis(part: str) -> np.ndarray:
    if part in _axis_cache:
        return _axis_cache[part]
    got = snap.rotation_axis(part)
    if got is None:
        raise ValueError(f"{part}: hole axis unknown")
    v = np.asarray(got[0], float)
    _axis_cache[part] = v / np.linalg.norm(v)
    return _axis_cache[part]


def candidates_for(point: np.ndarray, direction: np.ndarray,
                   inventory=BEAMS) -> list[tuple[Placed, int]]:
    """
    Every way to place a load-bearing part such that one of its holes lands on
    `point` with the hole axis along `direction`.
    """
    out = []
    d = np.asarray(direction, float); d = d / np.linalg.norm(d)
    for part, n in inventory:
        try:
            local = _local_hole_axis(part)
        except ValueError:
            continue
        rots = _rots(part)
        offs = hole_offsets(n)
        for ri in rots:
            R = ROTATIONS[ri]
            if abs(abs(float(np.dot(R @ local, d))) - 1.0) > 1e-6:
                continue
            for k in range(n):
                origin = np.asarray(point, float) - R @ offs[k]
                if np.any(np.abs(np.round(origin / HALF_STUD) * HALF_STUD - origin) > 1e-6):
                    continue                    # not on the lattice
                out.append((Placed(part, ri, tuple(np.round(origin, 3))), n))
    return out


@dataclass
class Requirement:
    shaft: str
    point: np.ndarray
    direction: np.ndarray


def bearing_requirements(layout: Layout, stations: list[Station],
                         per_shaft: int = 2, reach: float = 8.0) -> list[Requirement]:
    """
    Two bearing points per shaft, as far apart as the free stretches allow. Far
    apart, because a short bearing base lets the shaft whip anyway.
    """
    reqs = []
    for sid, pl in layout.place.items():
        free = free_intervals(stations, sid, reach)
        if not free:
            continue
        pts = []
        for lo, hi in free:
            for t in np.arange(np.ceil(lo), np.floor(hi) + 1):
                w = pl.point * HALF_STUD + t * HALF_STUD * pl.direction
                if np.all(np.abs(np.round(w / HALF_STUD) * HALF_STUD - w) < 1e-6):
                    pts.append((t, w))
        if len(pts) < per_shaft:
            continue
        pts.sort(key=lambda x: x[0])
        chosen = [pts[0], pts[-1]] if per_shaft == 2 else pts[:per_shaft]
        for _, w in chosen:
            reqs.append(Requirement(sid, w, pl.direction))
    # differential ports share a line: their bearing requirements coincide
    uniq, out = set(), []
    for r in reqs:
        k = (tuple(np.round(r.point, 3)), tuple(np.round(np.abs(r.direction), 3)))
        if k in uniq:
            continue
        uniq.add(k); out.append(r)
    return out


def synthesize(layout: Layout, stations: list[Station],
               max_parts: int = 10, restarts: int = 60,
               inventory=BEAMS, seed: int = 0) -> list[dict]:
    """
    Greedy set cover with restarts.

    This is not a search through a tree of partial solutions - that explodes,
    because with ten requirements and nearly two hundred candidates each there
    are more combinations than is sensible. It is a covering problem: every
    candidate beam covers a subset of the bearing requirements, and you look
    for the smallest cover whose parts do not touch each other.

    Choosing greedily on "most still-uncovered requirements per stud^3" reaches
    a good solution quickly. A few random restarts usually get you a little
    below that again.
    """
    rng = np.random.default_rng(seed)
    reqs = bearing_requirements(layout, stations)
    if not reqs:
        return []
    nholes = dict(inventory)

    # every candidate, and which requirements each of them covers
    pool: dict[Placed, set] = {}
    for i, r in enumerate(reqs):
        for cand, _ in candidates_for(r.point, r.direction, inventory):
            pool.setdefault(cand, set()).add(i)

    def satisfies(p: Placed, r: Requirement) -> bool:
        n = nholes[p.part]
        if abs(abs(float(np.dot(p.hole_axis(_local_hole_axis(p.part)),
                                r.direction))) - 1.0) > 1e-6:
            return False
        return bool((np.linalg.norm(p.holes(n) - r.point, axis=1) < 1e-6).any())

    # a beam can cover more requirements than the one it was generated for
    for cand in pool:
        for i, r in enumerate(reqs):
            if satisfies(cand, r):
                pool[cand].add(i)

    cell_cache: dict[Placed, frozenset] = {}

    def cells_of(p: Placed) -> frozenset:
        if p not in cell_cache:
            c = voxels(p.part, p.rot)
            shift = np.round(np.asarray(p.origin, float) / 5.0).astype(int)
            cell_cache[p] = frozenset(map(tuple, (c + shift).tolist()))
        return cell_cache[p]

    def volume(p: Placed) -> float:
        g = ldraw.geometry(p.part)
        return float(np.prod(g.size) / 8000.0)

    def bbox_of(parts) -> float:
        if not parts:
            return 0.0
        pts = []
        for p in parts:
            g = ldraw.geometry(p.part)
            v = g.verts @ ROTATIONS[p.rot].T + np.array(p.origin)
            pts.append(v.min(axis=0)); pts.append(v.max(axis=0))
        pts = np.array(pts)
        return float(np.prod(pts.max(axis=0) - pts.min(axis=0)) / 8000.0)

    # holes of every candidate, so connectivity can be tested
    from .rigidity import world_holes
    hole_cache: dict[Placed, frozenset] = {}

    def holes_of(p: Placed) -> frozenset:
        if p not in hole_cache:
            pts, _ = world_holes(p, nholes[p.part])
            hole_cache[p] = frozenset(tuple(np.round(q, 3)) for q in pts)
        return hole_cache[p]

    items = list(pool.items())
    best = []

    for attempt in range(restarts):
        uncovered = set(range(len(reqs)))
        chosen, occupied, placed_holes = [], set(), set()
        jitter = attempt > 0
        while uncovered and len(chosen) < max_parts:
            scored = []
            for cand, covers in items:
                gain = len(covers & uncovered)
                if gain == 0:
                    continue
                cells = cells_of(cand)
                if not _compatible(cells, frozenset(occupied)):
                    continue
                # connectivity: after the first part, every addition has to
                # share at least one hole with what is already there, or it floats
                shared = len(holes_of(cand) & placed_holes) if chosen else 1
                score = (gain + 0.5 * min(shared, 2)) / (volume(cand) ** 0.5)
                if jitter:
                    score *= rng.uniform(0.7, 1.3)
                scored.append((score, cand, covers, cells))
            if not scored:
                break
            scored.sort(key=lambda x: -x[0])
            _, cand, covers, cells = scored[0]
            chosen.append(cand)
            occupied |= cells
            placed_holes |= holes_of(cand)
            uncovered -= covers
        if uncovered:
            continue
        chosen = _repair_connectivity(chosen, cells_of, holes_of, nholes, inventory)
        best.append({"parts": sorted(chosen, key=lambda p: (p.part, p.origin)),
                     "count": len(chosen), "bbox_stud3": bbox_of(chosen)})

    uniq = {}
    for r in best:
        uniq[tuple(r["parts"])] = r
    out = sorted(uniq.values(), key=lambda r: (r["count"], r["bbox_stud3"]))
    return out


def _repair_connectivity(chosen, cells_of, holes_of, nholes, inventory):
    """
    Repair phase: the cover is complete, but the pieces hang loose. Add
    connecting beams until everything is one whole, each time taking the pair
    of pieces that is cheapest to bridge.
    """
    from .rigidity import components, find_joints

    for _ in range(12):
        comps = components(len(chosen), find_joints(chosen, inventory))
        if len(comps) <= 1:
            break
        comps.sort(key=len, reverse=True)
        base, other = comps[0], comps[1]
        ha = set().union(*(holes_of(chosen[i]) for i in base))
        hb = set().union(*(holes_of(chosen[i]) for i in other))
        occupied = set().union(*(cells_of(chosen[i]) for i in range(len(chosen))))
        options = connectors_between(ha, hb, inventory)
        placed = None
        for cand in sorted(options, key=lambda p: nholes[p.part]):
            if _compatible(cells_of(cand), frozenset(occupied)):
                placed = cand
                break
        if placed is None:
            break
        chosen = chosen + [placed]
    return chosen


def connectors_between(holes_a: set, holes_b: set, inventory=BEAMS) -> list[Placed]:
    """
    Beams tying two separate pieces together.

    These bear no shaft at all; they exist purely to create connectivity.
    Generated deliberately rather than blindly: take a hole from one piece and
    one from the other, and if they lie on a straight line with a multiple of
    20 LDU between them, a beam of the right length spans both.
    """
    out = []
    for ha in holes_a:
        for hb in holes_b:
            v = np.array(hb) - np.array(ha)
            L = np.linalg.norm(v)
            if L < 1e-6:
                continue
            k = L / STUD
            if abs(k - round(k)) > 1e-6:
                continue                      # not on the hole pitch
            k = int(round(k))
            d = v / L
            if np.count_nonzero(np.abs(d) > 1e-6) != 1:
                continue                      # straight directions only
            for part, n in inventory:
                if n < k + 1:
                    continue
                try:
                    local = _local_hole_axis(part)
                except ValueError:
                    continue
                offs = hole_offsets(n)
                for ri in _rots(part):
                    R = ROTATIONS[ri]
                    # the beam length direction has to lie along d
                    if abs(abs(float(np.dot(R @ np.array([0, 0, 1.0]), d))) - 1.0) > 1e-6:
                        continue
                    if abs(float(np.dot(R @ local, d))) > 1e-6:
                        continue              # the hole axis has to run across
                    for idx in range(n):
                        origin = np.array(ha) - R @ offs[idx]
                        if np.any(np.abs(np.round(origin / HALF_STUD) * HALF_STUD
                                         - origin) > 1e-6):
                            continue
                        p = Placed(part, ri, tuple(np.round(origin, 3)))
                        hs = {tuple(np.round(q, 3))
                              for q in (hole_offsets(n) @ R.T + origin)}
                        if ha in hs and hb in hs:
                            out.append(p)
    seen, uniq = set(), []
    for p in out:
        if p not in seen:
            seen.add(p); uniq.append(p)
    return uniq
