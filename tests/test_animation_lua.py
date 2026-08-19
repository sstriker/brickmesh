# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Sander Striker
"""Run the animation script brickmesh writes, against a stub of LDCad's API.

Nothing on the Go side can tell whether the Lua it emits is valid Lua, let alone
whether the angles come out right: it writes text.  Here the text is executed.
The stub records what each group is told to be, so a frame can be asked for and
the answer checked -- that a shaft turns the whole way round rather than
restarting each time the ratio changes, and that a driving ring is where its
state says it should be.

The script under test is internal/ldcad/testdata/shift.lua, which a Go test
keeps in step with the writer.
"""

from __future__ import annotations

import functools
import math
import pathlib

import pytest

lupa = pytest.importorskip("lupa", reason="lupa provides the Lua runtime")

SCRIPT = (pathlib.Path(__file__).resolve().parents[1]
          / "internal" / "ldcad" / "testdata" / "shift.lua")

SECONDS = 10.0
INPUT_TURNS = 4.0


def method(fn):
    """Let a Python method stand in for a Lua one.

    LDCad's objects are Lua tables, so the script calls them with a colon --
    ``grp:setOri(m)`` -- which passes the object as the first argument.  Coming
    through lupa the method is already bound, so it arrives with itself twice.
    Dropping the duplicate is what lets the stub be ordinary Python.
    """

    @functools.wraps(fn)
    def wrapper(self, *args):
        if args and args[0] is self:
            args = args[1:]
        return fn(self, *args)

    return wrapper



IDENTITY = (1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0)


def rotation(deg: float, x: float, y: float, z: float) -> tuple[float, ...]:
    """A 3x3 rotation about an axis, row major."""
    n = math.hypot(x, y, z) or 1.0
    x, y, z = x / n, y / n, z / n
    c, s_ = math.cos(math.radians(deg)), math.sin(math.radians(deg))
    k = 1.0 - c
    return (
        c + x * x * k, x * y * k - z * s_, x * z * k + y * s_,
        y * x * k + z * s_, c + y * y * k, y * z * k - x * s_,
        z * x * k - y * s_, z * y * k + x * s_, c + z * z * k,
    )


def mat_mul(a, b):
    return tuple(
        sum(a[r * 3 + i] * b[i * 3 + col] for i in range(3))
        for r in range(3)
        for col in range(3)
    )


def mat_apply(m, v):
    return tuple(sum(m[r * 3 + i] * v[i] for i in range(3)) for r in range(3))


def transpose(m):
    return (m[0], m[3], m[6], m[1], m[4], m[7], m[2], m[5], m[8])


# What each group's parts are turned to in the model before anything animates.
#
# NOT the identity, and that is the point.  A gear sits on a shaft, so its
# placement is a rotation, and the group inherits it.  The stub used to assume
# every group started square to the model, which is the one case where getting
# setOri wrong makes no difference -- so it could not see the bug that had every
# group in LDCad snapping to a fresh orientation instead of turning from its own.
START_ORI = {
    "shaft_input": rotation(90.0, 0.0, 1.0, 0.0),
    "shaft_output": rotation(90.0, 0.0, 1.0, 0.0),
    "shaft_low": rotation(90.0, 0.0, 1.0, 0.0),
    "shaft_high": rotation(90.0, 0.0, 1.0, 0.0),
    "ring_1": rotation(90.0, 0.0, 1.0, 0.0),
    "ring_2": rotation(90.0, 0.0, 1.0, 0.0),
    "ring_3": rotation(90.0, 0.0, 1.0, 0.0),
}


# Where each group's GROUP_DEF puts its centre, which is the one thing about a
# group that the script itself never states — it is in the model file.  The
# fixture in golden_test.go decides these.
CENTRES = {
    "shaft_input": (0.0, 0.0, 0.0),
    "shaft_output": (0.0, 0.0, -40.0),
    "shaft_low": (0.0, 0.0, -40.0),
    "shaft_high": (0.0, 0.0, -40.0),
    "ring_1": (30.0, 0.0, -40.0),
    "ring_2": (100.0, 0.0, -40.0),
    "ring_3": (170.0, 0.0, -40.0),
}


