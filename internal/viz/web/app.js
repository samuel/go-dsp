// The page knows nothing about filters. It asks /api/filters what exists,
// builds a control per parameter, and draws whatever curves come back from
// /api/analyze -- so adding a filter kind to the Go catalog adds it here with
// no change to this file.
'use strict';

const CHARTS = ['magnitude', 'phase', 'groupdelay', 'impulse', 'step', 'tone'];

let kinds = [];
let current = null;      // {kind, params, choices}
let inFlight = null;     // AbortController for the request being replaced
let timer = null;

const $ = (id) => document.getElementById(id);

init().catch((e) => { $('summary').textContent = 'could not start: ' + e.message; });

async function init() {
  const res = await fetch('/api/filters');
  if (!res.ok) throw new Error(await res.text());
  kinds = (await res.json()).kinds;

  const sel = $('kind');
  for (const k of kinds) {
    const o = document.createElement('option');
    o.value = k.name;
    o.textContent = k.label;
    sel.append(o);
  }
  sel.addEventListener('change', () => selectKind(sel.value));

  buildCharts();
  buildAdvanced();
  selectKind(kinds[0].name);
}

function buildCharts() {
  const host = $('charts');
  for (const name of CHARTS) {
    const fig = document.createElement('figure');
    fig.id = 'fig-' + name;
    fig.hidden = true;
    const cap = document.createElement('figcaption');
    cap.innerHTML = '<span></span>';
    const dl = document.createElement('a');
    dl.textContent = 'SVG';
    dl.download = '';
    cap.append(dl);
    const cv = document.createElement('canvas');
    fig.append(cap, cv);
    host.append(fig);
  }
  // The sweep is a raster the server draws, not a curve: it is a whole
  // spectrogram, and sending it as numbers would be megabytes of JSON to
  // redraw something the browser can decode for free.
  const fig = document.createElement('figure');
  fig.id = 'fig-sweep';
  const cap = document.createElement('figcaption');
  cap.innerHTML = '<span>Sweep spectrogram</span>';
  const img = document.createElement('img');
  img.alt = 'sweep spectrogram';
  fig.append(cap, img);
  host.append(fig);
}

// The measurement controls are the same for every filter, so they are built
// once rather than per kind.
const ADVANCED = [
  { name: 'points', label: 'Grid points', min: 128, max: 4096, step: 128, value: 1024 },
  { name: 'frame', label: 'Measurement frame', min: 1024, max: 65536, step: 1024, value: 16384 },
  { name: 'taps', label: 'Samples drawn', min: 16, max: 1024, step: 16, value: 128 },
  { name: 'toneHz', label: 'Tone (Hz)', min: 20, max: 20000, step: 1, value: 997 },
  { name: 'seconds', label: 'Sweep seconds', min: 1, max: 20, step: 0.5, value: 4 },
];
const advanced = {};

function buildAdvanced() {
  const host = $('advanced');
  for (const a of ADVANCED) {
    advanced[a.name] = a.value;
    host.append(slider(a, (v) => { advanced[a.name] = v; schedule(); }));
  }
}

function slider(p, onChange) {
  const label = document.createElement('label');
  label.textContent = p.label + (p.unit ? ' (' + p.unit + ')' : '');
  const row = document.createElement('div');
  row.className = 'row';

  const range = document.createElement('input');
  range.type = 'range';
  const num = document.createElement('input');
  num.type = 'number';

  // A parameter whose useful range spans decades gets a slider that does too,
  // or the bottom two decades are crushed into the first few pixels.
  const log = !!p.log && p.min > 0;
  const toSlider = (v) => (log ? Math.log(v) : v);
  const fromSlider = (v) => (log ? Math.exp(v) : v);

  range.min = toSlider(p.min);
  range.max = toSlider(p.max);
  range.step = log ? (toSlider(p.max) - toSlider(p.min)) / 500 : p.step || 'any';
  range.value = toSlider(p.value ?? p.default);

  num.min = p.min;
  num.max = p.max;
  num.step = p.step || 'any';
  num.value = round(p.value ?? p.default);

  range.addEventListener('input', () => {
    const v = clamp(fromSlider(+range.value), p.min, p.max);
    num.value = round(v);
    onChange(v);
  });
  num.addEventListener('input', () => {
    const v = clamp(+num.value, p.min, p.max);
    if (!Number.isFinite(v)) return;
    range.value = toSlider(v);
    onChange(v);
  });

  row.append(range, num);
  label.append(row);
  return label;
}

const round = (v) => (Math.abs(v) >= 100 ? Math.round(v) : +v.toPrecision(5));
const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

