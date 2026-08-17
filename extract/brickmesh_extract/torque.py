# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""
Torque propagation through a gear train, and tooth loads.

The propagation maths is exact. The FAILURE LIMITS ARE NOT — they are
community/measured figures that I could not verify from a primary source in
this session. Every one is marked with a source and a `verified` flag.
Replace them with your own measurements before you trust a PASS.
"""
from __future__ import annotations

from dataclasses import dataclass

# --------------------------------------------------------------------------
# EDIT THIS TABLE. Values in Ncm (torque) and N (force).
# --------------------------------------------------------------------------
LIMITS = {
    "motor_stall_Ncm": {
        "pu_xl_88014": dict(value=40.0, source="community figure for PF/PU XL", verified=False),
        "pu_l_88013":  dict(value=20.0, source="estimate, PF L measured lower", verified=False),
    },
    "axle_torsion_Ncm": {
        "axle_standard": dict(value=15.0, source="rule of thumb, twists before it breaks", verified=False),
    },
    "gear_tooth_force_N": {
        "gear_8t":   dict(value=40.0, source="8t is the classic first failure", verified=False),
        "gear_16t+": dict(value=90.0, source="estimate", verified=False),
    },
    "differential_slip_Ncm": {
        "diff_62821": dict(value=25.0, source="estimate; measure yours", verified=False),
    },
}

EFFICIENCY = {
    "spur": 0.94,
    "bevel": 0.90,
    "worm": 0.45,     # worms are lossy; that is the price of self-locking
    "diff": 0.90,
}


@dataclass
class Stage:
    name: str
    driver_teeth: int
    driven_teeth: int
    kind: str = "spur"

    @property
    def ratio(self) -> float:
        return self.driven_teeth / self.driver_teeth

    @property
    def eff(self) -> float:
        return EFFICIENCY.get(self.kind, 0.9)


def pitch_radius_mm(teeth: int) -> float:
    """LEGO gear pitch radius. Follows from mesh distance = (t1+t2)/16 studs."""
    return teeth / 2.0


def tooth_force_N(torque_Ncm: float, teeth: int) -> float:
    """Tangential load on the tooth flank."""
    torque_Nm = torque_Ncm / 100.0
    radius_m = pitch_radius_mm(teeth) / 1000.0
    return torque_Nm / radius_m


def propagate(input_torque_Ncm: float, stages: list[Stage]):
    """Walk the train, reporting torque and tooth load at every stage."""
    t = input_torque_Ncm
    rows = []
    for s in stages:
        t_out = t * s.ratio * s.eff
        rows.append({
            "stage": s.name,
            "ratio": s.ratio,
            "torque_in_Ncm": t,
            "torque_out_Ncm": t_out,
            "force_driver_N": tooth_force_N(t, s.driver_teeth),
            "force_driven_N": tooth_force_N(t_out, s.driven_teeth),
            "driver_teeth": s.driver_teeth,
            "driven_teeth": s.driven_teeth,
        })
        t = t_out
    return rows


def assess(rows) -> list[str]:
    out = []
    small = LIMITS["gear_tooth_force_N"]["gear_8t"]["value"]
    big = LIMITS["gear_tooth_force_N"]["gear_16t+"]["value"]
    axle = LIMITS["axle_torsion_Ncm"]["axle_standard"]["value"]

    for r in rows:
        for side in ("driver", "driven"):
            teeth = r[f"{side}_teeth"]
            f = r[f"force_{side}_N"]
            lim = small if teeth <= 12 else big
            if f > lim:
                out.append(
                    f"FAIL  {r['stage']}: {teeth}t ziet {f:.0f} N tandkracht, "
                    f"grens {lim:.0f} N — dit wiel slaat over.")
            elif f > 0.7 * lim:
                out.append(
                    f"WARN  {r['stage']}: {teeth}t ziet {f:.0f} N, "
                    f"{100*f/lim:.0f}% van de grens.")
        if r["torque_out_Ncm"] > axle:
            out.append(
                f"WARN  {r['stage']}: as na deze trap draagt "
                f"{r['torque_out_Ncm']:.1f} Ncm, boven de {axle:.0f} Ncm vuistregel. "
                f"Houd deze as kort en dubbel gelagerd.")
    if not out:
        out.append("Geen overschrijdingen — maar zie de waarschuwing over de grenswaarden.")
    return out


def unverified_notice() -> list[str]:
    msg = []
    for group, entries in LIMITS.items():
        for k, v in entries.items():
            if not v["verified"]:
                msg.append(f"  {group}.{k} = {v['value']}  ({v['source']})")
    return msg
