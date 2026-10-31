(function () {
  "use strict";
  const D = JSON.parse(document.getElementById("data").textContent);
  const $ = (id) => document.getElementById(id);
  const SVGNS = "http://www.w3.org/2000/svg";

  // ---- formatting ---------------------------------------------------------

  function fmtTime(ms) {
    if (!ms) return "-";
    return new Date(ms).toISOString().replace("T", " ").replace(/\.\d+Z$/, "Z");
  }
  function fmtShort(ms) { return new Date(ms).toISOString().slice(5, 16).replace("T", " "); }
  function fmtBytes(b) {
    const u = ["B", "kB", "MB", "GB", "TB"];
    let i = 0;
    while (b >= 1000 && i < u.length - 1) { b /= 1000; i++; }
    return (i ? b.toFixed(1) : b) + " " + u[i];
  }
  function fmtCount(n) {
    if (n >= 1e9) return (n / 1e9).toFixed(1) + "G";
    if (n >= 1e6) return (n / 1e6).toFixed(1) + "M";
    if (n >= 1e4) return (n / 1e3).toFixed(1) + "k";
    return String(n);
  }
  function el(tag, attrs, text) {
    const e = document.createElement(tag);
    for (const k in attrs || {}) e.setAttribute(k, attrs[k]);
    if (text !== undefined) e.textContent = text;
    return e;
  }
  function svg(tag, attrs) {
    const e = document.createElementNS(SVGNS, tag);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    return e;
  }
  const css = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

  // ---- tooltip --------------------------------------------------------------

  const tip = $("tooltip");
  function showTip(ev, title, lines) {
    tip.replaceChildren(el("div", { class: "t" }, title));
    for (const l of lines) tip.appendChild(el("div", {}, l));
    tip.hidden = false;
    const r = tip.getBoundingClientRect();
    let x = ev.clientX + 14, y = ev.clientY + 14;
    if (x + r.width > window.innerWidth - 8) x = ev.clientX - r.width - 14;
    if (y + r.height > window.innerHeight - 8) y = ev.clientY - r.height - 14;
    tip.style.left = x + "px";
    tip.style.top = y + "px";
  }
  function hideTip() { tip.hidden = true; }

  // ---- header ---------------------------------------------------------------

  const vm = D.vm, qan = D.qan, args = D.args, meta = D.meta;
  // Go writes empty slices as null
  vm.chunks = vm.chunks || [];
  vm.services = vm.services || [];
  qan.files = qan.files || [];
  qan.services = qan.services || [];
  const vmDecoded = D.deep && vm.format === "json";
  (function () {
    const m = $("meta");
    m.appendChild(el("span", { class: "file" }, D.file.split("/").pop()));
    const parts = [fmtBytes(D.size), "PMM " + meta["pmm-server-version"], args.source];
    if (D.encrypted) parts.push("encrypted");
    if (!D.deep) parts.push("quick mode: chunk contents not read");
    m.appendChild(document.createTextNode(" · " + parts.join(" · ")));
  })();

  // ---- theme: dark by default, the choice is remembered per browser ---------

  const themeBtn = $("theme-toggle");
  // The button shows the theme it switches to: a sun in dark, a moon in light.
  const SUN = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">' +
    '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>';
  const MOON = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
    '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>';
  function applyTheme(t) {
    document.documentElement.setAttribute("data-theme", t);
    themeBtn.innerHTML = t === "dark" ? SUN : MOON;
    const label = t === "dark" ? "Switch to light theme" : "Switch to dark theme";
    themeBtn.title = label;
    themeBtn.setAttribute("aria-label", label);
  }
  (function () {
    let t = "dark";
    try { t = localStorage.getItem("pmm-dump-info-theme") || "dark"; } catch (_) { /* storage blocked */ }
    applyTheme(t);
  })();
  themeBtn.addEventListener("click", () => {
    const t = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
    applyTheme(t);
    try { localStorage.setItem("pmm-dump-info-theme", t); } catch (_) { /* storage blocked */ }
    drawCharts();
  });

  // ---- alerts and dump --------------------------------------------------------

  (function () {
    const ul = $("alerts");
    if (!D.alerts || !D.alerts.length) {
      ul.appendChild(el("li", { class: "ok" }, "Nothing unusual found."));
      return;
    }
    for (const a of D.alerts) {
      const li = el("li", { class: a.level });
      li.appendChild(el("span", { class: "tag" }, a.level === "warn" ? "Warning" : "Info"));
      li.appendChild(el("span", {}, a.text));
      ul.appendChild(li);
    }
  })();

  (function () {
    const dl = $("dump");
    const add = (k, v, mono) => {
      dl.appendChild(el("dt", {}, k));
      dl.appendChild(el("dd", mono ? { class: "mono" } : {}, v || "-"));
    };
    add("pmm-dump", meta.version["git-branch"] + "@" + meta.version["git-commit"]);
    add("PMM Server", meta["pmm-server-version"]);
    add("Exported from", args.source);
    add("PMM timezone", meta["pmm-server-timezone"] || "-");
    add("Encrypted", D.encrypted ? "yes" : "no");
    add("Requested range", (args.start || "-") + " -> " + (args.end || "-"));
    add("Core metrics", (args.dumpCore ? "yes" : "no") + " (" + vm.format + " format)");
    add("QAN", args.dumpQAN ? "yes" : "no");
    if (args.instances) add("--instance", args.instances.join(", "), true);
    if (args.dashboards) add("--dashboard", args.dashboards.join(", "), true);
    if (args.tsSelector) add("--ts-selector", args.tsSelector, true);
    if (args.where) add("--where", args.where, true);
    if (meta["pmm-server-services"]) {
      add("Services in meta", meta["pmm-server-services"].map((s) => s.name + " (" + s["node-name"] + ")").join(", "), true);
    }
    if (D.other) add("Other files", D.other.join(", "), true);
  })();

  // ---- tables (click a header to sort) ----------------------------------------

  function table(t, cols, rows, emptyText) {
    t.replaceChildren();
    const thead = t.createTHead(), tbody = t.createTBody();
    const hr = thead.insertRow();
    let sortCol = -1, asc = true;
    cols.forEach((c, i) => {
      const th = el("th", { class: c.num ? "n" : "" }, c.title);
      th.addEventListener("click", () => {
        asc = sortCol === i ? !asc : !c.num;
        sortCol = i;
        hr.querySelectorAll("th").forEach((x) => x.classList.remove("sorted", "asc"));
        th.classList.add("sorted");
        if (asc) th.classList.add("asc");
        const key = c.sort || c.value;
        rows.sort((a, b) => {
          const x = key(a), y = key(b);
          return (x < y ? -1 : x > y ? 1 : 0) * (asc ? 1 : -1);
        });
        body();
      });
      hr.appendChild(th);
    });
    function body() {
      tbody.replaceChildren();
      if (!rows.length) {
        const td = tbody.insertRow().insertCell();
        td.colSpan = cols.length;
        td.className = "muted";
        td.textContent = emptyText;
        return;
      }
      for (const r of rows) {
        const tr = tbody.insertRow();
        for (const c of cols) {
          const td = tr.insertCell();
          if (c.cls) td.className = c.cls;
          if (c.num) td.classList.add("n");
          const v = c.render ? c.render(r) : c.value(r);
          if (v instanceof Node) td.appendChild(v); else td.textContent = v;
        }
      }
    }
    body();
  }

  const qanBy = {};
  for (const q of qan.services || []) qanBy[q.name] = q;

  function badge(text, cls) { return el("span", { class: "badge " + (cls || "") }, text); }
  const noQAN = (k) => k === "node" || k === "pmm-server"; // nodes and PMM Server internals have no QAN
  function famText(f) {
    return Object.entries(f || {}).sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1))
      .map(([k, v]) => k + " " + v).join(", ");
  }

  (function () {
    const services = vm.services || [];
    $("vm-sub").textContent = "- " + vm.chunks.length + " chunks, " + fmtBytes(vm.bytes) +
      (vmDecoded ? ", " + fmtCount(vm.samples) + " samples, " + fmtCount(vm.series) + " distinct series, " +
        fmtTime(vm.first) + " -> " + fmtTime(vm.last) : "");
    if (!vmDecoded) {
      $("services").appendChild(el("caption", { class: "muted" },
        D.deep ? "Native format: services are not decoded." : "Quick mode: run without -quick to list services."));
      return;
    }
    const hasQAN = (qan.services || []).length > 0;
    table($("services"), [
      { title: "Service", value: (s) => s.name, cls: "name" },
      { title: "Type", value: (s) => s.kind, render: (s) => badge(s.kind, s.kind === "node" ? "node" : "") },
      { title: "Node", value: (s) => s.node, cls: "name" },
      { title: "First", value: (s) => s.first, render: (s) => fmtTime(s.first) },
      { title: "Last", value: (s) => s.last, render: (s) => fmtTime(s.last) },
      { title: "Series", value: (s) => s.series, render: (s) => fmtCount(s.series), num: true },
      { title: "Samples", value: (s) => s.samples, render: (s) => fmtCount(s.samples), num: true },
      { title: "QAN", value: (s) => (qanBy[s.name] ? qanBy[s.name].rows : -1),
        render: (s) => noQAN(s.kind) ? "" : qanBy[s.name] ? badge(fmtCount(qanBy[s.name].rows) + " rows") : badge(hasQAN ? "none" : "-", hasQAN ? "no" : "node") },
      { title: "Metric families (distinct series)", value: (s) => famText(s.families), cls: "fam" },
    ], services.slice(), "No series in the vm/ chunks.");
  })();

  (function () {
    $("qan-sub").textContent = "- " + qan.files.length + " files, " + fmtBytes(qan.bytes) +
      (D.deep && qan.rows ? ", " + fmtCount(qan.rows) + " rows, " + fmtTime(qan.first) + " -> " + fmtTime(qan.last) + " · " + qan.schema : "");
    let empty = "No QAN in this dump.";
    if (args.dumpQAN && !qan.files.length) {
      empty = "QAN was requested but there are no ch/ files" +
        (D.log.chRows !== undefined ? " (log.json: ClickHouse returned " + D.log.chRows + " rows)." : ".");
    } else if (!D.deep && qan.files.length) {
      empty = "Quick mode: run without -quick to read the QAN rows.";
    }
    table($("qan"), [
      { title: "Service", value: (s) => s.name, cls: "name" },
      { title: "Type", value: (s) => s.kind || "", render: (s) => s.kind ? badge(s.kind) : "-" },
      { title: "Node", value: (s) => s.node || "", cls: "name" },
      { title: "First", value: (s) => s.first, render: (s) => fmtTime(s.first) },
      { title: "Last", value: (s) => s.last, render: (s) => fmtTime(s.last) },
      { title: "Rows", value: (s) => s.rows, render: (s) => fmtCount(s.rows), num: true },
      { title: "Distinct queries", value: (s) => s.queries, num: true },
    ], (qan.services || []).slice(), empty);
  })();

  (function () {
    const L = D.log;
    $("log-sub").textContent = L.present ? "- " + L.lines + " lines, " + (L.first || "-") + " -> " + (L.last || "-") + ", " +
      Object.entries(L.levels).map(([k, v]) => k + " " + v).join(", ") + " · warnings, errors and ClickHouse lines" : "- no log.json in the dump";
    const pre = $("log");
    if (!L.notable || !L.notable.length) { pre.textContent = "No warnings or errors."; return; }
    for (const l of L.notable) {
      const cls = / (WARN|ERROR|FATAL|PANIC) /.exec(l);
      pre.appendChild(el("span", cls ? { class: cls[1] === "WARN" ? "warn" : "error" } : {}, l + "\n"));
    }
  })();
  $("cmd").textContent = args.masked;

  // ---- charts -------------------------------------------------------------------

  // Time span of everything that has data: chunk names and QAN buckets.
  function span() {
    let lo = Infinity, hi = -Infinity;
    for (const c of vm.chunks) { lo = Math.min(lo, c.start); hi = Math.max(hi, c.end); }
    if (qan.first) { lo = Math.min(lo, qan.first); hi = Math.max(hi, qan.last + 60000); }
    return isFinite(lo) ? [lo, hi] : null;
  }

  function timeAxis(g, x, x0, x1, y0, y1) {
    const ms = x1 - x0;
    const steps = [60e3, 300e3, 900e3, 1800e3, 3600e3, 3 * 3600e3, 6 * 3600e3, 12 * 3600e3, 86400e3, 7 * 86400e3];
    const width = x(x1) - x(x0);
    let step = steps[steps.length - 1];
    for (const s of steps) { if (ms / s <= Math.max(2, width / 110)) { step = s; break; } }
    for (let t = Math.ceil(x0 / step) * step; t <= x1; t += step) {
      g.appendChild(svg("line", { class: "grid", x1: x(t), x2: x(t), y1: y0, y2: y1 }));
      const tx = svg("text", { x: x(t), y: y1 + 14, "text-anchor": "middle" });
      tx.textContent = fmtShort(t);
      g.appendChild(tx);
    }
  }

  function drawCoverage() {
    const box = $("coverage");
    box.replaceChildren();
    const legend = $("coverage-legend");
    legend.replaceChildren();
    const sp = span();
    if (!vmDecoded && !(D.deep && qan.rows)) {
      $("coverage-panel").hidden = true;
      return;
    }
    const cMetrics = css("--series-1"), cQAN = css("--series-2");
    const keys = [];
    if (vmDecoded) keys.push(["Metrics", cMetrics]);
    if (qan.services.length) keys.push(["QAN", cQAN]);
    for (const [name, c] of keys) {
      const it = el("span", { class: "item" });
      const sw = el("span", { class: "swatch" });
      sw.style.background = c;
      it.append(sw, name);
      legend.appendChild(it);
    }

    // rows: every service with metrics, then QAN-only services
    const rows = [];
    for (const s of vm.services || []) rows.push({ name: s.name, kind: s.kind, vm: s, qan: noQAN(s.kind) ? null : qanBy[s.name] });
    for (const q of qan.services || []) if (!rows.some((r) => r.name === q.name && !noQAN(r.kind))) rows.push({ name: q.name, kind: q.kind || "-", qan: q });

    const labelW = Math.min(300, 40 + Math.max(...rows.map((r) => (r.name.length + r.kind.length + 3) * 6.6)));
    const W = Math.max(box.clientWidth - 18, labelW + 300), rowH = 26, top = 4, plotW = W - labelW - 12;
    const H = top + rows.length * rowH + 22;
    const [x0, x1] = sp;
    const x = (t) => labelW + ((t - x0) / (x1 - x0)) * plotW;
    const root = svg("svg", { width: W, height: H, role: "img", "aria-label": "Coverage of metrics and QAN per service over time" });
    const grid = svg("g", {});
    root.appendChild(grid);
    timeAxis(grid, x, x0, x1, top, top + rows.length * rowH);

    rows.forEach((r, i) => {
      const y = top + i * rowH;
      const lab = svg("text", { x: labelW - 8, y: y + rowH / 2 + 4, "text-anchor": "end", class: "lab" });
      lab.textContent = r.name + " (" + r.kind + ")";
      root.appendChild(lab);
      root.appendChild(svg("rect", { class: "track", x: labelW, y: y + 4, width: plotW, height: rowH - 8, rx: 3 }));
      const laneH = r.qan && r.vm ? (rowH - 10) / 2 : rowH - 8;
      let laneY = y + 4;
      if (r.vm) {
        // merge consecutive chunks with samples into one segment
        let segStart = -1;
        vm.chunks.forEach((c, j) => {
          const has = r.vm.coverage[j] > 0;
          if (has && segStart < 0) segStart = j;
          const last = j === vm.chunks.length - 1;
          if (segStart >= 0 && (!has || last)) {
            const endIdx = has ? j : j - 1;
            const xa = x(vm.chunks[segStart].start), xb = x(vm.chunks[endIdx].end);
            root.appendChild(svg("rect", { x: xa, y: laneY, width: Math.max(1, xb - xa), height: laneH, rx: 2, fill: cMetrics }));
            segStart = -1;
          }
        });
        laneY += laneH + 2;
      }
      if (r.qan) {
        const keys = Object.keys(r.qan.buckets || {}).map(Number).sort((a, b) => a - b);
        let a = null, b = null;
        const flush = () => {
          if (a === null) return;
          const xa = x(a), xb = x(b + 60000);
          root.appendChild(svg("rect", { x: xa, y: laneY, width: Math.max(1, xb - xa), height: laneH, rx: 2, fill: cQAN }));
        };
        for (const k of keys) {
          if (a !== null && k - b <= 120000) { b = k; continue; }
          flush();
          a = b = k;
        }
        flush();
      }
      // hover: what this row has at the pointer time
      const hit = svg("rect", { x: labelW, y: y, width: plotW, height: rowH, fill: "transparent" });
      hit.addEventListener("mousemove", (ev) => {
        const pt = root.getBoundingClientRect();
        const t = x0 + ((ev.clientX - pt.left - labelW) / plotW) * (x1 - x0);
        const lines = [];
        if (r.vm) {
          const j = vm.chunks.findIndex((c) => t >= c.start && t < c.end);
          if (j >= 0) lines.push("Metrics: " + fmtCount(r.vm.coverage[j]) + " samples in chunk " + fmtShort(vm.chunks[j].start) + " - " + fmtShort(vm.chunks[j].end));
          else lines.push("Metrics: outside the exported chunks");
        }
        if (r.qan) {
          let n = 0;
          for (const k in r.qan.buckets) if (Math.abs(Number(k) - t) < 300000) n += r.qan.buckets[k];
          lines.push("QAN: " + fmtCount(n) + " rows within 5 min");
        } else if (!noQAN(r.kind) && (qan.services || []).length) {
          lines.push("QAN: none for this service");
        }
        showTip(ev, r.name + " · " + fmtTime(t), lines);
      });
      hit.addEventListener("mouseleave", hideTip);
      root.appendChild(hit);
    });
    box.appendChild(root);
  }

  function drawChunks() {
    const box = $("chunks");
    box.replaceChildren();
    if (!vmDecoded || !vm.chunks.length) {
      $("chunks-panel").hidden = true;
      return;
    }
    const color = css("--series-1");
    const W = Math.max(box.clientWidth - 18, 400), H = 180, left = 56, right = 8, top = 8, bottom = 22;
    const x0 = vm.chunks[0].start, x1 = vm.chunks[vm.chunks.length - 1].end;
    const plotW = W - left - right, plotH = H - top - bottom;
    const x = (t) => left + ((t - x0) / (x1 - x0)) * plotW;
    const maxS = Math.max(1, ...vm.chunks.map((c) => c.samples));
    const y = (v) => top + plotH - (v / maxS) * plotH;
    const root = svg("svg", { width: W, height: H, role: "img", "aria-label": "Samples per chunk over time" });
    const g = svg("g", {});
    root.appendChild(g);
    timeAxis(g, x, x0, x1, top, top + plotH);
    for (const f of [0, 0.5, 1]) {
      g.appendChild(svg("line", { class: "grid", x1: left, x2: W - right, y1: y(maxS * f), y2: y(maxS * f) }));
      const t = svg("text", { x: left - 6, y: y(maxS * f) + 4, "text-anchor": "end" });
      t.textContent = fmtCount(Math.round(maxS * f));
      g.appendChild(t);
    }
    for (const c of vm.chunks) {
      const xa = x(c.start) + 1, w = Math.max(1, x(c.end) - x(c.start) - 2);
      const h = Math.max(0, top + plotH - y(c.samples));
      root.appendChild(svg("rect", { x: xa, y: top + plotH - h, width: w, height: h, rx: Math.min(2, w / 2), fill: color }));
      const hit = svg("rect", { x: x(c.start), y: top, width: Math.max(1, x(c.end) - x(c.start)), height: plotH, fill: "transparent" });
      hit.addEventListener("mousemove", (ev) => showTip(ev, "vm/" + c.name, [
        fmtTime(c.start) + " -> " + fmtTime(c.end),
        fmtCount(c.samples) + " samples, " + fmtCount(c.lines) + " series, " + fmtBytes(c.bytes) + " compressed",
      ]));
      hit.addEventListener("mouseleave", hideTip);
      root.appendChild(hit);
    }
    box.appendChild(root);
  }

  function drawCharts() {
    drawCoverage();
    drawChunks();
  }
  drawCharts();
  let resizeTimer;
  window.addEventListener("resize", () => {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(drawCharts, 150);
  });
})();
