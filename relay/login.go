// Primary author: Navjyot Nishant
// Created on: 2026-07-17
// Last updated: 2026-07-17
// Description: The dedicated login page served at /login — the single sign-in
//
//	surface for the relay's human console. It offers OIDC sign-in (the
//	"Sign in with <provider>" button) and, for bootstrap, a field to paste
//	RELAYENT_ADMIN_TOKEN. On success the browser is routed by role: an admin
//	to /admin, a regular user to / (their own status page). If the visitor
//	already has a session, /login redirects immediately rather than showing
//	the form again.
//
//	Like the other pages it is self-contained and CSP-nonce'd; the token is
//	validated by a probe XHR and kept only in the URL hash it hands to /admin,
//	never persisted here.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"net/http"
	"strings"
)

// loginPage serves /login. Available only in multi-tenant mode (there is nothing
// to sign into otherwise). An existing admin session is bounced to /admin, a
// non-admin session to /, so /login is never a dead end for the already-signed-in.
func (s *server) loginPage(w http.ResponseWriter, r *http.Request) {
	if !s.store.Enabled() {
		writeErr(w, http.StatusNotFound, "login is not enabled on this relay")
		return
	}
	if s.oidc != nil {
		if p := s.oidc.principalFromSession(r); p != nil {
			if p.Can(ScopeAdmin) {
				http.Redirect(w, r, "/admin", http.StatusFound)
			} else {
				http.Redirect(w, r, "/", http.StatusFound)
			}
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

	ssoBlock := ""
	if s.oidc != nil {
		ssoBlock = `<a class="ssobtn" href="/v1/auth/login"><button type="button">Sign in with ` +
			htmlEscape(s.oidc.providerName) + `</button></a>`
	}
	page := strings.Replace(loginHTML, "%NONCE%", nonce, 1)
	page = strings.Replace(page, "%SSO_BUTTON%", ssoBlock, 1)
	_, _ = w.Write([]byte(page))
}

const loginHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Relayent — Sign in</title>
<style>
  :root {
    --bg:#0e1014; --card:#181c24; --line:#262b36; --fg:#e6e9ef;
    --muted:#8b93a4; --bad:#f2635f; --accent:#6ea8fe;
  }
  @media (prefers-color-scheme: light) {
    :root { --bg:#f4f6f9; --card:#fff; --line:#e3e6ec; --fg:#1a1d23; --muted:#5d6472; }
  }
  * { box-sizing:border-box; }
  html,body { height:100%; }
  body { margin:0; background:
      radial-gradient(1200px 600px at 50% -10%, color-mix(in srgb,var(--accent) 14%,transparent), transparent),
      var(--bg);
    color:var(--fg); font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
    display:flex; align-items:center; justify-content:center; padding:1.25rem; }
  .card { background:var(--card); border:1px solid var(--line); border-radius:16px;
    padding:2rem 1.75rem; width:100%; max-width:400px;
    box-shadow:0 20px 50px -20px rgba(0,0,0,.5); }
  .mark { width:40px; height:40px; border-radius:11px;
    background:linear-gradient(135deg,var(--accent),#9f7bff); margin-bottom:1.1rem; }
  h1 { margin:0 0 .3rem; font-size:1.35rem; letter-spacing:-.02em; }
  .sub { color:var(--muted); margin:0 0 1.5rem; font-size:.92rem; }
  .ssobtn { display:block; text-decoration:none; }
  .ssobtn button { width:100%; background:var(--accent); color:#0b1020; border:0;
    padding:.7rem 1rem; border-radius:10px; font:inherit; font-weight:700; cursor:pointer; }
  .ssobtn button:hover { filter:brightness(1.08); }
  .divider { display:flex; align-items:center; gap:.75rem; color:var(--muted);
    font-size:.78rem; text-transform:uppercase; letter-spacing:.08em; margin:1.25rem 0; }
  .divider::before,.divider::after { content:""; flex:1; height:1px; background:var(--line); }
  label { display:block; color:var(--muted); font-size:.8rem; margin-bottom:.35rem; }
  .row { display:flex; gap:.5rem; }
  input { flex:1; min-width:0; background:var(--bg); border:1px solid var(--line);
    color:var(--fg); padding:.6rem .7rem; border-radius:10px; font:inherit; }
  .row button { background:transparent; color:var(--fg); border:1px solid var(--line);
    border-radius:10px; padding:.6rem .9rem; font:inherit; font-weight:600; cursor:pointer; }
  .row button:hover { border-color:var(--accent); }
  .err { display:none; color:var(--bad); font-size:.85rem; margin-top:.7rem; }
  .err.show { display:block; }
  .foot { color:var(--muted); font-size:.78rem; margin-top:1.5rem; text-align:center; }
  .foot a { color:var(--muted); }
</style>
</head>
<body>
  <div class="card">
    <div class="mark"></div>
    <h1>Sign in to Relayent</h1>
    <p class="sub">Use your identity provider, or the bootstrap admin token.</p>

    %SSO_BUTTON%

    <div class="divider" id="or">or</div>

    <label for="tok">Bootstrap admin token</label>
    <div class="row">
      <input id="tok" type="password" placeholder="RELAYENT_ADMIN_TOKEN" autocomplete="off">
      <button id="usetok" type="button">Continue</button>
    </div>
    <p id="err" class="err"></p>

    <p class="foot">Regular users are taken to their status page. Admins reach the console.</p>
  </div>

<script nonce="%NONCE%">
const $ = id => document.getElementById(id);
// If OIDC isn't configured there's no SSO button; the divider label is then noise.
if (!document.querySelector(".ssobtn")) { const o = $("or"); if (o) o.textContent = "Admin token"; }

function nextTarget() {
  const p = new URLSearchParams(location.search).get("next");
  // Only allow same-origin absolute paths — never an external redirect.
  return (p && p.startsWith("/") && !p.startsWith("//")) ? p : "/admin";
}

async function useToken() {
  const t = $("tok").value.trim();
  const err = $("err");
  err.className = "err";
  if (!t) { err.textContent = "Enter the admin token."; err.className = "err show"; return; }
  try {
    // Validate the token against an admin-only endpoint before routing.
    const r = await fetch("/v1/admin/config", { headers: { "Authorization": "Bearer " + t } });
    if (r.status === 401 || r.status === 403) { err.textContent = "That token was rejected."; err.className = "err show"; return; }
    if (!r.ok) { err.textContent = "Unexpected error (" + r.status + ")."; err.className = "err show"; return; }
    // Hand the token to /admin via the URL fragment (never sent to the server, never stored).
    location.assign(nextTarget() + "#token=" + encodeURIComponent(t));
  } catch (e) { err.textContent = "Network error."; err.className = "err show"; }
}
$("usetok").onclick = useToken;
$("tok").addEventListener("keydown", e => { if (e.key === "Enter") useToken(); });
</script>
</body>
</html>`
