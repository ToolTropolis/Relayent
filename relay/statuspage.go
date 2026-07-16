// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: Self-contained HTML status page served at the relay root. It asks
//
//	for a pairing key in the browser and calls the same /v1 API a consumer would,
//	so nothing privileged is exposed server-side and the key never leaves the page.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http"
)

// statusPage serves the dashboard. Any unmatched path 404s so this doesn't
// swallow bad /v1 routes.
func (s *server) statusPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(statusHTML))
}

const statusHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Relayent — Status</title>
<style>
  :root {
    --bg: #0f1115; --card: #171a21; --line: #262b36; --fg: #e6e9ef;
    --muted: #949cad; --ok: #37d67a; --bad: #f2635f; --warn: #f2c15f; --accent: #6ea8fe;
  }
  @media (prefers-color-scheme: light) {
    :root { --bg:#f6f7f9; --card:#fff; --line:#e3e6ec; --fg:#1a1d23; --muted:#5d6472; }
  }
  * { box-sizing: border-box; }
  body { margin:0; padding:2rem 1rem; background:var(--bg); color:var(--fg);
    font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif; }
  .wrap { max-width: 820px; margin: 0 auto; }
  h1 { margin:0 0 .25rem; font-size:1.5rem; letter-spacing:-.02em; }
  .sub { color:var(--muted); margin:0 0 1.5rem; }
  .card { background:var(--card); border:1px solid var(--line); border-radius:12px;
    padding:1.1rem 1.25rem; margin-bottom:1rem; }
  .card h2 { margin:0 0 .85rem; font-size:.8rem; text-transform:uppercase;
    letter-spacing:.08em; color:var(--muted); font-weight:600; }
  .row { display:flex; justify-content:space-between; gap:1rem; padding:.4rem 0;
    border-bottom:1px solid var(--line); }
  .row:last-child { border-bottom:0; }
  .row .k { color:var(--muted); }
  .row .v { font-variant-numeric:tabular-nums; text-align:right; }
  .pill { display:inline-flex; align-items:center; gap:.4rem; font-weight:600; }
  .dot { width:8px; height:8px; border-radius:50%; display:inline-block; }
  .ok .dot{background:var(--ok)} .ok{color:var(--ok)}
  .bad .dot{background:var(--bad)} .bad{color:var(--bad)}
  .warn .dot{background:var(--warn)} .warn{color:var(--warn)}
  input { background:var(--bg); border:1px solid var(--line); color:var(--fg);
    padding:.55rem .7rem; border-radius:8px; font:inherit; flex:1; min-width:0; }
  button { background:var(--accent); color:#0b1020; border:0; padding:.55rem 1rem;
    border-radius:8px; font:inherit; font-weight:600; cursor:pointer; }
  button:hover { filter:brightness(1.08); }
  .keyrow { display:flex; gap:.6rem; margin-bottom:1rem; }
  table { width:100%; border-collapse:collapse; }
  th,td { text-align:left; padding:.5rem .4rem; border-bottom:1px solid var(--line); }
  th { color:var(--muted); font-size:.78rem; text-transform:uppercase; letter-spacing:.06em; }
  td:last-child, th:last-child { text-align:right; }
  code { background:var(--bg); border:1px solid var(--line); padding:.1rem .35rem;
    border-radius:5px; font-size:.85em; }
  .empty { color:var(--muted); font-style:italic; }
  .err { color:var(--bad); }
  .foot { color:var(--muted); font-size:.85rem; text-align:center; margin-top:1.5rem; }
</style>
</head>
<body>
<div class="wrap">
  <h1>Relayent</h1>
  <p class="sub">Relay status &amp; bridge capabilities</p>

  <div class="keyrow">
    <input id="key" type="password" placeholder="Pairing key" autocomplete="off" spellcheck="false">
    <button id="go">Connect</button>
  </div>

  <div id="err" class="card err" style="display:none"></div>

  <div class="card">
    <h2>Relay</h2>
    <div class="row"><span class="k">Health</span><span class="v" id="health">—</span></div>
    <div class="row"><span class="k">Version</span><span class="v" id="version">—</span></div>
    <div class="row"><span class="k">Uptime</span><span class="v" id="uptime">—</span></div>
    <div class="row"><span class="k">Pairing key enforced</span><span class="v" id="pairing">—</span></div>
    <div class="row"><span class="k">Pending jobs (your key)</span><span class="v" id="pending">—</span></div>
  </div>

  <div class="card">
    <h2>Bridge</h2>
    <div class="row"><span class="k">Status</span><span class="v" id="bonline">—</span></div>
    <div class="row"><span class="k">Host</span><span class="v" id="bhost">—</span></div>
    <div class="row"><span class="k">Version</span><span class="v" id="bver">—</span></div>
    <div class="row"><span class="k">Last reported</span><span class="v" id="brep">—</span></div>
  </div>

  <div class="card">
    <h2>Backends</h2>
    <table>
      <thead><tr><th>Backend</th><th>CLI installed</th><th>Adapter</th><th>Ready</th></tr></thead>
      <tbody id="backends"><tr><td colspan="4" class="empty">Connect to view</td></tr></tbody>
    </table>
  </div>

  <p class="foot">Auto-refreshes every 5s while connected. The pairing key stays in this page.</p>
</div>

<script>
const $ = id => document.getElementById(id);
let key = "", timer = null;

const pill = (good, goodText, badText) =>
  '<span class="pill ' + (good ? 'ok' : 'bad') + '"><span class="dot"></span>' +
  (good ? goodText : badText) + '</span>';

function fmtUptime(s) {
  if (s < 60) return s + "s";
  const m = Math.floor(s / 60), h = Math.floor(m / 60), d = Math.floor(h / 24);
  if (d > 0) return d + "d " + (h % 24) + "h";
  if (h > 0) return h + "h " + (m % 60) + "m";
  return m + "m " + (s % 60) + "s";
}

async function api(path) {
  const r = await fetch(path, { headers: { Authorization: "Bearer " + key } });
  if (!r.ok) {
    let msg = r.status + "";
    try { msg = (await r.json()).error || msg; } catch (e) {}
    throw new Error(msg);
  }
  return r.json();
}

async function refresh() {
  try {
    const [st, caps] = await Promise.all([
      api("/v1/status"),
      api("/v1/bridge/capabilities"),
    ]);
    $("err").style.display = "none";

    $("health").innerHTML = pill(st.status === "ok", "ok", "down");
    $("version").textContent = st.version || "—";
    $("uptime").textContent = fmtUptime(st.uptime_seconds || 0);
    $("pairing").textContent = st.require_pairing ? "yes" : "no (any key)";
    $("pending").textContent = st.pending_jobs;

    $("bonline").innerHTML = pill(st.bridge_online, "online", "offline");
    const c = caps.capabilities || {};
    $("bhost").textContent = c.hostname || "—";
    $("bver").textContent = c.version || "—";
    $("brep").textContent = caps.reported_at
      ? new Date(caps.reported_at).toLocaleString() : "never";

    const tb = $("backends");
    const list = c.backends || [];
    if (!list.length) {
      tb.innerHTML = '<tr><td colspan="4" class="empty">No bridge has reported yet</td></tr>';
    } else {
      tb.innerHTML = list.map(b =>
        "<tr><td><code>" + b.name + "</code></td>" +
        "<td>" + pill(b.installed, "yes", "not found") + "</td>" +
        "<td>" + (b.supported
          ? '<span class="pill ok"><span class="dot"></span>implemented</span>'
          : '<span class="pill warn"><span class="dot"></span>stub</span>') + "</td>" +
        "<td>" + pill(b.ready, "ready", "no") + "</td></tr>"
      ).join("");
    }
  } catch (e) {
    $("err").textContent = "Error: " + e.message;
    $("err").style.display = "block";
  }
}

function connect() {
  key = $("key").value.trim();
  if (!key) return;
  if (timer) clearInterval(timer);
  refresh();
  timer = setInterval(refresh, 5000);
}

$("go").addEventListener("click", connect);
$("key").addEventListener("keydown", e => { if (e.key === "Enter") connect(); });
</script>
</body>
</html>`