class Group:
    """One LDCad group: what the script sets is what we read back."""

    def __init__(self, name: str) -> None:
        self.name = name
        self.centre = CENTRES.get(name, (0.0, 0.0, 0.0))
        self.start = START_ORI.get(name, IDENTITY)
        self.ori = self.start
        self.pos: tuple[float, float, float] | None = None
        self.angle = 0.0
        self.axis = (0.0, 0.0, 0.0)

    @method
    def getOri(self):
        return Matrix(self.ori)

    @method
    def getPosOri(self):
        m = Matrix(self.ori)
        m.pos = self.pos if self.pos is not None else self.centre
        return m

    @method
    def setOri(self, m):  # LDCad's own spelling
        self.ori = m.m
        self.angle, self.axis = m.angle, m.axis

    @method
    def setPos(self, x, y, z):
        self.pos = (x, y, z)

    @method
    def setPosOri(self, m):
        self.ori, self.pos = m.m, m.pos
        self.angle, self.axis = m.angle, m.axis


class Matrix:
    """A 3x3 orientation, and the turn that was last multiplied into it.

    The angle is carried alongside the matrix because a matrix cannot tell 370
    degrees from 10, and several of the tests below are about exactly that --
    a shaft accumulating turns across a shift rather than restarting. Geometry
    comes from the matrix; how far something has turned comes from the angle.
    """

    def __init__(self, data=IDENTITY) -> None:
        self.m = tuple(data)
        self.pos = None
        self.angle = 0.0
        self.axis = (0.0, 0.0, 0.0)

    @method
    def clone(self):
        m = Matrix(self.m)
        m.pos, m.angle, m.axis = self.pos, self.angle, self.axis
        return m

    @method
    def setIdentity(self):
        self.m = IDENTITY

    @method
    def setRotate(self, deg, x, y, z):
        self.m = rotation(deg, x, y, z)
        self.angle, self.axis = deg, (x, y, z)

    # These two are the way round LDCad behaves, which is not the way round its
    # reference describes them. The reference says AB is self*rotate and BA is
    # rotate*self; measured against LDCad, AB is the one that turns a group
    # about an axis in the model's frame. A gear placed turned, asked for a
    # quarter turn about x, kept its axis on x under AB and lost it under BA.
    #
    # It cannot be seen at all on a group whose main item is placed square,
    # since then the orders coincide. That is why it took a model with gears in
    # it to notice.

    @method
    def mulRotateAB(self, deg, x, y, z):
        """Measured: the rotation lands in the model's frame."""
        self.m = mat_mul(rotation(deg, x, y, z), self.m)
        self.angle, self.axis = deg, (x, y, z)

    @method
    def mulRotateBA(self, deg, x, y, z):
        self.m = mat_mul(self.m, rotation(deg, x, y, z))
        self.angle, self.axis = deg, (x, y, z)

    @method
    def setPos(self, x, y, z):
        self.pos = (x, y, z)

    @method
    def setTranslate(self, x, y, z):
        self.pos = (x, y, z)


class Animation:
    def __init__(self, name: str) -> None:
        self.name = name
        self.length = SECONDS
        self.frame_time = 0.0
        self.events: dict[str, str] = {}

    @method
    def setLength(self, seconds):
        self.length = seconds

    @method
    def setEvent(self, when, fn):
        self.events[when] = fn

    @method
    def getLength(self):
        return self.length

    @method
    def getFrameTime(self):
        return self.frame_time


class Subfile:
    def __init__(self, stub: Stub) -> None:
        self.stub = stub

    @method
    def getGroup(self, name):
        return self.stub.group(name)


class Stub:
    """Just enough of LDCad for the script to run."""

    def __init__(self) -> None:
        self.groups: dict[str, Group] = {}
        self.animations: dict[str, Animation] = {}
        self.current: Animation | None = None

    def group(self, name: str) -> Group:
        return self.groups.setdefault(name, Group(name))


