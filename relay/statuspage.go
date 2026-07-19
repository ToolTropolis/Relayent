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
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
)

// statusPage serves the dashboard. Any unmatched path 404s so this doesn't
// swallow bad /v1 routes.
//
// The page's own script runs under a per-request nonce rather than
// 'unsafe-inline'. That distinction is load-bearing: 'unsafe-inline' also
// authorises inline event handlers (onerror=, onclick=), so any markup that ever
// reached the DOM could execute. With a nonce, only this script runs, and the
// page's CSP overrides the blanket one from securityHeaders.
func (s *server) statusPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	// On a multi-tenant relay, the root is a human entry point, not a
	// pairing-key prompt — that prompt asks for a credential OIDC deployments
	// don't hand out, so an anonymous visitor would see an unfillable form.
	// Route by session instead: an admin goes to their console, a signed-in
	// regular user sees their OWN status page, and anyone not signed in is sent
	// to /login. The pairing-key status page remains reachable at /status for
	// ops who still hold a key. Single-key mode (no store) is unchanged: "/"
	// serves the pairing-key page directly.
	if s.store.Enabled() && s.oidc != nil {
		p := s.oidc.principalFromSession(r)
		if p == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if p.Can(ScopeAdmin) {
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}
		// A signed-in non-admin gets their own scoped status page. It must
		// RENDER here (not redirect), because /login sends regular users to
		// "/" — a redirect back would bounce the two in a loop.
		s.renderPage(w, mePageHTML)
		return
	}
	s.renderPage(w, statusHTML)
}

// classicStatusPage serves the pairing-key global status dashboard at /status,
// in both modes, for operators who authenticate with a pairing key rather than
// an OIDC session. On "/" this page is only reached in single-key mode.
func (s *server) classicStatusPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/status" {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	s.renderPage(w, statusHTML)
}

// renderPage writes a nonce'd, no-store HTML page under the status CSP. The
// page's own script runs under a per-request nonce rather than 'unsafe-inline'
// (which would also authorise injected inline event handlers).
func (s *server) renderPage(w http.ResponseWriter, page string) {
	nonce, err := scriptNonce()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'unsafe-inline'; img-src data:; "+
			"connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(strings.Replace(page, "%NONCE%", nonce, 1)))
}

// scriptNonce returns a fresh 128-bit CSP nonce. It must be unpredictable and
// never reused across responses, or an attacker could embed a known nonce in
// injected markup and defeat the policy.
func scriptNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b), nil
}

const statusHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Relayent — Status</title>
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg%20width%3D%22512%22%20height%3D%22512%22%20viewBox%3D%220%200%20512%20512%22%20fill%3D%22none%22%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20role%3D%22img%22%20aria-label%3D%22Relayent%22%3E%3Ctitle%3ERelayent%3C%2Ftitle%3E%3Cdefs%3E%3ClinearGradient%20id%3D%22rl-bg%22%20x1%3D%2296%22%20y1%3D%2272%22%20x2%3D%22416%22%20y2%3D%22440%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23818cf8%22%2F%3E%3Cstop%20offset%3D%220.55%22%20stop-color%3D%22%236366f1%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%234f46e5%22%2F%3E%3C%2FlinearGradient%3E%3ClinearGradient%20id%3D%22rl-path%22%20x1%3D%22150%22%20y1%3D%22330%22%20x2%3D%22362%22%20y2%3D%22182%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.55%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.95%22%2F%3E%3C%2FlinearGradient%3E%3C%2Fdefs%3E%3Crect%20x%3D%2248%22%20y%3D%2248%22%20width%3D%22416%22%20height%3D%22416%22%20rx%3D%22104%22%20fill%3D%22url%28%23rl-bg%29%22%2F%3E%3Crect%20x%3D%2248.5%22%20y%3D%2248.5%22%20width%3D%22415%22%20height%3D%22415%22%20rx%3D%22103.5%22%20fill%3D%22none%22%20stroke%3D%22%23ffffff%22%20stroke-opacity%3D%220.18%22%20stroke-width%3D%221%22%2F%3E%3Cpath%20d%3D%22M150%20330%20L256%20214%20L362%20182%22%20fill%3D%22none%22%20stroke%3D%22url%28%23rl-path%29%22%20stroke-width%3D%2226%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%2F%3E%3Ccircle%20cx%3D%22150%22%20cy%3D%22330%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22362%22%20cy%3D%22182%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2266%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.14%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2246%22%20fill%3D%22%23ffffff%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2221%22%20fill%3D%22%234f46e5%22%2F%3E%3C%2Fsvg%3E">
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
  .lede { margin:0 0 1rem; color:var(--muted); }
  .steps { margin:0; padding:0; list-style:none; counter-reset:step; }
  .steps li { position:relative; padding:0 0 1.1rem 2rem; counter-increment:step; }
  .steps li:last-child { padding-bottom:0; }
  .steps li::before { content:counter(step); position:absolute; left:0; top:.05rem;
    width:1.4rem; height:1.4rem; border-radius:50%; background:var(--line);
    color:var(--fg); font-size:.75rem; font-weight:700; display:flex;
    align-items:center; justify-content:center; }
  .stepk { font-weight:600; margin-bottom:.4rem; }
  pre { background:var(--bg); border:1px solid var(--line); border-radius:8px;
    padding:.6rem .75rem; margin:0 0 .4rem; overflow-x:auto;
    font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,monospace; }
  code { font:12.5px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;
    background:var(--bg); border:1px solid var(--line); border-radius:4px; padding:.05rem .3rem; }
  .hint { color:var(--muted); font-size:.85rem; margin:.35rem 0 0; }
  .note { display:flex; gap:.5rem; align-items:flex-start; margin-top:.85rem;
    padding:.6rem .7rem; border-radius:8px; font-size:.85rem; line-height:1.45;
    border:1px solid var(--line); }
  .note.bad { background:color-mix(in srgb, var(--bad) 10%, transparent);
    border-color:color-mix(in srgb, var(--bad) 35%, transparent); color:var(--fg); }
  .note.warn { background:color-mix(in srgb, var(--warn) 10%, transparent);
    border-color:color-mix(in srgb, var(--warn) 35%, transparent); color:var(--fg); }
  .note.ok { background:color-mix(in srgb, var(--ok) 8%, transparent);
    border-color:color-mix(in srgb, var(--ok) 30%, transparent); color:var(--fg); }
  .note b { display:block; margin-bottom:.15rem; }
  .mono { font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:.9em; }
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

  <div class="card" id="seccard" style="display:none">
    <h2>Security</h2>
    <div class="row"><span class="k">Encrypted connection (TLS)</span><span class="v" id="stls">—</span></div>
    <div class="row"><span class="k">Exposure</span><span class="v" id="sreach">—</span></div>
    <div class="row"><span class="k">Your key</span><span class="v" id="skey">—</span></div>
    <div id="secnotes"></div>
  </div>

  <div class="card">
    <h2>Bridge</h2>
    <div class="row"><span class="k">Status</span><span class="v" id="bonline">—</span></div>
    <div class="row"><span class="k">Host</span><span class="v" id="bhost">—</span></div>
    <div class="row"><span class="k">Version</span><span class="v" id="bver">—</span></div>
    <div class="row"><span class="k">Last reported</span><span class="v" id="brep">—</span></div>
  </div>

  <div class="card" id="setupcard" style="display:none">
    <h2>Connect a machine</h2>
    <p class="lede">No bridge is polling with your key. Run this on the machine whose
    AI subscription you want to use — it needs Claude Code, Codex or Cursor installed
    and signed in.</p>
    <ol class="steps">
      <li>
        <div class="stepk">Install the bridge</div>
        <pre id="cmd-install">curl -fsSL <span id="origin-a"></span>/install.sh | sh</pre>
        <p class="hint">Prefer to read it first? Download it, read it, then run it —
        that advice applies to any <code>curl | sh</code>, including this one.</p>
      </li>
      <li>
        <div class="stepk">Pair it with this relay</div>
        <pre>relayent-bridge setup</pre>
        <p class="hint">It asks for this relay's URL (<span id="origin-b"></span>)
        and your pairing key. Nothing is saved until it verifies both.</p>
      </li>
      <li>
        <div class="stepk">Keep it running in the background</div>
        <pre>relayent-bridge install</pre>
        <p class="hint">Starts at login and restarts on failure. Remove any time with
        <code>relayent-bridge uninstall</code>.</p>
      </li>
    </ol>
    <p class="hint">The bridge only dials out — it opens no ports on that machine,
    stores no credentials, and runs jobs in an empty folder, not your personal files.</p>
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

<script nonce="%NONCE%">
const $ = id => document.getElementById(id);
let key = "", timer = null;

// pill returns markup for a status pill. Only ever call this with literal text
// controlled by this page — never with values from an API response.
const pill = (good, goodText, badText) =>
  '<span class="pill ' + (good ? 'ok' : 'bad') + '"><span class="dot"></span>' +
  (good ? goodText : badText) + '</span>';

// pillEl builds the same pill as a DOM node, for use where untrusted data sits
// alongside it in a row. Text is set via textContent, so it can never be parsed
// as markup.
function pillEl(cls, text) {
  const span = document.createElement("span");
  span.className = "pill " + cls;
  const dot = document.createElement("span");
  dot.className = "dot";
  span.appendChild(dot);
  span.appendChild(document.createTextNode(text));
  return span;
}

