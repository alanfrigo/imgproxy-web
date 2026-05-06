// imgproxy-web UI: vanilla JS, no build step. Loads the option catalog from
// /api/options, generates a form, builds a Spec object, and POSTs to either
// /api/convert (uploads) or /api/convert-url (remote URLs).

const $ = (sel) => document.querySelector(sel);

const state = {
  files: [],          // File[]
  catalog: null,
  values: {},         // option key → input value(s)
};

(async function init() {
  await pingHealth();
  await loadCatalog();
  bindSources();
  bindForm();
  bindActions();
  refreshPreview();
})();

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

// --- Form rendering ------------------------------------------------------

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
  i.dataset.key = opt.key;
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
  i.dataset.key = opt.key;
  i.addEventListener("change", () => onChange(opt.key, i.checked));
  wrap.appendChild(i);
  return wrap;
}

function selectInput(opt) {
  const s = document.createElement("select");
  s.dataset.key = opt.key;
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
}

// --- Sources -------------------------------------------------------------

function bindSources() {
  const dz = $("#dropzone");
  const picker = $("#picker");
  const inp = $("#files");
  const list = $("#filelist");
  picker.addEventListener("click", () => inp.click());
  inp.addEventListener("change", () => addFiles(Array.from(inp.files)));
  dz.addEventListener("dragover", (e) => { e.preventDefault(); dz.classList.add("drag"); });
  dz.addEventListener("dragleave", () => dz.classList.remove("drag"));
  dz.addEventListener("drop", (e) => {
    e.preventDefault(); dz.classList.remove("drag");
    addFiles(Array.from(e.dataTransfer.files));
  });
  $("#urls").addEventListener("input", refreshPreview);

  function addFiles(arr) {
    state.files.push(...arr);
    redraw();
  }
  function redraw() {
    list.replaceChildren();
    state.files.forEach((f, i) => {
      const li = document.createElement("li");
      const left = document.createElement("span");
      left.append(document.createTextNode(f.name + " "));
      const sz = document.createElement("span");
      sz.className = "muted";
      sz.textContent = "(" + humanBytes(f.size) + ")";
      left.append(sz);
      const rm = document.createElement("span");
      rm.className = "rm";
      rm.textContent = "remove";
      rm.addEventListener("click", () => {
        state.files.splice(i, 1);
        redraw();
      });
      li.append(left, rm);
      list.appendChild(li);
    });
    refreshPreview();
  }
}

// --- Spec building -------------------------------------------------------

function buildSpec() {
  const raw = $("#raw").value.trim();
  if (raw) return { raw };
  const opts = {};
  for (const [k, v] of Object.entries(state.values)) {
    if (v === null || v === undefined || v === "") continue;
    if (Array.isArray(v) && v.length === 0) continue;
    if (typeof v === "object" && !Array.isArray(v) && Object.values(v).every(x => x === "" || x === null || x === undefined || x === false)) continue;
    opts[k] = coerce(k, v);
  }
  return { options: opts };
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
  const parts = [];
  for (const k of order) parts.push(localEncode(k, spec.options[k]));
  return parts.filter(Boolean).join("/");
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

// --- Actions -------------------------------------------------------------

function bindForm() {
  $("#raw").addEventListener("input", refreshPreview);
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
  try {
    const blob = useUpload ? await postUpload() : await postURLs(urls);
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = "imgproxy-batch.zip";
    document.body.appendChild(a);
    a.click();
    a.remove();
    status.textContent = "done · ZIP downloaded";
    status.className = "status ok";
  } catch (e) {
    status.textContent = "error: " + e.message;
    status.className = "status error";
  } finally {
    btn.disabled = false;
  }
}

async function postUpload() {
  const fd = new FormData();
  fd.append("spec", JSON.stringify(buildSpec()));
  for (const f of state.files) fd.append("file", f);
  const r = await fetch("/api/convert", { method: "POST", body: fd });
  if (!r.ok) throw new Error(await r.text());
  return r.blob();
}

async function postURLs(urls) {
  const r = await fetch("/api/convert-url", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ urls, spec: buildSpec() }),
  });
  if (!r.ok) throw new Error(await r.text());
  return r.blob();
}

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
