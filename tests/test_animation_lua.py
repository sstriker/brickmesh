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
        self.angle = 0.0
        self.axis = (0.0, 0.0, 0.0)
        self.pos: tuple[float, float, float] | None = None
        self.centre = CENTRES.get(name, (0.0, 0.0, 0.0))

    @method
    def setOri(self, m):  # LDCad's own spelling
        self.angle, self.axis = m.angle, m.axis

    @method
    def setPos(self, x, y, z):
        self.pos = (x, y, z)

    @method
    def setPosOri(self, m):
        self.angle, self.axis, self.pos = m.angle, m.axis, m.pos


class Matrix:
    def __init__(self) -> None:
        self.angle = 0.0
        self.axis = (0.0, 0.0, 0.0)
        self.pos = None

    @method
    def setRotate(self, deg, x, y, z):
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

    A group's placement is its CENTRE, not the model's origin.  LDCad's
    scripting reference is explicit: setPos "applies to the groups center
    position not the main item's true position", and getPos "returns the
    position of the linked LDCad group current center point".  So the contents
    are held relative to the centre, and the placement says where that centre
    goes and how it is turned:

        p' = R*(p - centre) + (position or centre)

    This used to model it as p' = R*p + t about the origin, which is what the
    generator was written against, and both were wrong together — so these tests
    passed while LDCad scattered the parts.  A stub can only ever check that the
    code agrees with the model of LDCad it was built from; it took opening a
    file in LDCad to find out the model was wrong.
    """
    cx, cy, cz = group.centre
    x, y, z = point[0] - cx, point[1] - cy, point[2] - cz
    ax, ay, az = group.axis
    th = math.radians(group.angle)
    c, s_ = math.cos(th), math.sin(th)
    dot = ax * x + ay * y + az * z
    # Rodrigues.
    rx = x * c + (ay * z - az * y) * s_ + ax * dot * (1 - c)
    ry = y * c + (az * x - ax * z) * s_ + ay * dot * (1 - c)
    rz = z * c + (ax * y - ay * x) * s_ + az * dot * (1 - c)
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


def test_the_output_ends_where_the_ratios_say_it_should(lua_and_stub):
    runtime, stub = lua_and_stub
    frame(runtime, stub, "shift", 1.0)
    per_segment = INPUT_TURNS * 360 / 3
    want = sum(r * per_segment for r in (-1 / 3, -0.6, -1.0))
    assert stub.group("shaft_output").angle == pytest.approx(want, abs=1.0)
    assert not math.isnan(stub.group("shaft_output").angle)


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
    """The control, and the reason these tests can be trusted again.

    The generator used to add t = q - R*q to every turning group, to move the
    pivot from the model origin onto the shaft.  Against a stub that also
    believed placement was about the origin, that was right and everything
    passed.  Against LDCad it threw each group off its axis by twice its
    distance from the origin.

    So: hand a group exactly that old offset and check the axis does NOT hold
    still.  If this ever stops failing, `applied` has drifted back to the model
    the generator was written against and the tests above mean nothing.
    """
    g = Group("shaft_output")  # centre (0, 0, -40), axis along x
    g.angle, g.axis = 90.0, (1.0, 0.0, 0.0)

    # t = q - R*q for q = the group's centre, which is what used to be emitted.
    qx, qy, qz = g.centre
    th = math.radians(g.angle)
    c, s_ = math.cos(th), math.sin(th)
    # q lies across the axis here, so R*q is the two-term form.
    rqx, rqy, rqz = (
        qx * c + (0.0 * qz - 0.0 * qy) * s_,
        qy * c + (0.0 * qx - 1.0 * qz) * s_,
        qz * c + (1.0 * qy - 0.0 * qx) * s_,
    )
    g.pos = (qx - rqx, qy - rqy, qz - rqz)

    on_the_axis = (123.0, 0.0, -40.0)
    moved = applied(g, on_the_axis)
    assert math.dist(moved, on_the_axis) > 1.0, (
        "the old pivot correction left a point on the axis where it was, so "
        "these tests cannot tell the two conventions apart"
    )