@pytest.fixture(scope="module")
def lua_and_stub():
    if not SCRIPT.exists():
        pytest.skip(f"{SCRIPT} is missing; run the Go tests to write it")

    runtime = lupa.LuaRuntime(unpack_returned_tuples=True)
    stub = Stub()

    def animation(name):
        stub.animations.setdefault(name, Animation(name))
        return stub.animations[name]

    def get_current():
        return stub.current

    # ldc.animation is both called and reached into, so it needs to be a table
    # that is also callable -- which is what LDCad's own API is.
    runtime.execute("""
      make_ldc = function(animation, get_current, subfile, matrix)
        local ldc = {}
        ldc.animation = setmetatable({getCurrent=get_current},
                                     {__call=function(_, n) return animation(n) end})
        ldc.subfile = subfile
        ldc.matrix = matrix
        return ldc
      end
    """)
    runtime.globals()["ldc"] = runtime.globals()["make_ldc"](
        animation, get_current, lambda: Subfile(stub), Matrix)

    runtime.execute(SCRIPT.read_text())
    return runtime, stub


def applied(group: Group, point) -> tuple[float, float, float]:
    """Where a part at `point` ends up, under the transform the group was given.

    Two things decide it, and both were got wrong in turn.

    A group's placement is its CENTRE, not the model's origin.  The scripting
    reference: setPos "applies to the groups center position not the main item's
    true position", getPos "returns the position of the linked LDCad group
    current center point".  So the contents are held relative to the centre.

    And setOri is ABSOLUTE -- it replaces a group's orientation rather than
    adding to it.  A group whose parts are already turned, which is every group
    holding a gear on a shaft, therefore moves by the difference between the
    orientation it is given and the one it started with:

        p' = (ori * start^-1) * (p - centre) + (position or centre)

    Setting the orientation to a bare rotation R, as the generator first did,
    gives R * start^-1 -- which is only the rotation you wanted when the group
    started square to the model.  That is why START_ORI above is not the
    identity: with it, a stub can tell the two apart.
    """
    delta = mat_mul(group.ori, transpose(group.start))
    cx, cy, cz = group.centre
    local = (point[0] - cx, point[1] - cy, point[2] - cz)
    rx, ry, rz = mat_apply(delta, local)
    tx, ty, tz = group.pos if group.pos is not None else group.centre
    return (rx + tx, ry + ty, rz + tz)


def frame(runtime, stub, animation_name: str, t: float) -> Stub:
    """Ask the script for one frame, at a fraction t of the way through."""
    ani = stub.animations[animation_name]
    ani.frame_time = t * ani.length
    stub.current = ani
    runtime.globals()[ani.events["start"]]()
    runtime.globals()[ani.events["frame"]]()
    return stub


def test_the_script_registers_every_animation(lua_and_stub):
    _, stub = lua_and_stub
    assert set(stub.animations) == {"1st", "2nd", "3rd", "shift"}
    for ani in stub.animations.values():
        assert ani.length == SECONDS
        assert set(ani.events) == {"start", "frame"}


def test_a_held_state_turns_at_its_own_ratio(lua_and_stub):
    runtime, stub = lua_and_stub
    frame(runtime, stub, "2nd", 0.5)
    # Half the animation is half the input's turns, in degrees.
    input_deg = 0.5 * INPUT_TURNS * 360
    assert stub.group("shaft_input").angle == pytest.approx(input_deg)
    assert stub.group("shaft_output").angle == pytest.approx(-0.6 * input_deg)


def test_a_ring_sits_against_its_gear_in_its_own_state(lua_and_stub):
    runtime, stub = lua_and_stub
    frame(runtime, stub, "2nd", 0.5)
    # ring_2 is the one 2nd engages: it stays hard against its gear.  The
    # others have slid a half stud clear of theirs.
    assert applied(stub.group("ring_2"), (100.0, 0.0, -40.0)) == pytest.approx(
        (100.0, 0.0, -40.0))
    assert applied(stub.group("ring_1"), (30.0, 0.0, -40.0)) == pytest.approx(
        (40.0, 0.0, -40.0))
    assert applied(stub.group("ring_3"), (170.0, 0.0, -40.0)) == pytest.approx(
        (180.0, 0.0, -40.0))


