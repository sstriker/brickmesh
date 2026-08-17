# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
The functional layer: shafts and transmissions, independent of any part.

This is where you work a mechanism out before placing anything. Ratios,
directions of rotation, degrees of freedom, torque, and the geometric
requirements that follow from them. This is the layer where an idea dies or
survives, and you want to know which before you start building.

The core: every transmission is one linear equation between shaft speeds. The
whole mechanism is therefore a matrix, and its null space is your degrees of
freedom. A subtractor should have two (drive and steer); if only one comes out,
your train is locked.
"""
from __future__ import annotations

import itertools
import math
from dataclasses import dataclass

import numpy as np

HALF_STUD = 10.0          # LDU. A center distance lands on a whole half stud
                          # when the two tooth counts SUM to a multiple of 8;
                          # 8t+12t and 36t+40t are the pairs that do not.


# --------------------------------------------------------------------------
# elements
# --------------------------------------------------------------------------

@dataclass
class Shaft:
    id: str
    bearings: int = 0                  # number of bearing points
    domain: str = "technic-studless"
    note: str = ""


@dataclass
class Mesh:
    """A gear pair. Externally meshing, so the direction of rotation reverses."""
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
    def center_distance_halfstuds(self) -> float | None:
        """Only meaningful for parallel shafts."""
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
    Three ports. The case speed is the average of both outputs:
        2*w_case - w_1 - w_2 = 0
    This is the only transmission with more than two ports, and that is exactly
    why a subtractor can exist.
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
# mechanism
# --------------------------------------------------------------------------

class Mechanism:
    def __init__(self, name: str):
        self.name = name
        self.shafts: dict[str, Shaft] = {}
        self.links: list = []
        self.inputs: dict[str, float] = {}
        self.outputs: list[str] = []

    # ---- construction ----

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

    # ---- kinematics ----

    @property
    def _index(self) -> dict:
        return {s: i for i, s in enumerate(self.shafts)}

    def _matrix(self) -> np.ndarray:
        idx, n = self._index, len(self.shafts)
        if not self.links:
            return np.zeros((0, n))
        return np.vstack([l.equation(idx, n) for l in self.links])

    def dof(self) -> int:
        """Degrees of freedom = number of shafts minus the rank of the constraints."""
        A = self._matrix()
        rank = np.linalg.matrix_rank(A) if len(A) else 0
        return len(self.shafts) - int(rank)

    def solve(self) -> dict[str, float] | None:
        """Speeds of all shafts, given the driven shafts."""
        idx, n = self._index, len(self.shafts)
        A = list(self._matrix())
        rhs = [0.0] * len(A)
        for sid, w in self.inputs.items():
            row = np.zeros(n); row[idx[sid]] = 1.0
            A.append(row); rhs.append(w)
        A = np.array(A); rhs = np.array(rhs)
        if np.linalg.matrix_rank(A) < n:
            return None                      # underdetermined
        sol, *_ = np.linalg.lstsq(A, rhs, rcond=None)
        if not np.allclose(A @ sol, rhs, atol=1e-8):
            return None                      # inconsistent
        return {s: float(sol[idx[s]]) for s in self.shafts}

    # ---- checks ----

    def check_dof(self) -> list[Finding]:
        d, k = self.dof(), len(self.inputs)
        if d == 0:
            return [Finding("FAIL", "dof",
                            "mechanism has 0 degrees of freedom: the train is locked "
                            "and cannot turn")]
        if k < d:
            return [Finding("WARN", "dof",
                            f"{d} degrees of freedom but {k} driven shafts — "
                            f"{d-k} motion(s) remain undetermined")]
        if k > d:
            return [Finding("FAIL", "dof",
                            f"{k} drives on {d} degrees of freedom — overdetermined, "
                            f"the motors work against each other")]
        return [Finding("OK", "dof",
                        f"{d} degrees of freedom, {k} drives: determined")]

    def check_bearings(self) -> list[Finding]:
        out = []
        for s in self.shafts.values():
            if s.bearings < 2:
                out.append(Finding("FAIL", "bearings",
                                   f"shaft '{s.id}' has {s.bearings} bearing point(s). "
                                   f"Fewer than two means it whips under load."))
        if not out:
            out.append(Finding("OK", "bearings", "every shaft borne at both ends"))
        return out

    def check_domains(self) -> list[Finding]:
        out = []
        for l in self.links:
            ids = ([l.a, l.b] if isinstance(l, Mesh) else [l.case, l.out_a, l.out_b])
            doms = {self.shafts[i].domain for i in ids if i in self.shafts}
            if len(doms) > 1:
                out.append(Finding(
                    "FAIL", "grid",
                    f"transmission between {ids} crosses grid domains {doms}. "
                    f"Technic bricks sit at 24 LDU vertically, liftarms at 20 — "
                    f"the holes do not line up."))
        if not out:
            out.append(Finding("OK", "grid", "one grid domain, no transitions"))
        return out

    def check_closure(self) -> list[Finding]:
        """
        Closure of gear loops. Three shafts driving each other in a ring fix
        three center distances. That triangle has to close on the lattice, or
        you will not get the third gear in.
        """
        out = []
        spur = [l for l in self.links if isinstance(l, Mesh) and l.kind == "spur"]
        dist = {}
        adj: dict[str, set] = {}
        for m in spur:
            dist[frozenset((m.a, m.b))] = m.center_distance_halfstuds
            adj.setdefault(m.a, set()).add(m.b)
            adj.setdefault(m.b, set()).add(m.a)

        for trio in itertools.combinations(adj, 3):
            a, b, c = trio
            keys = [frozenset((a, b)), frozenset((b, c)), frozenset((a, c))]
            if not all(k in dist for k in keys):
                continue
            d_ab, d_bc, d_ac = (dist[k] for k in keys)
            if d_ab + d_bc <= d_ac or d_ab + d_ac <= d_bc or d_bc + d_ac <= d_ab:
                out.append(Finding("FAIL", "loop closure",
                                   f"{a}-{b}-{c}: center distances {d_ab}/{d_bc}/{d_ac} "
                                   f"half studs do not form a triangle"))
                continue
            # third point: A at (0,0), B at (d_ab,0)
            x = (d_ab**2 + d_ac**2 - d_bc**2) / (2 * d_ab)
            y2 = d_ac**2 - x**2
            ok_x = abs(x - round(x)) < 1e-9
            y = math.sqrt(max(y2, 0.0))
            ok_y = abs(y - round(y)) < 1e-9
            if ok_x and ok_y:
                out.append(Finding("OK", "loop closure",
                                   f"{a}-{b}-{c} closes on the lattice: "
                                   f"third shaft at ({x:.0f}, {y:.0f}) half studs"))
            else:
                out.append(Finding(
                    "FAIL", "loop closure",
                    f"{a}-{b}-{c} does NOT close on the lattice: the third shaft "
                    f"would land at ({x:.3f}, {y:.3f}) half studs. Pick a different "
                    f"tooth count or add an idler."))
        if not out:
            out.append(Finding("OK", "loop closure", "no gear loops present"))
        return out

    def backlash(self, path: list[str]) -> float:
        """Accumulated backlash along a path of shafts, in degrees at the output."""
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
        print("\n  Speeds:")
        if sol is None:
            print("    not solvable with the current drives")
        else:
            for s, w in sol.items():
                mark = "  <- drive" if s in self.inputs else (
                    "  <- output" if s in self.outputs else "")
                print(f"    {s:16s} {w:+8.3f}{mark}")
