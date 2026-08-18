--[[
  Animation for 3-speed gearbox, written by brickmesh.

  Every group below turns at the ratio the functional layer solved for, so what
  you see is the mechanism this model actually is. Speeds are turns of that
  group per turn of the input shaft; the input itself makes 4 turns over the
  10 seconds of the animation.
]]

function onStart0()
  local sf=ldc.subfile()
  grp0_0=sf:getGroup("shaft_input")
  grp0_1=sf:getGroup("shaft_output")
  grp0_2=sf:getGroup("shaft_first")
  grp0_3=sf:getGroup("shaft_second")
  grp0_4=sf:getGroup("shaft_third")
  ring0_0=sf:getGroup("ring_1")
  ring0_1=sf:getGroup("ring_2")
  ring0_2=sf:getGroup("ring_3")
end

function onFrame0()
  local ani=ldc.animation.getCurrent()
  local t=ani:getFrameTime()/ani:getLength()
  local input=t*4*360 --degrees turned by the input
  local m=ldc.matrix()

  --shaft_input turns 1.0000 per turn of the input
  m:setRotate(input*1.000000, 1, 0, 0)
  grp0_0:setOri(m)

  --shaft_output turns -0.3333 per turn of the input
  m:setRotate(input*-0.333333, 1, 0, 0)
  grp0_1:setOri(m)

  --shaft_first turns -0.3333 per turn of the input
  m:setRotate(input*-0.333333, 1, 0, 0)
  grp0_2:setOri(m)

  --shaft_second turns -0.6000 per turn of the input
  m:setRotate(input*-0.600000, 1, 0, 0)
  grp0_3:setOri(m)

  --shaft_third turns -1.0000 per turn of the input
  m:setRotate(input*-1.000000, 1, 0, 0)
  grp0_4:setOri(m)

  --ring_1 turns with its shaft and sits engaged
  m:setRotate(input*-0.333333, 1, 0, 0)
  m:setPos(30, 0, -40)
  ring0_0:setPosOri(m)

  --ring_2 turns with its shaft and sits clear of its gear
  m:setRotate(input*-0.333333, 1, 0, 0)
  m:setPos(110, 0, -40)
  ring0_1:setPosOri(m)

  --ring_3 turns with its shaft and sits clear of its gear
  m:setRotate(input*-0.333333, 1, 0, 0)
  m:setPos(180, 0, -40)
  ring0_2:setPosOri(m)
end

function onStart1()
  local sf=ldc.subfile()
  grp1_0=sf:getGroup("shaft_input")
  grp1_1=sf:getGroup("shaft_output")
  grp1_2=sf:getGroup("shaft_first")
  grp1_3=sf:getGroup("shaft_second")
  grp1_4=sf:getGroup("shaft_third")
  ring1_0=sf:getGroup("ring_1")
  ring1_1=sf:getGroup("ring_2")
  ring1_2=sf:getGroup("ring_3")
end

function onFrame1()
  local ani=ldc.animation.getCurrent()
  local t=ani:getFrameTime()/ani:getLength()
  local input=t*4*360 --degrees turned by the input
  local m=ldc.matrix()

  --shaft_input turns 1.0000 per turn of the input
  m:setRotate(input*1.000000, 1, 0, 0)
  grp1_0:setOri(m)

  --shaft_output turns -0.6000 per turn of the input
  m:setRotate(input*-0.600000, 1, 0, 0)
  grp1_1:setOri(m)

  --shaft_first turns -0.3333 per turn of the input
  m:setRotate(input*-0.333333, 1, 0, 0)
  grp1_2:setOri(m)

  --shaft_second turns -0.6000 per turn of the input
  m:setRotate(input*-0.600000, 1, 0, 0)
  grp1_3:setOri(m)

  --shaft_third turns -1.0000 per turn of the input
  m:setRotate(input*-1.000000, 1, 0, 0)
  grp1_4:setOri(m)

  --ring_1 turns with its shaft and sits clear of its gear
  m:setRotate(input*-0.600000, 1, 0, 0)
  m:setPos(40, 0, -40)
  ring1_0:setPosOri(m)

  --ring_2 turns with its shaft and sits engaged
  m:setRotate(input*-0.600000, 1, 0, 0)
  m:setPos(100, 0, -40)
  ring1_1:setPosOri(m)

  --ring_3 turns with its shaft and sits clear of its gear
  m:setRotate(input*-0.600000, 1, 0, 0)
  m:setPos(180, 0, -40)
  ring1_2:setPosOri(m)
end

function onStart2()
  local sf=ldc.subfile()
  grp2_0=sf:getGroup("shaft_input")
  grp2_1=sf:getGroup("shaft_output")
  grp2_2=sf:getGroup("shaft_first")
  grp2_3=sf:getGroup("shaft_second")
  grp2_4=sf:getGroup("shaft_third")
  ring2_0=sf:getGroup("ring_1")
  ring2_1=sf:getGroup("ring_2")
  ring2_2=sf:getGroup("ring_3")
end

