# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Rigidity check.

A structure that bears every axle is not yet a structure. Two beams that touch
nowhere hang loose in the air, and two beams with a single pin between them are
a hinge. That is exactly the mistake you only discover once the thing goes limp
in your hands.

The test is the mobility formula. For pin joints with parallel axes the planar
form applies:

    M = 3(n-1) - 2j

with n parts and j pin joints. M > 0 means it hinges. If the pin axes are not
all parallel, the spatial form M = 6(n-1) - 5j applies instead.

Note why the planar form is needed: computed spatially, a square of four beams
would look rigid, while it is really a four-bar linkage with one degree of
freedom. That is the classic Gruebler paradox, and exactly the case you run
into constantly in LEGO.
"""
from __future__ import annotations

from dataclasses import dataclass

import numpy as np

from .voxel import ROTATIONS
from .synth import Placed, hole_offsets, _local_hole_axis, BEAMS

TOL = 1e-6


@dataclass
class Joint:
    a: int
    b: int
    point: tuple
    axis: tuple


def world_holes(p: Placed, n_holes: int):
    R = ROTATIONS[p.rot]
    pts = hole_offsets(n_holes) @ R.T + np.array(p.origin)
    axis = R @ _local_hole_axis(p.part)
    return pts, axis / np.linalg.norm(axis)


def find_joints(parts: list[Placed], inventory=BEAMS) -> list[Joint]:
    """Coincident holes with parallel axes: a pin can go through there."""
    nholes = dict(inventory)
    data = []
    for p in parts:
        pts, ax = world_holes(p, nholes[p.part])
        data.append((pts, ax))

    joints = []
    for i in range(len(parts)):
        for j in range(i + 1, len(parts)):
            pi, ai = data[i]
            pj, aj = data[j]
            if abs(abs(float(np.dot(ai, aj))) - 1.0) > 1e-6:
                continue                       # holes do not line up
            for a in pi:
                for b in pj:
                    if np.linalg.norm(a - b) < TOL:
                        joints.append(Joint(i, j, tuple(np.round(a, 3)),
                                            tuple(np.round(np.abs(ai), 3))))
    return joints


def components(n: int, joints: list[Joint]) -> list[list[int]]:
    parent = list(range(n))

    def find(x):
        while parent[x] != x:
            parent[x] = parent[parent[x]]; x = parent[x]
        return x

    for jt in joints:
        parent[find(jt.a)] = find(jt.b)
    groups: dict[int, list[int]] = {}
    for i in range(n):
        groups.setdefault(find(i), []).append(i)
    return list(groups.values())


def mobility(n_parts: int, joints: list[Joint]) -> tuple[int, str]:
    if n_parts <= 1:
        return 0, "single part"
    axes = {jt.axis for jt in joints}
    j = len(joints)
    if len(axes) <= 1:
        return 3 * (n_parts - 1) - 2 * j, "planar"
    return 6 * (n_parts - 1) - 5 * j, "spatial"


def analyze(parts: list[Placed], inventory=BEAMS) -> list:
    from .mech import Finding

    out = []
    joints = find_joints(parts, inventory)
    comps = components(len(parts), joints)

    if len(comps) > 1:
        loose = [len(c) for c in comps]
        out.append(Finding(
            "FAIL", "connectivity",
            f"the structure falls apart into {len(comps)} separate pieces "
            f"(sizes {loose}). Parts attached to nothing carry nothing."))
        for c in comps:
            if len(c) == 1:
                p = parts[c[0]]
                out.append(Finding("FAIL", "connectivity",
                                   f"  {p.part} at {p.origin} floats free"))
        return out

    m, kind = mobility(len(parts), joints)
    if m > 0:
        out.append(Finding(
            "FAIL", "rigidity",
            f"{len(parts)} parts, {len(joints)} pin joints, "
            f"mobility M = {m} ({kind}). The structure hinges. "
            f"Add {m} joint(s), or triangulate it with a 3-4-5."))
    else:
        out.append(Finding(
            "OK", "rigidity",
            f"{len(parts)} parts, {len(joints)} pin joints, M = {m} "
            f"({kind}): rigid{' (overconstrained, normal in LEGO)' if m < 0 else ''}"))
    return out


def joint_summary(parts: list[Placed], inventory=BEAMS) -> dict:
    joints = find_joints(parts, inventory)
    per_pair: dict[tuple, int] = {}
    for jt in joints:
        per_pair[(jt.a, jt.b)] = per_pair.get((jt.a, jt.b), 0) + 1
    return {
        "joints": len(joints),
        "pairs": len(per_pair),
        "hinges": [k for k, v in per_pair.items() if v == 1],
        "rigid_pairs": [k for k, v in per_pair.items() if v >= 2],
    }
