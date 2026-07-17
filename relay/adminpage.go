// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: The admin dashboard — a self-contained HTML page served at
//
//	/admin that drives the existing /v1/admin/* API. It adds NO new backend
//	logic or authority: every action goes through the same scope-gated,
//	tested endpoints, so the page is a convenience layer, not a new attack
//	surface. Like the status page it is CSP-nonce'd, has no external assets,
//	and never renders prompt/result content (the API never returns it).
//
//	Layout: an app shell with a grouped sidebar — Admin (Users, Audit),
//	Configure (Status, Enrol a bridge, Settings), and App credentials. A tiny
//	client-side router swaps views; there is one page, no reloads.
//
//	Auth: it uses the browser's OIDC session cookie automatically; for
//	bootstrap (before an OIDC admin exists) an admin can paste the
//	RELAYENT_ADMIN_TOKEN, which is sent as a Bearer header from the page and
//	held only in memory, never persisted.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http"
	"strings"
)

// adminPage serves the dashboard. Available only when a store exists (multi-
// tenant mode); otherwise there is nothing to administer.
func (s *server) adminPage(w http.ResponseWriter, r *http.Request) {
	if !s.store.Enabled() {
		writeErr(w, http.StatusNotFound, "admin is not enabled on this relay")
		return
	}
	// Route by session, server-side, so the console is a clean destination:
	//   - a signed-in admin gets the console,
	//   - a signed-in NON-admin is sent to their own status page (/),
	//   - a visitor with NO OIDC session still gets the console HTML, because the
	//     bootstrap admin authenticates by pasting RELAYENT_ADMIN_TOKEN (a
	//     client-side XHR bearer, not a session) — its boot() probe then either
	//     shows the console or, on 401/403, redirects to /login.
	if s.oidc != nil {
		if p := s.oidc.principalFromSession(r); p != nil && !p.Can(ScopeAdmin) {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}
	nonce, err := scriptNonce()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'unsafe-inline'; "+
			"connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	// Sign-in lives on /login now; the console renders only for an admin (an OIDC
	// admin session, or a bootstrap token adopted from /login's #token hand-off).
	page := strings.Replace(adminHTML, "%NONCE%", nonce, 1)
	_, _ = w.Write([]byte(page))
}

// htmlEscape is a minimal escaper for the one interpolated value (the provider
// name), which comes from a known allowlist but is escaped anyway on principle —
// untrusted-looking data never reaches HTML unescaped in this codebase.
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

const adminHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Relayent — Admin</title>
<style>
  :root {
    --bg:#0e1014; --panel:#14171d; --card:#181c24; --line:#262b36; --fg:#e6e9ef;
    --muted:#8b93a4; --ok:#37d67a; --bad:#f2635f; --warn:#f2c15f; --accent:#6ea8fe;
    --accent-soft:color-mix(in srgb,var(--accent) 16%,transparent);
    --sidebar:250px;
  }
  @media (prefers-color-scheme: light) {
    :root { --bg:#f4f6f9; --panel:#fff; --card:#fff; --line:#e3e6ec; --fg:#1a1d23;
      --muted:#5d6472; --accent-soft:color-mix(in srgb,var(--accent) 12%,transparent); }
  }
  * { box-sizing:border-box; }
  html,body { height:100%; }
  body { margin:0; background:var(--bg); color:var(--fg);
    font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif; }

  /* App shell: fixed sidebar + scrolling main. */
  .shell { display:grid; grid-template-columns:var(--sidebar) 1fr; min-height:100vh; }
  .side { background:var(--panel); border-right:1px solid var(--line);
    display:flex; flex-direction:column; position:sticky; top:0; height:100vh; }
  .brand { display:flex; align-items:center; gap:.6rem; padding:1.15rem 1.25rem;
    border-bottom:1px solid var(--line); }
  .brand .mark { width:26px; height:26px; border-radius:7px;
    background:linear-gradient(135deg,var(--accent),#9f7bff); flex:none; }
  .brand b { font-size:1.05rem; letter-spacing:-.01em; }
  .brand span { color:var(--muted); font-size:.72rem; }
  nav { padding:.75rem .6rem; overflow-y:auto; flex:1; }
  .navgroup { color:var(--muted); font-size:.68rem; text-transform:uppercase;
    letter-spacing:.09em; font-weight:700; padding:.9rem .65rem .35rem; }
  .navlink { display:flex; align-items:center; gap:.6rem; width:100%; text-align:left;
    background:none; border:0; color:var(--fg); font:inherit; cursor:pointer;
    padding:.5rem .65rem; border-radius:8px; margin-bottom:1px; }
  .navlink:hover { background:var(--accent-soft); }
  .navlink.active { background:var(--accent-soft); color:var(--accent); font-weight:600; }
  .navlink .ic { width:16px; text-align:center; opacity:.9; }
  .whoami { border-top:1px solid var(--line); padding:.85rem 1.1rem;
    display:flex; align-items:center; justify-content:space-between; gap:.5rem; font-size:.82rem; }
  .whoami .who { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .whoami a { color:var(--muted); text-decoration:none; }
  .whoami a:hover { color:var(--fg); }

  main { min-width:0; padding:1.75rem 2rem 3rem; }
  .head { margin-bottom:1.25rem; }
  .head h1 { margin:0 0 .2rem; font-size:1.4rem; letter-spacing:-.02em; }
  .head p { margin:0; color:var(--muted); font-size:.9rem; }

  .card { background:var(--card); border:1px solid var(--line); border-radius:12px;
    padding:1.1rem 1.25rem; margin-bottom:1rem; }
  .card h2 { margin:0 0 .85rem; font-size:.75rem; text-transform:uppercase;
    letter-spacing:.08em; color:var(--muted); font-weight:700; }
  .card h2 .note { text-transform:none; letter-spacing:0; font-weight:400; }

  table { width:100%; border-collapse:collapse; }
  th,td { text-align:left; padding:.55rem .6rem; border-bottom:1px solid var(--line);
    font-variant-numeric:tabular-nums; vertical-align:middle; }
  th { color:var(--muted); font-size:.72rem; text-transform:uppercase; letter-spacing:.05em; }
  tr:last-child td { border-bottom:0; }
  .tablewrap { overflow-x:auto; }

  .pill { display:inline-flex; align-items:center; gap:.35rem; font-weight:600; font-size:.85rem; }
  .dot { width:8px; height:8px; border-radius:50%; display:inline-block; }
  .ok .dot{background:var(--ok)} .ok{color:var(--ok)}
  .bad .dot{background:var(--bad)} .bad{color:var(--bad)}
  .tag { font-size:.72rem; font-weight:600; padding:.1rem .5rem; border-radius:999px;
    border:1px solid var(--line); color:var(--muted); }
  .tag.admin { color:var(--accent); border-color:var(--accent-soft); background:var(--accent-soft); }
  .muted { color:var(--muted); }

  input,button,select { font:inherit; }
  input,select { background:var(--bg); border:1px solid var(--line); color:var(--fg);
    padding:.5rem .65rem; border-radius:8px; }
  button { background:var(--accent); color:#0b1020; border:0; padding:.5rem .9rem;
    border-radius:8px; font-weight:600; cursor:pointer; }
  button:hover { filter:brightness(1.08); }
  button.ghost { background:transparent; color:var(--fg); border:1px solid var(--line); }
  button.danger { background:transparent; color:var(--bad); border:1px solid color-mix(in srgb,var(--bad) 45%,transparent); }
  button.sm { padding:.32rem .6rem; font-size:.82rem; }
  .row { display:flex; gap:.6rem; flex-wrap:wrap; align-items:center; margin-bottom:.7rem; }
  .row:last-child { margin-bottom:0; }
  .grow { flex:1; min-width:0; }
  .actions { display:flex; gap:.4rem; flex-wrap:wrap; justify-content:flex-end; }
  code { font:12.5px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;
    background:var(--bg); border:1px solid var(--line); border-radius:4px; padding:.05rem .3rem;
    word-break:break-all; }

  .kv { display:grid; grid-template-columns:170px 1fr; gap:.15rem .5rem; }
  .kv .k { color:var(--muted); padding:.35rem 0; }
  .kv .v { padding:.35rem 0; font-variant-numeric:tabular-nums; word-break:break-all; }
  .stat { display:flex; gap:1.5rem; flex-wrap:wrap; }
  .stat .n { font-size:1.6rem; font-weight:700; letter-spacing:-.02em; }
  .stat .l { color:var(--muted); font-size:.78rem; text-transform:uppercase; letter-spacing:.05em; }

  .banner { display:none; padding:.75rem .95rem; border-radius:10px; margin-bottom:1.1rem;
    border:1px solid var(--line); }
  .banner.show { display:block; }
  .banner.ok { border-color:color-mix(in srgb,var(--ok) 40%,transparent);
    background:color-mix(in srgb,var(--ok) 10%,transparent); }
  .banner.bad { border-color:color-mix(in srgb,var(--bad) 40%,transparent);
    background:color-mix(in srgb,var(--bad) 10%,transparent); }
  .banner.secret { border-color:color-mix(in srgb,var(--warn) 45%,transparent);
    background:color-mix(in srgb,var(--warn) 12%,transparent); }
  .hint { color:var(--muted); font-size:.82rem; margin:.15rem 0 0; }
  .view { display:none; }
  .view.active { display:block; }

  /* Sign-in takes the whole shell (no sidebar) until authenticated. */
  .signwrap { min-height:100vh; display:flex; align-items:center; justify-content:center; padding:1rem; }
  .signcard { max-width:420px; width:100%; }
  .signcard .mark { width:34px; height:34px; border-radius:9px;
    background:linear-gradient(135deg,var(--accent),#9f7bff); margin-bottom:.9rem; }

  @media (max-width:760px) {
    .shell { grid-template-columns:1fr; }
    .side { position:static; height:auto; flex-direction:column; }
    nav { display:flex; flex-wrap:wrap; gap:.25rem; }
    .navgroup { width:100%; padding:.4rem .5rem .1rem; }
    main { padding:1.25rem 1rem 2.5rem; }
    .kv { grid-template-columns:1fr; }
  }
</style>
</head>
<body>

<!-- App shell (shown once authenticated; unauthenticated visitors are sent to /login). -->
<div id="shell" class="shell" style="display:none">
  <aside class="side">
    <div class="brand">
      <div class="mark"></div>
      <div><b>Relayent</b><br><span>Admin console</span></div>
    </div>
    <nav>
      <div class="navgroup">Admin</div>
      <button class="navlink" data-view="users"><span class="ic">◱</span> Users</button>
      <button class="navlink" data-view="audit"><span class="ic">≣</span> Audit</button>
      <div class="navgroup">Configure</div>
      <button class="navlink" data-view="status"><span class="ic">◈</span> Relay &amp; bridges</button>
      <button class="navlink" data-view="enroll"><span class="ic">＋</span> Enrol a bridge</button>
      <button class="navlink" data-view="settings"><span class="ic">⚙</span> Settings</button>
      <div class="navgroup">Integration</div>
      <button class="navlink" data-view="creds"><span class="ic">⚿</span> App credentials</button>
    </nav>
    <div class="whoami">
      <span class="who" id="whoami" title="">—</span>
      <a href="/v1/auth/logout" id="logout">Sign out</a>
    </div>
  </aside>

  <main>
    <div id="banner" class="banner"></div>

    <!-- USERS -->
    <section id="view-users" class="view">
      <div class="head"><h1>Users</h1><p>People with a bridge on this relay. Roles and lifecycle are managed here.</p></div>
      <div class="card">
        <h2>Add a user</h2>
        <div class="row">
          <input id="nsub" class="grow" placeholder="user id (OIDC sub, or any id)">
          <input id="nemail" class="grow" placeholder="email">
          <button id="adduser">Add user</button>
        </div>
        <p class="hint">Normally a user is created automatically on their first sign-in; add one here to pre-provision.</p>
      </div>
      <div class="card">
        <h2>All users</h2>
        <div class="tablewrap"><table>
          <thead><tr><th>User</th><th>Role</th><th>Bridge</th><th>Pending</th><th>Bridges</th><th></th></tr></thead>
          <tbody id="users"><tr><td colspan="6" class="muted">Loading…</td></tr></tbody>
        </table></div>
      </div>
    </section>

    <!-- AUDIT -->
    <section id="view-audit" class="view">
      <div class="head"><h1>Audit</h1><p>Per-user activity — timestamps, events, and byte counts. Never prompt or result content.</p></div>
      <div class="card">
        <h2>Recent activity <span class="note muted">— no content, ever</span></h2>
        <div class="tablewrap"><table>
          <thead><tr><th>When</th><th>Event</th><th>User</th><th>Backend</th><th>Status</th><th>Bytes</th></tr></thead>
          <tbody id="audit"><tr><td colspan="6" class="muted">Loading…</td></tr></tbody>
        </table></div>
      </div>
    </section>

    <!-- STATUS -->
    <section id="view-status" class="view">
      <div class="head"><h1>Relay &amp; bridges</h1><p>Live health of the relay and each user's bridge.</p></div>
      <div class="card">
        <h2>At a glance</h2>
        <div class="stat">
          <div><div class="n" id="s-users">—</div><div class="l">Users</div></div>
          <div><div class="n" id="s-online">—</div><div class="l">Bridges online</div></div>
          <div><div class="n" id="s-pending">—</div><div class="l">Pending jobs</div></div>
        </div>
      </div>
      <div class="card">
        <h2>Bridge presence</h2>
        <div class="tablewrap"><table>
          <thead><tr><th>User</th><th>Bridge</th><th>Enrolled bridges</th><th>Pending</th></tr></thead>
          <tbody id="presence"><tr><td colspan="4" class="muted">Loading…</td></tr></tbody>
        </table></div>
      </div>
    </section>

    <!-- ENROLL -->
    <section id="view-enroll" class="view">
      <div class="head"><h1>Enrol a bridge</h1><p>Mint a one-time token for a user. Send it to them out-of-band; their bridge redeems it once.</p></div>
      <div class="card">
        <h2>Mint an enrolment token</h2>
        <div class="row">
          <select id="enrolluser" class="grow"><option value="">Loading users…</option></select>
          <button id="mint">Mint token</button>
        </div>
        <p class="hint">The token is shown once, here, and never recoverable. It expires in 15 minutes by default.</p>
      </div>
    </section>

    <!-- SETTINGS -->
    <section id="view-settings" class="view">
      <div class="head"><h1>Settings</h1><p>The relay's effective configuration. These values are set in the relay's environment
        (<code>.env</code> / compose) and applied with <code>docker compose up -d</code>; secrets are never shown here.</p></div>
      <div class="card">
        <h2>Relay</h2>
        <div class="kv" id="cfg-relay"><div class="k muted">Loading…</div><div class="v"></div></div>
      </div>
      <div class="card">
        <h2>Identity (OIDC)</h2>
        <div class="kv" id="cfg-oidc"><div class="k muted">Loading…</div><div class="v"></div></div>
      </div>
      <div class="card">
        <h2>Legacy &amp; bootstrap</h2>
        <div class="kv" id="cfg-legacy"><div class="k muted">Loading…</div><div class="v"></div></div>
        <p class="hint">To change any of these: edit the relay's <code>.env</code>, then run
          <code>docker compose up -d</code> (a restart alone does not re-read env). Editing config from
          this UI is intentionally not supported — the relay holds no writable secret store.</p>
      </div>
    </section>

    <!-- CREDENTIALS -->
    <section id="view-creds" class="view">
      <div class="head"><h1>App credentials</h1><p>Server-to-server keys for apps (e.g. EngageHub) that enqueue jobs on users' behalf.</p></div>
      <div class="card">
        <h2>Issue a credential</h2>
        <div class="row">
          <input id="appid" class="grow" placeholder="app id (e.g. engagehub)">
          <button id="addapp">Issue credential</button>
        </div>
        <p class="hint">The secret is shown once. Store it now — the relay keeps only a hash.</p>
      </div>
      <div class="card">
        <h2>Issued credentials</h2>
        <div class="tablewrap"><table>
          <thead><tr><th>App</th><th>ID</th><th>Scopes</th><th>Status</th><th></th></tr></thead>
          <tbody id="apps"><tr><td colspan="5" class="muted">Loading…</td></tr></tbody>
        </table></div>
      </div>
    </section>
  </main>
</div>

<script nonce="%NONCE%">
const $ = id => document.getElementById(id);
let token = ""; // bootstrap admin token, kept in memory only

function headers() {
  const h = {"Content-Type": "application/json"};
  if (token) h["Authorization"] = "Bearer " + token;
  return h;
}

function banner(msg, kind, where) {
  const b = $(where || "banner");
  b.textContent = msg;
  b.className = "banner show " + (kind || "");
  if (kind === "ok") setTimeout(() => { b.className = "banner"; }, 4000);
}

function showSecret(label, value) {
  const b = $("banner");
  b.className = "banner show secret";
  b.replaceChildren();
  const strong = document.createElement("strong");
  strong.textContent = label + " (shown once — copy it now): ";
  const codeEl = document.createElement("code");
  codeEl.textContent = value;   // textContent — never innerHTML
  b.appendChild(strong); b.appendChild(codeEl);
}

async function api(method, path, body) {
  const opt = {method, headers: headers(), credentials: "same-origin"};
  if (body) opt.body = JSON.stringify(body);
  const r = await fetch(path, opt);
  if (r.status === 401 || r.status === 403) {
    // Not (or no longer) an admin here — /login is the single sign-in surface.
    location.assign("/login?next=/admin");
    throw new Error("unauthorized");
  }
  if (!r.ok) {
    let m = r.status + "";
    try { m = (await r.json()).error || m; } catch (e) {}
    throw new Error(m);
  }
  return r.status === 204 ? null : r.json();
}

function showApp() { $("shell").style.display = "grid"; }

/* ---- view router ---- */
const VIEWS = ["users","audit","status","enroll","settings","creds"];
function go(view) {
  if (!VIEWS.includes(view)) view = "users";
  for (const v of VIEWS) $("view-" + v).classList.toggle("active", v === view);
  for (const b of document.querySelectorAll(".navlink"))
    b.classList.toggle("active", b.dataset.view === view);
  if (location.hash !== "#" + view) location.hash = view;
  loadView(view);
}
async function loadView(view) {
  try {
    if (view === "users")    await loadUsers();
    if (view === "audit")    await loadAudit();
    if (view === "status")   await loadStatus();
    if (view === "enroll")   await loadEnrollUsers();
    if (view === "settings") await loadConfig();
    if (view === "creds")    await loadApps();
  } catch (e) { if (e.message !== "unauthorized") banner("Error: " + e.message, "bad"); }
}

/* ---- helpers ---- */
function pill(good, gt, bt) {
  const s = document.createElement("span");
  s.className = "pill " + (good ? "ok" : "bad");
  const d = document.createElement("span"); d.className = "dot"; s.appendChild(d);
  s.appendChild(document.createTextNode(good ? gt : bt));
  return s;
}
function cell(text) { const td = document.createElement("td"); td.textContent = text; return td; }
function emptyRow(tb, cols, text) {
  const tr = document.createElement("tr"); const td = cell(text);
  td.colSpan = cols; td.className = "muted"; tr.appendChild(td); tb.appendChild(tr);
}
function btn(label, cls, fn) {
  const b = document.createElement("button"); b.textContent = label;
  b.className = cls; b.onclick = fn; return b;
}
function kv(container, pairs) {
  container.replaceChildren();
  for (const [k, v] of pairs) {
    const kd = document.createElement("div"); kd.className = "k"; kd.textContent = k;
    const vd = document.createElement("div"); vd.className = "v";
    if (v && v.node) vd.appendChild(v.node); else vd.textContent = (v === "" || v == null) ? "—" : v;
    container.appendChild(kd); container.appendChild(vd);
  }
}
function yesno(b) { return { node: pill(b, "yes", "no") }; }

/* ---- data cache for cross-view stats ---- */
let usersCache = [];

/* ---- USERS ---- */
async function loadUsers() {
  const data = await api("GET", "/v1/admin/users");
  usersCache = (data && data.users) || [];
  const tb = $("users"); tb.replaceChildren();
  if (!usersCache.length) { emptyRow(tb, 6, "No users yet."); return; }
  for (const u of usersCache) {
    const tr = document.createElement("tr");
    const who = document.createElement("td");
    who.textContent = (u.email || u.sub);
    if (u.disabled) { const m=document.createElement("span"); m.className="muted"; m.textContent=" (disabled)"; who.appendChild(m); }
    tr.appendChild(who);

    const roleTd = document.createElement("td");
    const tag = document.createElement("span");
    tag.className = "tag" + (u.role === "admin" ? " admin" : "");
    tag.textContent = u.role; roleTd.appendChild(tag); tr.appendChild(roleTd);

    const bt = document.createElement("td"); bt.appendChild(pill(u.bridge_online, "online", "offline")); tr.appendChild(bt);
    tr.appendChild(cell(String(u.pending_jobs)));
    tr.appendChild(cell(String(u.bridges)));

    const act = document.createElement("td");
    const wrap = document.createElement("div"); wrap.className = "actions";
    wrap.appendChild(btn("Enrol", "ghost sm", () => issueToken(u.sub)));
    if (u.role === "admin")
      wrap.appendChild(btn("Demote", "ghost sm", () => setRole(u.sub, "user")));
    else
      wrap.appendChild(btn("Make admin", "ghost sm", () => setRole(u.sub, "admin")));
    wrap.appendChild(btn(u.disabled ? "Enable" : "Disable", "ghost sm", () => setDisabled(u.sub, !u.disabled)));
    // Self-demote and self-delete are refused by the backend; the banner surfaces
    // the error if an admin tries it on their own row.
    wrap.appendChild(btn("Delete", "danger sm", () => deleteUser(u.sub, u.email || u.sub)));
    act.appendChild(wrap); tr.appendChild(act);
    tb.appendChild(tr);
  }
}

/* ---- AUDIT ---- */
async function loadAudit() {
  const data = await api("GET", "/v1/admin/audit?limit=50");
  const tb = $("audit"); tb.replaceChildren();
  const events = (data && data.events) || [];
  if (!events.length) { emptyRow(tb, 6, "No activity yet."); return; }
  for (const e of events) {
    const tr = document.createElement("tr");
    tr.appendChild(cell(new Date(e.ts).toLocaleString()));
    tr.appendChild(cell(e.event));
    tr.appendChild(cell(e.target_sub || "—"));
    tr.appendChild(cell(e.backend || "—"));
    tr.appendChild(cell(e.status || "—"));
    tr.appendChild(cell(String((e.prompt_len||0) + (e.result_len||0))));
    tb.appendChild(tr);
  }
}

/* ---- STATUS ---- */
async function loadStatus() {
  const data = await api("GET", "/v1/admin/users");
  const users = (data && data.users) || [];
  usersCache = users;
  const online = users.filter(u => u.bridge_online).length;
  const pending = users.reduce((n, u) => n + (u.pending_jobs||0), 0);
  $("s-users").textContent = String(users.length);
  $("s-online").textContent = String(online);
  $("s-pending").textContent = String(pending);
  const tb = $("presence"); tb.replaceChildren();
  if (!users.length) { emptyRow(tb, 4, "No users yet."); return; }
  for (const u of users) {
    const tr = document.createElement("tr");
    tr.appendChild(cell(u.email || u.sub));
    const bt = document.createElement("td"); bt.appendChild(pill(u.bridge_online, "online", "offline")); tr.appendChild(bt);
    tr.appendChild(cell(String(u.bridges)));
    tr.appendChild(cell(String(u.pending_jobs)));
    tb.appendChild(tr);
  }
}

/* ---- ENROLL ---- */
async function loadEnrollUsers() {
  const data = await api("GET", "/v1/admin/users");
  const users = (data && data.users) || [];
  const sel = $("enrolluser"); sel.replaceChildren();
  if (!users.length) { const o=document.createElement("option"); o.value=""; o.textContent="No users yet"; sel.appendChild(o); return; }
  for (const u of users) {
    const o = document.createElement("option"); o.value = u.sub; o.textContent = (u.email || u.sub); sel.appendChild(o);
  }
}
$("mint").onclick = () => { const sub = $("enrolluser").value; if (sub) issueToken(sub); };

/* ---- SETTINGS ---- */
async function loadConfig() {
  const c = await api("GET", "/v1/admin/config");
  kv($("cfg-relay"), [
    ["Version", c.version],
    ["Listen", c.listen],
    ["Trust proxy", yesno(c.trust_proxy)],
    ["Control-plane store", yesno(c.store_enabled)],
    ["Data dir", c.data_dir],
  ]);
  kv($("cfg-oidc"), c.oidc_enabled ? [
    ["Enabled", yesno(true)],
    ["Provider", c.oidc_provider],
    ["Issuer", c.oidc_issuer],
    ["Client ID", c.oidc_client_id],
    ["Redirect URL", c.oidc_redirect],
    ["Hosted domain", c.hosted_domain || "(any)"],
  ] : [["Enabled", yesno(false)], ["Note", "Sign-in uses the bootstrap admin token only."]]);
  kv($("cfg-legacy"), [
    ["Legacy pairing key", yesno(c.pairing_key_set)],
    ["Bootstrap admin token", yesno(c.admin_token_set)],
  ]);
}

/* ---- CREDENTIALS ---- */
async function loadApps() {
  const data = await api("GET", "/v1/admin/app-creds");
  const tb = $("apps"); tb.replaceChildren();
  const creds = (data && data.app_creds) || [];
  if (!creds.length) { emptyRow(tb, 5, "No app credentials."); return; }
  for (const c of creds) {
    const tr = document.createElement("tr");
    tr.appendChild(cell(c.app_id));
    const idc = document.createElement("td"); const code=document.createElement("code"); code.textContent=c.id; idc.appendChild(code); tr.appendChild(idc);
    tr.appendChild(cell((c.scopes || []).join(", ")));
    const st = document.createElement("td"); st.appendChild(pill(!c.revoked, "active", "revoked")); tr.appendChild(st);
    const act = document.createElement("td"); const wrap=document.createElement("div"); wrap.className="actions";
    if (!c.revoked) wrap.appendChild(btn("Revoke", "ghost sm", () => revokeApp(c.id)));
    act.appendChild(wrap); tr.appendChild(act); tb.appendChild(tr);
  }
}

/* ---- actions ---- */
async function issueToken(sub) {
  try {
    const r = await api("POST", "/v1/admin/enroll-tokens", {user_sub: sub});
    showSecret("Enrolment token for " + sub, r.token);
  } catch (e) { banner("Error: " + e.message, "bad"); }
}
async function setRole(sub, role) {
  try { await api("POST", "/v1/admin/users/" + encodeURIComponent(sub) + "/role", {role});
    banner((role === "admin" ? "Promoted " : "Demoted ") + sub, "ok"); loadUsers(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
}
async function setDisabled(sub, disabled) {
  try { await api("POST", "/v1/admin/users/" + encodeURIComponent(sub) + "/disabled?disabled=" + disabled);
    banner((disabled ? "Disabled " : "Enabled ") + sub, "ok"); loadUsers(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
}
async function deleteUser(sub, label) {
  if (!confirm("Delete user " + label + "? Their bridge bindings are not auto-revoked.")) return;
  try { await api("DELETE", "/v1/admin/users/" + encodeURIComponent(sub));
    banner("Deleted " + label, "ok"); loadUsers(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
}
async function revokeApp(id) {
  try { await api("POST", "/v1/admin/app-creds/" + encodeURIComponent(id) + "/revoke");
    banner("Revoked credential", "ok"); loadApps(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
}

$("adduser").onclick = async () => {
  const sub = $("nsub").value.trim(), email = $("nemail").value.trim();
  if (!sub || !email) { banner("user id and email are required", "bad"); return; }
  try { await api("POST", "/v1/admin/users", {sub, email});
    $("nsub").value = ""; $("nemail").value = ""; banner("User added", "ok"); loadUsers(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
};
$("addapp").onclick = async () => {
  const app_id = $("appid").value.trim();
  if (!app_id) { banner("app id is required", "bad"); return; }
  try { const r = await api("POST", "/v1/admin/app-creds", {app_id});
    $("appid").value = ""; showSecret("App credential for " + app_id, r.credential); loadApps(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
};
for (const b of document.querySelectorAll(".navlink"))
  b.onclick = () => go(b.dataset.view);
window.addEventListener("hashchange", () => go(location.hash.slice(1)));

/* Pick up a bootstrap token handed over from /login via the URL fragment
   (#token=...). The fragment is never sent to the server; we read it, keep the
   token in memory only, and strip it from the address bar immediately. */
function adoptTokenFromHash() {
  const h = location.hash || "";
  const m = h.match(/(?:^#|&)token=([^&]+)/);
  if (m) {
    token = decodeURIComponent(m[1]);
    const cleaned = h.replace(/(?:^#|&)token=[^&]+/, "").replace(/^#&/, "#");
    history.replaceState(null, "", location.pathname + location.search + (cleaned === "#" ? "" : cleaned));
  }
}

/* boot: confirm we're an admin; a 401/403 sends us to /login. */
async function boot() {
  adoptTokenFromHash();
  try {
    await api("GET", "/v1/admin/users");
    showApp();
    $("whoami").textContent = "Signed in";
    go(location.hash.slice(1) || "users");
  } catch (e) { if (e.message !== "unauthorized") banner("Error: " + e.message, "bad"); }
}
boot();
</script>
</body>
</html>`
