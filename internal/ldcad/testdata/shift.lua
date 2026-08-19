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
  ori0_0=grp0_0:getOri()
  grp0_1=sf:getGroup("shaft_output")
  ori0_1=grp0_1:getOri()
  grp0_2=sf:getGroup("shaft_first")
  ori0_2=grp0_2:getOri()
  grp0_3=sf:getGroup("shaft_second")
  ori0_3=grp0_3:getOri()
  grp0_4=sf:getGroup("shaft_third")
  ori0_4=grp0_4:getOri()
  ring0_0=sf:getGroup("ring_1")
  rori0_0=ring0_0:getOri()
  ring0_1=sf:getGroup("ring_2")
  rori0_1=ring0_1:getOri()
  ring0_2=sf:getGroup("ring_3")
  rori0_2=ring0_2:getOri()
end

function onFrame0()
  local ani=ldc.animation.getCurrent()
  local t=ani:getFrameTime()/ani:getLength()
  local input=t*4*360 --degrees turned by the input

  --shaft_input turns 1.0000 per turn of the input
  local a=input*1.000000
  local m0=ori0_0:clone()
  m0:mulRotateBA(a, 1, 0, 0)
  grp0_0:setOri(m0)

  --shaft_output turns -0.3333 per turn of the input
  local a=input*-0.333333
  local m1=ori0_1:clone()
  m1:mulRotateBA(a, 1, 0, 0)
  grp0_1:setOri(m1)

  --shaft_first turns -0.3333 per turn of the input
  local a=input*-0.333333
  local m2=ori0_2:clone()
  m2:mulRotateBA(a, 1, 0, 0)
  grp0_2:setOri(m2)

  --shaft_second turns -0.6000 per turn of the input
  local a=input*-0.600000
  local m3=ori0_3:clone()
  m3:mulRotateBA(a, 1, 0, 0)
  grp0_3:setOri(m3)

  --shaft_third turns -1.0000 per turn of the input
  local a=input*-1.000000
  local m4=ori0_4:clone()
  m4:mulRotateBA(a, 1, 0, 0)
  grp0_4:setOri(m4)

  --ring_1 turns with its shaft and sits engaged
  local a=input*-0.333333
  local rm0=rori0_0:clone()
  rm0:mulRotateBA(a, 1, 0, 0)
  ring0_0:setOri(rm0)
  ring0_0:setPos(30, 0, -40)

  --ring_2 turns with its shaft and sits clear of its gear
  local a=input*-0.333333
  local rm1=rori0_1:clone()
  rm1:mulRotateBA(a, 1, 0, 0)
  ring0_1:setOri(rm1)
  ring0_1:setPos(110, 0, -40)

  --ring_3 turns with its shaft and sits clear of its gear
  local a=input*-0.333333
  local rm2=rori0_2:clone()
  rm2:mulRotateBA(a, 1, 0, 0)
  ring0_2:setOri(rm2)
  ring0_2:setPos(180, 0, -40)
end

function onStart1()
  local sf=ldc.subfile()
  grp1_0=sf:getGroup("shaft_input")
  ori1_0=grp1_0:getOri()
  grp1_1=sf:getGroup("shaft_output")
  ori1_1=grp1_1:getOri()
  grp1_2=sf:getGroup("shaft_first")
  ori1_2=grp1_2:getOri()
  grp1_3=sf:getGroup("shaft_second")
  ori1_3=grp1_3:getOri()
  grp1_4=sf:getGroup("shaft_third")
  ori1_4=grp1_4:getOri()
  ring1_0=sf:getGroup("ring_1")
  rori1_0=ring1_0:getOri()
  ring1_1=sf:getGroup("ring_2")
  rori1_1=ring1_1:getOri()
  ring1_2=sf:getGroup("ring_3")
  rori1_2=ring1_2:getOri()
end