function onFrame2()
  local ani=ldc.animation.getCurrent()
  local t=ani:getFrameTime()/ani:getLength()
  local input=t*4*360 --degrees turned by the input
  local m=ldc.matrix()

  --shaft_input turns 1.0000 per turn of the input
  m:setRotate(input*1.000000, 1, 0, 0)
  grp2_0:setOri(m)

  --shaft_output turns -1.0000 per turn of the input
  m:setRotate(input*-1.000000, 1, 0, 0)
  grp2_1:setOri(m)

  --shaft_first turns -0.3333 per turn of the input
  m:setRotate(input*-0.333333, 1, 0, 0)
  grp2_2:setOri(m)

  --shaft_second turns -0.6000 per turn of the input
  m:setRotate(input*-0.600000, 1, 0, 0)
  grp2_3:setOri(m)

  --shaft_third turns -1.0000 per turn of the input
  m:setRotate(input*-1.000000, 1, 0, 0)
  grp2_4:setOri(m)

  --ring_1 turns with its shaft and sits clear of its gear
  m:setRotate(input*-1.000000, 1, 0, 0)
  m:setPos(40, 0, -40)
  ring2_0:setPosOri(m)

  --ring_2 turns with its shaft and sits clear of its gear
  m:setRotate(input*-1.000000, 1, 0, 0)
  m:setPos(110, 0, -40)
  ring2_1:setPosOri(m)

  --ring_3 turns with its shaft and sits engaged
  m:setRotate(input*-1.000000, 1, 0, 0)
  m:setPos(170, 0, -40)
  ring2_2:setPosOri(m)
end

function onStart3()
  local sf=ldc.subfile()
  grp3_0=sf:getGroup("shaft_input")
  grp3_1=sf:getGroup("shaft_output")
  grp3_2=sf:getGroup("shaft_first")
  grp3_3=sf:getGroup("shaft_second")
  grp3_4=sf:getGroup("shaft_third")
  ring3_0=sf:getGroup("ring_1")
  ring3_1=sf:getGroup("ring_2")
  ring3_2=sf:getGroup("ring_3")
end

function onFrame3()
  local ani=ldc.animation.getCurrent()
  local t=ani:getFrameTime()/ani:getLength()
  local segs=3
  local seg=math.floor(t*segs)
  if seg>segs-1 then seg=segs-1 end
  local u=t*segs-seg --0..1 within this segment
  local perSeg=4*360/segs --input degrees per segment
  local m=ldc.matrix()

  --speed[group][segment], in turns per turn of the input
  local speed={
    {1.000000, 1.000000, 1.000000}, --shaft_input
    {-0.333333, -0.600000, -1.000000}, --shaft_output
    {-0.333333, -0.333333, -0.333333}, --shaft_first
    {-0.600000, -0.600000, -0.600000}, --shaft_second
    {-1.000000, -1.000000, -1.000000}, --shaft_third
  }

  --Degrees a group has turned by now: every finished segment in full, plus
  --this segment's share.
  local function angle(sp)
    local a=0
    for k=1,seg do a=a+sp[k]*perSeg end
    return a+sp[seg+1]*u*perSeg
  end

  m:setRotate(angle(speed[1]), 1, 0, 0)
  grp3_0:setOri(m)
  m:setRotate(angle(speed[2]), 1, 0, 0)
  grp3_1:setOri(m)
  m:setRotate(angle(speed[3]), 1, 0, 0)
  grp3_2:setOri(m)
  m:setRotate(angle(speed[4]), 1, 0, 0)
  grp3_3:setOri(m)
  m:setRotate(angle(speed[5]), 1, 0, 0)
  grp3_4:setOri(m)

  --A ring holds its place for most of a segment and moves near the end of it,
  --so the shift is a thing that happens rather than a thing between frames.
  local shift=0.25
  local f=0
  if u>1-shift then f=(u-(1-shift))/shift end
  local nxt=seg+1
  if nxt>segs-1 then nxt=segs-1 end

  --ringSpeed[ring][segment]: a ring is splined to its shaft
  local ringSpeed={
    {-0.333333, -0.600000, -1.000000}, --ring_1
    {-0.333333, -0.600000, -1.000000}, --ring_2
    {-0.333333, -0.600000, -1.000000}, --ring_3
  }

  --where[ring][segment]: 0 engaged, 1 clear
  local where={
    {0, 1, 1}, --ring_1
    {1, 0, 1}, --ring_2
    {1, 1, 0}, --ring_3
  }

  do --ring_1
    local a=where[1][seg+1]
    local at=a+(where[1][nxt+1]-a)*f
    m:setRotate(angle(ringSpeed[1]), 1, 0, 0)
    m:setPos(30+(10)*at, 0+(0)*at, -40+(0)*at)
    ring3_0:setPosOri(m)
  end
  do --ring_2
    local a=where[2][seg+1]
    local at=a+(where[2][nxt+1]-a)*f
    m:setRotate(angle(ringSpeed[2]), 1, 0, 0)
    m:setPos(100+(10)*at, 0+(0)*at, -40+(0)*at)
    ring3_1:setPosOri(m)
  end
  do --ring_3
    local a=where[3][seg+1]
    local at=a+(where[3][nxt+1]-a)*f
    m:setRotate(angle(ringSpeed[3]), 1, 0, 0)
    m:setPos(170+(10)*at, 0+(0)*at, -40+(0)*at)
    ring3_2:setPosOri(m)
  end
end

function register()
  local ani0=ldc.animation("1st")
  ani0:setLength(10)
  ani0:setEvent('start', 'onStart0')
  ani0:setEvent('frame', 'onFrame0')

  local ani1=ldc.animation("2nd")
  ani1:setLength(10)
  ani1:setEvent('start', 'onStart1')
  ani1:setEvent('frame', 'onFrame1')

  local ani2=ldc.animation("3rd")
  ani2:setLength(10)
  ani2:setEvent('start', 'onStart2')
  ani2:setEvent('frame', 'onFrame2')

  local ani3=ldc.animation("shift")
  ani3:setLength(10)
  ani3:setEvent('start', 'onStart3')
  ani3:setEvent('frame', 'onFrame3')
end

register()
