# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Starheidscontrole.

Een constructie die alle assen draagt is nog geen constructie. Twee balken die
elkaar nergens raken hangen los in de lucht, en twee balken met een enkele pin
ertussen zijn een scharnier. Dat is precies de fout die je pas ontdekt als het
ding in je handen slap gaat hangen.

De toets is de beweeglijkheidsformule. Voor pinverbindingen met evenwijdige
assen geldt de vlakke vorm:

    M = 3(n-1) - 2j

met n onderdelen en j pinverbindingen. M > 0 betekent scharnieren. Staan de
pinassen niet allemaal evenwijdig, dan de ruimtelijke vorm M = 6(n-1) - 5j.

Let op waarom de vlakke vorm nodig is: ruimtelijk gerekend zou een vierkant van
vier balken star lijken, terwijl dat in werkelijkheid een vierstangenmechanisme
is met een vrijheidsgraad. Dat is de klassieke Gruebler-paradox, en precies het
geval dat je in LEGO voortdurend tegenkomt.
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
    """Samenvallende gaten met evenwijdige as: daar kan een pin doorheen."""
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
                continue                       # gaten staan niet in lijn
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
        return 0, "enkel onderdeel"
    axes = {jt.axis for jt in joints}
    j = len(joints)
    if len(axes) <= 1:
        return 3 * (n_parts - 1) - 2 * j, "vlak"
    return 6 * (n_parts - 1) - 5 * j, "ruimtelijk"


def analyse(parts: list[Placed], inventory=BEAMS) -> list:
    from .mech import Finding

    out = []
    joints = find_joints(parts, inventory)
    comps = components(len(parts), joints)

    if len(comps) > 1:
        loose = [len(c) for c in comps]
        out.append(Finding(
            "FAIL", "samenhang",
            f"de constructie valt uiteen in {len(comps)} losse stukken "
            f"(groottes {loose}). Onderdelen die nergens aan vastzitten dragen niets."))
        for c in comps:
            if len(c) == 1:
                p = parts[c[0]]
                out.append(Finding("FAIL", "samenhang",
                                   f"  {p.part} op {p.origin} zweeft los"))
        return out

    m, kind = mobility(len(parts), joints)
    if m > 0:
        out.append(Finding(
            "FAIL", "starheid",
            f"{len(parts)} onderdelen, {len(joints)} pinverbindingen, "
            f"beweeglijkheid M = {m} ({kind}). De constructie scharniert. "
            f"Voeg {m} verbinding(en) toe, of driehoek hem met een 3-4-5."))
    else:
        out.append(Finding(
            "OK", "starheid",
            f"{len(parts)} onderdelen, {len(joints)} pinverbindingen, M = {m} "
            f"({kind}): star{' (overbepaald, in LEGO normaal)' if m < 0 else ''}"))
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
