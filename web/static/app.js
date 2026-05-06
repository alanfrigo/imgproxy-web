// imgproxy-web UI: vanilla JS, no build step.
//
// Loads the option catalog from /api/options, generates a form, lives in
// localStorage for presets/history, mirrors state to location.hash so links
// share configuration, and renders a sticky live preview that calls
// /api/preview on every change (debounced, cancellable).

const $ = (sel) => document.querySelector(sel);
const PRESETS_KEY = "imgproxy-web:presets";
const HISTORY_KEY = "imgproxy-web:history";
const HISTORY_MAX = 20;

const state = {
  files: [],          // {file: File, outputName: string, dragKey: string}
  catalog: null,
  values: {},         // option key → raw value
  filenameTemplate: "{name}.{ext}",
  raw: "",
  previewIndex: 0,
  presets: {},
  history: [],
};

const debounce = (fn, ms) => {
  let t;
  return (...args) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms); };
};

(async function init() {
  state.presets = readJSON(PRESETS_KEY, {});
  state.history = readJSON(HISTORY_KEY, []);
  await pingHealth();
  await loadCatalog();
  bindSources();
  bindForm();
  bindActions();
  bindTopbar();
  applyHashState();          // restore from URL if present
  refreshPresetSelect();
  refreshPreview();
  schedulePreview();
})();

// --- Health + catalog ----------------------------------------------------

async function pingHealth() {
  const el = $("#health");
  try {
    const r = await fetch("/api/healthz");
    const j = await r.json();
    if (j.upstream === "ok") {
      el.textContent = "imgproxy ok";
      el.classList.add("ok");
    } else {
      el.textContent = "upstream: " + j.upstream;
      el.classList.add("bad");
    }
  } catch (e) {
    el.textContent = "sidecar offline";
    el.classList.add("bad");
  }
}

async function loadCatalog() {
  const r = await fetch("/api/options");
  state.catalog = await r.json();
  renderForm();
}

// --- Form rendering (same as v1, generated from schema) ------------------

function renderForm() {
  const root = $("#groups");
  root.replaceChildren();
  for (const g of state.catalog.groups) {
    const det = document.createElement("details");
    det.className = "group";
    if (["output", "resize"].includes(g.id)) det.open = true;
    const sum = document.createElement("summary");
    sum.textContent = g.label;
    det.appendChild(sum);
    if (g.description) {
      const p = document.createElement("p");
      p.className = "help muted";
      p.textContent = g.description;
      det.appendChild(p);
    }
    const fields = document.createElement("div");
    fields.className = "fields";
    for (const opt of g.options) fields.appendChild(renderField(opt));
    det.appendChild(fields);
    root.appendChild(det);
  }
}

function renderField(opt) {
  const wrap = document.createElement("div");
  wrap.className = "field" + (isCompound(opt.control) ? " compound" : "");
  const label = document.createElement("label");
  label.textContent = opt.label;
  wrap.appendChild(label);

  let input;
  switch (opt.control) {
    case "number":   input = scalarInput(opt, "number"); break;
    case "text":     input = scalarInput(opt, "text"); break;
    case "color":    input = scalarInput(opt, "color"); break;
    case "bool":     input = boolInput(opt); break;
    case "select":   input = selectInput(opt); break;
    case "list":     input = listInput(opt); break;
    case "pairs":
    case "fmt_qual": input = pairsInput(opt); break;
    case "gravity":  input = gravityInput(opt); break;
    case "crop":     input = cropInput(opt); break;
    case "extend":   input = extendInput(opt); break;
    case "trim":     input = trimInput(opt); break;
    case "padding":  input = paddingInput(opt); break;
    case "flip":     input = flipInput(opt); break;
    case "watermark": input = watermarkInput(opt); break;
    case "filename": input = scalarInput(opt, "text"); break;
    default:         input = scalarInput(opt, "text");
  }
  input.dataset.optKey = opt.key;
  wrap.appendChild(input);
  if (opt.description) {
    const d = document.createElement("div");
    d.className = "desc";
    d.textContent = opt.description;
    wrap.appendChild(d);
  }
  return wrap;
}

function isCompound(c) {
  return ["gravity", "crop", "extend", "trim", "padding", "watermark", "pairs", "fmt_qual"].includes(c);
}

