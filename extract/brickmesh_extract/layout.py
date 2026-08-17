# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
De meetkundige laag: van functionele graaf naar assen op het rooster.

De functionele laag zegt wat er met wat verbonden is. Deze laag zoekt uit
waar die assen dan liggen. Elke overbrenging legt een meetkundige eis op:

  evenwijdig kammend  ->  assen evenwijdig, loodrechte afstand (ta+tb)/8 halve studs
  conisch kammend     ->  assen haaks EN snijdend
  differentieel       ->  alle drie de poorten op dezelfde lijn

Dat is een beperkingsprobleem op een rooster, dus eindig en opsombaar. Er is
geen optimalisatie nodig, alleen doorzoeken met terugkrabbelen.

Eenheid is de halve stud (10 LDU), want alle tandaantallen zijn veelvouden van
vier en daarmee vallen alle hartafstanden op hele halve studs.
"""
from __future__ import annotations

import itertools
from dataclasses import dataclass

import numpy as np

from .mech import Mechanism, Mesh, Differential

HALF_STUD = 10.0

# Kanonieke richtingen. De pythagorese azimuts kun je aanzetten als je schuine
# assen wilt toestaan; ze kosten zoekruimte en leveren zelden compacter op.
AXIS_DIRS = [np.array(v, float) for v in ((1, 0, 0), (0, 1, 0), (0, 0, 1))]
PYTHAGOREAN_DIRS = [np.array(v, float) / 5.0 for v in
                    ((4, 3, 0), (3, 4, 0), (4, 0, 3), (3, 0, 4), (0, 4, 3), (0, 3, 4))]


def sum_of_two_squares(n: int) -> list[tuple[int, int]]:
    """Alle (a,b) met a^2+b^2 = n, inclusief tekens en volgorde."""
    out = []
    r = int(np.isqrt(n)) if hasattr(np, "isqrt") else int(n ** 0.5)
    for a in range(-r - 1, r + 2):
        b2 = n - a * a
        if b2 < 0:
            continue
        b = int(round(b2 ** 0.5))
        if b * b == b2:
            out.append((a, b))
            if b:
                out.append((a, -b))
    return sorted(set(out))


@dataclass
class Placement:
    """Een as als oneindige lijn: een punt erop plus een richting."""
    point: np.ndarray          # in halve studs
    direction: np.ndarray      # eenheidsvector

    def __post_init__(self):
        self.point = np.asarray(self.point, float)
        d = np.asarray(self.direction, float)
        self.direction = d / np.linalg.norm(d)

    def key(self):
        return (tuple(np.round(self.point, 6)), tuple(np.round(self.direction, 6)))


def _perp_basis(d: np.ndarray):
    tmp = np.array([1.0, 0, 0]) if abs(d[0]) < 0.9 else np.array([0, 1.0, 0])
    u = np.cross(d, tmp); u /= np.linalg.norm(u)
    return u, np.cross(d, u)


def parallel_distance(p: Placement, q: Placement) -> float | None:
    if abs(abs(float(np.dot(p.direction, q.direction))) - 1.0) > 1e-9:
        return None
    v = q.point - p.point
    return float(np.linalg.norm(v - np.dot(v, p.direction) * p.direction))


def axes_intersect(p: Placement, q: Placement, tol: float = 1e-6) -> bool:
    n = np.cross(p.direction, q.direction)
    if np.linalg.norm(n) < tol:
        return False
    return abs(float(np.dot(q.point - p.point, n / np.linalg.norm(n)))) < tol


def perpendicular(p: Placement, q: Placement) -> bool:
    return abs(float(np.dot(p.direction, q.direction))) < 1e-9


def line_distance(p: Placement, q: Placement) -> float:
    """Kortste afstand tussen twee oneindige lijnen, in halve studs."""
    n = np.cross(p.direction, q.direction)
    nn = np.linalg.norm(n)
    if nn < 1e-9:                                   # evenwijdig
        v = q.point - p.point
        return float(np.linalg.norm(v - np.dot(v, p.direction) * p.direction))
    return abs(float(np.dot(q.point - p.point, n / nn)))


# --------------------------------------------------------------------------

class Layout:
    def __init__(self, mech: Mechanism):
        self.mech = mech
        self.place: dict[str, Placement] = {}

    def bbox_volume(self) -> float:
        if not self.place:
            return 0.0
        pts = np.array([p.point for p in self.place.values()])
        size = pts.max(axis=0) - pts.min(axis=0) + 1
        return float(np.prod(size))

    def satisfied(self, link, a: str, b: str) -> bool:
        p, q = self.place[a], self.place[b]
        if isinstance(link, Mesh):
            if link.kind == "spur":
                d = parallel_distance(p, q)
                return d is not None and abs(d - link.centre_distance_halfstuds) < 1e-6
            if link.kind == "bevel":
                return perpendicular(p, q) and axes_intersect(p, q)
            if link.kind == "worm":
                return perpendicular(p, q)
        return True


def _links_between(mech: Mechanism, a: str, b: str):
    for l in mech.links:
        if isinstance(l, Mesh) and {l.a, l.b} == {a, b}:
            yield l


def _diff_groups(mech: Mechanism) -> list[list[str]]:
    return [[l.case, l.out_a, l.out_b] for l in mech.links
            if isinstance(l, Differential)]


def realise(mech: Mechanism, seed: str | None = None,
            allow_angled: bool = False, max_solutions: int = 20,
            span: int = 6, radius: dict[str, float] | None = None) -> list[Layout]:
    """
    Zoek roosterposities voor alle assen. Geeft oplossingen terug, oplopend
    gesorteerd op omhullend volume - dus compact eerst.
    """
    dirs = AXIS_DIRS + (PYTHAGOREAN_DIRS if allow_angled else [])
    shafts = list(mech.shafts)
    seed = seed or shafts[0]

    # differentieelpoorten delen een lijn: klap ze samen tot een klasse
    parent = {s: s for s in shafts}

    def find(x):
        while parent[x] != x:
            parent[x] = parent[parent[x]]; x = parent[x]
        return x

    for grp in _diff_groups(mech):
        for other in grp[1:]:
            parent[find(other)] = find(grp[0])

    classes: dict[str, list[str]] = {}
    for s in shafts:
        classes.setdefault(find(s), []).append(s)
    reps = list(classes)

    # buren tussen klassen
    adj: dict[str, set] = {r: set() for r in reps}
    for l in mech.links:
        if isinstance(l, Mesh):
            ra, rb = find(l.a), find(l.b)
            if ra != rb:
                adj[ra].add(rb); adj[rb].add(ra)

    order, seen = [], set()
    stack = [find(seed)]
    while stack:
        r = stack.pop(0)
        if r in seen:
            continue
        seen.add(r); order.append(r)
        stack.extend(sorted(adj[r] - seen))
    order += [r for r in reps if r not in seen]

    radius = radius or {}

    def clear(rep_a: str, rep_b: str, pa: Placement, pb: Placement) -> bool:
        """
        Assen die niet met elkaar kammen moeten uit elkaars weg blijven. Zonder
        deze eis landen twee differentiëlen vrolijk op dezelfde lijn - de graaf
        verbiedt dat immers nergens.
        """
        if rep_b in adj[rep_a]:
            return True                            # kammend paar: al geregeld
        need = (max(radius.get(s, 1.0) for s in classes[rep_a])
                + max(radius.get(s, 1.0) for s in classes[rep_b]))
        return line_distance(pa, pb) >= need - 1e-9

    solutions: list[Layout] = []

    def candidates(rep: str, placed: dict[str, Placement]) -> list[Placement]:
        anchors = [n for n in adj[rep] if n in placed]
        if not anchors:
            return [Placement([0, 0, 0], AXIS_DIRS[0])]
        anchor = anchors[0]
        p = placed[anchor]
        link = next(iter(_links_between(mech, classes[anchor][0], classes[rep][0])), None)
        if link is None:
            for a in classes[anchor]:
                for b in classes[rep]:
                    link = next(iter(_links_between(mech, a, b)), None)
                    if link:
                        break
                if link:
                    break
        out = []
        if link and link.kind == "spur":
            d2 = int(round(link.centre_distance_halfstuds ** 2))
            u, v = _perp_basis(p.direction)
            for a, b in sum_of_two_squares(d2):
                for t in range(-span, span + 1):
                    pt = p.point + a * u + b * v + t * p.direction
                    out.append(Placement(pt, p.direction))
        else:                                    # conisch of worm: haaks
            for d in dirs:
                if abs(float(np.dot(d, p.direction))) > 1e-9:
                    continue
                for t in range(-span, span + 1):
                    out.append(Placement(p.point + t * p.direction, d))
        uniq = {}
        for c in out:
            uniq.setdefault(c.key(), c)
        return list(uniq.values())

    def backtrack(i: int, placed: dict[str, Placement]):
        if len(solutions) >= max_solutions:
            return
        if i == len(order):
            lay = Layout(mech)
            for rep, pl in placed.items():
                for s in classes[rep]:
                    lay.place[s] = pl
            solutions.append(lay)
            return
        rep = order[i]
        for cand in candidates(rep, placed):
            trial = dict(placed); trial[rep] = cand
            ok = True
            for other in adj[rep]:
                if other not in trial:
                    continue
                for a in classes[rep]:
                    for b in classes[other]:
                        for l in _links_between(mech, a, b):
                            lay = Layout(mech)
                            lay.place = {a: trial[rep], b: trial[other]}
                            if not lay.satisfied(l, a, b):
                                ok = False
                if not ok:
                    break
            if ok:
                for other, po in placed.items():
                    if not clear(rep, other, cand, po):
                        ok = False
                        break
            if ok:
                backtrack(i + 1, trial)

    backtrack(0, {})
    solutions.sort(key=lambda L: L.bbox_volume())
    return solutions


# --------------------------------------------------------------------------
# stations: waar de tandwielen langs elke as zitten
# --------------------------------------------------------------------------

# Dikte in halve studs. Wordt gebruikt om overlap op dezelfde as te vinden en
# om de vrije stukken over te houden waar de lagers mogen komen.
GEAR_THICKNESS = {8: 2.0, 12: 2.0, 16: 1.0, 20: 2.0, 24: 1.0, 28: 2.0, 36: 1.0, 40: 1.0}


def effective_radius(teeth: int) -> float:
    """In halve studs. Volgt uit de steekregel: straal in studs = tanden/16."""
    return teeth / 8.0


@dataclass
class Station:
    shaft: str
    teeth: int
    axial: float               # coordinaat langs de asrichting, halve studs
    thickness: float
    origin: str = ""           # waar de waarde vandaan komt

    @property
    def span(self):
        return (self.axial - self.thickness / 2, self.axial + self.thickness / 2)


def solve_stations(mech: Mechanism, layout: Layout) -> tuple[list[Station], list]:
    """
    Bepaal de axiale positie van elk tandwiel.

    Conische paren geven een absolute positie: de assen snijden elkaar in een
    punt, en elk tandwiel staat daar op de effectieve straal van de ander
    vandaan. Evenwijdige paren geven alleen een gelijkheid - beide tandwielen
    moeten in hetzelfde vlak liggen. Dus: eerst de conische verankeren, dan de
    gelijkheden doorgeven.
    """
    from .mech import Finding

    stations: dict[tuple[str, int, int], Station] = {}
    findings: list = []

    def axial_of(shaft: str, world_pt: np.ndarray) -> float:
        p = layout.place[shaft]
        return float(np.dot(world_pt - p.point, p.direction))

    # 1. conische paren verankeren
    for i, l in enumerate(mech.links):
        if not (isinstance(l, Mesh) and l.kind == "bevel"):
            continue
        pa, pb = layout.place[l.a], layout.place[l.b]
        n = np.cross(pa.direction, pb.direction)
        w0 = pa.point - pb.point
        # snijpunt van twee elkaar snijdende lijnen
        denom = np.dot(np.cross(pb.direction, n), pa.direction)
        if abs(denom) < 1e-9:
            findings.append(Finding("FAIL", "station",
                                    f"{l.a}/{l.b}: assen snijden elkaar niet"))
            continue
        t = -np.dot(np.cross(pb.direction, n), w0) / denom
        P = pa.point + t * pa.direction
        # elk tandwiel op de effectieve straal van de ander vanaf het snijpunt
        for shaft, teeth, other in ((l.a, l.teeth_a, l.teeth_b),
                                    (l.b, l.teeth_b, l.teeth_a)):
            off = effective_radius(other)
            d = layout.place[shaft].direction
            for sign in (+1, -1):
                pos = P + sign * off * d
                if sign == +1:
                    stations[(shaft, teeth, i)] = Station(
                        shaft, teeth, axial_of(shaft, pos),
                        GEAR_THICKNESS.get(teeth, 2.0),
                        f"conisch met {other}t, effectieve straal {off} halve studs")

    # 2. evenwijdige paren: gelijk axiaal vlak
    for i, l in enumerate(mech.links):
        if not (isinstance(l, Mesh) and l.kind == "spur"):
            continue
        ka = next((k for k in stations if k[0] == l.a), None)
        kb = next((k for k in stations if k[0] == l.b), None)
        if ka and not kb:
            base = stations[ka].axial
        elif kb and not ka:
            base = stations[kb].axial
        elif ka and kb:
            if abs(stations[ka].axial - stations[kb].axial) > 1e-6:
                findings.append(Finding(
                    "FAIL", "station",
                    f"{l.a}/{l.b}: beide assen al verankerd op verschillende vlakken "
                    f"({stations[ka].axial:.2f} en {stations[kb].axial:.2f}) - kammen niet"))
            base = stations[ka].axial
        else:
            base = 0.0
        for shaft, teeth in ((l.a, l.teeth_a), (l.b, l.teeth_b)):
            stations.setdefault((shaft, teeth, i), Station(
                shaft, teeth, base, GEAR_THICKNESS.get(teeth, 2.0),
                "evenwijdig, zelfde vlak"))

    # Twee tandwielen met hetzelfde tandaantal op dezelfde as in hetzelfde vlak
    # zijn in werkelijkheid EEN tandwiel dat twee dingen tegelijk aandrijft.
    merged: dict[tuple, Station] = {}
    for st in stations.values():
        k = (st.shaft, st.teeth, round(st.axial, 6))
        if k in merged:
            merged[k].origin += " (deelt met een tweede ingrijping)"
        else:
            merged[k] = st
    result = list(merged.values())

    # 3. overlap op dezelfde as
    per_shaft: dict[str, list[Station]] = {}
    for st in result:
        per_shaft.setdefault(st.shaft, []).append(st)
    for shaft, sts in per_shaft.items():
        for x, y in itertools.combinations(sorted(sts, key=lambda s: s.axial), 2):
            lo1, hi1 = x.span; lo2, hi2 = y.span
            if min(hi1, hi2) - max(lo1, lo2) > 1e-6:
                findings.append(Finding(
                    "FAIL", "station",
                    f"as '{shaft}': {x.teeth}t op {x.axial:.2f} en {y.teeth}t op "
                    f"{y.axial:.2f} overlappen elkaar"))

    # 4. rooster
    for st in result:
        if abs(st.axial - round(st.axial)) > 1e-6:
            findings.append(Finding(
                "WARN", "station",
                f"as '{st.shaft}': {st.teeth}t op {st.axial:.3f} halve studs, "
                f"niet op het rooster"))

    if not findings:
        findings.append(Finding("OK", "station",
                                f"{len(result)} tandwielstations bepaald, geen conflicten"))
    return result, findings


def free_intervals(stations: list[Station], shaft: str,
                   reach: float = 12.0) -> list[tuple[float, float]]:
    """Stukken van een as waar geen tandwiel zit: daar kunnen de lagers heen."""
    occ = sorted(s.span for s in stations if s.shaft == shaft)
    free, cursor = [], -reach
    for lo, hi in occ:
        if lo - cursor > 0.5:
            free.append((cursor, lo))
        cursor = max(cursor, hi)
    if reach - cursor > 0.5:
        free.append((cursor, reach))
    return free
