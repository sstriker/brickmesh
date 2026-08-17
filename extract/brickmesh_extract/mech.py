# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
De functionele laag: assen en overbrengingen, los van elk onderdeel.

Hier reken je een mechanisme door voordat je iets plaatst. Verhoudingen,
draairichtingen, vrijheidsgraden, koppel, en de meetkundige eisen die eruit
volgen. Dit is de laag waar een idee sneuvelt of overeind blijft, en dat wil
je weten voor je begint te bouwen.

Kern: elke overbrenging is een lineaire vergelijking tussen assnelheden. Het
hele mechanisme is dus een matrix, en de nulruimte daarvan zijn je
vrijheidsgraden. Een subtractor hoort er twee te hebben (rijden en sturen);
komt er een uit, dan zit je trein op slot.
"""
from __future__ import annotations

import itertools
import math
from dataclasses import dataclass, field

import numpy as np

HALF_STUD = 10.0          # LDU. Alle tandaantallen zijn veelvouden van 4, dus
                          # hartafstanden vallen altijd op hele halve studs.


# --------------------------------------------------------------------------
# elementen
# --------------------------------------------------------------------------

@dataclass
class Shaft:
    id: str
    bearings: int = 0                  # aantal lagerpunten
    domain: str = "technic-studless"
    note: str = ""


@dataclass
class Mesh:
    """Tandwielpaar. Extern kammend, dus de draairichting keert om."""
    a: str
    b: str
    teeth_a: int
    teeth_b: int
    kind: str = "spur"                 # spur | bevel | worm | chain
    backlash_deg: float = 5.0

    @property
    def reverses(self) -> bool:
        return self.kind in ("spur", "bevel")

    @property
    def centre_distance_halfstuds(self) -> float | None:
        """Alleen zinvol bij evenwijdige assen."""
        if self.kind != "spur":
            return None
        return (self.teeth_a + self.teeth_b) / 8.0

    def equation(self, index: dict, n: int) -> np.ndarray:
        """t_a * w_a  (+/-) t_b * w_b = 0"""
        row = np.zeros(n)
        sign = 1.0 if self.reverses else -1.0
        row[index[self.a]] = self.teeth_a
        row[index[self.b]] = sign * self.teeth_b
        return row


@dataclass
class Differential:
    """
    Drie poorten. De huissnelheid is het gemiddelde van beide uitgangen:
        2*w_huis - w_1 - w_2 = 0
    Dit is de enige overbrenging met meer dan twee poorten, en precies daarom
    kan een subtractor bestaan.
    """
    case: str
    out_a: str
    out_b: str

    def equation(self, index: dict, n: int) -> np.ndarray:
        row = np.zeros(n)
        row[index[self.case]] = 2.0
        row[index[self.out_a]] = -1.0
        row[index[self.out_b]] = -1.0
        return row


@dataclass
class Finding:
    level: str
    check: str
    detail: str


# --------------------------------------------------------------------------
# mechanisme
# --------------------------------------------------------------------------

class Mechanism:
    def __init__(self, name: str):
        self.name = name
        self.shafts: dict[str, Shaft] = {}
        self.links: list = []
        self.inputs: dict[str, float] = {}
        self.outputs: list[str] = []

    # ---- opbouw ----

    def shaft(self, sid: str, **kw) -> str:
        self.shafts[sid] = Shaft(sid, **kw)
        return sid

    def mesh(self, a: str, b: str, ta: int, tb: int, kind: str = "spur", **kw):
        self.links.append(Mesh(a, b, ta, tb, kind, **kw))

    def differential(self, case: str, out_a: str, out_b: str):
        self.links.append(Differential(case, out_a, out_b))

    def drive(self, sid: str, speed: float = 1.0):
        self.inputs[sid] = speed

    def output(self, sid: str):
        self.outputs.append(sid)

    # ---- kinematica ----

    @property
    def _index(self) -> dict:
        return {s: i for i, s in enumerate(self.shafts)}

    def _matrix(self) -> np.ndarray:
        idx, n = self._index, len(self.shafts)
        if not self.links:
            return np.zeros((0, n))
        return np.vstack([l.equation(idx, n) for l in self.links])

    def dof(self) -> int:
        """Vrijheidsgraden = aantal assen min de rang van de beperkingen."""
        A = self._matrix()
        rank = np.linalg.matrix_rank(A) if len(A) else 0
        return len(self.shafts) - int(rank)

    def solve(self) -> dict[str, float] | None:
        """Snelheden van alle assen, gegeven de aangedreven assen."""
        idx, n = self._index, len(self.shafts)
        A = list(self._matrix())
        rhs = [0.0] * len(A)
        for sid, w in self.inputs.items():
            row = np.zeros(n); row[idx[sid]] = 1.0
            A.append(row); rhs.append(w)
        A = np.array(A); rhs = np.array(rhs)
        if np.linalg.matrix_rank(A) < n:
            return None                      # onderbepaald
        sol, *_ = np.linalg.lstsq(A, rhs, rcond=None)
        if not np.allclose(A @ sol, rhs, atol=1e-8):
            return None                      # strijdig
        return {s: float(sol[idx[s]]) for s in self.shafts}

    # ---- controles ----

    def check_dof(self) -> list[Finding]:
        d, k = self.dof(), len(self.inputs)
        if d == 0:
            return [Finding("FAIL", "vrijheidsgraden",
                            "mechanisme heeft 0 vrijheidsgraden: de trein zit op slot "
                            "en kan niet draaien")]
        if k < d:
            return [Finding("WARN", "vrijheidsgraden",
                            f"{d} vrijheidsgraden maar {k} aangedreven assen — "
                            f"{d-k} beweging(en) blijven onbepaald")]
        if k > d:
            return [Finding("FAIL", "vrijheidsgraden",
                            f"{k} aandrijvingen op {d} vrijheidsgraden — overbepaald, "
                            f"de motoren werken tegen elkaar in")]
        return [Finding("OK", "vrijheidsgraden",
                        f"{d} vrijheidsgraden, {k} aandrijvingen: bepaald")]

    def check_bearings(self) -> list[Finding]:
        out = []
        for s in self.shafts.values():
            if s.bearings < 2:
                out.append(Finding("FAIL", "lagering",
                                   f"as '{s.id}' heeft {s.bearings} lagerpunt(en). "
                                   f"Minder dan twee betekent zwiepen onder belasting."))
        if not out:
            out.append(Finding("OK", "lagering", "alle assen dubbel gelagerd"))
        return out

    def check_domains(self) -> list[Finding]:
        out = []
        for l in self.links:
            ids = ([l.a, l.b] if isinstance(l, Mesh) else [l.case, l.out_a, l.out_b])
            doms = {self.shafts[i].domain for i in ids if i in self.shafts}
            if len(doms) > 1:
                out.append(Finding(
                    "FAIL", "rooster",
                    f"overbrenging tussen {ids} kruist roosterdomeinen {doms}. "
                    f"Technic-stenen staan op 24 LDU verticaal, liftarms op 20 — "
                    f"gaten lijnen niet uit."))
        if not out:
            out.append(Finding("OK", "rooster", "een roosterdomein, geen overgangen"))
        return out

    def check_closure(self) -> list[Finding]:
        """
        Sluiting van tandwiellussen. Drie assen die elkaar rondom aandrijven
        leggen drie hartafstanden vast. Die driehoek moet op het rooster
        sluiten, anders krijg je het derde tandwiel er niet in.
        """
        out = []
        spur = [l for l in self.links if isinstance(l, Mesh) and l.kind == "spur"]
        dist = {}
        adj: dict[str, set] = {}
        for m in spur:
            dist[frozenset((m.a, m.b))] = m.centre_distance_halfstuds
            adj.setdefault(m.a, set()).add(m.b)
            adj.setdefault(m.b, set()).add(m.a)

        for trio in itertools.combinations(adj, 3):
            a, b, c = trio
            keys = [frozenset((a, b)), frozenset((b, c)), frozenset((a, c))]
            if not all(k in dist for k in keys):
                continue
            d_ab, d_bc, d_ac = (dist[k] for k in keys)
            if d_ab + d_bc <= d_ac or d_ab + d_ac <= d_bc or d_bc + d_ac <= d_ab:
                out.append(Finding("FAIL", "lussluiting",
                                   f"{a}-{b}-{c}: hartafstanden {d_ab}/{d_bc}/{d_ac} "
                                   f"halve studs vormen geen driehoek"))
                continue
            # derde punt: A op (0,0), B op (d_ab,0)
            x = (d_ab**2 + d_ac**2 - d_bc**2) / (2 * d_ab)
            y2 = d_ac**2 - x**2
            ok_x = abs(x - round(x)) < 1e-9
            y = math.sqrt(max(y2, 0.0))
            ok_y = abs(y - round(y)) < 1e-9
            if ok_x and ok_y:
                out.append(Finding("OK", "lussluiting",
                                   f"{a}-{b}-{c} sluit op het rooster: "
                                   f"derde as op ({x:.0f}, {y:.0f}) halve studs"))
            else:
                out.append(Finding(
                    "FAIL", "lussluiting",
                    f"{a}-{b}-{c} sluit NIET op het rooster: derde as zou op "
                    f"({x:.3f}, {y:.3f}) halve studs komen. Kies een ander "
                    f"tandaantal of voeg een tussenwiel toe."))
        if not out:
            out.append(Finding("OK", "lussluiting", "geen tandwiellussen aanwezig"))
        return out

    def backlash(self, path: list[str]) -> float:
        """Opgetelde dode gang langs een pad van assen, in graden aan de uitgang."""
        total, ratio = 0.0, 1.0
        for a, b in zip(path, path[1:]):
            m = next((l for l in self.links if isinstance(l, Mesh)
                      and {l.a, l.b} == {a, b}), None)
            if m is None:
                continue
            total += m.backlash_deg * ratio
            step = (m.teeth_a / m.teeth_b) if m.a == a else (m.teeth_b / m.teeth_a)
            ratio *= step
        return total

    def run_checks(self) -> list[Finding]:
        return (self.check_dof() + self.check_bearings()
                + self.check_domains() + self.check_closure())

    def report(self):
        print("=" * 70)
        print(f"  {self.name}")
        print("=" * 70)
        order = {"FAIL": 0, "WARN": 1, "OK": 2}
        for f in sorted(self.run_checks(), key=lambda f: order[f.level]):
            print(f"  {f.level:5s} [{f.check:14s}] {f.detail}")
        sol = self.solve()
        print("\n  Snelheden:")
        if sol is None:
            print("    niet oplosbaar met de huidige aandrijvingen")
        else:
            for s, w in sol.items():
                mark = "  <- aandrijving" if s in self.inputs else (
                    "  <- uitgang" if s in self.outputs else "")
                print(f"    {s:16s} {w:+8.3f}{mark}")