function scalarInput(opt, type) {
  const i = document.createElement("input");
  i.type = type;
  if (opt.placeholder) i.placeholder = opt.placeholder;
  if (opt.default) i.placeholder = (opt.placeholder ? opt.placeholder + " · " : "") + "default: " + opt.default;
  if (opt.min != null) i.min = opt.min;
  if (opt.max != null) i.max = opt.max;
  if (opt.step != null) i.step = opt.step;
  i.addEventListener("input", () => onChange(opt.key, i.value));
  return i;
}

function boolInput(opt) {
  const wrap = document.createElement("div");
  wrap.className = "row";
  const i = document.createElement("input");
  i.type = "checkbox";
  i.addEventListener("change", () => onChange(opt.key, i.checked));
  wrap.appendChild(i);
  return wrap;
}

function selectInput(opt) {
  const s = document.createElement("select");
  for (const c of opt.choices || []) {
    const o = document.createElement("option");
    o.value = c.value;
    o.textContent = c.label;
    s.appendChild(o);
  }
  if (opt.default) s.value = opt.default;
  s.addEventListener("change", () => onChange(opt.key, s.value));
  return s;
}

function listInput(opt) {
  const i = document.createElement("input");
  i.type = "text";
  i.placeholder = "comma- or space-separated";
  i.addEventListener("input", () => {
    const arr = i.value.split(/[,\s]+/).map(s => s.trim()).filter(Boolean);
    onChange(opt.key, arr);
  });
  return i;
}

function pairsInput(opt) {
  const wrap = document.createElement("div");
  wrap.className = "compound-grid";
  const i = document.createElement("input");
  i.type = "text";
  i.placeholder = "jpeg:90, webp:80";
  i.addEventListener("input", () => {
    const pairs = i.value.split(",").map(p => p.trim()).filter(Boolean).map(p => {
      const [f, q] = p.split(":").map(s => s.trim());
      return { format: f, quality: q };
    });
    onChange(opt.key, pairs);
  });
  wrap.appendChild(i);
  return wrap;
}

function gravityInput(opt) {
  const wrap = document.createElement("div");
  wrap.className = "compound-grid";
  const sel = document.createElement("select");
  for (const c of state.catalog.gravities) {
    const o = document.createElement("option");
    o.value = c.value; o.textContent = c.label;
    sel.appendChild(o);
  }
  const x = numInput("x"); x.step = "0.01";
  const y = numInput("y"); y.step = "0.01";
  const update = () => {
    const v = { type: sel.value, x: x.value, y: y.value };
    onChange(opt.key, v.type ? v : null);
  };
  [sel, x, y].forEach(el => el.addEventListener("input", update));
  wrap.append(sel, x, y);
  return wrap;
}

function cropInput(opt) {
  const wrap = document.createElement("div");
  wrap.className = "compound-grid";
  const w = numInput("width");
  const h = numInput("height");
  const gsel = document.createElement("select");
  for (const c of state.catalog.gravities) {
    const o = document.createElement("option");
    o.value = c.value; o.textContent = c.label;
    gsel.appendChild(o);
  }
  const gx = numInput("gx");
  const gy = numInput("gy");
  const update = () => {
    if (!w.value && !h.value) { onChange(opt.key, null); return; }
    onChange(opt.key, {
      width: w.value, height: h.value,
      gravity: gsel.value ? { type: gsel.value, x: gx.value, y: gy.value } : null,
    });
  };
  [w, h, gsel, gx, gy].forEach(el => el.addEventListener("input", update));
  wrap.append(w, h, gsel, gx, gy);
  return wrap;
}

function extendInput(opt) {
  const wrap = document.createElement("div");
  wrap.className = "compound-grid";
  const enabled = document.createElement("input");
  enabled.type = "checkbox";
  const lab = document.createElement("label");
  lab.style.display = "flex"; lab.style.gap = "4px";
  lab.append(enabled, document.createTextNode("enabled"));
  const gsel = document.createElement("select");
  for (const c of state.catalog.gravities) {
    const o = document.createElement("option");
    o.value = c.value; o.textContent = c.label;
    gsel.appendChild(o);
  }
  const gx = numInput("x");
  const gy = numInput("y");
  const update = () => {
    if (!enabled.checked) { onChange(opt.key, null); return; }
    onChange(opt.key, {
      enabled: true,
      gravity: gsel.value ? { type: gsel.value, x: gx.value, y: gy.value } : null,
    });
  };
  [enabled, gsel, gx, gy].forEach(el => el.addEventListener("input", update));
  wrap.append(lab, gsel, gx, gy);
  return wrap;
}