def test_the_shift_walks_through_the_states(lua_and_stub):
    """Each third of the shift animation is a state, and its ring comes home."""
    runtime, stub = lua_and_stub
    for i, ring in enumerate(["ring_1", "ring_2", "ring_3"]):
        # Early in a segment, before the ring starts moving on to the next.
        frame(runtime, stub, "shift", i / 3 + 0.05)
        engaged = 30.0 + 70 * i
        where = applied(stub.group(ring), (engaged, 0.0, -40.0))
        assert where[0] == pytest.approx(engaged), (
            f"{ring} should be engaged during segment {i}")


def test_the_shift_moves_the_ring_rather_than_teleporting_it(lua_and_stub):
    """The point of the animation: a ring is caught in between."""
    runtime, stub = lua_and_stub
    seen = set()
    for k in range(61):  # 61 so the last sample lands on the segment's end
        frame(runtime, stub, "shift", k / 60 * (1 / 3))
        seen.add(round(applied(stub.group("ring_1"), (30.0, 0.0, -40.0))[0], 3))
    assert 30.0 in seen, "ring_1 should start engaged"
    assert 40.0 in seen, "and end the segment clear"
    between = [v for v in seen if 30.0 < v < 40.0]
    assert len(between) >= 5, (
        f"ring_1 jumps from engaged to clear through {sorted(between)}; "
        "a shift you cannot watch is the thing this animation exists to fix")


def test_a_shaft_does_not_jump_when_the_ratio_changes(lua_and_stub):
    """Orientation is absolute, so the angle has to carry its whole history."""
    runtime, stub = lua_and_stub
    step = 1e-4
    for boundary in (1 / 3, 2 / 3):
        frame(runtime, stub, "shift", boundary - step)
        before = stub.group("shaft_output").angle
        frame(runtime, stub, "shift", boundary + step)
        after = stub.group("shaft_output").angle
        # Across a boundary the speed changes but the position cannot: the most
        # it may move in 2e-4 of the animation is a fraction of a degree.
        assert abs(after - before) < 1.0, (
            f"the output jumps {after - before:.1f} degrees at {boundary:.3f}; "
            "the cumulative sum is not carrying the finished segments")


SHIFT = 0.25  # the share of each segment a ring spends sliding


def test_the_output_ends_where_the_ratios_say_it_should(lua_and_stub):
    runtime, stub = lua_and_stub
    frame(runtime, stub, "shift", 1.0)
    per_segment = INPUT_TURNS * 360 / 3
    # Times (1 - SHIFT), because the output is driven through a ring and stands
    # still while that ring is between gears. It turns for three quarters of
    # each state and coasts to a stop for the last quarter.
    want = sum(r * per_segment * (1 - SHIFT) for r in (-1 / 3, -0.6, -1.0))
    assert stub.group("shaft_output").angle == pytest.approx(want, abs=1.0)
    assert not math.isnan(stub.group("shaft_output").angle)


def test_a_shaft_driven_through_a_ring_holds_still_while_the_ring_slides(lua_and_stub):
    """Nothing drives it, so it must not turn.

    A driving ring in neither gear is driving nothing. The clutch gears on that
    shaft keep going, because the input still reaches them through their mesh,
    but the shaft itself is free — and turning it anyway showed a drive that was
    not there. Spotted by someone watching the model, not by any check here.
    """
    runtime, stub = lua_and_stub

    # The first segment runs 0 .. 1/3 of the animation; its ring slides through
    # the last quarter of that, from 0.25 to 1/3.
    start_of_slide = (1 / 3) * (1 - SHIFT)
    frame(runtime, stub, "shift", start_of_slide + 1e-4)
    held = stub.group("shaft_output").angle
    gear = stub.group("shaft_first").angle

    frame(runtime, stub, "shift", (1 / 3) - 1e-4)
    assert stub.group("shaft_output").angle == pytest.approx(held, abs=0.5), (
        "the output kept turning while its ring was between gears, with "
        "nothing engaged to drive it"
    )
    moved = abs(stub.group("shaft_first").angle - gear)
    assert moved > 10, (
        f"the clutch gear only moved {moved:.1f} degrees during the shift; it "
        "is driven by the input through its mesh and should not have stopped"
    )