function selectKind(name) {
  const k = kinds.find((x) => x.name === name);
  if (!k) return;
  current = { kind: name, params: {}, choices: {} };

  const host = $('controls');
  host.textContent = '';

  for (const c of k.choices || []) {
    current.choices[c.name] = c.default;
    const label = document.createElement('label');
    label.textContent = c.label;
    const sel = document.createElement('select');
    for (const o of c.options) {
      const opt = document.createElement('option');
      opt.value = o;
      opt.textContent = o;
      sel.append(opt);
    }
    sel.value = c.default;
    sel.addEventListener('change', () => { current.choices[c.name] = sel.value; schedule(); });
    label.append(sel);
    host.append(label);
  }

  for (const p of k.params || []) {
    current.params[p.name] = p.default;
    host.append(slider(p, (v) => { current.params[p.name] = v; schedule(); }));
  }
  schedule(0);
}

// Dragging a slider fires continuously. Debouncing keeps the server from being
// asked for a hundred measurements a second, and aborting the request already
// in flight keeps the answers from arriving out of order -- without that, the
// picture ends up showing whichever measurement happened to finish last rather
// than the one for the current parameters.
function schedule(delay = 60) {
  clearTimeout(timer);
  timer = setTimeout(run, delay);
}

async function run() {
  if (inFlight) inFlight.abort();
  const ctrl = new AbortController();
  inFlight = ctrl;
  document.body.classList.add('busy');

  const body = {
    kind: current.kind,
    params: current.params,
    choices: current.choices,
    points: Math.round(advanced.points),
    frame: Math.round(advanced.frame),
    taps: Math.round(advanced.taps),
    toneHz: advanced.toneHz,
  };

  try {
    const res = await fetch('/api/analyze', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: ctrl.signal,
    });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || res.statusText);
    render(data);
    $('fig-sweep').querySelector('img').src = sweepURL();
  } catch (e) {
    if (e.name === 'AbortError') return;
    $('summary').textContent = 'error: ' + e.message;
  } finally {
    if (inFlight === ctrl) {
      inFlight = null;
      document.body.classList.remove('busy');
    }
  }
}

function query(extra) {
  const q = new URLSearchParams({ kind: current.kind, ...extra });
  for (const [k, v] of Object.entries(current.params)) q.set(k, v);
  for (const [k, v] of Object.entries(current.choices)) q.set(k, v);
  return q;
}

function sweepURL() {
  const q = query({
    seconds: advanced.seconds,
    w: 900, h: 300, stft: 1024, palette: 'heat',
  });
  return '/api/sweep.png?' + q.toString();
}

function render(data) {
  const m = data.metrics;
  const bits = [`${fmt(m.inRate)} Hz in, ${fmt(m.outRate)} Hz out, measured ${m.method}`];
  if (m.cutoffHz > 0) bits.push(`cutoff ${m.cutoffHz.toFixed(1)} Hz`);
  if (m.transitionHz > 0) bits.push(`transition ${m.transitionHz.toFixed(0)} Hz`);
  bits.push(`passband ${m.passbandRippleDB.toFixed(3)} dB p-p`);
  if (m.stopbandPeakDB < 0) bits.push(`stopband ${m.stopbandPeakDB.toFixed(1)} dB`);
  bits.push(`delay ${m.groupDelaySamples.toFixed(2)} samples`);
  if (m.sfdrDB !== undefined) bits.push(`SFDR ${m.sfdrDB.toFixed(1)} dB`);
  $('summary').textContent = bits.join('   ');

  const warn = $('warnings');
  warn.textContent = '';
  for (const w of data.warnings || []) {
    const s = document.createElement('span');
    s.textContent = w;
    warn.append(s);
  }

  for (const name of CHARTS) {
    const fig = $('fig-' + name);
    const c = data.curves[name];
    fig.hidden = !c;
    if (!c) continue;
    fig.querySelector('figcaption span').textContent = c.title;
    const a = fig.querySelector('a');
    a.href = '/api/chart.svg?' + query({ chart: name }).toString();
    a.download = current.kind + '-' + name + '.svg';
    draw(fig.querySelector('canvas'), c);
  }
}

const fmt = (v) => (v >= 1000 ? (v / 1000).toFixed(v % 1000 ? 1 : 0) + 'k' : String(v));

