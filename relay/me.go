// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: The signed-in user's own surface. On a multi-tenant relay a
//
//	regular (non-admin) user landing on "/" gets a status page scoped entirely
//	to them: whether THEIR bridge is online, what it can run, and how many jobs
//	they have queued — never anyone else's, and never any prompt or result
//	content. It is backed by GET /v1/me, which authenticates by OIDC SESSION
//	only and derives the subject from that session, so there is no target_user
//	parameter and thus no way to read another user's status. Admins have the
//	console; this is the counterpart for everyone else.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http"
	"time"

	"github.com/ToolTropolis/Relayent/internal/api"
)

// me returns the caller's own status, scoped to their OIDC session. Unlike the
// pairing-key /v1/status, this is authenticated by the session cookie and never
// accepts a target_user — the subject is always the signed-in user, so a user
// can only ever see their own bridge and pending count. Content is never
// included.
func (s *server) me(w http.ResponseWriter, r *http.Request) {
	if !s.store.Enabled() || s.oidc == nil {
		writeErr(w, http.StatusNotFound, "not available on this relay")
		return
	}
	p := s.oidc.principalFromSession(r)
	if p == nil {
		writeErr(w, http.StatusUnauthorized, "sign in required")
		return
	}
	sub := p.UserID // from the session only — never from the request

	resp := api.MeResponse{
		Sub:          sub,
		BridgeOnline: s.q.BridgeOnline(sub),
		PendingJobs:  s.q.PendingCount(sub),
	}
	if u, err := s.store.GetUser(sub); err == nil {
		resp.Email = u.Email
		resp.DisplayName = u.DisplayName
	}
	caps, reportedAt, _ := s.q.Capabilities(sub)
	caps.Backends = s.filterDisabledBackends(caps.Backends)
	resp.Capabilities = caps
	if !reportedAt.IsZero() {
		resp.ReportedAt = reportedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// mePageHTML is the signed-in user's status page. Its script runs under the
// per-request nonce injected by renderPage; it reads only GET /v1/me (which is
// session-scoped) and the relay's /v1/health. It builds the DOM with
// textContent, never innerHTML, so a bridge-reported string can never execute.
const mePageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Relayent — Your status</title>
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg%20width%3D%22512%22%20height%3D%22512%22%20viewBox%3D%220%200%20512%20512%22%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%3E%3Crect%20x%3D%2248%22%20y%3D%2248%22%20width%3D%22416%22%20height%3D%22416%22%20rx%3D%22104%22%20fill%3D%22%236366f1%22%2F%3E%3Cpath%20d%3D%22M150%20330%20L256%20214%20L362%20182%22%20fill%3D%22none%22%20stroke%3D%22%23fff%22%20stroke-width%3D%2226%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%2F%3E%3Ccircle%20cx%3D%22150%22%20cy%3D%22330%22%20r%3D%2230%22%20fill%3D%22%23fff%22%2F%3E%3Ccircle%20cx%3D%22362%22%20cy%3D%22182%22%20r%3D%2230%22%20fill%3D%22%23fff%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2246%22%20fill%3D%22%23fff%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2221%22%20fill%3D%22%234f46e5%22%2F%3E%3C%2Fsvg%3E">
<style>
  :root {
    --bg: #0f1115; --card: #171a21; --line: #262b36; --fg: #e6e9ef;
    --muted: #949cad; --ok: #37d67a; --bad: #f2635f; --warn: #f2c15f; --accent: #6ea8fe;
  }
  * { box-sizing: border-box; }
  body { margin:0; padding:2rem 1.25rem; background:var(--bg); color:var(--fg);
    font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif; }
  .wrap { max-width: 720px; margin: 0 auto; }
  .top { display:flex; align-items:center; justify-content:space-between; gap:1rem; margin-bottom:1.5rem; }
  h1 { margin:0 0 .2rem; font-size:1.4rem; letter-spacing:-.02em; }
  .sub { color:var(--muted); margin:0; font-size:.9rem; }
  a.signout { color:var(--muted); text-decoration:none; border:1px solid var(--line);
    padding:.4rem .8rem; border-radius:8px; font-size:.85rem; white-space:nowrap; }
  a.signout:hover { color:var(--fg); border-color:var(--muted); }
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
  .dot { width:8px; height:8px; border-radius:50%; display:inline-block; background:var(--muted); }
  .ok .dot{background:var(--ok)} .ok{color:var(--ok)}
  .bad .dot{background:var(--bad)} .bad{color:var(--bad)}
  table { width:100%; border-collapse:collapse; }
  th,td { text-align:left; padding:.5rem .4rem; border-bottom:1px solid var(--line); }
  th { color:var(--muted); font-size:.78rem; text-transform:uppercase; letter-spacing:.06em; }
  tr:last-child td { border-bottom:0; }
  .empty { color:var(--muted); font-style:italic; }
  .note { display:flex; gap:.5rem; align-items:flex-start; margin-top:.25rem;
    padding:.7rem .8rem; border-radius:8px; font-size:.88rem; line-height:1.5;
    border:1px solid var(--line); }
  .note.warn { background:color-mix(in srgb, var(--warn) 10%, transparent);
    border-color:color-mix(in srgb, var(--warn) 35%, transparent); }
  .note b { display:block; margin-bottom:.15rem; }
  .foot { color:var(--muted); font-size:.85rem; text-align:center; margin-top:1.5rem; }
  .foot a { color:var(--accent); text-decoration:none; }
  .err { color:var(--bad); }
</style>
</head>
<body>
<div class="wrap">
  <div class="top">
    <div>
      <h1>Your Relayent status</h1>
      <p class="sub" id="who">—</p>
    </div>
    <a class="signout" href="/v1/auth/logout">Sign out</a>
  </div>

  <div class="card">
    <h2>Your bridge</h2>
    <div class="row"><span class="k">Status</span><span class="v" id="bonline">—</span></div>
    <div class="row"><span class="k">Host</span><span class="v" id="bhost">—</span></div>
    <div class="row"><span class="k">Version</span><span class="v" id="bver">—</span></div>
    <div class="row"><span class="k">Last reported</span><span class="v" id="brep">—</span></div>
    <div class="row"><span class="k">Pending jobs</span><span class="v" id="pending">—</span></div>
    <div id="noBridge" class="note warn" hidden>
      <span>⚠</span>
      <span><b>No bridge is paired yet.</b>Install the Relayent bridge on the machine whose CLI subscription you want to use, then ask an admin for a one-time enrolment token to pair it. Once it is online, your backends appear below.</span>
    </div>
  </div>

  <div class="card">
    <h2>What your bridge can run</h2>
    <table>
      <thead><tr><th>Backend</th><th>Ready</th></tr></thead>
      <tbody id="backends"><tr><td colspan="2" class="empty">Loading…</td></tr></tbody>
    </table>
  </div>

  <p class="foot" id="foot">
    Auto-refreshes every 5s · <a href="https://github.com/ToolTropolis/Relayent/blob/main/INSTALL.md">Install the bridge</a>
  </p>
</div>

<script nonce="%NONCE%">
(function () {
  function set(id, text) { var el = document.getElementById(id); if (el) el.textContent = text; }
  function pill(id, ok, okText, badText) {
    var el = document.getElementById(id);
    if (!el) return;
    el.textContent = "";
    el.className = "v pill " + (ok ? "ok" : "bad");
    var d = document.createElement("span"); d.className = "dot"; el.appendChild(d);
    el.appendChild(document.createTextNode(ok ? okText : badText));
  }
  function ago(iso) {
    if (!iso) return "never";
    var t = Date.parse(iso); if (isNaN(t)) return "—";
    var s = Math.max(0, Math.round((Date.now() - t) / 1000));
    if (s < 60) return s + "s ago";
    if (s < 3600) return Math.round(s / 60) + "m ago";
    return Math.round(s / 3600) + "h ago";
  }

  function renderBackends(caps) {
    var tb = document.getElementById("backends");
    tb.textContent = "";
    var list = (caps && caps.backends) || [];
    if (!list.length) {
      var tr = document.createElement("tr");
      var td = document.createElement("td");
      td.colSpan = 2; td.className = "empty";
      td.textContent = "No backends reported yet.";
      tr.appendChild(td); tb.appendChild(tr);
      return;
    }
    list.forEach(function (b) {
      var tr = document.createElement("tr");
      var name = document.createElement("td");
      name.textContent = String(b.name || "");
      var ready = document.createElement("td");
      var span = document.createElement("span");
      span.className = "pill " + (b.ready ? "ok" : "bad");
      var dot = document.createElement("span"); dot.className = "dot"; span.appendChild(dot);
      span.appendChild(document.createTextNode(b.ready ? "ready" : (b.installed ? "not signed in" : "not installed")));
      ready.appendChild(span);
      tr.appendChild(name); tr.appendChild(ready);
      tb.appendChild(tr);
    });
  }

  function load() {
    fetch("/v1/me", { headers: { "Accept": "application/json" } })
      .then(function (r) {
        if (r.status === 401) { location.href = "/login"; return null; }
        if (!r.ok) throw new Error("status " + r.status);
        return r.json();
      })
      .then(function (m) {
        if (!m) return;
        set("who", m.display_name || m.email || m.sub || "Signed in");
        pill("bonline", !!m.bridge_online, "online", "offline");
        var caps = m.capabilities || {};
        set("bhost", caps.hostname || "—");
        set("bver", caps.version || "—");
        set("brep", ago(m.reported_at));
        set("pending", (m.pending_jobs != null ? m.pending_jobs : "—"));
        var hasBridge = !!m.bridge_online || (caps.backends && caps.backends.length) || m.reported_at;
        document.getElementById("noBridge").hidden = !!hasBridge;
        renderBackends(caps);
      })
      .catch(function (e) {
        set("foot", "");
        var f = document.getElementById("foot");
        f.className = "foot err";
        f.textContent = "Could not load your status: " + e.message;
      });
  }

  load();
  setInterval(load, 5000);
})();
</script>
</body>
</html>`
