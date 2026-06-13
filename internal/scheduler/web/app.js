// くべくべ web UI: the operating pair lives entirely in the browser and falls
// through the grid like real puyo; only the final lock is sent to the
// scheduler, which binds the two Pods to nodes.
"use strict";

const SKY_ROWS = 2; // spawn area above the field

let state = null;
// pivot position in bottom-based grid coords; rot is the satellite side:
// 0 = above, 1 = right, 2 = below, 3 = left
let pair = { col: 0, row: 0, rot: 0, key: "" };
let prevAllClears = null;
let prevChain = 0;
let dropping = false;

const grid = document.getElementById("grid");
const nodesEl = document.getElementById("nodes");
const nextEl = document.getElementById("next");
const banner = document.getElementById("banner");
const message = document.getElementById("message");
const modeBtn = document.getElementById("modeBtn");
const tooltip = document.getElementById("tooltip");
const eventsEl = document.getElementById("events");

// ---- pod tooltip ----------------------------------------------------------

function attachTooltip(cell, pod) {
  cell.addEventListener("mouseenter", () => {
    tooltip.textContent = pod.manifest || pod.name;
    tooltip.classList.remove("hidden");
  });
  cell.addEventListener("mousemove", (e) => {
    tooltip.style.left = Math.min(e.clientX + 14, window.innerWidth - 380) + "px";
    tooltip.style.top = Math.min(e.clientY + 14, window.innerHeight - 240) + "px";
  });
  cell.addEventListener("mouseleave", () => tooltip.classList.add("hidden"));
}

// ---- pod event log (status transitions) -----------------------------------

let prevPods = null; // name -> {phase, node, color}

function snapshotPods(s) {
  const m = {};
  for (const col of s.columns) for (const p of col.pods) m[p.name] = p;
  for (const p of s.queue) m[p.name] = p;
  return m;
}

function phaseClass(phase) {
  return { Terminating: "terminating", Running: "running" }[phase] || "";
}

function logEvent(color, name, html) {
  const li = document.createElement("li");
  const t = new Date().toLocaleTimeString("ja-JP", { hour12: false });
  li.innerHTML =
    `<span class="dot ${color}"></span>` +
    `<span><span class="t">${t}</span> <b>${name}</b> ${html}</span>`;
  eventsEl.prepend(li);
  while (eventsEl.children.length > 30) eventsEl.removeChild(eventsEl.lastChild);
}

function logTransitions(cur) {
  if (prevPods === null) {
    prevPods = cur;
    return;
  }
  for (const [name, p] of Object.entries(cur)) {
    const old = prevPods[name];
    if (!old) {
      logEvent(p.color, name, `created <span class="${phaseClass(p.phase)}">${p.phase}</span>`);
      continue;
    }
    if (!old.node && p.node) {
      logEvent(p.color, name, `scheduled → ${p.node}`);
    }
    if (old.phase !== p.phase) {
      logEvent(p.color, name, `${old.phase} → <span class="${phaseClass(p.phase)}">${p.phase}</span>`);
    }
  }
  for (const [name, old] of Object.entries(prevPods)) {
    if (!cur[name]) {
      logEvent(old.color, name, `${old.phase} → <span class="deleted">deleted</span>`);
    }
  }
  prevPods = cur;
}

function cols() {
  return state.columns.length;
}
function rows() {
  return state.maxHeight + SKY_ROWS;
}
function stackHeight(c) {
  return state.columns[c].pods.length;
}

function currentPair() {
  return state.queue.filter((q) => !q.ojama).slice(0, 2);
}

function operable() {
  return (
    state &&
    state.mode === "manual" &&
    state.stable &&
    state.phase !== "GameOver" &&
    currentPair().length >= 2 &&
    cols() > 0 &&
    !dropping
  );
}

function satOffset(rot) {
  return [
    [0, 1],
    [1, 0],
    [0, -1],
    [-1, 0],
  ][rot];
}