function onFrame1()
  local ani=ldc.animation.getCurrent()
  local t=ani:getFrameTime()/ani:getLength()
  local input=t*4*360 --degrees turned by the input

  --shaft_input turns 1.0000 per turn of the input
  local a=input*1.000000
  local m0=ori1_0:clone()
  m0:mulRotateBA(a, 1, 0, 0)
  grp1_0:setOri(m0)

  --shaft_output turns -0.6000 per turn of the input
  local a=input*-0.600000
  local m1=ori1_1:clone()
  m1:mulRotateBA(a, 1, 0, 0)
  grp1_1:setOri(m1)

  --shaft_first turns -0.3333 per turn of the input
  local a=input*-0.333333
  local m2=ori1_2:clone()
  m2:mulRotateBA(a, 1, 0, 0)
  grp1_2:setOri(m2)

  --shaft_second turns -0.6000 per turn of the input
  local a=input*-0.600000
  local m3=ori1_3:clone()
  m3:mulRotateBA(a, 1, 0, 0)
  grp1_3:setOri(m3)

  --shaft_third turns -1.0000 per turn of the input
  local a=input*-1.000000
  local m4=ori1_4:clone()
  m4:mulRotateBA(a, 1, 0, 0)
  grp1_4:setOri(m4)

  --ring_1 turns with its shaft and sits clear of its gear
  local a=input*-0.600000
  local rm0=rori1_0:clone()
  rm0:mulRotateBA(a, 1, 0, 0)
  ring1_0:setOri(rm0)
  ring1_0:setPos(40, 0, -40)

  --ring_2 turns with its shaft and sits engaged
  local a=input*-0.600000
  local rm1=rori1_1:clone()
  rm1:mulRotateBA(a, 1, 0, 0)
  ring1_1:setOri(rm1)
  ring1_1:setPos(100, 0, -40)

  --ring_3 turns with its shaft and sits clear of its gear
  local a=input*-0.600000
  local rm2=rori1_2:clone()
  rm2:mulRotateBA(a, 1, 0, 0)
  ring1_2:setOri(rm2)
  ring1_2:setPos(180, 0, -40)
end

function onStart2()
  local sf=ldc.subfile()
  grp2_0=sf:getGroup("shaft_input")
  ori2_0=grp2_0:getOri()
  grp2_1=sf:getGroup("shaft_output")
  ori2_1=grp2_1:getOri()
  grp2_2=sf:getGroup("shaft_first")
  ori2_2=grp2_2:getOri()
  grp2_3=sf:getGroup("shaft_second")
  ori2_3=grp2_3:getOri()
  grp2_4=sf:getGroup("shaft_third")
  ori2_4=grp2_4:getOri()
  ring2_0=sf:getGroup("ring_1")
  rori2_0=ring2_0:getOri()
  ring2_1=sf:getGroup("ring_2")
  rori2_1=ring2_1:getOri()
  ring2_2=sf:getGroup("ring_3")
  rori2_2=ring2_2:getOri()
end

function onFrame2()
  local ani=ldc.animation.getCurrent()
  local t=ani:getFrameTime()/ani:getLength()
  local input=t*4*360 --degrees turned by the input

  --shaft_input turns 1.0000 per turn of the input
  local a=input*1.000000
  local m0=ori2_0:clone()
  m0:mulRotateBA(a, 1, 0, 0)
  grp2_0:setOri(m0)

  --shaft_output turns -1.0000 per turn of the input
  local a=input*-1.000000
  local m1=ori2_1:clone()
  m1:mulRotateBA(a, 1, 0, 0)
  grp2_1:setOri(m1)

  --shaft_first turns -0.3333 per turn of the input
  local a=input*-0.333333
  local m2=ori2_2:clone()
  m2:mulRotateBA(a, 1, 0, 0)
  grp2_2:setOri(m2)

  --shaft_second turns -0.6000 per turn of the input
  local a=input*-0.600000
  local m3=ori2_3:clone()
  m3:mulRotateBA(a, 1, 0, 0)
  grp2_3:setOri(m3)

  --shaft_third turns -1.0000 per turn of the input
  local a=input*-1.000000
  local m4=ori2_4:clone()
  m4:mulRotateBA(a, 1, 0, 0)
  grp2_4:setOri(m4)

  --ring_1 turns with its shaft and sits clear of its gear
  local a=input*-1.000000
  local rm0=rori2_0:clone()
  rm0:mulRotateBA(a, 1, 0, 0)
  ring2_0:setOri(rm0)
  ring2_0:setPos(40, 0, -40)

  --ring_2 turns with its shaft and sits clear of its gear
  local a=input*-1.000000
  local rm1=rori2_1:clone()
  rm1:mulRotateBA(a, 1, 0, 0)
  ring2_1:setOri(rm1)
  ring2_1:setPos(110, 0, -40)

  --ring_3 turns with its shaft and sits engaged
  local a=input*-1.000000
  local rm2=rori2_2:clone()
  rm2:mulRotateBA(a, 1, 0, 0)
  ring2_2:setOri(rm2)
  ring2_2:setPos(170, 0, -40)
