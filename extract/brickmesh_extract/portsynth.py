# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Verkenner op basis van poorten in plaats van balken.

De vorige versie ging ervan uit dat een dragend onderdeel gaten op een rij
heeft langs zijn lengterichting, allemaal dezelfde kant op. Dat klopt voor
liftarms en voor niets anders in de catalogus. Een hoekverbinder heeft twee
gaten die loodrecht op elkaar staan, een askoppeling heeft mannelijke
uiteinden, en een plaat met pingaten heeft weer een andere indeling.

Deze versie leest de poorten uit de catalogus: positie, richting, en of het
rond of kruisvormig is. Dat laatste is geen detail:

    een lager moet een ROND gat zijn.

Een as in een kruisvormig gat kan niet draaien. Een verkenner die dat
onderscheid niet maakt levert constructies op die vastzitten.
"""
from __future__ import annotations

from dataclasses import dataclass

import numpy as np

from . import catalog
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
    """Gaten en pennen van een onderdeel als arrays: [x,y,z,ax,ay,az,kruis]."""
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
    """Alleen de rotaties die de gat-as op de gevraagde richting leggen."""
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
    Alle manieren om een onderdeel zo te leggen dat een van zijn gaten op
    `point` valt met de as langs `axis`.

    Bij bearing=True tellen alleen RONDE gaten mee: een as moet erin kunnen
    draaien.
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
    Waar twee geplaatste onderdelen aan elkaar kunnen. Drie gevallen:
      pen in gat        - direct, geen extra onderdeel
      gat op gat        - er moet een pin tussen, dus een onderdeel erbij
      as in rond gat    - draait vrij, dat is een lager en geen verbinding
    """
    ha, pna = world_ports(pa, cat)
    hb, pnb = world_ports(pb, cat)
    links = []
    for male, female, who in ((pna, hb, "a-pen"), (pnb, ha, "b-pen")):
        for m in male:
            for f in female:
                if np.linalg.norm(m[:3] - f[:3]) > 0.5:
                    continue
                if abs(abs(float(np.dot(m[3:6], f[3:6]))) - 1.0) > 1e-6:
                    continue
                if m[6] == CROSS and f[6] == ROUND:
                    continue                    # as in rond gat: draait, geen verbinding
                links.append((who, tuple(np.round(m[:3], 2)), "direct"))
    for a in ha:
        for b in hb:
            if np.linalg.norm(a[:3] - b[:3]) > 0.5:
                continue
            if abs(abs(float(np.dot(a[3:6], b[3:6]))) - 1.0) > 1e-6:
                continue
            if a[6] == ROUND and b[6] == ROUND:
                links.append(("gat-gat", tuple(np.round(a[:3], 2)), "pin nodig"))
    return links
