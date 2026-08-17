# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Search based on ports rather than on beams.

The previous version assumed that a load-bearing part has holes in a row along
its length, all facing the same way. That holds for liftarms and for nothing
else in the catalog. An angle connector has two holes perpendicular to each
other, an axle coupler has male ends, and a plate with pin holes has yet
another layout again.

This version reads the ports from the catalog: position, direction, and whether
it is round or cross-shaped. That last one is no detail:

    a bearing has to be a ROUND hole.

An axle in a cross-shaped hole cannot turn. A search that does not make that
distinction produces structures that seize up.
"""
from __future__ import annotations

from dataclasses import dataclass

import numpy as np

from .voxel import ROTATIONS

STUD = 20.0
HALF_STUD = 10.0
TOL = 1e-6

ROUND, CROSS = 0, 1


@dataclass(frozen=True, order=True)
class Placed:
    part: str
    rot: int
    origin: tuple

    def ports(self, cat) -> tuple[np.ndarray, np.ndarray]:
        return world_ports(self, cat)


_pcache: dict[str, tuple] = {}


def part_ports(pid: str, cat: dict):
    """Holes and pins of a part as arrays: [x,y,z,ax,ay,az,cross]."""
    if pid not in _pcache:
        e = cat[pid]
        h = np.array(e["holes"], float).reshape(-1, 7) if e["holes"] else np.zeros((0, 7))
        p = np.array(e["pins"], float).reshape(-1, 7) if e["pins"] else np.zeros((0, 7))
        _pcache[pid] = (h, p)
    return _pcache[pid]


def world_ports(pl: Placed, cat: dict):
    h, p = part_ports(pl.part, cat)
    R = ROTATIONS[pl.rot]
    o = np.array(pl.origin, float)
    def xf(a):
        if not len(a):
            return a
        out = a.copy()
        out[:, :3] = a[:, :3] @ R.T + o
        out[:, 3:6] = a[:, 3:6] @ R.T
        return out
    return xf(h), xf(p)


def rotations_mapping(axis_local: np.ndarray, axis_world: np.ndarray) -> list[int]:
    """Only the rotations that put the hole axis along the requested direction."""
    out = []
    a = axis_local / np.linalg.norm(axis_local)
    b = axis_world / np.linalg.norm(axis_world)
    for i, R in enumerate(ROTATIONS):
        if abs(abs(float(np.dot(R @ a, b))) - 1.0) < 1e-9:
            out.append(i)
    return out


def placements_for(point, axis, cat: dict, bearing: bool = True,
                   max_per_part: int = 40) -> list[Placed]:
    """
    Every way to place a part such that one of its holes lands on `point` with
    the axis along `axis`.

    With bearing=True only ROUND holes count: an axle has to be able to turn
    inside it.
    """
    point = np.asarray(point, float)
    axis = np.asarray(axis, float); axis = axis / np.linalg.norm(axis)
    out = []
    for pid, e in cat.items():
        holes, _ = part_ports(pid, cat)
        if not len(holes):
            continue
        n = 0
        for hole in holes:
            if bearing and hole[6] == CROSS:
                continue
            for ri in rotations_mapping(hole[3:6], axis):
                R = ROTATIONS[ri]
                origin = point - R @ hole[:3]
                if np.any(np.abs(origin / HALF_STUD - np.round(origin / HALF_STUD)) > 1e-6):
                    continue
                out.append(Placed(pid, ri, tuple(np.round(origin, 3))))
                n += 1
                if n >= max_per_part:
                    break
            if n >= max_per_part:
                break
    seen, uniq = set(), []
    for p in out:
        if p not in seen:
            seen.add(p); uniq.append(p)
    return uniq


def connectable(pa: Placed, pb: Placed, cat: dict) -> list[tuple]:
    """
    Where two placed parts can join. Three cases:
      pin in hole        - direct, no extra part
      hole on hole       - a pin has to go between, so one part more
      axle in round hole - turns freely, that is a bearing and not a joint
    """
    ha, pna = world_ports(pa, cat)
    hb, pnb = world_ports(pb, cat)
    links = []
    for male, female, who in ((pna, hb, "a-pin"), (pnb, ha, "b-pin")):
        for m in male:
            for f in female:
                if np.linalg.norm(m[:3] - f[:3]) > 0.5:
                    continue
                if abs(abs(float(np.dot(m[3:6], f[3:6]))) - 1.0) > 1e-6:
                    continue
                if m[6] == CROSS and f[6] == ROUND:
                    continue                    # axle in round hole: turns, not a joint
                links.append((who, tuple(np.round(m[:3], 2)), "direct"))
    for a in ha:
        for b in hb:
            if np.linalg.norm(a[:3] - b[:3]) > 0.5:
                continue
            if abs(abs(float(np.dot(a[3:6], b[3:6]))) - 1.0) > 1e-6:
                continue
            if a[6] == ROUND and b[6] == ROUND:
                links.append(("hole-hole", tuple(np.round(a[:3], 2)), "pin needed"))
    return links