// pillCell wraps pillEl in a <td>.
function pillCell(good, goodText, badText) {
  const td = document.createElement("td");
  td.appendChild(pillEl(good ? "ok" : "bad", good ? goodText : badText));
  return td;
}

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

// renderSecurity grades this relay's actual posture. It deliberately names
// problems plainly rather than reassuring: a user who trusts a page that says
// "secure" when it is not is worse off than one who was told the truth.
function renderSecurity(st) {
  $("seccard").style.display = "";

  const exposed = !!st.network_reachable;
  const tls = !!st.tls;

  $("stls").innerHTML = tls
    ? pill(true, "yes", "")
    : (exposed ? pill(false, "", "NO — traffic is in the clear")
               : '<span class="pill warn"><span class="dot"></span>not needed (localhost)</span>');

  $("sreach").innerHTML = exposed
    ? '<span class="pill warn"><span class="dot"></span>reachable from the network</span>'
    : '<span class="pill ok"><span class="dot"></span>localhost only</span>';

  let keyHTML = '<span class="mono">' + (st.key_fingerprint || "—") + "</span>";
  if (st.key_retiring) {
    keyHTML += ' <span class="pill warn"><span class="dot"></span>retiring</span>';
  }
  $("skey").innerHTML = keyHTML;

  const notes = [];
  // The worst case first: a public relay with no encryption.
  if (exposed && !tls) {
    notes.push(['bad', 'This relay is reachable from the network without TLS.',
      'Every pairing key and prompt crosses the network in plaintext and can be read ' +
      'or stolen in transit. Put it behind HTTPS before using it for anything real — ' +
      'see deploy/docker-compose.yml for automatic free certificates.']);
  }
  if (!st.require_pairing) {
    notes.push(['bad', 'No fixed pairing key is configured.',
      'Any caller can invent a key and use this relay. This is only acceptable on ' +
      'localhost — the relay refuses to start this way when network-reachable.']);
  }
  if (st.key_retiring) {
    notes.push(['warn', 'The key you are using is being rotated out.',
      'It still works, but will stop once the operator removes it. Switch to the new key.']);
  }
  if (st.rotation_active && !st.key_retiring) {
    notes.push(['warn', 'A key rotation is in progress.',
      'Older keys are still accepted. Once every bridge reports a non-retiring key, ' +
      'drop the old ones from RELAYENT_PAIRING_KEY.']);
  }
  if (!notes.length) {
    notes.push(['ok', exposed ? 'This relay is served over TLS with a pairing key enforced.'
                              : 'This relay is bound to localhost and not reachable from the network.',
      'Remember: anyone holding the pairing key can send jobs to your machine and ' +
      'spend your subscription. Treat it like a password.']);
  }
  $("secnotes").innerHTML = notes.map(n =>
    '<div class="note ' + n[0] + '"><div><b>' + n[1] + '</b>' + n[2] + '</div></div>'
  ).join("");
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

    renderSecurity(st);

    // Setup instructions are only useful when there is nothing connected; once a
    // bridge is online they are noise, so they collapse away.
    $("setupcard").style.display = st.bridge_online ? "none" : "";
    const origin = window.location.origin;
    $("origin-a").textContent = origin;
    $("origin-b").textContent = origin;

    $("bonline").innerHTML = pill(st.bridge_online, "online", "offline");
    const c = caps.capabilities || {};
    $("bhost").textContent = c.hostname || "—";
    $("bver").textContent = c.version || "—";
    $("brep").textContent = caps.reported_at
      ? new Date(caps.reported_at).toLocaleString() : "never";

    const tb = $("backends");
    const list = c.backends || [];
    tb.replaceChildren();
    if (!list.length) {
      const tr = document.createElement("tr");
      const td = document.createElement("td");
      td.colSpan = 4; td.className = "empty";
      td.textContent = "No bridge has reported yet";
      tr.appendChild(td); tb.appendChild(tr);
    } else {
      // b.name is attacker-controllable: it arrives verbatim from whoever POSTed
      // /v1/bridge/capabilities with a valid key. It MUST reach the DOM as text,
      // never as parsed HTML — building this row by string concatenation into
      // innerHTML was a stored XSS that could read the pairing key out of this
      // very page. The pills below are built from server-controlled booleans only.
      for (const b of list) {
        const tr = document.createElement("tr");

        const nameTd = document.createElement("td");
        const code = document.createElement("code");
        code.textContent = b.name == null ? "—" : String(b.name);
        nameTd.appendChild(code);
        tr.appendChild(nameTd);

        tr.appendChild(pillCell(!!b.installed, "yes", "not found"));

        const supTd = document.createElement("td");
        supTd.appendChild(b.supported
          ? pillEl("ok", "implemented")
          : pillEl("warn", "stub"));
        tr.appendChild(supTd);

        tr.appendChild(pillCell(!!b.ready, "ready", "no"));
        tb.appendChild(tr);
      }
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
