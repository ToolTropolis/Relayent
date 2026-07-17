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

	// The SSO button reflects the ACTUAL configured provider ("Sign in with
	// Google"), and is hidden entirely when OIDC is off — otherwise it would be a
	// dead button that 404s. Only the token field shows in that case.
	ssoBlock := ""
	if s.oidc != nil {
		ssoBlock = `<a href="/v1/auth/login"><button>Sign in with ` +
			htmlEscape(s.oidc.providerName) + `</button></a>`
	}

	page := strings.Replace(adminHTML, "%NONCE%", nonce, 1)
	page = strings.Replace(page, "%SSO_BUTTON%", ssoBlock, 1)
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
    --bg:#0f1115; --card:#171a21; --line:#262b36; --fg:#e6e9ef;
    --muted:#949cad; --ok:#37d67a; --bad:#f2635f; --warn:#f2c15f; --accent:#6ea8fe;
  }
  @media (prefers-color-scheme: light) {
    :root { --bg:#f6f7f9; --card:#fff; --line:#e3e6ec; --fg:#1a1d23; --muted:#5d6472; }
  }
  * { box-sizing:border-box; }
  body { margin:0; padding:2rem 1rem; background:var(--bg); color:var(--fg);
    font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif; }
  .wrap { max-width:960px; margin:0 auto; }
  h1 { margin:0 0 .25rem; font-size:1.5rem; letter-spacing:-.02em; }
  .sub { color:var(--muted); margin:0 0 1.5rem; }
  .card { background:var(--card); border:1px solid var(--line); border-radius:12px;
    padding:1.1rem 1.25rem; margin-bottom:1rem; }
  .card h2 { margin:0 0 .85rem; font-size:.8rem; text-transform:uppercase;
    letter-spacing:.08em; color:var(--muted); font-weight:600; }
  table { width:100%; border-collapse:collapse; }
  th,td { text-align:left; padding:.5rem .6rem; border-bottom:1px solid var(--line);
    font-variant-numeric:tabular-nums; }
  th { color:var(--muted); font-size:.75rem; text-transform:uppercase; letter-spacing:.05em; }
  tr:last-child td { border-bottom:0; }
  .pill { display:inline-flex; align-items:center; gap:.35rem; font-weight:600; font-size:.85rem; }
  .dot { width:8px; height:8px; border-radius:50%; display:inline-block; }
  .ok .dot{background:var(--ok)} .ok{color:var(--ok)}
  .bad .dot{background:var(--bad)} .bad{color:var(--bad)}
  .muted { color:var(--muted); }
  input,button,select { font:inherit; }
  input,select { background:var(--bg); border:1px solid var(--line); color:var(--fg);
    padding:.5rem .65rem; border-radius:8px; }
  button { background:var(--accent); color:#0b1020; border:0; padding:.5rem .9rem;
    border-radius:8px; font-weight:600; cursor:pointer; }
  button:hover { filter:brightness(1.08); }
  button.ghost { background:transparent; color:var(--fg); border:1px solid var(--line); }
  .row { display:flex; gap:.6rem; flex-wrap:wrap; align-items:center; margin-bottom:.6rem; }
  .grow { flex:1; min-width:0; }
  code { font:12.5px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace;
    background:var(--bg); border:1px solid var(--line); border-radius:4px; padding:.05rem .3rem;
    word-break:break-all; }
  .banner { display:none; padding:.7rem .9rem; border-radius:8px; margin-bottom:1rem;
    border:1px solid var(--line); }
  .banner.show { display:block; }
  .banner.ok { border-color:color-mix(in srgb,var(--ok) 40%,transparent);
    background:color-mix(in srgb,var(--ok) 10%,transparent); }
  .banner.bad { border-color:color-mix(in srgb,var(--bad) 40%,transparent);
    background:color-mix(in srgb,var(--bad) 10%,transparent); }
  .secret { border-color:color-mix(in srgb,var(--warn) 45%,transparent);
    background:color-mix(in srgb,var(--warn) 12%,transparent); }
  .foot { color:var(--muted); font-size:.8rem; margin-top:1.5rem; }
</style>
</head>
<body>
<div class="wrap">
  <h1>Relayent Admin</h1>
  <p class="sub">Manage users, enrol bridges, issue app credentials, and view per-user
  activity. This dashboard never shows prompt or result content.</p>

  <div id="banner" class="banner"></div>

  <div id="authcard" class="card" style="display:none">
    <h2>Sign in</h2>
    <p class="muted">You are not authenticated as an admin. Sign in with your identity
    provider, or paste the bootstrap admin token.</p>
    <div class="row">
      %SSO_BUTTON%
      <input id="tok" class="grow" type="password" placeholder="or paste RELAYENT_ADMIN_TOKEN"
        autocomplete="off">
      <button id="usetok" class="ghost">Use token</button>
    </div>
  </div>

  <div id="app" style="display:none">
    <div class="card">
      <h2>Users</h2>
      <div class="row">
        <input id="nsub"   placeholder="user id (OIDC sub, or any id)">
        <input id="nemail" placeholder="email">
        <button id="adduser">Add user</button>
      </div>
      <table>
        <thead><tr><th>User</th><th>Role</th><th>Bridge</th><th>Pending</th><th>Bridges</th><th></th></tr></thead>
        <tbody id="users"><tr><td colspan="6" class="muted">Loading…</td></tr></tbody>
      </table>
    </div>

    <div class="card">
      <h2>App credentials</h2>
      <div class="row">
        <input id="appid" placeholder="app id (e.g. engagehub)">
        <button id="addapp">Issue credential</button>
      </div>
      <table>
        <thead><tr><th>App</th><th>ID</th><th>Scopes</th><th>Status</th><th></th></tr></thead>
        <tbody id="apps"><tr><td colspan="5" class="muted">Loading…</td></tr></tbody>
      </table>
    </div>

    <div class="card">
      <h2>Recent activity <span class="muted">— no content, ever</span></h2>
      <table>
        <thead><tr><th>When</th><th>Event</th><th>User</th><th>Backend</th><th>Status</th><th>Bytes</th></tr></thead>
        <tbody id="audit"><tr><td colspan="6" class="muted">Loading…</td></tr></tbody>
      </table>
    </div>
  </div>

  <p class="foot">Every action here uses the same scope-gated /v1/admin API. Secrets
  (enrolment tokens, app credentials) are shown once — copy them immediately.</p>
</div>

<script nonce="%NONCE%">
const $ = id => document.getElementById(id);
let token = ""; // bootstrap admin token, kept in memory only

function headers() {
  const h = {"Content-Type": "application/json"};
  if (token) h["Authorization"] = "Bearer " + token;
  return h;
}

function banner(msg, kind) {
  const b = $("banner");
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
  if (r.status === 401 || r.status === 403) { showAuth(); throw new Error("unauthorized"); }
  if (!r.ok) {
    let m = r.status + "";
    try { m = (await r.json()).error || m; } catch (e) {}
    throw new Error(m);
  }
  return r.status === 204 ? null : r.json();
}

function showAuth() { $("app").style.display = "none"; $("authcard").style.display = ""; }
function showApp()  { $("authcard").style.display = "none"; $("app").style.display = ""; }

function pill(good, gt, bt) {
  const s = document.createElement("span");
  s.className = "pill " + (good ? "ok" : "bad");
  const d = document.createElement("span"); d.className = "dot"; s.appendChild(d);
  s.appendChild(document.createTextNode(good ? gt : bt));
  return s;
}

function cell(text) { const td = document.createElement("td"); td.textContent = text; return td; }

async function loadUsers() {
  const data = await api("GET", "/v1/admin/users");
  const tb = $("users"); tb.replaceChildren();
  const users = (data && data.users) || [];
  if (!users.length) { tb.innerHTML = ""; const tr=document.createElement("tr");
    const td=cell("No users yet."); td.colSpan=6; td.className="muted"; tr.appendChild(td); tb.appendChild(tr); return; }
  for (const u of users) {
    const tr = document.createElement("tr");
    const who = document.createElement("td");
    who.textContent = (u.email || u.sub);
    if (u.disabled) { const m=document.createElement("span"); m.className="muted"; m.textContent=" (disabled)"; who.appendChild(m); }
    tr.appendChild(who);
    tr.appendChild(cell(u.role));
    const bt = document.createElement("td"); bt.appendChild(pill(u.bridge_online, "online", "offline")); tr.appendChild(bt);
    tr.appendChild(cell(String(u.pending_jobs)));
    tr.appendChild(cell(String(u.bridges)));
    const act = document.createElement("td");
    const enrol = document.createElement("button"); enrol.textContent = "Enrol bridge"; enrol.className="ghost";
    enrol.onclick = () => issueToken(u.sub);
    const tog = document.createElement("button"); tog.textContent = u.disabled ? "Enable" : "Disable"; tog.className="ghost";
    tog.style.marginLeft = ".4rem";
    tog.onclick = () => setDisabled(u.sub, !u.disabled);
    act.appendChild(enrol); act.appendChild(tog); tr.appendChild(act);
    tb.appendChild(tr);
  }
}

async function loadApps() {
  const data = await api("GET", "/v1/admin/app-creds");
  const tb = $("apps"); tb.replaceChildren();
  const creds = (data && data.app_creds) || [];
  if (!creds.length) { const tr=document.createElement("tr"); const td=cell("No app credentials."); td.colSpan=5; td.className="muted"; tr.appendChild(td); tb.appendChild(tr); return; }
  for (const c of creds) {
    const tr = document.createElement("tr");
    tr.appendChild(cell(c.app_id));
    const idc = document.createElement("td"); const code=document.createElement("code"); code.textContent=c.id; idc.appendChild(code); tr.appendChild(idc);
    tr.appendChild(cell((c.scopes || []).join(", ")));
    const st = document.createElement("td"); st.appendChild(pill(!c.revoked, "active", "revoked")); tr.appendChild(st);
    const act = document.createElement("td");
    if (!c.revoked) { const rev=document.createElement("button"); rev.textContent="Revoke"; rev.className="ghost";
      rev.onclick=()=>revokeApp(c.id); act.appendChild(rev); }
    tr.appendChild(act); tb.appendChild(tr);
  }
}

async function loadAudit() {
  const data = await api("GET", "/v1/admin/audit?limit=50");
  const tb = $("audit"); tb.replaceChildren();
  const events = (data && data.events) || [];
  if (!events.length) { const tr=document.createElement("tr"); const td=cell("No activity yet."); td.colSpan=6; td.className="muted"; tr.appendChild(td); tb.appendChild(tr); return; }
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

async function refresh() {
  try { await loadUsers(); await loadApps(); await loadAudit(); showApp(); }
  catch (e) { if (e.message !== "unauthorized") banner("Error: " + e.message, "bad"); }
}

async function issueToken(sub) {
  try {
    const r = await api("POST", "/v1/admin/enroll-tokens", {user_sub: sub});
    showSecret("Enrolment token for " + sub, r.token);
  } catch (e) { banner("Error: " + e.message, "bad"); }
}
async function setDisabled(sub, disabled) {
  try { await api("POST", "/v1/admin/users/" + encodeURIComponent(sub) + "/disabled?disabled=" + disabled);
    banner((disabled ? "Disabled " : "Enabled ") + sub, "ok"); refresh(); }
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
    $("nsub").value = ""; $("nemail").value = ""; banner("User added", "ok"); refresh(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
};
$("addapp").onclick = async () => {
  const app_id = $("appid").value.trim();
  if (!app_id) { banner("app id is required", "bad"); return; }
  try { const r = await api("POST", "/v1/admin/app-creds", {app_id});
    $("appid").value = ""; showSecret("App credential for " + app_id, r.credential); loadApps(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
};
$("usetok").onclick = () => { token = $("tok").value.trim(); $("tok").value = ""; refresh(); };

// On load: try the (cookie) session first; fall back to the token prompt on 401/403.
refresh();
</script>
</body>
</html>`
