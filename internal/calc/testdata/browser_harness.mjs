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