def test_a_shaft_turns_about_its_own_axis(lua_and_stub):
    """The property the whole animation rests on.

    A group turns about its own centre, so a shaft whose centre is on its axis
    needs nothing but a rotation.  Any point on that axis must stay exactly
    where it is, at every angle.

    This held before too, against a stub that modelled LDCad as turning about
    the origin and a generator that compensated for it.  Two wrongs agreeing is
    what a stub cannot catch.
    """
    runtime, stub = lua_and_stub
    on_the_axis = (123.0, 0.0, -40.0)  # shaft_output runs along x at z=-40
    for t in (0.0, 0.17, 0.33, 0.5, 0.81, 1.0):
        frame(runtime, stub, "2nd", t)
        where = applied(stub.group("shaft_output"), on_the_axis)
        assert where == pytest.approx(on_the_axis, abs=1e-6), (
            f"at t={t} a point on the shaft's own axis moved to {where}; "
            "the group is turning about the origin instead"
        )


def test_a_part_off_the_axis_sweeps_a_circle_around_it(lua_and_stub):
    """And the parts that should move, move the right way: a gear's rim stays
    at a constant distance from its own shaft, not from the origin."""
    runtime, stub = lua_and_stub
    axis_point = (123.0, 0.0, -40.0)
    rim = (123.0, 20.0, -40.0)  # 20 LDU off the axis
    for t in (0.0, 0.25, 0.5, 0.75):
        frame(runtime, stub, "2nd", t)
        where = applied(stub.group("shaft_output"), rim)
        radius = math.dist(where, axis_point)
        assert radius == pytest.approx(20.0, abs=1e-6), (
            f"at t={t} the rim is {radius:.2f} from its own axis, not 20"
        )


def test_the_pivot_correction_would_now_be_caught():
    """Control for the first wrong belief: that placement was about the origin.

    The generator used to add t = q - R*q to every turning group, to drag the
    pivot from the model origin onto the shaft. Against a stub that shared the
    belief it was right, and everything passed. Against LDCad it threw each
    group off its axis by twice its distance from the origin.

    So: give a group the correct orientation but that old offset as a position,
    and require the axis NOT to hold still.
    """
    g = Group("shaft_output")  # centre (0, 0, -40), axis along x
    g.ori = mat_mul(rotation(90.0, 1.0, 0.0, 0.0), g.start)

    qx, qy, qz = g.centre
    rqx, rqy, rqz = mat_apply(rotation(90.0, 1.0, 0.0, 0.0), g.centre)
    g.pos = (qx - rqx, qy - rqy, qz - rqz)

    on_the_axis = (123.0, 0.0, -40.0)
    assert math.dist(applied(g, on_the_axis), on_the_axis) > 1.0, (
        "the old pivot correction left a point on the axis where it was, so "
        "these tests cannot tell that convention from the right one"
    )


def test_a_bare_rotation_would_now_be_caught():
    """Control for the second wrong belief: that setOri adds to what is there.

    It replaces. A group holding a gear on a shaft starts already turned, so
    handing it a bare rotation snaps it to that orientation instead of turning
    it by that much -- which is what LDCad showed, and what the stub could not
    show while it assumed every group started square to the model.

    So: give a group the bare rotation the generator used to set, and require
    the axis NOT to hold still.
    """
    g = Group("shaft_output")
    g.ori = rotation(90.0, 1.0, 0.0, 0.0)  # no regard for where it started

    on_the_axis = (123.0, 0.0, -40.0)
    assert math.dist(applied(g, on_the_axis), on_the_axis) > 1.0, (
        "a bare rotation left a point on the axis where it was; START_ORI must "
        "have drifted back to the identity, and with it goes the only reason "
        "this file can tell the two spellings of setOri apart"
    )
