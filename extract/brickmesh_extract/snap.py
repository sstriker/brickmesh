# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
LDCad shadow library reader.

The shadow library (Roland Melkert, CC BY-SA 4.0) annotates LDraw parts with
snap metadata: exactly where every axle hole and pin sits and which way it
points. That is authoritative data, so it replaces the bounding-box axis
heuristic in core.infer_native_axis, which could only ever guess.

What it does NOT contain is gear meshing. There is no gear meta in the whole
library, by design - see the LDraw forum discussion on the JBrickBuilder
connection model. So this module fixes axes and grids, not tooth engagement.
"""
from __future__ import annotations

import os
import re
import tarfile
import urllib.request
from dataclasses import dataclass

import numpy as np

SHADOW_URL = "https://codeload.github.com/RolandMelkert/LDCadShadowLibrary/tar.gz/refs/heads/main"
SHADOW_DIR = os.path.expanduser("~/.cache/brickmesh-shadow")

_META = re.compile(r"!LDCAD\s+(SNAP_\w+)\s*(.*)")
_ATTR = re.compile(r"\[(\w+)=([^\]]*)\]")


def ensure_library(dest: str | None = None) -> str:
    # Resolved at call time, not bound as a default: as a default argument
    # SHADOW_DIR is captured at import and pointing the extractor at another
    # directory silently has no effect.
    dest = dest or SHADOW_DIR
    root = os.path.join(dest, "LDCadShadowLibrary-main")
    if os.path.isdir(root):
        return root
    os.makedirs(dest, exist_ok=True)
    tmp = os.path.join(dest, "shadow.tgz")
    urllib.request.urlretrieve(SHADOW_URL, tmp)
    with tarfile.open(tmp) as tf:
        tf.extractall(dest)
    return root


@dataclass
class Snap:
    kind: str                 # SNAP_CYL / SNAP_INCL / ...
    gender: str               # M / F / ''
    pos: np.ndarray           # LDU, in part coordinates
    ori: np.ndarray           # 3x3
    group: str = ""           # named connection group, see is_generic
    secs: str = ""
    grid: str = ""
    ref: str = ""

    @property
    def axis(self) -> np.ndarray:
        """LDCad snap cylinders point along +Y before the ori matrix is applied."""
        return self.ori @ np.array([0.0, 1.0, 0.0])

    @property
    def is_generic(self) -> bool:
        """
        Snaps carrying a [group=...] belong to one specific connection: crane
        arm, door hinge, electrical plug, ball joint. Those must not be treated
        as an ordinary pin or an ordinary hole - it is what makes a liftarm
        appear to have a male port along its whole length, when that is really
        the slot for a crane-arm clamp.
        """
        return not self.group

    @property
    def is_axle(self) -> bool:
        return self.secs.strip().startswith("A")


def _parse_attrs(rest: str) -> dict:
    return {k: v.strip() for k, v in _ATTR.findall(rest)}


def snaps(part: str, root: str | None = None) -> list[Snap]:
    root = root or ensure_library()
    name = part if part.endswith(".dat") else part + ".dat"
    for sub in ("parts", "p"):
        path = os.path.join(root, sub, name)
        if os.path.exists(path):
            break
    else:
        return []

    out = []
    with open(path, encoding="utf-8", errors="replace") as fh:
        for line in fh:
            m = _META.search(line)
            if not m:
                continue
            kind, rest = m.group(1), m.group(2)
            a = _parse_attrs(rest)
            pos = np.array([float(x) for x in a.get("pos", "0 0 0").split()])
            o = a.get("ori")
            ori = (np.array([float(x) for x in o.split()]).reshape(3, 3)
                   if o else np.eye(3))
            out.append(Snap(kind=kind, gender=a.get("gender", ""), group=a.get("group", ""), pos=pos, ori=ori,
                            secs=a.get("secs", ""), grid=a.get("grid", ""),
                            ref=a.get("ref", "")))
    return out


def rotation_axis(part: str) -> tuple[np.ndarray, str] | None:
    """
    The part's true rotation axis, taken from its axle-hole snap rather than
    guessed from a bounding box. Returns (unit vector, provenance) or None.
    """
    for s in snaps(part):
        if s.kind == "SNAP_CYL" and s.gender == "F" and s.is_axle:
            return s.axis, "LDCad shadow library, axle hole"
    for s in snaps(part):
        if s.kind == "SNAP_CYL" and s.gender == "F":
            return s.axis, "LDCad shadow library, round hole"
    # Many parts define their hole indirectly, through an include pointing at a
    # generic hole definition. Those are always female.
    for s in snaps(part):
        if s.kind == "SNAP_INCL" and ("hole" in s.ref or "conn" in s.ref):
            return s.axis, f"LDCad shadow library, include {s.ref}"
    return None


def mounting_points(part: str) -> list[tuple[np.ndarray, np.ndarray]]:
    """All female snap positions and axes: the real grid anchors of a part."""
    return [(s.pos, s.axis) for s in snaps(part)
            if s.kind == "SNAP_CYL" and s.gender == "F"]


if __name__ == "__main__":
    root = ensure_library()
    print(f"shadow library: {root}\n")
    for p in ["62821", "3648b", "32269", "32525", "64179", "32270", "3647"]:
        sn = snaps(p, root)
        ax = rotation_axis(p)
        if not sn:
            print(f"  {p:8s} NO shadow file - fall back on the heuristic")
            continue
        axtxt = f"axis {np.round(ax[0],2)}" if ax else "no axis info"
        print(f"  {p:8s} {len(sn):2d} snaps   {axtxt}")
        for s in sn:
            print(f"           {s.kind:11s} {s.gender or '-'} pos={np.round(s.pos,0)} "
                  f"secs={s.secs[:22]:24s} grid={s.grid}")