function trimInput(opt) {
  const wrap = document.createElement("div");
  wrap.className = "compound-grid";
  const thr = numInput("threshold");
  const color = document.createElement("input");
  color.type = "color";
  const eh = checkboxLabel("equal H");
  const ev = checkboxLabel("equal V");
  const update = () => {
    if (!thr.value) { onChange(opt.key, null); return; }
    onChange(opt.key, {
      threshold: thr.value,
      color: color.value && color.value !== "#000000" ? color.value : "",
      equal_h: eh.input.checked,
      equal_v: ev.input.checked,
    });
  };
  [thr, color, eh.input, ev.input].forEach(el => el.addEventListener("input", update));
  wrap.append(thr, color, eh.wrap, ev.wrap);
  return wrap;
}

function paddingInput(opt) {
  const wrap = document.createElement("div");
  wrap.className = "compound-grid";
  const t = numInput("top"), r = numInput("right"), b = numInput("bottom"), l = numInput("left");
  const update = () => {
    if ([t, r, b, l].every(el => !el.value)) { onChange(opt.key, null); return; }
    onChange(opt.key, [Number(t.value || 0), Number(r.value || 0), Number(b.value || 0), Number(l.value || 0)]);
  };
  [t, r, b, l].forEach(el => el.addEventListener("input", update));
  wrap.append(t, r, b, l);
  return wrap;
}

function flipInput(opt) {
  const sel = document.createElement("select");
  [["", "(none)"], ["horizontal", "Horizontal"], ["vertical", "Vertical"], ["both", "Both"]].forEach(([v, l]) => {
    const o = document.createElement("option"); o.value = v; o.textContent = l;
    sel.appendChild(o);
  });
  sel.addEventListener("change", () => onChange(opt.key, sel.value));
  return sel;
}

function watermarkInput(opt) {
  const wrap = document.createElement("div");
  wrap.className = "compound-grid";
  const op = numInput("opacity"); op.step = "0.05"; op.min = "0"; op.max = "1";
  const gsel = document.createElement("select");
  for (const c of state.catalog.gravities) {
    const o = document.createElement("option"); o.value = c.value; o.textContent = c.label;
    gsel.appendChild(o);
  }
  const gx = numInput("x");
  const gy = numInput("y");
  const sc = numInput("scale"); sc.step = "0.05";
  const update = () => {
    if (!op.value) { onChange(opt.key, null); return; }
    onChange(opt.key, {
      opacity: op.value,
      gravity: gsel.value,
      x: gx.value, y: gy.value, scale: sc.value,
    });
  };
  [op, gsel, gx, gy, sc].forEach(el => el.addEventListener("input", update));
  wrap.append(op, gsel, gx, gy, sc);
  return wrap;
}

function numInput(placeholder) {
  const i = document.createElement("input");
  i.type = "number"; i.placeholder = placeholder; i.step = "any";
  return i;
}

function checkboxLabel(text) {
  const wrap = document.createElement("label");
  wrap.style.display = "flex"; wrap.style.gap = "4px"; wrap.style.alignItems = "center";
  const input = document.createElement("input");
  input.type = "checkbox";
  wrap.append(input, document.createTextNode(text));
  return { wrap, input };
}

function onChange(key, val) {
  if (val === "" || val === null || val === undefined) {
    delete state.values[key];
  } else {
    state.values[key] = val;
  }
  refreshPreview();
  syncHash();
  schedulePreview();
}

// --- Sources -------------------------------------------------------------

function bindSources() {
  const dz = $("#dropzone");
  const picker = $("#picker");
  const inp = $("#files");
  picker.addEventListener("click", () => inp.click());
  inp.addEventListener("change", () => addFiles(Array.from(inp.files)));
  dz.addEventListener("dragover", (e) => { e.preventDefault(); dz.classList.add("drag"); });
  dz.addEventListener("dragleave", () => dz.classList.remove("drag"));
  dz.addEventListener("drop", (e) => {
    e.preventDefault(); dz.classList.remove("drag");
    addFiles(Array.from(e.dataTransfer.files));
  });
  $("#urls").addEventListener("input", () => {
    refreshPreview();
    schedulePreview();
  });
  $("#filename-template").addEventListener("input", (e) => {
    state.filenameTemplate = e.target.value;
    syncHash();
    refreshPreview();
  });
}

function addFiles(arr) {
  for (const f of arr) {
    state.files.push({ file: f, outputName: defaultOutputName(f.name), dragKey: cryptoRandom() });
  }
  redrawFileList();
  refreshPreviewSampleOptions();
  refreshPreview();
  schedulePreview();
}