function satPos(p) {
  const [dx, dy] = satOffset(p.rot);
  return [p.col + dx, p.row + dy];
}

function cellFree(c, r) {
  return c >= 0 && c < cols() && r >= 0 && r < rows() && r >= stackHeight(c);
}

function pairFits(p) {
  const [sc, sr] = satPos(p);
  return cellFree(p.col, p.row) && cellFree(sc, sr);
}

// reset the pair when a new one arrives, and lift it if the stacks grew
function syncPair() {
  const q = currentPair();
  if (q.length < 2) return;
  if (pair.key !== q[0].name) {
    pair = { col: Math.floor((cols() - 1) / 2), row: state.maxHeight, rot: 0, key: q[0].name };
  }
  let guard = 0;
  while (!pairFits(pair) && guard++ < rows()) pair.row++;
}

function tryMove(dx, dy) {
  const np = { ...pair, col: pair.col + dx, row: pair.row + dy };
  if (!pairFits(np)) return false;
  pair = np;
  return true;
}

// Rotation keeps the pivot in place; when the satellite is blocked the pair
// climbs a single-step ledge, kicks away from walls / taller stacks, or
// floats up off the floor — the usual puyo kick rules.
function tryRotate(dir) {
  const np = { ...pair, rot: (pair.rot + (dir > 0 ? 1 : 3)) % 4 };
  if (pairFits(np)) {
    pair = np;
    return;
  }
  const [sc] = satPos(np);
  if (np.rot === 1 || np.rot === 3) {
    // single-step ledge: climb on top of it
    const up = { ...np, row: np.row + 1 };
    if (sc >= 0 && sc < cols() && stackHeight(sc) === np.row + 1 && pairFits(up)) {
      pair = up;
      return;
    }
    // wall or a 2+ step: kick the pair away
    const kick = { ...np, col: np.col - (sc - np.col) };
    if (pairFits(kick)) {
      pair = kick;
      return;
    }
  } else if (np.rot === 2) {
    // rotating below while grounded: float up one row
    const up = { ...np, row: np.row + 1 };
    if (pairFits(up)) {
      pair = up;
      return;
    }
  } else if (np.rot === 0) {
    // rotating above while at the ceiling: push down one row
    const down = { ...np, row: np.row - 1 };
    if (pairFits(down)) {
      pair = down;
      return;
    }
  }
  // rotation blocked: keep the old orientation
}

function softDrop() {
  if (!tryMove(0, -1)) lock(); // grounded: another ↓ locks the pair
}

function hardDrop() {
  let guard = 0;
  while (tryMove(0, -1) && guard++ < rows()) {}
  lock();
}

// placements bottom-first, the way the scheduler stacks them
function placements() {
  const q = currentPair();
  const [sc] = satPos(pair);
  const pivot = { pod: q[0].name, node: state.columns[pair.col].node };
  const sat = { pod: q[1].name, node: state.columns[sc].node };
  return pair.rot === 2 ? [sat, pivot] : [pivot, sat];
}

function validDrop() {
  const heights = {};
  for (const c of state.columns) heights[c.node] = c.pods.length;
  for (const p of placements()) {
    if (++heights[p.node] > state.maxHeight) return false;
  }
  return true;
}

async function lock() {
  if (!operable() || !validDrop()) return;
  dropping = true;
  render();
  try {
    const res = await fetch("api/drop", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ placements: placements() }),
    });
    message.textContent = res.ok ? "" : await res.text();
  } catch (e) {
    message.textContent = String(e);
  }
  dropping = false;
}

async function toggleMode() {
  const mode = state && state.mode === "manual" ? "auto" : "manual";
  await fetch("api/mode", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mode }),
  });
}

function cellAt(cells, c, r) {
  // bottom-based (c, r) to the top-down grid
  return cells[(rows() - 1 - r) * cols() + c];
}

