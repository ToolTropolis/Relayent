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
	page = strings.ReplaceAll(page, "%VERSION%", htmlEscape(Version))
	_, _ = w.Write([]byte(page))
}

const loginHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Relayent — Sign in</title>
<style>
  /* Shares the console's identity: cool indigo-biased neutral + one flat accent. */
  :root {
    --bg:#0b0d12; --card:#141821; --card-2:#181d27; --line:#222836; --line-soft:#1a1f2b;
    --fg:#e8eaf0; --fg-dim:#c2c7d4; --muted:#8790a2; --faint:#5b6478;
    --accent:#6366f1; --accent-fg:#c7cbff;
    --accent-soft:color-mix(in srgb,var(--accent) 18%,transparent); --bad:#ef4444;
    color-scheme:dark;
  }
  @media (prefers-color-scheme: light) {
    :root { --bg:#f7f8fa; --card:#ffffff; --card-2:#f7f8fb; --line:#e5e8ef; --line-soft:#eef0f5;
      --fg:#141824; --fg-dim:#3a4152; --muted:#5b6373; --faint:#98a0b0;
      --accent:#5457ee; --accent-fg:#3f43d6;
      --accent-soft:color-mix(in srgb,var(--accent) 12%,transparent); color-scheme:light; }
  }
  :root[data-theme="dark"] { --bg:#0b0d12; --card:#141821; --card-2:#181d27; --line:#222836;
    --line-soft:#1a1f2b; --fg:#e8eaf0; --fg-dim:#c2c7d4; --muted:#8790a2; --faint:#5b6478;
    --accent:#6366f1; --accent-fg:#c7cbff;
    --accent-soft:color-mix(in srgb,var(--accent) 18%,transparent); color-scheme:dark; }
  :root[data-theme="light"] { --bg:#f7f8fa; --card:#ffffff; --card-2:#f7f8fb; --line:#e5e8ef;
    --line-soft:#eef0f5; --fg:#141824; --fg-dim:#3a4152; --muted:#5b6373; --faint:#98a0b0;
    --accent:#5457ee; --accent-fg:#3f43d6;
    --accent-soft:color-mix(in srgb,var(--accent) 12%,transparent); color-scheme:light; }

  * { box-sizing:border-box; }
  html,body { height:100%; }
  body { margin:0; color:var(--fg);
    font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
    -webkit-font-smoothing:antialiased;
    background:
      radial-gradient(900px 500px at 50% -15%, var(--accent-soft), transparent 70%),
      var(--bg);
    display:flex; flex-direction:column; align-items:center; justify-content:center;
    padding:1.5rem; gap:1.5rem; }
  ::selection { background:var(--accent-soft); }

  .card { background:linear-gradient(180deg,var(--card-2),var(--card)); border:1px solid var(--line);
    border-radius:18px; padding:2.1rem 1.9rem; width:100%; max-width:404px;
    box-shadow:0 1px 0 rgba(255,255,255,.04) inset, 0 30px 60px -28px rgba(0,0,0,.65); }
  .brandline { display:flex; align-items:center; gap:.65rem; margin-bottom:1.5rem; }
  .mark { width:34px; height:34px; border-radius:10px; flex:none; position:relative;
    background:linear-gradient(150deg,#818cf8,var(--accent) 55%,#4f46e5);
    box-shadow:0 6px 18px -6px color-mix(in srgb,var(--accent) 60%,transparent); }
  .mark::after { content:""; position:absolute; inset:0; border-radius:10px;
    box-shadow:inset 0 1px 0 rgba(255,255,255,.35); }
  .brandline b { font-size:1.02rem; letter-spacing:-.015em; font-weight:650; }
  .brandline span { display:block; color:var(--faint); font-size:.68rem; text-transform:uppercase;
    letter-spacing:.06em; }
  h1 { margin:0 0 .3rem; font-size:1.4rem; letter-spacing:-.025em; font-weight:650; }
  .sub { color:var(--muted); margin:0 0 1.6rem; font-size:.9rem; }
  .ssobtn { display:block; text-decoration:none; }
  .ssobtn button { width:100%; display:flex; align-items:center; justify-content:center; gap:.55rem;
    background:var(--accent); color:#fff; border:0; padding:.75rem 1rem; border-radius:11px;
    font:inherit; font-weight:650; cursor:pointer; transition:filter .12s ease, transform .04s ease; }
  .ssobtn button:hover { filter:brightness(1.09); }
  .ssobtn button:active { transform:translateY(1px); }
  .divider { display:flex; align-items:center; gap:.8rem; color:var(--faint);
    font-size:.72rem; text-transform:uppercase; letter-spacing:.1em; margin:1.35rem 0; }
  .divider::before,.divider::after { content:""; flex:1; height:1px; background:var(--line-soft); }
  label { display:block; color:var(--muted); font-size:.78rem; margin-bottom:.4rem; font-weight:500; }
  .row { display:flex; gap:.55rem; }
  input { flex:1; min-width:0; background:var(--bg); border:1px solid var(--line);
    color:var(--fg); padding:.65rem .75rem; border-radius:11px; font:inherit;
    transition:border-color .12s ease, box-shadow .12s ease; }
  input:focus { outline:none; border-color:var(--accent); box-shadow:0 0 0 3px var(--accent-soft); }
  input::placeholder { color:var(--faint); }
  .row button { background:transparent; color:var(--fg-dim); border:1px solid var(--line);
    border-radius:11px; padding:.65rem 1rem; font:inherit; font-weight:600; cursor:pointer;
    transition:border-color .12s ease, color .12s ease; }
  .row button:hover { border-color:var(--accent); color:var(--fg); }
  .row button:active { transform:translateY(1px); }
  .err { display:none; color:var(--bad); font-size:.84rem; margin-top:.8rem; }
  .err.show { display:block; }
  .foot { color:var(--faint); font-size:.78rem; margin-top:1.6rem; padding-top:1.2rem;
    border-top:1px solid var(--line-soft); text-align:center; }

  .pagefoot { display:flex; align-items:center; gap:1rem; color:var(--faint); font-size:.78rem; }
  .pagefoot a { display:inline-flex; align-items:center; gap:.4rem; color:var(--muted);
    text-decoration:none; }
  .pagefoot a:hover { color:var(--fg); }
  .pagefoot svg { width:15px; height:15px; fill:currentColor; }
  .pagefoot .sep { width:1px; height:12px; background:var(--line); }
  @media (prefers-reduced-motion:reduce) { *{animation:none !important; transition:none !important;} }
</style>
</head>
<body>
  <div class="card">
    <div class="brandline">
      <div class="mark"></div>
      <div><b>Relayent</b><span>Admin &amp; access</span></div>
    </div>
    <h1>Sign in</h1>
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

  <footer class="pagefoot">
    <a href="https://github.com/ToolTropolis/Relayent" target="_blank" rel="noopener noreferrer">
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
      GitHub
    </a>
    <span class="sep"></span>
    <span>Relayent v%VERSION% · MIT</span>
  </footer>

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
