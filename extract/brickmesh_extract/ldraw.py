# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
LDraw part fetching and geometry resolution.

Coordinate system reminder (LDraw):
  1 stud        = 20 LDU  (X and Z)
  1 brick       = 24 LDU  (Y)
  1 plate       =  8 LDU  (Y)
  +Y points DOWN.
"""
from __future__ import annotations

import json
import os
import urllib.request
from dataclasses import dataclass

import numpy as np

MIRROR = "https://raw.githubusercontent.com/mpetrov/ldraw-parts/master"
SEARCH_DIRS = ["parts", "p", "parts/s", "p/48", "p/8"]
CACHE = os.path.expanduser("~/.cache/brickcheck")

LDU_STUD = 20.0
LDU_BRICK = 24.0
LDU_PLATE = 8.0


class PartNotFound(Exception):
    pass


def _cache_path(name: str) -> str:
    return os.path.join(CACHE, name.replace("/", "__").replace("\\", "__"))


def fetch(name: str) -> str:
    """Fetch a .dat by name, trying each LDraw search directory. Cached on disk."""
    name = name.replace("\\", "/").lower()
    cp = _cache_path(name)
    if os.path.exists(cp):
        with open(cp, encoding="utf-8", errors="replace") as fh:
            return fh.read()

    os.makedirs(CACHE, exist_ok=True)
    last = None
    for d in SEARCH_DIRS:
        url = f"{MIRROR}/{d}/{name}"
        try:
            with urllib.request.urlopen(url, timeout=30) as r:
                if r.status == 200:
                    text = r.read().decode("utf-8", errors="replace")
                    with open(cp, "w", encoding="utf-8") as fh:
                        fh.write(text)
                    return text
        except Exception as exc:  # 404s land here too
            last = exc
    raise PartNotFound(f"{name} not found in mirror ({last})")


def _line_matrix(tok: list[str]) -> tuple[np.ndarray, np.ndarray]:
    v = [float(t) for t in tok[2:14]]
    trans = np.array(v[0:3], dtype=float)
    rot = np.array(v[3:12], dtype=float).reshape(3, 3)
    return rot, trans


def _resolve(name: str, _depth: int = 0):
    """
    Recursively expand a part into a cloud of vertices in the part's own
    coordinate frame. Used for bounding boxes and collision tests.

    NOTE: repeated references to the same subfile must NOT be deduplicated.
    A gear is one tooth primitive instantiated N times at N matrices; dropping
    the repeats collapses the part into a sliver.
    """
    if _depth > 24:
        return np.zeros((0, 3)), []

    text = fetch(name)
    pts: list[np.ndarray] = []
    tris: list[np.ndarray] = []

    for raw in text.splitlines():
        tok = raw.strip().split()
        if not tok:
            continue
        try:
            kind = int(tok[0])
        except ValueError:
            continue

        if kind == 1 and len(tok) >= 15:
            sub = " ".join(tok[14:]).lower()
            rot, trans = _line_matrix(tok)
            try:
                child, ctris = _resolve(sub, _depth + 1)
            except PartNotFound:
                continue
            if len(child):
                pts.append(child @ rot.T + trans)
            for t in ctris:
                tris.append(t @ rot.T + trans)

        elif kind in (2, 3, 4):
            n = {2: 2, 3: 3, 4: 4}[kind]
            need = 2 + n * 3
            if len(tok) >= need:
                arr = np.array([float(x) for x in tok[2:need]], dtype=float)
                pts.append(arr.reshape(n, 3))
                if kind == 3:
                    tris.append(arr.reshape(3, 3))
                elif kind == 4:
                    q = arr.reshape(4, 3)
                    tris.append(q[[0, 1, 2]])
                    tris.append(q[[0, 2, 3]])

    if not pts:
        return np.zeros((0, 3)), []
    return np.vstack(pts), tris


def resolve_vertices(name: str) -> np.ndarray:
    return _resolve(name)[0]


@dataclass
class PartGeometry:
    name: str
    title: str
    verts: np.ndarray
    tris: list | None = None

    def mesh(self):
        """A real triangle mesh, for proper interference testing."""
        import trimesh
        if not self.tris:
            return None
        tri = np.array(self.tris)
        v = tri.reshape(-1, 3)
        f = np.arange(len(v)).reshape(-1, 3)
        m = trimesh.Trimesh(vertices=v, faces=f, process=True)
        return m

    @property
    def bbox(self) -> tuple[np.ndarray, np.ndarray]:
        return self.verts.min(axis=0), self.verts.max(axis=0)

    @property
    def size(self) -> np.ndarray:
        lo, hi = self.bbox
        return hi - lo

    @property
    def thin_axis(self) -> int:
        """
        Index of the shortest bbox dimension. For disc-shaped parts (gears,
        bushes) this is the rotation axis in the part's default orientation.
        Reported by the validator so it can be eyeballed, never trusted blindly.
        """
        return int(np.argmin(self.size))


_geo_cache: dict[str, PartGeometry] = {}


def geometry(name: str) -> PartGeometry:
    name = name.lower()
    if not name.endswith(".dat"):
        name += ".dat"
    if name in _geo_cache:
        return _geo_cache[name]

    text = fetch(name)
    title = ""
    for line in text.splitlines():
        s = line.strip()
        if s.startswith("0 ") and not s.startswith("0 !") and not s.startswith("0 Name:"):
            title = s[2:].strip()
            break

    verts, tris = _resolve(name)
    if len(verts) == 0:
        raise PartNotFound(f"{name} resolved to no geometry")

    g = PartGeometry(name=name, title=title, verts=verts, tris=tris)
    _geo_cache[name] = g
    return g


def cache_manifest() -> dict:
    if not os.path.isdir(CACHE):
        return {}
    return {"cached_files": len(os.listdir(CACHE)), "dir": CACHE}


if __name__ == "__main__":
    for p in ["32270", "62821", "3647", "3648b", "32269", "4716", "3705", "32316"]:
        try:
            g = geometry(p)
            lo, hi = g.bbox
            print(f"{p:8s} {g.title[:42]:44s} size={np.round(g.size,1)} thin_axis={'XYZ'[g.thin_axis]}")
        except PartNotFound as e:
            print(f"{p:8s} ERROR: {e}")
    print(json.dumps(cache_manifest()))
