# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Echte doorsnijdingstest op driehoekniveau.

De eerdere aanpak mat afstanden tussen losse hoekpunten van driehoeken. Die
wolk is te dun: twee tandwielen die langs elkaar schuren en twee die netjes
kammen gaven allebei "afstand ~0". Dit is de vervanging: FCL doet een echte
driehoek-tegen-driehoek test, precies de klasse toets die Stud.io ook doet.

De regel voor een correcte tandwielstand:
  - bij MINSTENS EEN tandstand mag er geen doorsnijding zijn (dat is de speling)
  - bij een halve steek verdraaid MOET er wel doorsnijding zijn (de tanden
    grijpen dan werkelijk in elkaar in plaats van er langs te lopen)
Voldoet een positie aan het eerste maar niet aan het tweede, dan schuurt hij
er alleen maar langs.
"""
from __future__ import annotations

import numpy as np
import trimesh

from . import ldraw
from .core import rot


def _mesh(part: str) -> trimesh.Trimesh:
    m = ldraw.geometry(part).mesh()
    if m is None:
        raise ValueError(f"{part}: geen mesh")
    return m


def _transform(orient: np.ndarray, pos: np.ndarray) -> np.ndarray:
    T = np.eye(4)
    T[:3, :3] = orient
    T[:3, 3] = pos
    return T


def engagement(part_a: str, orient_a, pos_a,
               part_b: str, orient_b, pos_b,
               phase_axis: str = "x", phase_steps: int = 24,
               pitch_deg: float = 30.0) -> dict:
    """
    Draai onderdeel B over een volledige tandsteek en kijk per stand of de
    meshes elkaar doorsnijden.
    """
    ma, mb = _mesh(part_a), _mesh(part_b)
    ma = ma.copy(); ma.apply_transform(_transform(np.asarray(orient_a), np.asarray(pos_a)))

    mgr = trimesh.collision.CollisionManager()
    mgr.add_object("a", ma)

    hits, dists = [], []
    for k in range(phase_steps):
        ang = k * pitch_deg / phase_steps
        o = np.asarray(orient_b) @ rot(phase_axis, ang)
        m = mb.copy(); m.apply_transform(_transform(o, np.asarray(pos_b)))
        hits.append(bool(mgr.in_collision_single(m)))
        dists.append(float(mgr.min_distance_single(m)))

    n_free = sum(1 for h in hits if not h)
    return {
        "phases": phase_steps,
        "free_phases": n_free,
        "colliding_phases": phase_steps - n_free,
        "min_distance": min(dists),
        "min_distance_when_free": min([d for d, h in zip(dists, hits) if not h], default=None),
        "verdict": _verdict(n_free, phase_steps, min(dists)),
    }


def _verdict(n_free: int, total: int, min_dist: float) -> str:
    if n_free == 0:
        return "KLEMT - doorsnijding bij elke tandstand"
    if n_free == total:
        if min_dist > 2.0:
            return "GEEN CONTACT - tandwielen raken elkaar niet"
        return "SCHUURT - vrij bij elke stand, dus tanden grijpen niet in"
    return "KAMT - vrij bij sommige standen, klemt bij andere"


if __name__ == "__main__":
    Z = np.eye(3)
    RY = rot("y", 90)
    RING = 23.5

    print("12t tegen het 28t kroonwiel van het differentieel")
    print("Diff staat in de oorsprong, as langs Z, tandkrans op Z=+20..+27\n")
    print(f"  {'radiaal':>8}{'axiaal':>8}  {'vrij':>6}{'klem':>6}{'minafst':>9}   oordeel")

    for off in (30.0, 35.0, 40.0):
        for z in (5.0, 8.0, 12.0, 38.0, 42.0, 46.0):
            try:
                r = engagement("62821.dat", Z, [0, 0, 0],
                               "32270.dat", RY, [off, 0, z],
                               phase_axis="x", pitch_deg=30.0, phase_steps=18)
            except Exception as exc:
                print(f"  {off:8.1f}{z:8.1f}  fout: {exc}")
                continue
            print(f"  {off:8.1f}{z:8.1f}  {r['free_phases']:6d}"
                  f"{r['colliding_phases']:6d}{r['min_distance']:9.2f}   {r['verdict']}")


def mesh_lock_robust(part_a: str, orient_a, pos_a,
                     part_b: str, orient_b, pos_b,
                     teeth_b: int, teeth_a: int,
                     spin_axis: str = "z", steps: int = 72) -> dict:
    """
    mesh_lock, maar bestand tegen de open-schil-fout.

    LDraw-onderdelen zijn geen gesloten volumes (het differentieel heeft 835
    randlussen), en FCL geeft bij zo'n schil af en toe een verkeerd antwoord
    afhankelijk van de orientatie. Een tandkrans is echter symmetrisch onder
    rotatie over een heel aantal tanden, dus dezelfde meting bij vier standen
    van A moet hetzelfde opleveren. Wijkt er een af, dan is dat de artefact.
    """
    from collections import Counter
    results = []
    for k in range(4):
        ang = k * 360.0 / 4
        if (teeth_a * k) % 4 != 0 and k not in (0, 2):
            pass                       # rotatie hoeft geen tandveelvoud te zijn
        o = np.asarray(orient_a) @ rot("z", ang)
        r = mesh_lock(part_a, o, pos_a, part_b, orient_b, pos_b,
                      teeth_b, spin_axis=spin_axis, steps=steps)
        results.append(r)
    counts = Counter(r["windows"] for r in results)
    modal, n = counts.most_common(1)[0]
    winner = next(r for r in results if r["windows"] == modal)
    winner = dict(winner)
    winner["agreement"] = f"{n}/4"
    winner["outliers"] = [r["windows"] for r in results if r["windows"] != modal]
    if n < 3:
        winner["verdict"] = f"ONBETROUWBAAR - metingen oneens {sorted(counts)}"
    return winner


def mesh_lock(part_a: str, orient_a, pos_a,
              part_b: str, orient_b, pos_b,
              teeth_b: int, spin_axis: str = "z", steps: int = 144) -> dict:
    """
    De beslissende ingrijpingstoets: draai B een volle omwenteling terwijl A
    stilstaat.

    Grijpen de tanden echt in, dan is B grotendeels geblokkeerd en blijven er
    precies `teeth_b` smalle vrije vensters over, een per tand, op de tandsteek
    uit elkaar. Die vensterbreedte IS de speling.

    Draait B vrij rond, dan raken de tandwielen elkaar niet of schuren ze
    alleen langs de toppen - hoe dicht ze ook bij elkaar staan.
    """
    ma = _mesh(part_a).copy()
    ma.apply_transform(_transform(np.asarray(orient_a), np.asarray(pos_a)))
    mgr = trimesh.collision.CollisionManager()
    mgr.add_object("a", ma)
    mb = _mesh(part_b)

    free = []
    for k in range(steps):
        ang = k * 360.0 / steps
        o = np.asarray(orient_b) @ rot(spin_axis, ang)
        m = mb.copy(); m.apply_transform(_transform(o, np.asarray(pos_b)))
        if not mgr.in_collision_single(m):
            free.append(ang)

    if not free:
        return {"verdict": "TE DIEP - geen enkele stand vrij, niet te monteren",
                "windows": 0, "expected_windows": teeth_b, "window_spacing_deg": 0.0,
                "expected_spacing_deg": 360.0 / teeth_b,
                "backlash_deg": 0.0, "free_fraction": 0.0}
    if len(free) == steps:
        return {"verdict": "GEEN INGRIJPING - draait vrij rond",
                "windows": 0, "expected_windows": teeth_b, "window_spacing_deg": 0.0,
                "expected_spacing_deg": 360.0 / teeth_b,
                "backlash_deg": 360.0, "free_fraction": 1.0}

    # opeenvolgende standen samenvoegen tot vensters
    step = 360.0 / steps
    windows, cur = [], [free[0]]
    for a in free[1:]:
        if a - cur[-1] <= step * 1.5:
            cur.append(a)
        else:
            windows.append(cur); cur = [a]
    windows.append(cur)
    if len(windows) > 1 and (free[0] + 360.0) - windows[-1][-1] <= step * 1.5:
        windows[0] = windows[-1] + windows[0]; windows.pop()

    starts = [w[0] for w in windows]
    spacing = np.diff(starts) if len(starts) > 1 else np.array([360.0])
    expected = 360.0 / teeth_b
    ok = len(windows) == teeth_b and abs(np.median(spacing) - expected) < expected * 0.15

    return {
        "verdict": ("KAMT" if ok else
                    f"TWIJFEL - {len(windows)} vensters, verwacht {teeth_b}"),
        "windows": len(windows),
        "expected_windows": teeth_b,
        "window_spacing_deg": float(np.median(spacing)),
        "expected_spacing_deg": expected,
        "backlash_deg": float(np.median([len(w) for w in windows]) * step),
        "free_fraction": len(free) / steps,
    }
