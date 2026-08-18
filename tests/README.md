# What is left of the Python

The engine was Python once. It was ported to Go module by module, and
`docs/findings.md` records what each port was checked against — the extractor
byte-identical over 2,649 parts, the collision test matching FCL numerically,
torque agreeing across eighteen trains, the tooth phase gear by gear.

Then the Python went. A second implementation that has fallen behind is worse
than no second implementation: it answers confidently and wrongly, and by the
end this one knew nothing about clutch gears, driving-ring systems, axle
joiners, shift schedules or tooth phase.

What is here tests the Go:

- **`test_animation_lua.py`** runs the LDCad animation script the engine writes,
  under a real Lua runtime, against a stub of LDCad's API. Nothing on the Go
  side can tell whether the text it emits is valid Lua, let alone whether the
  angles come out right. This has caught a fall-through at the end of the
  animation and a rotation happening about the wrong point.
- **`fixtures/`** are synthetic parts — plain boxes and a square tube, authored
  from scratch rather than taken from any library, so nothing here inherits the
  LDraw or LDCad terms. The Go tests read them. `generate_fixtures.py` is what
  wrote them and can rewrite them.