function render() {
  if (!state) return;

  document.getElementById("namespace").textContent = state.namespace;
  document.getElementById("chain").textContent = state.chain;
  document.getElementById("maxChain").textContent = state.maxChain;
  document.getElementById("totalErased").textContent = state.totalErased;
  document.getElementById("allClears").textContent = state.allClears;
  document.getElementById("phase").textContent = state.phase || "-";
  modeBtn.textContent = "mode: " + state.mode;

  grid.style.gridTemplateColumns = `repeat(${cols()}, 42px)`;
  nodesEl.style.gridTemplateColumns = `repeat(${cols()}, 42px)`;

  grid.innerHTML = "";
  const cells = [];
  for (let i = 0; i < rows() * cols(); i++) {
    const div = document.createElement("div");
    div.className = "cell" + (i < SKY_ROWS * cols() ? " sky" : "");
    grid.appendChild(div);
    cells.push(div);
  }

  state.columns.forEach((col, c) => {
    col.pods.forEach((p, r) => {
      if (r >= rows()) return;
      const cell = cellAt(cells, c, r);
      cell.classList.add(p.color);
      if (!p.stable) cell.classList.add("unstable");
      if (p.ojama) cell.classList.add("ojama");
      attachTooltip(cell, p);
    });
  });

  if (operable()) {
    syncPair();
    const q = currentPair();
    const [sc, sr] = satPos(pair);
    const pv = cellAt(cells, pair.col, pair.row);
    pv.classList.remove("sky");
    pv.classList.add(q[0].color, "ghost", "pivot");
    attachTooltip(pv, q[0]);
    const st = cellAt(cells, sc, sr);
    st.classList.remove("sky");
    st.classList.add(q[1].color, "ghost");
    attachTooltip(st, q[1]);
  }

  nodesEl.innerHTML = "";
  state.columns.forEach((col) => {
    const div = document.createElement("div");
    div.textContent = col.node;
    nodesEl.appendChild(div);
  });

  nextEl.innerHTML = "";
  state.queue.slice(2).forEach((q) => {
    const div = document.createElement("div");
    div.className = "cell " + q.color + (q.ojama ? " ojama" : "");
    attachTooltip(div, q);
    nextEl.appendChild(div);
  });

  if (state.phase === "GameOver") {
    showBanner("GAME OVER", "gameover", 0);
  } else if (prevAllClears !== null && state.allClears > prevAllClears) {
    showBanner("全消し!!", "allclear", 2500);
  } else if (state.chain > 1 && state.chain > prevChain) {
    showBanner(state.chain + "連鎖!", "chain", 1500);
  } else if (banner.classList.contains("gameover")) {
    banner.className = "hidden";
  }
  prevAllClears = state.allClears;
  prevChain = state.chain;
}

let bannerTimer = null;
function showBanner(text, cls, ms) {
  banner.textContent = text;
  banner.className = cls;
  clearTimeout(bannerTimer);
  if (ms > 0) bannerTimer = setTimeout(() => (banner.className = "hidden"), ms);
}

document.addEventListener("keydown", (e) => {
  if (!operable()) return;
  syncPair();
  switch (e.key) {
    case "ArrowLeft":
      tryMove(-1, 0);
      break;
    case "ArrowRight":
      tryMove(1, 0);
      break;
    case "z":
    case "Z":
      tryRotate(-1);
      break;
    case "x":
    case "X":
      tryRotate(1);
      break;
    case "ArrowDown":
      e.preventDefault();
      softDrop();
      break;
    case "ArrowUp":
    case " ":
      e.preventDefault();
      hardDrop();
      return;
    default:
      return;
  }
  e.preventDefault();
  render();
});

modeBtn.addEventListener("click", toggleMode);

function connect() {
  const es = new EventSource("api/events");
  es.onmessage = (ev) => {
    state = JSON.parse(ev.data);
    logTransitions(snapshotPods(state));
    render();
  };
  es.onerror = () => {
    es.close();
    message.textContent = "再接続中…";
    setTimeout(connect, 2000);
  };
}
connect();
