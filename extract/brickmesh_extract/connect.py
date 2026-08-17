# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Verbinden van losse stukken.

Twee onderdelen kunnen alleen rechtstreeks aan elkaar als hun gaten toevallig
samenvallen met dezelfde richting. In een echt mechanisme is dat zelden zo: de
lagers dragen assen die haaks op elkaar staan, dus hun gaten wijzen alle
kanten op.

Wat je nodig hebt is een KETTING van tussenonderdelen die de ene gatrichting
naar de andere brengt. Dat is een Steinerboom-probleem, en tussen twee stukken
komt het neer op een kortste pad: elk toegevoegd onderdeel breidt de
bereikbare poorten uit, en je zoekt de goedkoopste weg tot je een poort van het
andere stuk raakt.

A* met als heuristiek de resterende afstand gedeeld door hoever een onderdeel
maximaal reikt. Zonder heuristiek dwaalt de zoeker af: er zijn bijna duizend
plaatsingen per poort.
"""
from __future__ import annotations

import heapq
from dataclasses import dataclass

import numpy as np

from .portsynth import Placed, world_ports, part_ports, rotations_mapping, ROUND, CROSS
from .voxel import ROTATIONS, voxels

HALF_STUD = 10.0
MAX_REACH = 220.0          # langste onderdeel in de voorraad, in LDU


def _port_key(p, a):
    return (tuple(np.round(p, 2)), tuple(np.round(np.abs(a), 3)))


def placements_at(point, axis, cat: dict, want_gender: str = "F",
                  want_kind=None, cap: int = 25) -> list[Placed]:
    """
    Plaatsingen waarbij een poort van het onderdeel op `point` valt met de as
    langs `axis`. want_gender 'F' zoekt gaten, 'M' zoekt pennen.
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
    """Alle poorten van een stuk, als (positie, as, soort, geslacht)."""
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
    Goedkoopste ketting onderdelen die stuk A aan stuk B knoopt.
    Geeft de toe te voegen onderdelen terug, of None.
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

    HW = 6.0        # gewicht van de heuristiek; zonder dit is A* gewoon BFS

    def h(pl: Placed) -> float:
        h_, pn = world_ports(pl, cat)
        pts = np.vstack([x[:3] for x in list(h_) + list(pn)]) if len(h_) or len(pn) else None
        if pts is None:
            return 1e9
        axs = np.vstack([x[3:6] for x in list(h_) + list(pn)])
        axs = axs / np.linalg.norm(axs, axis=1, keepdims=True)
        # Alleen poorten met dezelfde as kunnen ooit verbonden worden. Meet je
        # de afstand zonder dat onderscheid, dan beloont de heuristiek juist de
        # kandidaten die nutteloos boven op het doel liggen met een gat dwars
        # erop, en snijdt hij de haakse verbinders weg die het wel oplossen.
        align = np.abs(axs @ target_axes.T)
        d = np.linalg.norm(pts[:, None, :] - target_pts[None, :, :], axis=2)
        d = np.where(align > 1 - 1e-6, d, np.inf)
        best = float(d.min())
        if not np.isfinite(best):
            # geen enkele as komt overeen: waardeer op hoe dicht de richtingen
            # bij elkaar komen, want dat is wat een volgende schakel moet doen
            return HW * (1.0 + (1.0 - float(align.max())))
        return HW * best / MAX_REACH

    start_frontier = tuple(sorted(_port_key(p[0], p[1]) for p in ports_a))
    heap = [(0.0, 0, 0, tuple(), start_frontier)]
    seen = set()
    counter = 0

    while heap:
        f, g, _, chain, frontier = heapq.heappop(heap)
        if frontier in seen:
            continue
        seen.add(frontier)

        if set(frontier) & target_keys:
            return list(chain)
        if len(chain) >= max_parts:
            continue

        used = frozenset().union(*[_cells(p) for p in chain]) if chain else frozenset()
        occupied = base_cells | used

        # Poorten sorteren op afstand tot het doel en alleen de dichtstbijzijnde
        # uitbreiden. Willekeurig afsnijden gooit juist de nuttige poorten weg.
        fr = sorted(frontier,
                    key=lambda k: float(np.min(np.linalg.norm(
                        target_pts - np.array(k[0]), axis=1))))
        cands = []
        for key in fr[:8]:
            pos, ax = np.array(key[0]), np.array(key[1])
            # pen in het bereikbare gat, of gat op gat met een pin ertussen
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
            heapq.heappush(heap, (ng + hv, ng, counter, chain + (pl,), nf))

    return None


# --------------------------------------------------------------------------
# index: het verschil tussen 7,7 ms en microseconden per zoekvraag
# --------------------------------------------------------------------------

_INDEX: dict = {}


def _axis_key(a) -> tuple:
    """Gaten zijn richtingloos: +Y en -Y zijn hetzelfde gat."""
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
    Draai de vraag om.

    De zoeker vraagt telkens: welk onderdeel heeft een gat op DIT punt met DEZE
    richting. Dat beantwoorden door alle onderdelen langs te lopen kost 7,7 ms.
    Maar de verzameling antwoorden hangt niet van het punt af - alleen van de
    richting. Dus reken een keer voor elke combinatie van onderdeel, gat en
    rotatie uit welke wereldrichting eruit komt, en groepeer daarop.

    Een zoekvraag wordt dan: tabel opzoeken, en per treffer origin = punt min
    de voorgedraaide gatpositie. Dat is een aftrekking.
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
    """Zelfde antwoord als placements_at, maar via de index."""
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
    Zelfde opzoeking, maar filter en scoor VECTORIEEL en maak pas objecten van
    de beste k.

    Dat is waar de tijd zat. De index vindt in microseconden 2600 kandidaten,
    en vervolgens kost het 16 milliseconden om er 2600 Python-objecten van te
    maken die daarna vrijwel allemaal worden weggegooid. Score eerst, bouw
    daarna alleen wat je houdt.
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