function defaultOutputName(srcName) {
  const dot = srcName.lastIndexOf(".");
  return dot > 0 ? srcName.slice(0, dot) : srcName;
}

function cryptoRandom() {
  const a = new Uint8Array(8);
  crypto.getRandomValues(a);
  return [...a].map(b => b.toString(16).padStart(2, "0")).join("");
}

function redrawFileList() {
  const list = $("#filelist");
  list.replaceChildren();
  state.files.forEach((entry, i) => {
    const card = document.createElement("div");
    card.className = "file-card";
    card.draggable = true;
    card.dataset.idx = i;

    const handle = document.createElement("span");
    handle.className = "file-handle";
    handle.textContent = "⠿";
    card.appendChild(handle);

    const thumb = document.createElement("img");
    thumb.className = "file-thumb";
    thumb.src = URL.createObjectURL(entry.file);
    thumb.alt = "";
    card.appendChild(thumb);

    const meta = document.createElement("div");
    meta.className = "file-name";
    const top = document.createElement("div");
    top.textContent = entry.file.name;
    const sub = document.createElement("div");
    sub.className = "file-meta";
    sub.textContent = humanBytes(entry.file.size);
    meta.append(top, sub);
    card.appendChild(meta);

    const ren = document.createElement("input");
    ren.className = "file-rename";
    ren.value = entry.outputName;
    ren.placeholder = "output stem";
    ren.addEventListener("input", () => {
      state.files[i].outputName = ren.value;
    });
    card.appendChild(ren);

    const rm = document.createElement("span");
    rm.className = "file-rm";
    rm.textContent = "✕";
    rm.title = "remove";
    rm.addEventListener("click", () => {
      state.files.splice(i, 1);
      redrawFileList();
      refreshPreviewSampleOptions();
      refreshPreview();
      schedulePreview();
    });
    card.appendChild(rm);

    card.addEventListener("dragstart", (e) => {
      card.classList.add("dragging");
      e.dataTransfer.setData("text/plain", String(i));
      e.dataTransfer.effectAllowed = "move";
    });
    card.addEventListener("dragend", () => card.classList.remove("dragging"));
    card.addEventListener("dragover", (e) => {
      e.preventDefault();
      card.classList.add("dragover");
    });
    card.addEventListener("dragleave", () => card.classList.remove("dragover"));
    card.addEventListener("drop", (e) => {
      e.preventDefault();
      card.classList.remove("dragover");
      const from = Number(e.dataTransfer.getData("text/plain"));
      if (Number.isNaN(from) || from === i) return;
      const [moved] = state.files.splice(from, 1);
      state.files.splice(i, 0, moved);
      redrawFileList();
      refreshPreviewSampleOptions();
      refreshPreview();
      schedulePreview();
    });

    list.appendChild(card);
  });
}

// --- Spec building -------------------------------------------------------

function buildSpec() {
  state.raw = $("#raw").value.trim();
  const spec = {
    filename_template: state.filenameTemplate,
  };
  if (state.raw) {
    spec.raw = state.raw;
  } else {
    const opts = {};
    for (const [k, v] of Object.entries(state.values)) {
      if (v === null || v === undefined || v === "") continue;
      if (Array.isArray(v) && v.length === 0) continue;
      if (typeof v === "object" && !Array.isArray(v) && Object.values(v).every(x => x === "" || x === null || x === undefined || x === false)) continue;
      opts[k] = coerce(k, v);
    }
    spec.options = opts;
  }
  return spec;
}

function coerce(key, v) {
  const numericKeys = new Set([
    "quality", "max_bytes", "width", "height", "min_width", "min_height",
    "dpr", "zoom", "blur", "sharpen", "pixelate", "rotate", "expires",
    "max_src_resolution", "max_src_file_size", "max_animation_frames",
    "max_animation_frame_resolution", "max_result_dimension",
  ]);
  if (numericKeys.has(key) && typeof v === "string") {
    const n = Number(v);
    return Number.isFinite(n) ? n : v;
  }
  return v;
}

function refreshPreview() {
  const spec = buildSpec();
  const optsStr = previewOptionsString(spec);
  let suffix;
  if (state.files.length > 0) {
    suffix = "local:///<id>";
  } else if (firstURL()) {
    suffix = "plain/" + firstURL();
  } else {
    suffix = "<source>";
  }
  $("#urlpreview").textContent = "/" + (optsStr ? optsStr + "/" : "") + suffix
    + "    (" + (state.files.length || urlList().length) + " items)";
}

