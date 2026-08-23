// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Sander Striker
//
// Drives the real page in a real browser.
//
// Everything else about the page is checked without one: the module's answers
// against the engine's, the worker's messages, the camera's arithmetic. None of
// that can link a shader. A vertex attribute that does not exist, a varying
// spelled two ways, a stride off by one — all of them compile fine in Go, pass
// the buffer checks, and produce a blank canvas in front of a person.
//
// So this loads the page over http, builds a model, and looks at the pixels.
import http from "node:http";
import fs from "node:fs";
import path from "node:path";
// Resolved rather than imported by name: ESM looks for node_modules beside the
// importing file, and this file lives in the repository while the browser may
// be installed anywhere. BRICKMESH_PLAYWRIGHT points at the package; without it
// the ordinary name is tried, which is what a repository-level install gives.
const { chromium } = await import(
  process.env.BRICKMESH_PLAYWRIGHT
    ? new URL("index.mjs", `file://${process.env.BRICKMESH_PLAYWRIGHT}/`).href
    : "playwright-core");

const [dir] = process.argv.slice(2);

const TYPES = {
  ".html": "text/html", ".js": "text/javascript", ".css": "text/css",
  ".wasm": "application/wasm", ".json": "application/json",
  ".bin": "application/octet-stream", ".txt": "text/plain",
};

