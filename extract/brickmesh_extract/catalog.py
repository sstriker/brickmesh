# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Parts catalog for structural search.

The shadow library describes, per part, where the holes and pins sit, but
compresses repeating holes into a grid notation. A search that has to build
structures needs individual coordinates, so we expand them.

Grid notation: [grid=<countA> <countB> <stepA> <stepB>], where a count preceded
by C means centered rather than counting up from zero.
"""
from __future__ import annotations

import json
import os
from dataclasses import asdict, dataclass

import numpy as np

from . import snap

CACHE = os.path.expanduser("~/.cache/brickcheck-catalog.json")


def parse_grid(spec: str) -> tuple[int, int, float, float, bool, bool]:
    """'C 3 1 20 0' -> (3, 1, 20, 0, True, False)"""
    if not spec:
        return 1, 1, 0.0, 0.0, False, False
    tok = spec.split()
    i = 0
    counts, centered = [], []
    for _ in range(2):
        if tok[i].upper() == "C":
            centered.append(True); i += 1
        else:
            centered.append(False)
        counts.append(int(tok[i])); i += 1
    sa, sb = float(tok[i]), float(tok[i + 1])
    return counts[0], counts[1], sa, sb, centered[0], centered[1]


def _offsets(count: int, spacing: float, centered: bool) -> np.ndarray:
    if count <= 1:
        return np.array([0.0])
    idx = np.arange(count, dtype=float)
    if centered:
        idx -= (count - 1) / 2.0
    return idx * spacing


def expand(s: snap.Snap) -> list[np.ndarray]:
    """All concrete positions of a snap, with the grid expanded."""
    ca, cb, sa, sb, cca, ccb = parse_grid(s.grid)
    # The grid lies in the snap's LOCAL frame, not in some arbitrary
    # perpendicular basis. The cylinder points along Y locally, so the grid
    # directions are local X and Z, carried over by the orientation matrix.
    u = s.ori @ np.array([1.0, 0.0, 0.0])
    v = s.ori @ np.array([0.0, 0.0, 1.0])
    out = []
    for da in _offsets(ca, sa, cca):
        for db in _offsets(cb, sb, ccb):
            out.append(s.pos + da * u + db * v)
    return out


@dataclass
class CatalogEntry:
    part: str
    holes: list          # [[x,y,z,ax,ay,az,cross], ...] female
    pins: list           # [[x,y,z,ax,ay,az,cross], ...] male
    axle_holes: int      # how many of those are cross-shaped
    title: str = ""

    @property
    def axes(self) -> int:
        """How many different hole directions. >1 = perpendicular connector."""
        return len({tuple(np.round(np.abs(h[3:6]), 2)) for h in self.holes})

    @property
    def n_holes(self) -> int:
        return len(self.holes)


def entry(part: str) -> CatalogEntry | None:
    sn = snap.snaps(part)
    if not sn:
        return None
    holes, pins, naxle = [], [], 0
    for s in sn:
        if s.kind not in ("SNAP_CYL", "SNAP_INCL"):
            continue
        if not s.is_generic:
            continue          # crane-arm slot, door hinge, plug: not a pin
        female = (s.gender != "M")
        for p in expand(s):
            rec = [*np.round(p, 2), *np.round(s.axis, 3), 1 if s.is_axle else 0]
            if female:
                holes.append(rec)
                if s.is_axle:
                    naxle += 1
            else:
                pins.append(rec)
    return CatalogEntry(part=part, holes=holes, pins=pins, axle_holes=naxle)


# Order matters: the first matching pattern wins, so specific before general.
FAMILIES = [
    ("liftarm",         r"^Technic (Beam|Liftarm)"),
    ("angle connector", r"Angle Connector|Axle and Pin Connector|"
                        r"Connector Perpendicular|Joiner Perpendicular"),
    ("axle coupler",    r"Axle Joiner|Axle Connector|Axle Extender|Axle Coupling"),
    ("pin connector",   r"Pin Connector|Pin Joiner|Pin Hole Connector|"
                        r"^Technic Pin\b"),
    ("axle with hole",  r"Axle .*with .*(Hole|Pin)|Axlehole"),
    ("plate with hole", r"^Plate .*(Hole|Pin|Axle)|^Technic Plate"),
    ("brick with hole", r"^Technic Brick|^Brick .*(Hole|Pin|Axle)"),
    ("panel",           r"^Technic Panel|^Panel"),
    ("bush",            r"Technic Bush|Bush "),
]


def titles(parts_dir: str) -> dict:
    """Titles from the LDraw library, so families can be filtered on."""
    out = {}
    for fn in os.listdir(parts_dir):
        if not fn.endswith(".dat"):
            continue
        try:
            with open(os.path.join(parts_dir, fn), encoding="utf-8",
                      errors="replace") as fh:
                out[fn[:-4]] = fh.readline().strip().lstrip("0 ").strip()
        except OSError:
            pass
    return out


def by_family(cat: dict, parts_dir: str) -> dict:
    """Group the catalog by part family."""
    import re
    tt = titles(parts_dir)
    groups = {name: [] for name, _ in FAMILIES}
    groups["other"] = []
    for pid, e in cat.items():
        t = tt.get(pid, "")
        for name, pat in FAMILIES:
            if re.search(pat, t):
                groups[name].append((pid, t, len(e["holes"]), len(e["pins"])))
                break
        else:
            groups["other"].append((pid, t, len(e["holes"]), len(e["pins"])))
    return groups


def build(limit: int | None = None) -> dict:
    root = snap.ensure_library()
    names = sorted(f[:-4] for f in os.listdir(os.path.join(root, "parts"))
                   if f.endswith(".dat"))
    if limit:
        names = names[:limit]
    cat = {}
    for n in names:
        try:
            e = entry(n)
        except Exception:
            continue
        if e and (e.holes or e.pins):
            cat[n] = asdict(e)
    return cat


def load_or_build() -> dict:
    if os.path.exists(CACHE):
        with open(CACHE) as fh:
            return json.load(fh)
    cat = build()
    with open(CACHE, "w") as fh:
        json.dump(cat, fh)
    return cat


if __name__ == "__main__":
    # first validate against what the probe physically found
    print("Validation of the grid expansion against the probe measurement (64179):\n")
    e = entry("64179")
    byax = {}
    for h in e.holes:
        key = tuple(np.round(h[3:6], 1))
        byax.setdefault(key, []).append(h[:3])
    for ax, ps in sorted(byax.items()):
        ps = sorted(ps)
        print(f"  axis {ax}: {len(ps)} holes")
        for p in ps:
            print(f"      {np.array(p)}")
    print("\n  probe found earlier: axis X -> Z in {-40, 0, +40}; axis Z -> X in {-20, 0, +20}\n")

    cat = load_or_build()
    print(f"Catalog: {len(cat)} parts with connection geometry")
    tot = sum(len(v["holes"]) for v in cat.values())
    print(f"  {tot} holes in total")
    big = sorted(cat.items(), key=lambda kv: -len(kv[1]["holes"]))[:8]
    print("\n  parts with the most holes:")
    for k, v in big:
        print(f"    {k:10s} {len(v['holes']):4d} holes, {len(v['pins']):3d} pins")


# --------------------------------------------------------------------------
# usability and cost
# --------------------------------------------------------------------------

def usable(cat: dict, parts_dir: str) -> dict:
    """
    Filter out everything that is not a real orderable part.

    LDraw has subparts with a ~ before the title: the front of a motor housing,
    the body of a horse. Those exist only as a building block inside another
    file and cannot be ordered or used on their own. Leave them in and the
    search will invent structures out of parts that do not exist.
    """
    tt = titles(parts_dir)
    out = {}
    for pid, e in cat.items():
        t = tt.get(pid, "")
        if t.startswith("~") or t.startswith("=") or t.lower().startswith("moved"):
            continue
        if "obsolete" in t.lower():
            continue
        e = dict(e); e["title"] = t
        out[pid] = e
    return out


# Preference tiers. The search starts at tier 1 and widens only when it fails
# there. That way you pay for the larger inventory only when you need it,
# instead of on every single query.
TIERS = {
    1: r"^Technic (Beam|Liftarm)|^Technic Pin\b|Technic Pin with Friction|"
       r"Technic Axle Joiner|Angle Connector|Technic Bush|^Technic Axle\b",
    2: r"^Technic ",
    3: r".",
}


def tier_of(title: str) -> int:
    import re
    for t in (1, 2, 3):
        if re.search(TIERS[t], title):
            return t
    return 3


def cost(entry: dict, owned: set | None = None) -> float:
    """
    What it 'costs' to use a part.

    If you own it, it is cheap. If you do not, it has to be ordered and that
    counts. On top of that a preference for common Technic parts: that is the
    bias you are after - the search may reach for something exotic, but it has
    to have a reason to.
    """
    pid = entry.get("part") or entry.get("id", "")
    t = tier_of(entry.get("title", ""))
    base = {1: 1.0, 2: 2.5, 3: 8.0}[t]
    if owned is not None and pid in owned:
        base *= 0.4
    return base


def inventory_by_tier(cat: dict, tier: int) -> dict:
    return {p: e for p, e in cat.items() if tier_of(e.get("title", "")) <= tier}


def infer_missing_holes(cat: dict) -> dict:
    """
    Fill in the row of holes where the shadow library names only one.

    For a liftarm the shadow data often describes only the end hole and leaves
    the rest to the geometry of the part itself. For a search that is useless:
    two liftarms side by side then appear to have nowhere to couple. The
    missing holes lie at the known pitch of 20 LDU along the length, centered
    on the origin.

    Apply this only where the count is clearly wrong; parts with a complete
    grid in the shadow data are left alone.
    """
    from . import ldraw
    out = {}
    for pid, e in cat.items():
        e = dict(e)
        holes = list(e["holes"])
        try:
            g = ldraw.geometry(pid)
        except Exception:
            out[pid] = e
            continue
        size = g.size
        long_axis = int(np.argmax(size))
        length = float(size[long_axis])
        expect = int(round(length / 20.0))
        if expect >= 3 and len(holes) < expect and len(holes) >= 1:
            base = holes[0]
            axis = np.array(base[3:6], float)
            # the length direction must not be the hole axis
            if abs(axis[long_axis]) < 0.5:
                d = np.zeros(3); d[long_axis] = 1.0
                k = np.arange(expect) - (expect - 1) / 2.0
                new = []
                for t in k * 20.0:
                    p = np.array(base[:3], float)
                    p[long_axis] = t
                    new.append([*np.round(p, 2), *base[3:6], base[6]])
                holes = new
                e["inferred_holes"] = True
        e["holes"] = holes
        out[pid] = e
    return out