function previewOptionsString(spec) {
  if (spec.raw) return spec.raw.replace(/^\/+|\/+$/g, "");
  const order = Object.keys(spec.options || {}).sort();
  return order.map(k => localEncode(k, spec.options[k])).filter(Boolean).join("/");
}

function localEncode(key, val) {
  const SHORT = {
    format: "f", quality: "q", format_quality: "fq", max_bytes: "mb",
    resizing_type: "rt", width: "w", height: "h", min_width: "mw", min_height: "mh",
    dpr: "dpr", zoom: "z", enlarge: "el", extend: "ex", extend_aspect_ratio: "exar",
    gravity: "g", crop: "c", padding: "pd", trim: "t",
    rotate: "rot", auto_rotate: "ar", flip: "fl",
    background: "bg", blur: "bl", sharpen: "sh", pixelate: "pix",
    watermark: "wm",
    strip_metadata: "sm", keep_copyright: "kcr", strip_color_profile: "scp", enforce_thumbnail: "eth",
    filename: "fn", return_attachment: "att", raw: "raw", cachebuster: "cb", expires: "exp", skip_processing: "skp",
    preset: "pr",
    max_src_resolution: "msr", max_src_file_size: "msfs", max_animation_frames: "maf", max_animation_frame_resolution: "mafr", max_result_dimension: "mrd",
  };
  const sh = SHORT[key]; if (!sh) return "";
  if (val === null || val === undefined || val === "") return "";
  if (typeof val === "boolean") return val ? `${sh}:1` : "";
  if (typeof val === "number") return val ? `${sh}:${val}` : "";
  if (typeof val === "string") return `${sh}:${val}`;
  if (Array.isArray(val)) {
    if (key === "format_quality") {
      const flat = val.flatMap(p => [p.format, p.quality]).filter(Boolean);
      return flat.length ? `${sh}:${flat.join(":")}` : "";
    }
    return val.length ? `${sh}:${val.join(":")}` : "";
  }
  if (typeof val === "object") {
    if (key === "gravity") {
      if (!val.type) return "";
      const xs = [val.type, val.x, val.y].filter(v => v !== "" && v !== undefined && v !== null);
      return `${sh}:${xs.join(":")}`;
    }
    if (key === "crop") {
      const xs = [val.width || 0, val.height || 0];
      if (val.gravity?.type) {
        xs.push(val.gravity.type);
        if (val.gravity.x !== "") xs.push(val.gravity.x);
        if (val.gravity.y !== "") xs.push(val.gravity.y);
      }
      return `${sh}:${xs.join(":")}`;
    }
    if (key === "extend" || key === "extend_aspect_ratio") {
      if (!val.enabled) return "";
      const xs = [1];
      if (val.gravity?.type) xs.push(val.gravity.type);
      return `${sh}:${xs.join(":")}`;
    }
    if (key === "trim") {
      const xs = [val.threshold || 0];
      if (val.color) xs.push(val.color.replace("#", ""));
      return `${sh}:${xs.join(":")}`;
    }
    if (key === "watermark") {
      const xs = [val.opacity];
      if (val.gravity) xs.push(val.gravity);
      if (val.x) xs.push(val.x); if (val.y) xs.push(val.y);
      if (val.scale) xs.push(val.scale);
      return `${sh}:${xs.join(":")}`;
    }
  }
  return "";
}

// --- Live preview --------------------------------------------------------

let previewAbort = null;
const schedulePreview = debounce(() => runPreview(), 300);

