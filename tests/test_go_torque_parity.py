# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""The Go torque port against the Python it was ported from.

Same approach as test_go_parity.py for the extractor: the two implementations
are run over the same trains and required to agree.  A port that is only read
for resemblance is a port nobody has checked.
"""

from __future__ import annotations

import json
import pathlib
import shutil
import subprocess

import pytest

from brickmesh_extract.torque import Stage, propagate

pytestmark = pytest.mark.skipif(shutil.which("go") is None, reason="needs go")

TRAINS = [
    [("8t to 24t", 8, 24, "spur")],
    [("24t to 8t", 24, 8, "spur")],
    [("worm", 1, 24, "worm")],
    [("bevel", 12, 20, "bevel")],
    [("one", 8, 24, "spur"), ("two", 8, 24, "spur")],
    [("one", 12, 20, "spur"), ("two", 16, 16, "spur"), ("three", 20, 12, "spur")],
]
INPUTS = [1.0, 5.0, 40.0]


def go_rows(torque: float, train) -> list[dict]:
    """Run the same train through the Go package, via its command."""
    payload = json.dumps({
        "input_ncm": torque,
        "stages": [
            {"name": name, "driver_teeth": a, "driven_teeth": b, "kind": kind}
            for name, a, b, kind in train
        ],
    })
    out = subprocess.run(
        ["go", "run", "./cmd/brickmesh-torque", "--json"],
        input=payload, capture_output=True, text=True, check=True,
        cwd=pathlib.Path(__file__).resolve().parents[1],
    )
    return json.loads(out.stdout)


@pytest.mark.parametrize("torque_ncm", INPUTS)
@pytest.mark.parametrize("train", TRAINS, ids=lambda t: "+".join(s[0] for s in t))
def test_go_and_python_agree(torque_ncm, train):
    py = propagate(torque_ncm, [Stage(n, a, b, k) for n, a, b, k in train])
    go = go_rows(torque_ncm, train)

    assert len(go) == len(py), "different number of stages"
    for g, p in zip(go, py):
        assert g["Stage"] == p["stage"]
        for go_key, py_key in [
            ("Ratio", "ratio"),
            ("TorqueInNcm", "torque_in_Ncm"),
            ("TorqueOutNcm", "torque_out_Ncm"),
            ("ForceDriverN", "force_driver_N"),
            ("ForceDrivenN", "force_driven_N"),
        ]:
            assert g[go_key] == pytest.approx(p[py_key], rel=1e-12), (
                f"{p['stage']}: {go_key} is {g[go_key]}, python says {p[py_key]}"
            )
