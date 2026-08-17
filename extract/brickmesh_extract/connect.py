# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Connecting separate pieces.

Two parts can only join directly if their holes happen to coincide with the
same direction. In a real mechanism that is rarely so: the bearings carry
shafts that stand perpendicular to each other, so their holes point every which
way.

What you need is a CHAIN of intermediate parts carrying one hole direction over
to the other. That is a Steiner tree problem, and between two pieces it comes
down to a shortest path: every part added widens the set of reachable ports,
and you look for the cheapest route until you touch a port of the other piece.

A* with, as its heuristic, the remaining distance divided by how far a part
reaches at most. Without a heuristic the search wanders off: there are close to
a thousand placements per port.
"""
from __future__ import annotations

import heapq

import numpy as np

from .portsynth import ROUND, Placed, part_ports, rotations_mapping, world_ports
from .voxel import ROTATIONS, voxels

HALF_STUD = 10.0
MAX_REACH = 220.0          # longest part in the inventory, in LDU


def _port_key(p, a):
    return (tuple(np.round(p, 2)), tuple(np.round(np.abs(a), 3)))


def placements_at(point, axis, cat: dict, want_gender: str = "F",
                  want_kind=None, cap: int = 25) -> list[Placed]:
    """
    Placements where a port of the part lands on `point` with the axis along
    `axis`. want_gender 'F' looks for holes, 'M' looks for pins.
    """
    point = np.asarray(point, float)
    axis = np.asarray(axis, float); axis = axis / np.linalg.norm(axis)
    out, seen = [], set()
    for pid in cat:
        holes, pins = part_ports(pid, cat)
        table = holes if want_gender == "F" else pins
        if not len(table):
            continue
        n = 0
        for row in table:
            if want_kind is not None and row[6] != want_kind:
                continue
            for ri in rotations_mapping(row[3:6], axis):
                origin = point - ROTATIONS[ri] @ row[:3]
                if np.any(np.abs(origin / HALF_STUD
                                 - np.round(origin / HALF_STUD)) > 1e-6):
                    continue
                pl = Placed(pid, ri, tuple(np.round(origin, 3)))
                if pl in seen:
                    continue
                seen.add(pl); out.append(pl); n += 1
                if n >= cap:
                    break
            if n >= cap:
                break
    return out


def free_ports(parts: list[Placed], cat: dict) -> list[tuple]:
    """Every port of a piece, as (position, axis, kind, gender)."""
    out = []
    for p in parts:
        h, pn = world_ports(p, cat)
        for r in h:
            out.append((r[:3], r[3:6], int(r[6]), "F"))
        for r in pn:
            out.append((r[:3], r[3:6], int(r[6]), "M"))
    return out


def _cells(pl: Placed) -> frozenset:
    c = voxels(pl.part, pl.rot)
    shift = np.round(np.asarray(pl.origin, float) / 5.0).astype(int)
    return frozenset(map(tuple, (c + shift).tolist()))


CONTACT_FRACTION = 0.12


def _compatible(a: frozenset, b: frozenset) -> bool:
    shared = len(a & b)
    return shared == 0 or shared <= CONTACT_FRACTION * min(len(a), len(b))


def connect(comp_a: list[Placed], comp_b: list[Placed], cat: dict,
            blocked: frozenset = frozenset(), max_parts: int = 3,
            beam: int = 14, cost_fn=None) -> list[Placed] | None:
    """
    Cheapest chain of parts tying piece A to piece B.
    Returns the parts to add, or None.
    """
    cost_fn = cost_fn or (lambda pl: 1.0)

    ports_a = free_ports(comp_a, cat)
    ports_b = free_ports(comp_b, cat)
    if not ports_a or not ports_b:
        return None
    target_pts = np.array([p[0] for p in ports_b])
    target_axes = np.array([p[1] / np.linalg.norm(p[1]) for p in ports_b])
    target_keys = {_port_key(p[0], p[1]) for p in ports_b}

    base_cells = frozenset().union(*[_cells(p) for p in comp_a + comp_b]) | blocked

    HW = 6.0        # heuristic weight; without it A* is just BFS

    def h(pl: Placed) -> float:
        h_, pn = world_ports(pl, cat)
        pts = np.vstack([x[:3] for x in list(h_) + list(pn)]) if len(h_) or len(pn) else None
        if pts is None:
            return 1e9
        axs = np.vstack([x[3:6] for x in list(h_) + list(pn)])
        axs = axs / np.linalg.norm(axs, axis=1, keepdims=True)
        # Only ports sharing an axis can ever be connected. Measure the
        # distance without that distinction and the heuristic rewards exactly
        # those candidates sitting uselessly on top of the target with a hole
        # across it, and prunes away the perpendicular connectors that do solve
        # it.
        align = np.abs(axs @ target_axes.T)
        d = np.linalg.norm(pts[:, None, :] - target_pts[None, :, :], axis=2)
        d = np.where(align > 1 - 1e-6, d, np.inf)
        best = float(d.min())
        if not np.isfinite(best):
            # no axis matches at all: score on how close the directions come
            # to each other, since that is what a next link has to fix
            return HW * (1.0 + (1.0 - float(align.max())))
        return HW * best / MAX_REACH

    start_frontier = tuple(sorted(_port_key(p[0], p[1]) for p in ports_a))
    heap = [(0.0, 0, 0, tuple(), start_frontier)]
    seen = set()
    counter = 0

    while heap:
        _f, g, _, chain, frontier = heapq.heappop(heap)
        if frontier in seen:
            continue
        seen.add(frontier)

        if set(frontier) & target_keys:
            return list(chain)
        if len(chain) >= max_parts:
            continue

        used = frozenset().union(*[_cells(p) for p in chain]) if chain else frozenset()
        occupied = base_cells | used

        # Sort ports by distance to the target and expand only the nearest.
        # Truncating arbitrarily throws away precisely the useful ports.
        fr = sorted(frontier,
                    key=lambda k: float(np.min(np.linalg.norm(
                        target_pts - np.array(k[0]), axis=1))))
        cands = []
        for key in fr[:8]:
            pos, ax = np.array(key[0]), np.array(key[1])
            # pin into the reachable hole, or hole on hole with a pin between
            for pl in (placements_at(pos, ax, cat, "M", cap=14)
                       + placements_at(pos, ax, cat, "F", ROUND, cap=14)):
                if pl in chain:
                    continue
                if not _compatible(_cells(pl), occupied):
                    continue
                cands.append(pl)

        scored = sorted({c: h(c) for c in cands}.items(), key=lambda kv: kv[1])
        for pl, hv in scored[:beam]:
            ng = g + cost_fn(pl)
            hh, pn = world_ports(pl, cat)
            newkeys = {_port_key(r[:3], r[3:6]) for r in list(hh) + list(pn)}
            nf = tuple(sorted(set(frontier) | newkeys))
            counter += 1
            heapq.heappush(heap, (ng + hv, ng, counter, (*chain, pl), nf))

    return None


# --------------------------------------------------------------------------
# index: the difference between 7.7 ms and microseconds per query
# --------------------------------------------------------------------------

_INDEX: dict = {}


def _axis_key(a) -> tuple:
    """Holes have no direction: +Y and -Y are the same hole."""
    a = np.asarray(a, float)
    a = a / np.linalg.norm(a)
    for v in a:
        if abs(v) > 1e-9:
            if v < 0:
                a = -a
            break
    return tuple(np.round(a, 3))


def build_index(cat: dict) -> dict:
    """
    Turn the question around.

    The search keeps asking: which part has a hole at THIS point with THIS
    direction. Answering that by walking every part costs 7.7 ms. But the set
    of answers does not depend on the point - only on the direction. So compute
    once, for every combination of part, hole and rotation, which world
    direction comes out, and group on that.

    A query then becomes: look up the table, and per hit origin = point minus
    the pre-rotated hole position. That is one subtraction.
    """
    idx: dict = {}
    for pid, e in cat.items():
        for gender, arr in (("F", e["holes"]), ("M", e["pins"])):
            for rec in arr:
                pos = np.array(rec[:3], float)
                ax = np.array(rec[3:6], float)
                kind = int(rec[6])
                for ri, R in enumerate(ROTATIONS):
                    key = (_axis_key(R @ ax), gender, kind)
                    idx.setdefault(key, []).append((pid, ri, R @ pos))
    for k, v in idx.items():
        parts = np.array([x[0] for x in v], dtype=object)
        rots = np.array([x[1] for x in v], dtype=np.int16)
        offs = np.array([x[2] for x in v], dtype=np.float32)
        idx[k] = (parts, rots, offs)
    return idx


def index_for(cat: dict) -> dict:
    key = id(cat)
    if key not in _INDEX:
        _INDEX[key] = build_index(cat)
    return _INDEX[key]


def placements_indexed(point, axis, cat: dict, gender: str = "F",
                       kind: int | None = None) -> list[Placed]:
    """The same answer as placements_at, but through the index."""
    idx = index_for(cat)
    point = np.asarray(point, float)
    key_axis = _axis_key(axis)
    out = []
    kinds = (0, 1) if kind is None else (kind,)
    for k in kinds:
        hit = idx.get((key_axis, gender, k))
        if hit is None:
            continue
        parts, rots, offs = hit
        origins = point[None, :] - offs
        ok = np.all(np.abs(origins / 10.0 - np.round(origins / 10.0)) < 1e-4, axis=1)
        for pid, ri, o in zip(parts[ok], rots[ok], origins[ok]):
            out.append(Placed(pid, int(ri), tuple(np.round(o, 3))))
    return out


def placements_topk(point, axis, cat: dict, gender: str = "F",
                    kind: int | None = None, target_pts=None,
                    k: int = 24) -> list[Placed]:
    """
    The same lookup, but filter and score VECTORIZED and only build objects for
    the best k.

    That is where the time went. The index finds 2600 candidates in
    microseconds, and then it costs 16 milliseconds to turn them into 2600
    Python objects that are almost all thrown away again. Score first, build
    only what you keep.
    """
    idx = index_for(cat)
    point = np.asarray(point, float)
    key_axis = _axis_key(axis)
    kinds = (0, 1) if kind is None else (kind,)

    P, R_, O = [], [], []
    for kk in kinds:
        hit = idx.get((key_axis, gender, kk))
        if hit is None:
            continue
        parts, rots, offs = hit
        P.append(parts); R_.append(rots); O.append(offs)
    if not P:
        return []
    parts = np.concatenate(P); rots = np.concatenate(R_); offs = np.concatenate(O)

    origins = point[None, :] - offs
    ok = np.all(np.abs(origins / 10.0 - np.round(origins / 10.0)) < 1e-4, axis=1)
    if not ok.any():
        return []
    parts, rots, origins = parts[ok], rots[ok], origins[ok]

    if target_pts is not None and len(origins):
        tp = np.asarray(target_pts, float)
        d = np.linalg.norm(origins[:, None, :] - tp[None, :, :], axis=2).min(axis=1)
        order = np.argsort(d)[:k]
    else:
        order = np.arange(min(k, len(origins)))

    return [Placed(str(parts[i]), int(rots[i]), tuple(np.round(origins[i], 3)))
            for i in order]