async function runPreview() {
  const sample = pickSample();
  const stateEl = $("#preview-state");
  const srcImg = $("#preview-src");
  const outImg = $("#preview-out");
  const srcStats = $("#preview-src-stats");
  const outStats = $("#preview-out-stats");
  const summary = $("#preview-summary");

  if (!sample) {
    srcImg.removeAttribute("src");
    outImg.removeAttribute("src");
    srcStats.textContent = "—"; outStats.textContent = "—";
    summary.classList.add("empty"); summary.textContent = "";
    stateEl.textContent = "Drop a file or paste a URL to see a live preview.";
    stateEl.classList.remove("error");
    return;
  }

  // Source render.
  if (sample.kind === "file") {
    const url = URL.createObjectURL(sample.file);
    srcImg.src = url;
    srcStats.textContent = `${sample.file.name} · ${humanBytes(sample.file.size)}`;
  } else {
    srcImg.src = sample.url;
    srcStats.textContent = sample.url;
  }

  if (previewAbort) previewAbort.abort();
  previewAbort = new AbortController();

  const spec = buildSpec();
  stateEl.textContent = "rendering preview…";
  stateEl.classList.remove("error");

  try {
    let resp;
    if (sample.kind === "file") {
      const fd = new FormData();
      fd.append("file", sample.file);
      fd.append("spec", JSON.stringify(spec));
      resp = await fetch("/api/preview", { method: "POST", body: fd, signal: previewAbort.signal });
    } else {
      resp = await fetch("/api/preview", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url: sample.url, spec }),
        signal: previewAbort.signal,
      });
    }
    if (!resp.ok) {
      const txt = await resp.text();
      throw new Error(txt || resp.statusText);
    }
    const blob = await resp.blob();
    const url = URL.createObjectURL(blob);
    outImg.src = url;
    outStats.textContent = `${humanBytes(blob.size)} · ${blob.type || "image"}`;

    const srcSize = sample.kind === "file" ? sample.file.size : null;
    if (srcSize) {
      const delta = blob.size - srcSize;
      const pct = ((delta / srcSize) * 100).toFixed(0);
      const sign = delta >= 0 ? "+" : "−";
      summary.textContent = `${humanBytes(srcSize)} → ${humanBytes(blob.size)}  (${sign}${Math.abs(pct)}%)`;
      summary.classList.remove("empty");
    } else {
      summary.classList.add("empty"); summary.textContent = "";
    }
    stateEl.textContent = "";
  } catch (e) {
    if (e.name === "AbortError") return;
    stateEl.textContent = "preview error: " + e.message;
    stateEl.classList.add("error");
    outImg.removeAttribute("src");
    outStats.textContent = "—";
    summary.classList.add("empty");
  }
}

function pickSample() {
  if (state.files.length > 0) {
    const i = Math.min(state.previewIndex, state.files.length - 1);
    return { kind: "file", file: state.files[i].file };
  }
  const urls = urlList();
  if (urls.length > 0) {
    const i = Math.min(state.previewIndex, urls.length - 1);
    return { kind: "url", url: urls[i] };
  }
  return null;
}

function refreshPreviewSampleOptions() {
  const sel = $("#preview-sample");
  sel.replaceChildren();
  const items = state.files.length > 0
    ? state.files.map((e, i) => ({ value: i, label: `${i + 1}. ${e.file.name}` }))
    : urlList().map((u, i) => ({ value: i, label: `${i + 1}. ${u}` }));
  if (items.length === 0) {
    const o = document.createElement("option");
    o.value = ""; o.textContent = "(none)";
    sel.appendChild(o);
    return;
  }
  for (const it of items) {
    const o = document.createElement("option");
    o.value = String(it.value); o.textContent = it.label;
    sel.appendChild(o);
  }
  sel.value = String(Math.min(state.previewIndex, items.length - 1));
}

// --- Top bar / actions ---------------------------------------------------

function bindTopbar() {
  $("#preset-save").addEventListener("click", presetSave);
  $("#preset-delete").addEventListener("click", presetDelete);
  $("#preset-export").addEventListener("click", presetExport);
  $("#preset-import").addEventListener("click", () => $("#preset-import-file").click());
  $("#preset-import-file").addEventListener("change", presetImport);
  $("#preset-select").addEventListener("change", (e) => presetLoad(e.target.value));
  $("#share").addEventListener("click", share);
  $("#history-toggle").addEventListener("click", () => $("#history-drawer").hidden = false);
  $("#history-close").addEventListener("click", () => $("#history-drawer").hidden = true);
  $("#history-clear").addEventListener("click", () => {
    state.history = [];
    saveJSON(HISTORY_KEY, state.history);
    redrawHistory();
  });
  $("#reset").addEventListener("click", reset);
  $("#preview-sample").addEventListener("change", (e) => {
    state.previewIndex = Number(e.target.value || 0);
    schedulePreview();
  });
  $("#preview-refresh").addEventListener("click", () => runPreview());
  redrawHistory();
}

function bindForm() {
  $("#raw").addEventListener("input", () => {
    refreshPreview();
    syncHash();
    schedulePreview();
  });
}

function bindActions() {
  $("#convert").addEventListener("click", convert);
}