end

function onStart3()
  local sf=ldc.subfile()
  grp3_0=sf:getGroup("shaft_input")
  ori3_0=grp3_0:getOri()
  grp3_1=sf:getGroup("shaft_output")
  ori3_1=grp3_1:getOri()
  grp3_2=sf:getGroup("shaft_first")
  ori3_2=grp3_2:getOri()
  grp3_3=sf:getGroup("shaft_second")
  ori3_3=grp3_3:getOri()
  grp3_4=sf:getGroup("shaft_third")
  ori3_4=grp3_4:getOri()
  ring3_0=sf:getGroup("ring_1")
  rori3_0=ring3_0:getOri()
  ring3_1=sf:getGroup("ring_2")
  rori3_1=ring3_1:getOri()
  ring3_2=sf:getGroup("ring_3")
  rori3_2=ring3_2:getOri()
end

function onFrame3()
  local ani=ldc.animation.getCurrent()
  local t=ani:getFrameTime()/ani:getLength()
  local segs=3
  --how long each state is held, as a share of the animation
  local frac={0.333333, 0.333333, 0.333333}
  local seg,acc=0,0
  for k=1,segs do
    --The last state takes whatever is left, so t=1 lands in it rather than
    --running off the end of the table.
    if t<acc+frac[k] or k==segs then seg=k-1 break end
    acc=acc+frac[k]
  end
  local u=(t-acc)/frac[seg+1] --0..1 within this segment
  if u<0 then u=0 elseif u>1 then u=1 end
  local turns=4
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
    for k=1,seg do a=a+sp[k]*frac[k]*turns*360 end
    return a+sp[seg+1]*u*frac[seg+1]*turns*360
  end

  local a0=angle(speed[1])
  local m0=ori3_0:clone()
  m0:mulRotateBA(a0, 1, 0, 0)
  grp3_0:setOri(m0)
  local a1=angle(speed[2])
  local m1=ori3_1:clone()
  m1:mulRotateBA(a1, 1, 0, 0)
  grp3_1:setOri(m1)
  local a2=angle(speed[3])
  local m2=ori3_2:clone()
  m2:mulRotateBA(a2, 1, 0, 0)
  grp3_2:setOri(m2)
  local a3=angle(speed[4])
  local m3=ori3_3:clone()
  m3:mulRotateBA(a3, 1, 0, 0)
  grp3_3:setOri(m3)
  local a4=angle(speed[5])
  local m4=ori3_4:clone()
  m4:mulRotateBA(a4, 1, 0, 0)
  grp3_4:setOri(m4)

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
    local ra=angle(ringSpeed[1])
    local rm=rori3_0:clone()
    rm:mulRotateBA(ra, 1, 0, 0)
    ring3_0:setOri(rm)
    ring3_0:setPos(30+(10)*at, 0+(0)*at, -40+(0)*at)
  end
  do --ring_2
    local a=where[2][seg+1]
    local at=a+(where[2][nxt+1]-a)*f
    local ra=angle(ringSpeed[2])
    local rm=rori3_1:clone()
    rm:mulRotateBA(ra, 1, 0, 0)
    ring3_1:setOri(rm)
    ring3_1:setPos(100+(10)*at, 0+(0)*at, -40+(0)*at)
  end
  do --ring_3
    local a=where[3][seg+1]
    local at=a+(where[3][nxt+1]-a)*f
    local ra=angle(ringSpeed[3])
    local rm=rori3_2:clone()
    rm:mulRotateBA(ra, 1, 0, 0)
    ring3_2:setOri(rm)
    ring3_2:setPos(170+(10)*at, 0+(0)*at, -40+(0)*at)
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