const server = http.createServer((req, res) => {
  const rel = decodeURIComponent(req.url.split("?")[0]).replace(/^\/+/, "") || "index.html";
  const file = path.join(dir, rel);
  if (!file.startsWith(dir) || !fs.existsSync(file) || fs.statSync(file).isDirectory()) {
    res.writeHead(404).end("not found");
    return;
  }
  res.writeHead(200, { "content-type": TYPES[path.extname(file)] || "application/octet-stream" });
  fs.createReadStream(file).pipe(res);
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const base = `http://127.0.0.1:${server.address().port}/`;

const problems = [];
function must(ok, what) {
  console.log(`  ${ok ? "ok  " : "FAIL"}  ${what}`);
  if (!ok) problems.push(what);
}

// SwiftShader, because there is no GPU here and a headless browser without one
// gives no WebGL at all — which would make every check below pass vacuously.
const browser = await chromium.launch({
  args: ["--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader"],
});
const page = await browser.newPage({ viewport: { width: 900, height: 700 } });

const errors = [];
page.on("pageerror", (e) => errors.push(String(e)));
page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
page.on("requestfailed", (r) => errors.push(`request failed: ${r.url()}`));
page.on("response", (r) => {
  if (r.status() >= 400) errors.push(`${r.status()} ${r.url()}`);
});

await page.goto(base, { waitUntil: "load" });

// The context first: without it the rest says nothing.
const gl = await page.evaluate(() => {
  const c = document.createElement("canvas");
  const ctx = c.getContext("webgl");
  return ctx ? ctx.getParameter(ctx.VERSION) : null;
});
must(gl !== null, `WebGL is available (${gl})`);

// The shaders link. This is the thing nothing else can check.
// On a canvas of its own: a second context on the page's canvas would either
// hand back the first one or break it, and either way the answer would be about
// this harness rather than about the page.
const shader = await page.evaluate(() => {
  const c = document.createElement("canvas");
  const v = createViewer(c);
  return v ? "linked" : "createViewer returned null";
});
must(shader === "linked", `the viewer's shaders link (${shader})`);

await page.waitForFunction(() => typeof loadExample === "function", null, { timeout: 30000 });
await page.evaluate(() => loadExample("examples/reduction.json"));
await page.waitForFunction(
  () => document.querySelectorAll("#answer .finding").length > 0, null, { timeout: 60000 });

await page.click("#build");
await page.waitForFunction(
  () => !document.getElementById("view").hidden, null, { timeout: 180000 });

const painted = await canvasStats(page);
must(painted.colours > 1, `the canvas is painted, not blank (${painted.colours} distinct colours)`);

// A finding that knows its parts is clickable, and clicking it changes what is
// on the screen. That is the whole point of drawing a model here.
const clickable = await page.evaluate(() =>
  [...document.querySelectorAll("#answer .finding.points")].map((n) =>
    n.querySelector(".check").textContent));
must(clickable.length > 0, `a finding offers to point at parts (${clickable.join(", ")})`);

if (clickable.length > 0) {
  const before = await canvasStats(page);
  await page.evaluate(() => document.querySelector("#answer .finding.points").click());
  await page.waitForTimeout(1500);
  const after = await canvasStats(page);
  must(after.hash !== before.hash, "clicking a finding changes what is drawn");
  must(after.warm > before.warm,
    `the highlighted parts are warmer (${before.warm} -> ${after.warm} warm pixels)`);
}

// Zooming, every way it can be done.
//
// It used to be a wheel and only a wheel, with nothing on the page saying so
// and touch-action: none on the canvas — which on a touchscreen meant no zoom
// at all and no way to find that out. So each route is exercised rather than
// assumed, and the picture has to actually change.
must(await page.isVisible("#viewbar"), "the model's controls appear with it");

const view = page.locator("#view");
// Read fresh every time, never once. The canvas moves when the page's own
// layout changes — the view bar appearing with the model, or a row of example
// buttons wrapping to a second line — and a box read before that puts the
// pointer somewhere the canvas no longer is. Adding four examples was enough
// to make the wheel miss it.
const boxNow = async () => await view.boundingBox();
let box = await boxNow();
const frame = async () => {
  await page.waitForTimeout(600); // SwiftShader is not instant
  return (await view.screenshot()).toString("base64");
};

const home = await frame();
box = await boxNow();
await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
await page.mouse.wheel(0, -400);
must(await frame() !== home, "the wheel zooms");

let was = await frame();
await page.click("#zoom-in");
must(await frame() !== was, "the zoom-in button works");
was = await frame();
await page.click("#zoom-out");
must(await frame() !== was, "the zoom-out button works");

// Two fingers, dispatched as pointer events because that is what a touchscreen
// sends and what the viewer listens for.
was = await frame();
box = await boxNow();
await page.evaluate(({ cx, cy }) => {
  const el = document.getElementById("view");
  const send = (type, pts) => pts.forEach((p) => el.dispatchEvent(new PointerEvent(type, {
    pointerId: p.id, pointerType: "touch", clientX: p.x, clientY: p.y, bubbles: true,
  })));
  send("pointerdown", [{ id: 1, x: cx - 60, y: cy }, { id: 2, x: cx + 60, y: cy }]);
  send("pointermove", [{ id: 1, x: cx - 150, y: cy }, { id: 2, x: cx + 150, y: cy }]);
  send("pointerup", [{ id: 1, x: cx - 150, y: cy }, { id: 2, x: cx + 150, y: cy }]);
}, { cx: box.x + box.width / 2, cy: box.y + box.height / 2 });
must(await frame() !== was, "a two-finger pinch zooms");

// Keys, for anyone not using a pointer.
await view.focus();
was = await frame();
await page.keyboard.press("+");
must(await frame() !== was, "the + key zooms");

// And a way back, since it is easy to end up inside a gearbox. Asked as
// idempotence rather than against some frame captured earlier: what matters is
// that reset lands in the same place whatever was done to the view, and an
// earlier frame also carries whatever else has changed on the page since.
await page.click("#zoom-reset");
const rested = await frame();
box = await boxNow();
await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
await page.mouse.wheel(0, -500);
await page.mouse.move(box.x + 80, box.y + 80);
await page.mouse.down();
await page.mouse.move(box.x + 300, box.y + 220);
await page.mouse.up();
must(await frame() !== rested, "turning and zooming moves the view");
await page.click("#zoom-reset");
must(await frame() === rested, "reset lands in the same place every time");

must((await page.textContent("body")).toLowerCase().includes("zoom"),
  "the page says the model can be zoomed");

// The size bound, which is the one control that can refuse: ask for a frame
// inside two studs of depth and the answer must be that none was found, naming
// the cap, rather than the best violation of it handed back quietly.
await page.fill("#max-z", "2");
await page.click("#build");
await page.waitForFunction(
  () => [...document.querySelectorAll("#answer .finding .detail")]
    .some((n) => n.textContent.includes("envelope was capped")),
  null, { timeout: 180000 }).then(() => must(true, "a bound too small is refused, and says which"))
  .catch(() => must(false, "a frame was asked for inside two studs and nothing mentioned the cap"));

// The animation. A model that does not move is the failure this is here to
// catch: the transforms are computed on the page and applied in the shader, and
// either half can be silently wrong while everything still draws.
//
// Waiting for the BUILD BUTTON rather than for findings. The findings from the
// previous build are still on the page when this one starts, so waiting for
// them returns at once and everything after races a build that is still going —
// which is exactly what happened: the animation was started, and then the build
// finished and reset it.
await page.fill("#max-z", "");
await page.evaluate(() => loadExample("examples/gearbox-2-speed.json"));
await page.waitForFunction(
  () => document.querySelector("#spec").value.includes("2-speed"),
  null, { timeout: 30000 });
await page.click("#build");
await page.waitForFunction(
  () => document.getElementById("build").disabled, null, { timeout: 30000 });
await page.waitForFunction(
  () => !document.getElementById("build").disabled, null, { timeout: 180000 });

const canPlay = await page.evaluate(() => {
  const el = document.getElementById("play");
  return !!el && !el.hidden;
});
must(canPlay, "a gearbox offers to be played");

if (canPlay) {
  const still = await frame();
  await page.click("#play");
  await page.waitForTimeout(700);
  must(await frame() !== still, "playing moves the model");

  await page.click("#play"); // pause
  const held = await frame();
  await page.waitForTimeout(500);
  must(await frame() === held, "pausing stops it where it is");

  // Choosing a state moves the ring, without anything having to play.
  const states = await page.evaluate(() =>
    [...document.querySelectorAll("#state option")].map((o) => o.value));
  must(states.length >= 2, `a two-speed offers its states (${states.join(", ")})`);
  if (states.length >= 2) {
    await page.selectOption("#state", states[0]);
    const first = await frame();
    await page.selectOption("#state", states[1]);
    must(await frame() !== first, "changing state moves the driving ring");
  }

  // The shift walks the states rather than holding one, so the ring is in a
  // different place at different points of it — which a steady state, however
  // it turns, never is. Sampled with the animation paused, so what moves is the
  // walk and not the clock.
  if (states.includes("shift")) {
    await page.selectOption("#state", "shift");
    const seen = new Set();
    for (const at of [0, 0.3, 0.62, 0.95]) {
      await page.evaluate((t) => showFrame(t * 4), at); // 4 input turns across
      seen.add(await frame());
    }
    must(seen.size >= 3,
      `the shift walks its states (${seen.size} distinct pictures of 4)`);
  }
}

must(errors.length === 0, `no page errors${errors.length ? ": " + errors.join(" | ") : ""}`);

await browser.close();
server.close();

if (problems.length) {
  console.log(`\n${problems.length} problem(s) in the browser`);
  process.exit(1);
}
console.log("\nthe page works in a browser");

// canvasStats reads the pixels back: how varied they are, how many lean warm,
// and a hash so two frames can be compared.
async function canvasStats(page) {
  return page.evaluate(() => {
    const c = document.getElementById("view");
    const gl = c.getContext("webgl");
    // Drawn and read in one go. Without preserveDrawingBuffer the contents are
    // gone once the frame is composited, so reading later gives a blank canvas
    // and every check below would fail for a reason that is not the page's.
    viewer.draw();
    const px = new Uint8Array(c.width * c.height * 4);
    gl.readPixels(0, 0, c.width, c.height, gl.RGBA, gl.UNSIGNED_BYTE, px);
    const seen = new Set();
    let warm = 0, hash = 0;
    for (let i = 0; i < px.length; i += 4) {
      const r = px[i], g = px[i + 1], b = px[i + 2];
      seen.add((r >> 4 << 8) | (g >> 4 << 4) | (b >> 4));
      // Warm meaning red-dominant, which is what a highlighted part is pulled
      // towards. The page's own background is blue-grey, so this does not count
      // the sky.
      if (r > 110 && r > g + 40 && r > b + 40) warm++;
      hash = (hash * 31 + r + g * 3 + b * 7) | 0;
    }
    return { colours: seen.size, warm, hash };
  });
}