async function convert() {
  const status = $("#status");
  const btn = $("#convert");
  status.className = "status";
  status.textContent = "";
  const urls = urlList();
  const useUpload = state.files.length > 0;
  if (!useUpload && urls.length === 0) {
    status.textContent = "Add files or paste URLs first.";
    status.className = "status error";
    return;
  }
  btn.disabled = true;
  status.textContent = `processing ${useUpload ? state.files.length : urls.length} item(s)…`;
  const spec = buildSpec();
  try {
    const blob = useUpload ? await postUpload(spec) : await postURLs(urls, spec);
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "imgproxy-batch.zip";
    document.body.appendChild(a);
    a.click();
    a.remove();
    status.textContent = "done · ZIP downloaded";
    status.className = "status ok";
    pushHistory({
      ts: Date.now(),
      count: useUpload ? state.files.length : urls.length,
      sample: useUpload ? state.files[0]?.file.name : urls[0],
      spec,
    });
  } catch (e) {
    status.textContent = "error: " + e.message;
    status.className = "status error";
  } finally {
    btn.disabled = false;
  }
}

async function postUpload(spec) {
  const fd = new FormData();
  fd.append("spec", JSON.stringify(spec));
  for (const entry of state.files) fd.append("file", entry.file, entry.outputName + "." + (entry.file.name.split(".").pop() || ""));
  const r = await fetch("/api/convert", { method: "POST", body: fd });
  if (!r.ok) throw new Error(await r.text());
  return r.blob();
}

async function postURLs(urls, spec) {
  const r = await fetch("/api/convert-url", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ urls, spec }),
  });
  if (!r.ok) throw new Error(await r.text());
  return r.blob();
}

// --- Presets -------------------------------------------------------------

function presetSave() {
  const name = prompt("Preset name");
  if (!name) return;
  const spec = buildSpec();
  state.presets[name] = { spec, raw: state.raw, filenameTemplate: state.filenameTemplate, values: structuredClone(state.values) };
  saveJSON(PRESETS_KEY, state.presets);
  refreshPresetSelect(name);
}

function presetDelete() {
  const name = $("#preset-select").value;
  if (!name || !state.presets[name]) return;
  if (!confirm(`Delete preset "${name}"?`)) return;
  delete state.presets[name];
  saveJSON(PRESETS_KEY, state.presets);
  refreshPresetSelect();
}

function presetLoad(name) {
  if (!name) return;
  const p = state.presets[name];
  if (!p) return;
  state.values = structuredClone(p.values || {});
  state.raw = p.raw || "";
  state.filenameTemplate = p.filenameTemplate || "{name}.{ext}";
  $("#raw").value = state.raw;
  $("#filename-template").value = state.filenameTemplate;
  applyValuesToInputs();
  syncHash();
  refreshPreview();
  schedulePreview();
}