// One function draws every curve. It is the only non-trivial thing here: axis
// mapping, ticks, and a polyline, on a canvas scaled for the display's pixel
// density so the lines are not soft on a retina screen.
function draw(canvas, c) {
  const cssW = canvas.clientWidth || 440;
  const cssH = Math.round(cssW * 0.52);
  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.round(cssW * dpr);
  canvas.height = Math.round(cssH * dpr);
  canvas.style.height = cssH + 'px';

  const s = getComputedStyle(document.body);
  const fg = s.getPropertyValue('--fg').trim() || '#111';
  const muted = s.getPropertyValue('--muted').trim() || '#777';
  const line = s.getPropertyValue('--line').trim() || '#ddd';
  const accent = s.getPropertyValue('--accent').trim() || '#1f77b4';

  const ctx = canvas.getContext('2d');
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, cssW, cssH);
  ctx.font = '10px ui-monospace, SFMono-Regular, Menlo, monospace';

  const L = 52, R = 10, T = 10, B = 28;
  const w = cssW - L - R, h = cssH - T - B;
  if (w <= 0 || h <= 0) return;

  const finite = (v) => Number.isFinite(v) && Math.abs(v) < 1e299;
  let xs = c.x, ys = c.y;
  let xlo = Infinity, xhi = -Infinity, ylo = Infinity, yhi = -Infinity;
  for (let i = 0; i < xs.length; i++) {
    if (c.xLog && !(xs[i] > 0)) continue;
    if (finite(xs[i])) { xlo = Math.min(xlo, xs[i]); xhi = Math.max(xhi, xs[i]); }
    if (finite(ys[i])) { ylo = Math.min(ylo, ys[i]); yhi = Math.max(yhi, ys[i]); }
  }
  if (c.yMin !== undefined && c.yMin !== null) ylo = c.yMin;
  if (c.yMax !== undefined && c.yMax !== null) yhi = c.yMax;
  if (!(xhi > xlo)) { xlo -= 1; xhi += 1; }
  if (!(yhi > ylo)) { ylo -= 1; yhi += 1; } else { const p = (yhi - ylo) * 0.06; ylo -= p; yhi += p; }

  const lx = c.xLog ? Math.log10(xlo) : xlo, hx = c.xLog ? Math.log10(xhi) : xhi;
  const px = (v) => L + ((c.xLog ? Math.log10(v) : v) - lx) / (hx - lx) * w;
  const py = (v) => T + h - (v - ylo) / (yhi - ylo) * h;

  ctx.strokeStyle = line;
  ctx.fillStyle = muted;
  ctx.lineWidth = 1;
  ctx.textAlign = 'right';
  ctx.textBaseline = 'middle';
  for (const t of niceTicks(ylo, yhi, 5)) {
    const y = Math.round(py(t)) + 0.5;
    if (y < T || y > T + h) continue;
    ctx.beginPath(); ctx.moveTo(L, y); ctx.lineTo(L + w, y); ctx.stroke();
    ctx.fillText(fmtTick(t, yhi - ylo), L - 5, y);
  }
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  const xt = c.xLog ? logTicks(xlo, xhi) : niceTicks(xlo, xhi, 5);
  for (const t of xt) {
    const x = Math.round(px(t)) + 0.5;
    if (x < L || x > L + w) continue;
    ctx.beginPath(); ctx.moveTo(x, T); ctx.lineTo(x, T + h); ctx.stroke();
    ctx.fillText(c.xLog ? fmt(t) : fmtTick(t, xhi - xlo), x, T + h + 5);
  }

  ctx.save();
  ctx.beginPath(); ctx.rect(L, T, w, h); ctx.clip();
  ctx.strokeStyle = accent;
  ctx.lineWidth = 1.4;
  ctx.beginPath();
  let pen = false;
  for (let i = 0; i < xs.length; i++) {
    const xv = xs[i], yv = ys[i];
    if ((c.xLog && !(xv > 0)) || !Number.isFinite(xv) || !Number.isFinite(yv)) { pen = false; continue; }
    const X = px(xv), Y = py(Math.max(ylo - (yhi - ylo), Math.min(yhi + (yhi - ylo), yv)));
    if (pen) ctx.lineTo(X, Y); else { ctx.moveTo(X, Y); pen = true; }
  }
  ctx.stroke();
  ctx.restore();

  ctx.strokeStyle = muted;
  ctx.strokeRect(L + 0.5, T + 0.5, w, h);
  ctx.fillStyle = fg;
  ctx.textAlign = 'left';
  ctx.textBaseline = 'top';
  ctx.fillText(c.yLabel, 3, 2);
}

function niceTicks(lo, hi, want) {
  if (!(hi > lo)) return [lo];
  const raw = (hi - lo) / want;
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const n = raw / mag;
  const step = (n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10) * mag;
  const out = [];
  for (let v = Math.ceil(lo / step) * step; v <= hi + step * 1e-9 && out.length < 40; v += step) {
    out.push(Math.abs(v) < step * 1e-9 ? 0 : v);
  }
  return out;
}

function logTicks(lo, hi) {
  const out = [];
  for (let d = Math.floor(Math.log10(lo)); d <= Math.ceil(Math.log10(hi)); d++) {
    for (const m of [1, 2, 5]) {
      const v = m * Math.pow(10, d);
      if (v >= lo && v <= hi) out.push(v);
    }
  }
  return out;
}

function fmtTick(v, span) {
  const d = Math.max(0, -Math.floor(Math.log10(span / 5)));
  return d > 5 ? v.toExponential(1) : v.toFixed(Math.min(d, 5));
}