function presetExport() {
  const blob = new Blob([JSON.stringify(state.presets, null, 2)], { type: "application/json" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = "imgproxy-web-presets.json";
  document.body.appendChild(a);
  a.click();
  a.remove();
}

async function presetImport(e) {
  const file = e.target.files?.[0];
  if (!file) return;
  try {
    const text = await file.text();
    const parsed = JSON.parse(text);
    if (typeof parsed !== "object" || parsed === null) throw new Error("invalid presets file");
    state.presets = { ...state.presets, ...parsed };
    saveJSON(PRESETS_KEY, state.presets);
    refreshPresetSelect();
  } catch (err) {
    alert("Import failed: " + err.message);
  } finally {
    e.target.value = "";
  }
}

function refreshPresetSelect(selected) {
  const sel = $("#preset-select");
  sel.replaceChildren();
  const blank = document.createElement("option");
  blank.value = ""; blank.textContent = "(none)";
  sel.appendChild(blank);
  for (const name of Object.keys(state.presets).sort()) {
    const o = document.createElement("option");
    o.value = name; o.textContent = name;
    sel.appendChild(o);
  }
  if (selected) sel.value = selected;
}

// --- URL hash state ------------------------------------------------------

const syncHash = debounce(() => {
  const payload = {
    v: state.values,
    r: state.raw,
    t: state.filenameTemplate,
  };
  const enc = b64urlEncode(JSON.stringify(payload));
  history.replaceState(null, "", "#" + enc);
}, 150);

function applyHashState() {
  if (!location.hash || location.hash.length < 2) return;
  try {
    const json = b64urlDecode(location.hash.slice(1));
    const data = JSON.parse(json);
    state.values = data.v || {};
    state.raw = data.r || "";
    state.filenameTemplate = data.t || "{name}.{ext}";
    $("#raw").value = state.raw;
    $("#filename-template").value = state.filenameTemplate;
    applyValuesToInputs();
  } catch (e) { /* ignore bad hash */ }
}

function applyValuesToInputs() {
  // Walk fields and set input values from state.values.
  for (const fieldDiv of document.querySelectorAll(".field")) {
    const input = fieldDiv.querySelector("[data-opt-key]");
    if (!input) continue;
    const key = input.dataset.optKey;
    const val = state.values[key];
    if (val === undefined) continue;
    if (input.tagName === "SELECT") {
      input.value = val;
    } else if (input.type === "checkbox") {
      input.checked = !!val;
    } else if (input.tagName === "INPUT") {
      input.value = typeof val === "object" ? "" : String(val);
    }
    // Compound values (gravity/crop/etc) lose their fine-grained inputs on hash
    // restore — accepted limitation; raw mode is the workaround.
  }
}

function share() {
  syncHash.flush?.();
  navigator.clipboard.writeText(location.href).then(
    () => flashStatus("share link copied"),
    () => flashStatus("copy failed: " + location.href, true),
  );
}

function reset() {
  if (!confirm("Reset all options?")) return;
  state.values = {}; state.raw = ""; state.filenameTemplate = "{name}.{ext}";
  $("#raw").value = ""; $("#filename-template").value = "{name}.{ext}";
  document.querySelectorAll("[data-opt-key]").forEach(el => {
    if (el.type === "checkbox") el.checked = false;
    else el.value = el.tagName === "SELECT" ? (el.options[0]?.value ?? "") : "";
  });
  syncHash();
  refreshPreview();
  schedulePreview();
}

function flashStatus(msg, isErr) {
  const s = $("#status");
  s.textContent = msg;
  s.className = "status " + (isErr ? "error" : "ok");
  setTimeout(() => { s.textContent = ""; s.className = "status"; }, 1500);
}

// --- History -------------------------------------------------------------

function pushHistory(entry) {
  state.history.unshift(entry);
  if (state.history.length > HISTORY_MAX) state.history.length = HISTORY_MAX;
  saveJSON(HISTORY_KEY, state.history);
  redrawHistory();
}

function redrawHistory() {
  const list = $("#history-list");
  list.replaceChildren();
  if (state.history.length === 0) {
    const li = document.createElement("li");
    li.className = "muted";
    li.textContent = "(empty)";
    list.appendChild(li);
    return;
  }
  for (const h of state.history) {
    const li = document.createElement("li");
    const t = document.createElement("div");
    t.className = "h-time";
    t.textContent = new Date(h.ts).toLocaleString() + ` · ${h.count} file(s)` + (h.sample ? ` · ${h.sample}` : "");
    const s = document.createElement("div");
    s.className = "h-spec";
    const url = previewOptionsString(h.spec) || "(no options)";
    s.textContent = url;
    li.append(t, s);
    li.addEventListener("click", () => {
      state.values = (h.spec.options) || {};
      state.raw = h.spec.raw || "";
      state.filenameTemplate = h.spec.filename_template || "{name}.{ext}";
      $("#raw").value = state.raw;
      $("#filename-template").value = state.filenameTemplate;
      applyValuesToInputs();
      syncHash();
      refreshPreview();
      schedulePreview();
      $("#history-drawer").hidden = true;
    });
    list.appendChild(li);
  }
}

// --- utils ---------------------------------------------------------------

function urlList() {
  return $("#urls").value.split(/\r?\n/).map(s => s.trim()).filter(Boolean);
}

function firstURL() {
  return urlList()[0] || "";
}

function humanBytes(n) {
  const u = ["B", "KB", "MB", "GB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return n.toFixed(n < 10 && i > 0 ? 1 : 0) + " " + u[i];
}

function readJSON(key, fallback) {
  try { return JSON.parse(localStorage.getItem(key) || "") ?? fallback; }
  catch { return fallback; }
}
function saveJSON(key, v) { localStorage.setItem(key, JSON.stringify(v)); }

function b64urlEncode(s) {
  return btoa(unescape(encodeURIComponent(s))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}
function b64urlDecode(s) {
  const pad = s.length % 4 === 0 ? "" : "=".repeat(4 - (s.length % 4));
  return decodeURIComponent(escape(atob(s.replace(/-/g, "+").replace(/_/g, "/") + pad)));
}
