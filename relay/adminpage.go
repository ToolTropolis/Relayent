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
//	Configure (Status, Settings), and App credentials. A tiny client-side
//	router swaps views; there is one page, no reloads.
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
	//   - a signed-in console user (admin/operator/viewer) gets the console,
	//   - a signed-in plain user is sent to their own status page (/),
	//   - a visitor with NO OIDC session still gets the console HTML, because the
	//     bootstrap admin authenticates by pasting RELAYENT_ADMIN_TOKEN (a
	//     client-side XHR bearer, not a session) — its boot() probe then either
	//     shows the console or, on 401/403, redirects to /login.
	if s.oidc != nil {
		if p := s.oidc.principalFromSession(r); p != nil && !p.Can(ScopeAdminView) {
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
		"default-src 'none'; script-src 'nonce-"+nonce+"'; style-src 'unsafe-inline'; img-src data:; "+
			"connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	// Sign-in lives on /login now; the console renders only for an admin (an OIDC
	// admin session, or a bootstrap token adopted from /login's #token hand-off).
	page := strings.Replace(adminHTML, "%NONCE%", nonce, 1)
	page = strings.ReplaceAll(page, "%VERSION%", htmlEscape(Version))
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
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg%20width%3D%22512%22%20height%3D%22512%22%20viewBox%3D%220%200%20512%20512%22%20fill%3D%22none%22%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20role%3D%22img%22%20aria-label%3D%22Relayent%22%3E%3Ctitle%3ERelayent%3C%2Ftitle%3E%3Cdefs%3E%3ClinearGradient%20id%3D%22rl-bg%22%20x1%3D%2296%22%20y1%3D%2272%22%20x2%3D%22416%22%20y2%3D%22440%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23818cf8%22%2F%3E%3Cstop%20offset%3D%220.55%22%20stop-color%3D%22%236366f1%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%234f46e5%22%2F%3E%3C%2FlinearGradient%3E%3ClinearGradient%20id%3D%22rl-path%22%20x1%3D%22150%22%20y1%3D%22330%22%20x2%3D%22362%22%20y2%3D%22182%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.55%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.95%22%2F%3E%3C%2FlinearGradient%3E%3C%2Fdefs%3E%3Crect%20x%3D%2248%22%20y%3D%2248%22%20width%3D%22416%22%20height%3D%22416%22%20rx%3D%22104%22%20fill%3D%22url%28%23rl-bg%29%22%2F%3E%3Crect%20x%3D%2248.5%22%20y%3D%2248.5%22%20width%3D%22415%22%20height%3D%22415%22%20rx%3D%22103.5%22%20fill%3D%22none%22%20stroke%3D%22%23ffffff%22%20stroke-opacity%3D%220.18%22%20stroke-width%3D%221%22%2F%3E%3Cpath%20d%3D%22M150%20330%20L256%20214%20L362%20182%22%20fill%3D%22none%22%20stroke%3D%22url%28%23rl-path%29%22%20stroke-width%3D%2226%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%2F%3E%3Ccircle%20cx%3D%22150%22%20cy%3D%22330%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22362%22%20cy%3D%22182%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2266%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.14%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2246%22%20fill%3D%22%23ffffff%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2221%22%20fill%3D%22%234f46e5%22%2F%3E%3C%2Fsvg%3E">
<style>
  /* Palette: a cool indigo-biased neutral (chosen, not default grey) + one flat
     indigo accent + semantic good/warn/critical kept separate from the accent.
     Infrastructure-console register — quiet, dense, state legible at a glance. */
  :root {
    --bg:#0b0d12; --panel:#0f1218; --card:#141821; --card-2:#181d27;
    --line:#222836; --line-soft:#1a1f2b; --fg:#e8eaf0; --fg-dim:#c2c7d4;
    --muted:#8790a2; --faint:#5b6478;
    --accent:#6366f1; --accent-fg:#c7cbff;
    --accent-soft:color-mix(in srgb,var(--accent) 18%,transparent);
    --ok:#10b981; --warn:#f59e0b; --bad:#ef4444;
    --shadow:0 1px 0 rgba(255,255,255,.03) inset, 0 12px 32px -18px rgba(0,0,0,.6);
    --sidebar:264px;
    color-scheme:dark;
  }
  @media (prefers-color-scheme: light) {
    :root {
      --bg:#f7f8fa; --panel:#fbfcfe; --card:#ffffff; --card-2:#f7f8fb;
      --line:#e5e8ef; --line-soft:#eef0f5; --fg:#141824; --fg-dim:#3a4152;
      --muted:#5b6373; --faint:#98a0b0;
      --accent:#5457ee; --accent-fg:#3f43d6;
      --accent-soft:color-mix(in srgb,var(--accent) 12%,transparent);
      --shadow:0 1px 2px rgba(16,20,40,.04), 0 12px 28px -20px rgba(16,20,40,.18);
      color-scheme:light;
    }
  }
  /* The viewer's explicit toggle must win over the OS media query, both ways. */
  :root[data-theme="dark"] {
    --bg:#0b0d12; --panel:#0f1218; --card:#141821; --card-2:#181d27;
    --line:#222836; --line-soft:#1a1f2b; --fg:#e8eaf0; --fg-dim:#c2c7d4;
    --muted:#8790a2; --faint:#5b6478; --accent:#6366f1; --accent-fg:#c7cbff;
    --accent-soft:color-mix(in srgb,var(--accent) 18%,transparent);
    --shadow:0 1px 0 rgba(255,255,255,.03) inset, 0 12px 32px -18px rgba(0,0,0,.6);
    color-scheme:dark;
  }
  :root[data-theme="light"] {
    --bg:#f7f8fa; --panel:#fbfcfe; --card:#ffffff; --card-2:#f7f8fb;
    --line:#e5e8ef; --line-soft:#eef0f5; --fg:#141824; --fg-dim:#3a4152;
    --muted:#5b6373; --faint:#98a0b0; --accent:#5457ee; --accent-fg:#3f43d6;
    --accent-soft:color-mix(in srgb,var(--accent) 12%,transparent);
    --shadow:0 1px 2px rgba(16,20,40,.04), 0 12px 28px -20px rgba(16,20,40,.18);
    color-scheme:light;
  }
  * { box-sizing:border-box; }
  html,body { height:100%; }
  body { margin:0; background:var(--bg); color:var(--fg);
    font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
    -webkit-font-smoothing:antialiased; text-rendering:optimizeLegibility; }
  ::selection { background:var(--accent-soft); }
  a { color:var(--accent-fg); }

  /* App shell: fixed sidebar + scrolling main. */
  .shell { display:grid; grid-template-columns:var(--sidebar) 1fr; min-height:100vh; }
  .side { background:var(--panel); border-right:1px solid var(--line);
    display:flex; flex-direction:column; position:sticky; top:0; height:100vh; }
  .brand { display:flex; align-items:center; gap:.7rem; padding:1.25rem 1.35rem 1.1rem;
    border-bottom:1px solid var(--line-soft); }
  .brand .mark { width:30px; height:30px; border-radius:9px; flex:none; position:relative;
    background:center/contain no-repeat url("data:image/svg+xml,%3Csvg%20width%3D%22512%22%20height%3D%22512%22%20viewBox%3D%220%200%20512%20512%22%20fill%3D%22none%22%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20role%3D%22img%22%20aria-label%3D%22Relayent%22%3E%3Ctitle%3ERelayent%3C%2Ftitle%3E%3Cdefs%3E%3ClinearGradient%20id%3D%22rl-bg%22%20x1%3D%2296%22%20y1%3D%2272%22%20x2%3D%22416%22%20y2%3D%22440%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23818cf8%22%2F%3E%3Cstop%20offset%3D%220.55%22%20stop-color%3D%22%236366f1%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%234f46e5%22%2F%3E%3C%2FlinearGradient%3E%3ClinearGradient%20id%3D%22rl-path%22%20x1%3D%22150%22%20y1%3D%22330%22%20x2%3D%22362%22%20y2%3D%22182%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.55%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.95%22%2F%3E%3C%2FlinearGradient%3E%3C%2Fdefs%3E%3Crect%20x%3D%2248%22%20y%3D%2248%22%20width%3D%22416%22%20height%3D%22416%22%20rx%3D%22104%22%20fill%3D%22url%28%23rl-bg%29%22%2F%3E%3Crect%20x%3D%2248.5%22%20y%3D%2248.5%22%20width%3D%22415%22%20height%3D%22415%22%20rx%3D%22103.5%22%20fill%3D%22none%22%20stroke%3D%22%23ffffff%22%20stroke-opacity%3D%220.18%22%20stroke-width%3D%221%22%2F%3E%3Cpath%20d%3D%22M150%20330%20L256%20214%20L362%20182%22%20fill%3D%22none%22%20stroke%3D%22url%28%23rl-path%29%22%20stroke-width%3D%2226%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%2F%3E%3Ccircle%20cx%3D%22150%22%20cy%3D%22330%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22362%22%20cy%3D%22182%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2266%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.14%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2246%22%20fill%3D%22%23ffffff%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2221%22%20fill%3D%22%234f46e5%22%2F%3E%3C%2Fsvg%3E");
    box-shadow:0 4px 14px -4px color-mix(in srgb,var(--accent) 60%,transparent); }
  .brand .mark::after { content:""; position:absolute; inset:0; border-radius:9px;
    box-shadow:inset 0 1px 0 rgba(255,255,255,.35); }
  .brand b { font-size:1.06rem; letter-spacing:-.02em; font-weight:650; }
  .brand span { color:var(--faint); font-size:.7rem; letter-spacing:.02em;
    text-transform:uppercase; }
  nav { padding:.6rem .7rem; overflow-y:auto; flex:1; }
  /* Collapsible group header (a button now). */
  .navgroup { display:flex; align-items:center; gap:.4rem; width:100%; text-align:left;
    background:none; border:0; cursor:pointer; color:var(--faint); font:inherit;
    font-size:.66rem; text-transform:uppercase; letter-spacing:.11em; font-weight:700;
    padding:1rem .7rem .3rem; }
  .navgroup:hover { color:var(--muted); }
  .navgroup .gcaret { font-size:.6rem; transition:transform .15s ease; display:inline-block; }
  .navgroup[aria-expanded="false"] .gcaret { transform:rotate(-90deg); }
  .navgroup-items { overflow:hidden; }
  .navgroup-items[hidden] { display:none; }
  .navlink { display:flex; align-items:center; gap:.65rem; width:100%; text-align:left;
    background:none; border:0; color:var(--fg-dim); font:inherit; font-size:.92rem; cursor:pointer;
    padding:.5rem .7rem; border-radius:8px; margin-bottom:1px; position:relative;
    transition:background .12s ease, color .12s ease; }
  .navlink:hover { background:color-mix(in srgb,var(--fg) 6%,transparent); color:var(--fg); }
  .navlink.active { background:var(--accent-soft); color:var(--accent-fg); font-weight:600; }
  .navlink.active::before { content:""; position:absolute; left:-.7rem; top:50%;
    transform:translateY(-50%); width:3px; height:1.05rem; border-radius:0 3px 3px 0;
    background:var(--accent); }
  .navlink .ic { width:17px; text-align:center; color:var(--faint); font-size:.95rem; }
  .navlink.active .ic { color:var(--accent); }
  .navlink .tw { transition:transform .15s ease; }

  /* Onboard an app: its own top-level entry, set apart with a distinct accent
     so it reads as the primary "start here" action rather than one more item
     grouped under Apps. */
  .navlink-onboard { margin:.3rem 0 .6rem; border:1px solid color-mix(in srgb,var(--warn) 35%,transparent);
    color:var(--warn); font-weight:600; }
  .navlink-onboard .ic { color:var(--warn); }
  .navlink-onboard:hover { background:color-mix(in srgb,var(--warn) 12%,transparent); color:var(--warn); }
  .navlink-onboard.active { background:color-mix(in srgb,var(--warn) 18%,transparent); color:var(--warn); }
  .navlink-onboard.active::before { background:var(--warn); }

  .subnav { display:flex; flex-direction:column; margin:1px 0 2px .55rem;
    padding-left:.55rem; border-left:1px solid var(--line); }
  .subnavlink { text-align:left; background:none; border:0; color:var(--muted);
    font:inherit; font-size:.86rem; cursor:pointer; padding:.34rem .7rem; border-radius:7px;
    transition:background .12s ease, color .12s ease; }
  .subnavlink:hover { background:color-mix(in srgb,var(--fg) 6%,transparent); color:var(--fg); }
  .subnavlink.active { color:var(--accent-fg); font-weight:600; }

  .whoami { border-top:1px solid var(--line-soft); padding:.85rem 1.1rem;
    display:flex; align-items:center; justify-content:space-between; gap:.5rem; font-size:.82rem; }
  .whoami .who { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;
    display:flex; align-items:center; gap:.45rem; color:var(--fg-dim); }
  .whoami .avatar { width:22px; height:22px; border-radius:50%; flex:none;
    background:linear-gradient(150deg,var(--accent),#4f46e5); color:#fff; font-size:.7rem;
    font-weight:700; display:grid; place-items:center; }
  .whoami a { color:var(--muted); text-decoration:none; }
  .whoami a:hover { color:var(--fg); }
  main { min-width:0; padding:1.1rem 2.25rem 2.5rem; display:flex; flex-direction:column;
    min-height:100vh; }

  /* Top-right utility bar: version + GitHub, right-aligned above the content. */
  .topbar { display:flex; align-items:center; justify-content:flex-end; gap:.9rem;
    margin:0 0 1.25rem; }
  .topbar .ver { font:600 .72rem/1 ui-monospace,SFMono-Regular,Menlo,monospace;
    color:var(--faint); }
  .topbar .gh { display:inline-flex; align-items:center; gap:.45rem; color:var(--muted);
    text-decoration:none; font-size:.84rem; font-weight:500; border:1px solid var(--line);
    padding:.4rem .7rem; border-radius:9px; transition:border-color .12s ease, color .12s ease; }
  .topbar .gh:hover { color:var(--fg); border-color:var(--accent); }
  .topbar .gh svg { width:15px; height:15px; fill:currentColor; }
  @media (max-width:760px) { .topbar { margin-bottom:.9rem; } }
  .head { margin-bottom:1.35rem; }
  .head h1 { margin:0 0 .25rem; font-size:1.55rem; line-height:1.15; letter-spacing:-.025em;
    font-weight:650; text-wrap:balance; }
  .head p { margin:0; color:var(--muted); font-size:.92rem; max-width:70ch; }

  .card { background:var(--card); border:1px solid var(--line); border-radius:14px;
    padding:1.2rem 1.35rem; margin-bottom:1.1rem; box-shadow:var(--shadow); }
  .card h2 { margin:0 0 .9rem; font-size:.72rem; text-transform:uppercase;
    letter-spacing:.09em; color:var(--muted); font-weight:700; }
  .card h2 .note { text-transform:none; letter-spacing:0; font-weight:400; color:var(--faint); }
  @keyframes flashcard { 0%{box-shadow:0 0 0 2px var(--accent), var(--shadow);} 100%{box-shadow:var(--shadow);} }
  .card.flash { animation:flashcard 1.1s ease-out; }

  table { width:100%; border-collapse:collapse; }
  th,td { text-align:left; padding:.6rem .65rem; border-bottom:1px solid var(--line-soft);
    font-variant-numeric:tabular-nums; vertical-align:middle; }
  th { color:var(--faint); font-size:.68rem; text-transform:uppercase; letter-spacing:.07em;
    font-weight:700; padding-top:.3rem; padding-bottom:.5rem; }
  tbody tr { transition:background .1s ease; }
  tbody tr:hover { background:color-mix(in srgb,var(--fg) 3%,transparent); }
  tr:last-child td { border-bottom:0; }
  .tablewrap { overflow-x:auto; }

  .pill { display:inline-flex; align-items:center; gap:.4rem; font-weight:600; font-size:.82rem; }
  .dot { width:7px; height:7px; border-radius:50%; display:inline-block; box-shadow:0 0 0 3px transparent; }
  .ok .dot{background:var(--ok); box-shadow:0 0 0 3px color-mix(in srgb,var(--ok) 18%,transparent)} .ok{color:var(--ok)}
  .bad .dot{background:var(--faint)} .bad{color:var(--muted)}
  .muted { color:var(--muted); }

  input,button,select { font:inherit; }
  input,select { background:var(--bg); border:1px solid var(--line); color:var(--fg);
    padding:.55rem .7rem; border-radius:9px; transition:border-color .12s ease, box-shadow .12s ease; }
  input:focus,select:focus { outline:none; border-color:var(--accent);
    box-shadow:0 0 0 3px var(--accent-soft); }
  input::placeholder { color:var(--faint); }
  button { background:var(--accent); color:#fff; border:0; padding:.55rem .95rem;
    border-radius:9px; font-weight:600; cursor:pointer; transition:filter .12s ease, transform .04s ease; }
  button:hover { filter:brightness(1.08); }
  button:active { transform:translateY(1px); }
  button:focus-visible { outline:2px solid var(--accent-fg); outline-offset:2px; }
  button.ghost { background:transparent; color:var(--fg-dim); border:1px solid var(--line); }
  button.ghost:hover { border-color:var(--accent); color:var(--fg); filter:none; }
  button.danger { background:transparent; color:var(--bad); border:1px solid color-mix(in srgb,var(--bad) 40%,transparent); }
  button.danger:hover { background:color-mix(in srgb,var(--bad) 12%,transparent); filter:none; }
  button.sm { padding:.34rem .62rem; font-size:.82rem; border-radius:8px; }
  button:disabled { opacity:.45; cursor:not-allowed; }
  .row { display:flex; gap:.6rem; flex-wrap:wrap; align-items:center; margin-bottom:.7rem; }
  .row:last-child { margin-bottom:0; }
  .grow { flex:1; min-width:0; }
  .actions { display:flex; gap:.4rem; flex-wrap:wrap; justify-content:flex-end; }
  code { font:12.5px/1.4 ui-monospace,SFMono-Regular,Menlo,monospace; color:var(--fg-dim);
    background:var(--bg); border:1px solid var(--line); border-radius:5px; padding:.08rem .35rem;
    word-break:break-all; }
  /* Click-to-copy command chip. */
  .cmd { display:flex; align-items:stretch; gap:0; margin:.35rem 0; max-width:100%; }
  .cmd code { flex:1; min-width:0; border-radius:6px 0 0 6px; border-right:0; padding:.3rem .5rem;
    white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .cmd .copybtn { flex:none; font:600 .72rem/1 -apple-system,sans-serif; color:var(--muted);
    background:var(--card-2); border:1px solid var(--line); border-radius:0 6px 6px 0; cursor:pointer;
    padding:0 .55rem; }
  .cmd .copybtn:hover { color:var(--fg); border-color:var(--accent); }
  .cmd .copybtn.ok { color:var(--ok); border-color:color-mix(in srgb,var(--ok) 40%,transparent); }

  .kv { display:grid; grid-template-columns:180px 1fr; gap:0 .5rem; }
  .kv .k { color:var(--muted); padding:.5rem 0; border-bottom:1px solid var(--line-soft); font-size:.9rem; }
  .kv .v { padding:.5rem 0; border-bottom:1px solid var(--line-soft);
    font-variant-numeric:tabular-nums; word-break:break-all; font-size:.9rem; }
  .kv .k:last-of-type, .kv .v:last-child { border-bottom:0; }

  /* Metric tiles — summary before detail. */
  .stat { display:grid; grid-template-columns:repeat(auto-fit,minmax(150px,1fr)); gap:.9rem; }
  .tile { border:1px solid var(--line); border-radius:12px; padding:1rem 1.1rem;
    background:linear-gradient(180deg,var(--card-2),var(--card)); }
  .tile .n { font-size:1.9rem; font-weight:700; letter-spacing:-.03em; line-height:1;
    font-variant-numeric:tabular-nums; }
  .tile .l { color:var(--muted); font-size:.72rem; text-transform:uppercase;
    letter-spacing:.06em; margin-top:.4rem; font-weight:600; }

  /* Demo-visitors: CSS bar chart (no external libs — CSP forbids them) + table grids. */
  .bars { display:flex; align-items:flex-end; gap:2px; height:120px; overflow-x:auto; padding-top:.3rem; }
  .bars .bar { flex:1 0 6px; min-width:6px; background:var(--accent-soft); border-radius:3px 3px 0 0;
    position:relative; align-self:flex-end; }
  .bars .bar > i { position:absolute; inset:auto 0 0; background:var(--accent); border-radius:3px 3px 0 0; }
  .bars .bar:hover { outline:1px solid var(--accent); }
  .grid2 { display:grid; grid-template-columns:repeat(auto-fit,minmax(240px,1fr)); gap:1rem; }
  .grid3 { display:grid; grid-template-columns:repeat(auto-fit,minmax(180px,1fr)); gap:1rem; }
  .brk { max-width:420px; }

  .banner { display:none; padding:.75rem .95rem; border-radius:10px; margin-bottom:1.1rem;
    border:1px solid var(--line); }
  .banner.show { display:block; }
  .banner.ok { border-color:color-mix(in srgb,var(--ok) 40%,transparent);
    background:color-mix(in srgb,var(--ok) 10%,transparent); }
  .banner.bad { border-color:color-mix(in srgb,var(--bad) 40%,transparent);
    background:color-mix(in srgb,var(--bad) 10%,transparent); }
  .banner.secret { border-color:color-mix(in srgb,var(--warn) 45%,transparent);
    background:color-mix(in srgb,var(--warn) 12%,transparent); }
  .copybtn { margin-left:.6rem; padding:.15rem .55rem; font-size:.78rem; cursor:pointer;
    border:1px solid var(--muted); border-radius:6px; background:transparent; color:inherit; }
  .dc-cred-list { display:flex; flex-direction:column; gap:.35rem; margin:.4rem 0 .2rem; }
  .dc-cred-list label { display:flex; align-items:center; gap:.5rem; font-size:.85rem; cursor:pointer; }
  .credrow { display:flex; align-items:center; gap:.5rem; }
  .credrow code { flex:1; word-break:break-all; margin-top:0; }
  .credrow .copybtn { margin-left:0; flex-shrink:0; }
  .wstep { display:inline-flex; align-items:center; justify-content:center; width:1.4rem; height:1.4rem;
    margin-right:.5rem; border-radius:50%; font-size:.82rem; font-weight:600;
    background:color-mix(in srgb,var(--warn) 20%,transparent); }
  .cmd { margin-top:.7rem; padding:.7rem .85rem; border-radius:8px; overflow-x:auto; white-space:pre;
    font-family:ui-monospace,monospace; font-size:.82rem; background:color-mix(in srgb,var(--muted) 12%,transparent);
    border:1px solid color-mix(in srgb,var(--muted) 30%,transparent); }
  .wsteps { display:flex; gap:.5rem; list-style:none; padding:0; margin:0 0 1.1rem; flex-wrap:wrap; }
  .wsteps li { display:flex; align-items:center; gap:.4rem; font-size:.85rem; color:var(--muted); }
  .wsteps li .wstep { background:color-mix(in srgb,var(--muted) 18%,transparent); }
  .wsteps li.active { color:inherit; font-weight:600; }
  .wsteps li.active .wstep { background:color-mix(in srgb,var(--warn) 30%,transparent); }
  .wsteps li.done .wstep { background:color-mix(in srgb,var(--ok) 30%,transparent); }
  .wsteps li + li::before { content:"›"; margin-right:.35rem; color:var(--muted); }
  .wnav { margin-top:1.1rem; }
  .ghost { background:transparent; border:1px solid var(--muted); color:inherit; }
  .ghost:disabled { opacity:.4; cursor:not-allowed; }
  .hint { color:var(--muted); font-size:.82rem; margin:.15rem 0 0; }
  .view { display:none; }
  .view.active { display:block; }
  /* Onboard runs as a centered modal over a backdrop, not a full-width view. */
  #view-onboard.active { position:fixed; inset:0; z-index:60; display:flex;
    align-items:center; justify-content:center; padding:1.5rem;
    background:color-mix(in srgb, #000 45%, transparent); }
  #view-onboard .obdialog { width:100%; max-width:560px; max-height:90vh; overflow-y:auto;
    padding:1.6rem 1.7rem; border-radius:14px; background:var(--bg);
    border:1px solid color-mix(in srgb,var(--muted) 30%,transparent);
    box-shadow:0 18px 50px color-mix(in srgb,#000 40%,transparent); position:relative; }
  #view-onboard .obclose { position:absolute; top:.7rem; right:.85rem; font-size:1.3rem;
    line-height:1; background:transparent; border:0; color:var(--muted); cursor:pointer; }

  /* Credits / footer — sits at the bottom of every view (main is a flex column). */
  .credits { margin-top:auto; padding-top:1.75rem; border-top:1px solid var(--line-soft);
    display:grid; grid-template-columns:1fr auto; gap:1rem 2rem; align-items:start;
    color:var(--muted); }
  .credits-brand { display:flex; align-items:center; gap:.7rem; }
  .credits-brand .mark { width:26px; height:26px; border-radius:8px; flex:none;
    background:center/contain no-repeat url("data:image/svg+xml,%3Csvg%20width%3D%22512%22%20height%3D%22512%22%20viewBox%3D%220%200%20512%20512%22%20fill%3D%22none%22%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20role%3D%22img%22%20aria-label%3D%22Relayent%22%3E%3Ctitle%3ERelayent%3C%2Ftitle%3E%3Cdefs%3E%3ClinearGradient%20id%3D%22rl-bg%22%20x1%3D%2296%22%20y1%3D%2272%22%20x2%3D%22416%22%20y2%3D%22440%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23818cf8%22%2F%3E%3Cstop%20offset%3D%220.55%22%20stop-color%3D%22%236366f1%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%234f46e5%22%2F%3E%3C%2FlinearGradient%3E%3ClinearGradient%20id%3D%22rl-path%22%20x1%3D%22150%22%20y1%3D%22330%22%20x2%3D%22362%22%20y2%3D%22182%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.55%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.95%22%2F%3E%3C%2FlinearGradient%3E%3C%2Fdefs%3E%3Crect%20x%3D%2248%22%20y%3D%2248%22%20width%3D%22416%22%20height%3D%22416%22%20rx%3D%22104%22%20fill%3D%22url%28%23rl-bg%29%22%2F%3E%3Crect%20x%3D%2248.5%22%20y%3D%2248.5%22%20width%3D%22415%22%20height%3D%22415%22%20rx%3D%22103.5%22%20fill%3D%22none%22%20stroke%3D%22%23ffffff%22%20stroke-opacity%3D%220.18%22%20stroke-width%3D%221%22%2F%3E%3Cpath%20d%3D%22M150%20330%20L256%20214%20L362%20182%22%20fill%3D%22none%22%20stroke%3D%22url%28%23rl-path%29%22%20stroke-width%3D%2226%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%2F%3E%3Ccircle%20cx%3D%22150%22%20cy%3D%22330%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22362%22%20cy%3D%22182%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2266%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.14%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2246%22%20fill%3D%22%23ffffff%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2221%22%20fill%3D%22%234f46e5%22%2F%3E%3C%2Fsvg%3E"); }
  .credits-brand b { display:block; font-size:.95rem; letter-spacing:-.01em; color:var(--fg); }
  .credits-brand span { font-size:.8rem; color:var(--muted); }
  .credits-links { display:flex; gap:1.1rem; align-items:center; }
  .credits-links a { color:var(--muted); text-decoration:none; font-size:.85rem; font-weight:500; }
  .credits-links a:hover { color:var(--fg); }
  .credits-legal { grid-column:1 / -1; display:flex; justify-content:space-between;
    gap:1rem; flex-wrap:wrap; font-size:.76rem; color:var(--faint);
    padding-top:1rem; border-top:1px solid var(--line-soft); }
  .credits-legal a { color:var(--muted); }
  @media (max-width:640px) { .credits { grid-template-columns:1fr; }
    .credits-legal { flex-direction:column; gap:.3rem; } }
  @media (prefers-reduced-motion:reduce) { *{animation:none !important; scroll-behavior:auto !important;
    transition:none !important;} }

  /* Guide accordion: each topic is a collapsible <details>. */
  details.help { padding:0; overflow:hidden; }
  details.help > summary { list-style:none; cursor:pointer; margin:0; padding:1.1rem 1.35rem;
    display:flex; align-items:center; gap:.6rem; font-size:.95rem; font-weight:600;
    letter-spacing:-.01em; color:var(--fg); }
  details.help > summary::-webkit-details-marker { display:none; }
  details.help > summary .caret { color:var(--faint); transition:transform .15s ease;
    font-size:.8rem; }
  details.help[open] > summary .caret { transform:rotate(90deg); }
  details.help > summary .note { margin-left:.15rem; font-weight:400; }
  details.help > summary:hover { color:var(--accent-fg); }
  details.help > .body { padding:0 1.35rem 1.15rem; }
  details.help + details.help { margin-top:.7rem; }

  .help-p { margin:.2rem 0 .6rem; max-width:70ch; }
  .help-p:last-child { margin-bottom:0; }
  .help-dl { display:grid; grid-template-columns:150px 1fr; gap:.5rem .9rem; margin:0; max-width:80ch; }
  .help-dl dt { color:var(--accent); font-weight:600; }
  .help-dl dd { margin:0; }
  .help-dl dt,.help-dl dd { padding-bottom:.5rem; border-bottom:1px solid var(--line); }
  .help-dl dt:last-of-type,.help-dl dd:last-child { border-bottom:0; padding-bottom:0; }
  @media (max-width:760px) { .help-dl { grid-template-columns:1fr; gap:.15rem; }
    .help-dl dt { border-bottom:0; padding-bottom:0; margin-top:.6rem; }
    .help-dl dd { border-bottom:1px solid var(--line); } }

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
      <button class="navgroup" data-group="overview" aria-expanded="true"><span class="gcaret">▾</span> Overview</button>
      <div class="navgroup-items" data-group-items="overview">
        <button class="navlink" data-view="status"><span class="ic">◈</span> Relay &amp; bridges</button>
      </div>
      <button class="navlink navlink-onboard" data-view="onboard"><span class="ic">✦</span> Onboard an app</button>
      <button class="navgroup" data-group="apps" aria-expanded="true"><span class="gcaret">▾</span> Apps</button>
      <div class="navgroup-items" data-group-items="apps">
        <button class="navlink" data-view="creds"><span class="ic">⚿</span> App credentials</button>
        <button class="navlink" data-view="decommission"><span class="ic">⌫</span> Decommission</button>
      </div>
      <button class="navgroup" data-group="users" aria-expanded="true"><span class="gcaret">▾</span> Users</button>
      <div class="navgroup-items" data-group-items="users">
        <button class="navlink" data-view="users"><span class="ic">◱</span> Users</button>
      </div>
      <button class="navgroup" data-group="operations" aria-expanded="false"><span class="gcaret">▾</span> Operations</button>
      <div class="navgroup-items" data-group-items="operations">
        <button class="navlink" data-view="audit"><span class="ic">≣</span> Audit</button>
        <button class="navlink" data-view="demostats"><span class="ic">◔</span> Demo visitors</button>
        <button class="navlink" data-view="backends"><span class="ic">◧</span> Backends</button>
        <button class="navlink" data-view="settings"><span class="ic">⚙</span> Settings</button>
      </div>
      <button class="navgroup" data-group="help" aria-expanded="false"><span class="gcaret">▾</span> Help</button>
      <div class="navgroup-items" data-group-items="help" hidden>
        <button class="navlink" id="guide-toggle" data-view="help" aria-expanded="false">
          <span class="ic tw" id="guide-caret">▸</span> Guide</button>
        <div class="subnav" id="guide-sub" hidden>
          <button class="subnavlink" data-topic="overview">Overview</button>
          <button class="subnavlink" data-topic="users">Users</button>
          <button class="subnavlink" data-topic="audit">Audit</button>
          <button class="subnavlink" data-topic="status">Relay &amp; bridges</button>
          <button class="subnavlink" data-topic="enroll">Enrol a bridge</button>
          <button class="subnavlink" data-topic="settings">Settings</button>
          <button class="subnavlink" data-topic="creds">App credentials</button>
          <button class="subnavlink" data-topic="signin">Sign-in &amp; landing</button>
        </div>
      </div>
    </nav>
    <div class="whoami">
      <span class="who" id="whoami" title=""><span class="avatar" id="avatar">R</span><span id="whoami-label">—</span></span>
      <a href="/v1/auth/logout" id="logout">Sign out</a>
    </div>
  </aside>

  <main>
    <div class="topbar">
      <span class="ver">v%VERSION%</span>
      <a class="gh" id="demolink" href="#" target="_blank" rel="noopener noreferrer" title="Open the public demo" style="display:none">
        <span aria-hidden="true">▷</span> View demo
      </a>
      <a class="gh" href="https://github.com/ToolTropolis/Relayent" target="_blank" rel="noopener noreferrer" title="Relayent on GitHub">
        <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
        GitHub
      </a>
    </div>
    <div id="banner" class="banner"></div>

    <!-- USERS -->
    <section id="view-users" class="view">
      <div class="head"><h1>Users</h1><p>People with a bridge on this relay. Roles and lifecycle are managed here.</p></div>
      <div class="card">
        <h2>Add a user</h2>
        <div class="row">
          <input id="nsub" class="grow" placeholder="user id (OIDC sub, or any id)">
          <input id="nemail" class="grow" placeholder="email">
          <select id="nrole">
            <option value="user" selected>user</option>
            <option value="viewer">viewer</option>
            <option value="operator">operator</option>
            <option value="admin">admin</option>
          </select>
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
          <thead><tr><th>When</th><th>Event</th><th>User</th><th>Backend</th><th>Status</th><th>Bytes</th><th>Host</th><th>Version</th></tr></thead>
          <tbody id="audit"><tr><td colspan="8" class="muted">Loading…</td></tr></tbody>
        </table></div>
      </div>
    </section>

    <!-- DEMO VISITORS -->
    <section id="view-demostats" class="view">
      <div class="head"><h1>Demo visitors</h1><p>Traffic to the public demo — aggregate counts and coarse buckets only. No IPs, no URLs, no per-visitor rows are ever stored.</p></div>
      <div class="card">
        <div class="row" style="justify-content:space-between;align-items:baseline">
          <h2>At a glance <span class="note muted" id="demo-range"></span></h2>
          <select id="demo-days">
            <option value="7">Last 7 days</option>
            <option value="30" selected>Last 30 days</option>
            <option value="90">Last 90 days</option>
            <option value="365">Last year</option>
          </select>
        </div>
        <div class="stat">
          <div class="tile"><div class="n" id="d-total">—</div><div class="l">Visits</div></div>
          <div class="tile"><div class="n" id="d-uniques">—</div><div class="l">Unique visitors</div></div>
          <div class="tile"><div class="n" id="d-today">—</div><div class="l">Today</div></div>
        </div>
      </div>
      <div class="card">
        <h2>Visits per day</h2>
        <div class="bars" id="d-series"><span class="muted">Loading…</span></div>
      </div>
      <div class="card">
        <h2>Where visitors are</h2>
        <div class="brk"><div class="tablewrap"><table>
          <thead><tr><th>Country</th><th>Visits</th></tr></thead>
          <tbody id="d-countries"><tr><td colspan="2" class="muted">Loading…</td></tr></tbody>
        </table></div></div>
        <p class="hint">Country comes from an offline GeoIP database on the relay (set <code>RELAYENT_GEOIP_DB</code>). Without it, country shows as <code>??</code> and everything else still works.</p>
      </div>
      <div class="card">
        <h2>How they got here</h2>
        <div class="grid2">
          <div class="tablewrap"><table>
            <thead><tr><th>Referrer</th><th>Visits</th></tr></thead>
            <tbody id="d-referrers"><tr><td colspan="2" class="muted">Loading…</td></tr></tbody>
          </table></div>
          <div class="tablewrap"><table>
            <thead><tr><th>Campaign source</th><th>Visits</th></tr></thead>
            <tbody id="d-sources"><tr><td colspan="2" class="muted">Loading…</td></tr></tbody>
          </table></div>
        </div>
        <p class="hint">Referrer is the referring site's host only (never the full URL). Campaign source is the <code>utm_source</code> parameter, when present.</p>
      </div>
      <div class="card">
        <h2>What they use</h2>
        <div class="grid3">
          <div class="tablewrap"><table>
            <thead><tr><th>Device</th><th>Visits</th></tr></thead>
            <tbody id="d-devices"><tr><td colspan="2" class="muted">Loading…</td></tr></tbody>
          </table></div>
          <div class="tablewrap"><table>
            <thead><tr><th>Browser</th><th>Visits</th></tr></thead>
            <tbody id="d-browsers"><tr><td colspan="2" class="muted">Loading…</td></tr></tbody>
          </table></div>
          <div class="tablewrap"><table>
            <thead><tr><th>OS</th><th>Visits</th></tr></thead>
            <tbody id="d-oses"><tr><td colspan="2" class="muted">Loading…</td></tr></tbody>
          </table></div>
        </div>
        <p class="hint">Device, browser and OS are coarse families parsed from the User-Agent — never a full fingerprint.</p>
      </div>
    </section>

    <!-- STATUS -->
    <section id="view-status" class="view">
      <div class="head"><h1>Relay &amp; bridges</h1><p>Live health of the relay and each user's bridge.</p></div>
      <div class="card">
        <h2>At a glance</h2>
        <div class="stat">
          <div class="tile"><div class="n" id="s-users">—</div><div class="l">Users</div></div>
          <div class="tile"><div class="n" id="s-online">—</div><div class="l">Bridges online</div></div>
          <div class="tile"><div class="n" id="s-pending">—</div><div class="l">Pending jobs</div></div>
        </div>
      </div>
      <div class="card">
        <h2>Bridges <span class="note muted">— one row per enrolled bridge</span></h2>
        <div class="tablewrap"><table>
          <thead><tr><th>User</th><th>Bridge ID</th><th>Presence</th><th>Host</th><th>Version</th><th>Enrolled</th><th>Last seen</th><th></th></tr></thead>
          <tbody id="presence"><tr><td colspan="8" class="muted">Loading…</td></tr></tbody>
        </table></div>
        <p class="hint">Presence, host, and version are tracked per user (whichever of that user's bridges last polled). Revoke retires a bridge — its credential stops working immediately.</p>
      </div>
    </section>

    <!-- BACKENDS -->
    <section id="view-backends" class="view">
      <div class="head"><h1>Backends</h1><p>Control which AI backends this relay exposes. A disabled backend is hidden from apps and refused at enqueue — use it to keep a public surface off paid subscriptions.</p></div>
      <div class="card">
        <h2>Backends <span class="note muted" id="backends-fresh"></span></h2>
        <div class="tablewrap"><table>
          <thead><tr><th>Backend</th><th>Policy</th><th>Readiness</th><th>What to do</th><th></th></tr></thead>
          <tbody id="backends"><tr><td colspan="5" class="muted">Loading…</td></tr></tbody>
        </table></div>
        <p class="hint"><b>Policy</b> is whether you <i>allow</i> this backend — a permission you set, independent of any machine. <b>Readiness</b> is whether a bridge can actually run it (the CLI installed and signed in). A backend serves jobs only when it is both <b>Allowed</b> and <b>Ready</b>; "Allowed" alone does not mean the CLI is installed. Readiness reflects the last bridge report.</p>
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

    <!-- ONBOARD AN APP (wizard over the existing app-cred / user / enrol-token APIs) -->
    <section id="view-onboard" class="view">
     <div class="obdialog" role="dialog" aria-modal="true" aria-label="Onboard an app">
      <button class="obclose" id="ob-x" aria-label="Close">×</button>
      <div class="head"><h1>Onboard an app</h1><p>Wire a new app (e.g. EngageHub) end to end: a credential to call the relay, a user whose subscription runs the jobs, and a bridge on that user's machine.</p></div>

      <ol class="wsteps" id="ob-stepper">
        <li data-step="0"><span class="wstep">1</span> App</li>
        <li data-step="1"><span class="wstep">2</span> User</li>
        <li data-step="2"><span class="wstep">3</span> Bridge</li>
        <li data-step="3"><span class="wstep">✓</span> Done</li>
      </ol>

      <div class="card wpanel" data-step="0">
        <h2>The app</h2>
        <p class="hint">A label for the calling app. It stays on the relay — the app only ever holds the credential.</p>
        <div class="row">
          <input id="ob-appid" class="grow" placeholder="app id (e.g. engagehub)">
          <button id="ob-mkcred">Mint credential</button>
        </div>
        <div id="ob-cred-box" hidden>
          <p class="hint">✓ Credential (shown once — copy it now):</p>
          <div class="credrow"><code id="ob-cred-val" class="cmd"></code><button type="button" class="copybtn" id="ob-cred-copy">Copy</button></div>
        </div>
      </div>

      <div class="card wpanel" data-step="1" hidden>
        <h2>The user</h2>
        <p class="hint">Whose CLI subscription runs the jobs. The sub must be <b>exactly</b> the identifier the app sends as <code>target_user</code> — email, username, id, verbatim.</p>
        <div class="row">
          <input id="ob-sub" class="grow" placeholder="user id / target_user (e.g. alice@example.com)">
          <button id="ob-mkuser">Create user</button>
        </div>
        <p class="hint" id="ob-user-done" hidden></p>
      </div>

      <div class="card wpanel" data-step="2" hidden>
        <h2>The bridge</h2>
        <p class="hint">Mint a one-time enrolment token, then run the bridge on the user's machine with it. It swaps the token for a bound credential on first start.</p>
        <div class="row">
          <button id="ob-mktoken">Mint enrolment token</button>
          <span id="ob-tokenhint" class="muted"></span>
        </div>
        <pre id="ob-bridgecmd" class="cmd" hidden></pre>
      </div>

      <div class="card wpanel" data-step="3" hidden>
        <h2>Give the app this config</h2>
        <pre id="ob-config" class="cmd"></pre>
        <p class="hint">The bridge (step 3) must be running on the user's machine before jobs will execute.</p>
      </div>

      <div class="row wnav">
        <button id="ob-back" class="ghost" disabled>← Back</button>
        <button id="ob-next" disabled>Next →</button>
        <button id="ob-restart" class="ghost" hidden>Onboard another</button>
        <button id="ob-done" hidden>Done</button>
      </div>
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

    <!-- DECOMMISSION -->
    <section id="view-decommission" class="view">
      <div class="head"><h1>Decommission</h1><p>Tear down a user cleanly: remove the user, every bridge bound to them, and any app credentials you select. This is permanent.</p></div>
      <div class="card">
        <h2>Choose a user</h2>
        <div class="row">
          <select id="dc-user" class="grow"><option value="">Loading users…</option></select>
          <button id="dc-load" class="ghost">Review</button>
        </div>
      </div>
      <div class="card" id="dc-review" hidden>
        <h2>Will be deleted</h2>
        <p class="hint"><b>User</b></p>
        <div class="cmd" id="dc-userline"></div>
        <p class="hint"><b>Bridges bound to this user</b> — their credentials stop working immediately.</p>
        <div class="cmd" id="dc-bridges"></div>
        <p class="hint"><b>App credentials</b> — not linked to a user, so tick the ones this app used. Unticked creds are left alone.</p>
        <div id="dc-creds" class="dc-cred-list"><span class="muted">Loading…</span></div>
        <div class="banner show bad" style="margin-top:1rem">
          <strong>Permanent.</strong> Type the user id <code id="dc-echo"></code> below to confirm.
        </div>
        <div class="row">
          <input id="dc-confirm" class="grow" placeholder="type the user id to confirm" autocomplete="off">
          <button id="dc-delete" disabled>Delete everything</button>
        </div>
      </div>
    </section>

    <!-- HELP -->
    <section id="view-help" class="view">
      <div class="head"><h1>Guide</h1><p>What each section does, and the ideas behind them. For full setup and the API, see the docs linked at the bottom.</p></div>

      <details class="help card" id="help-overview" open>
        <summary><span class="caret" aria-hidden="true">▸</span> The big picture</summary>
        <div class="body">
        <p class="help-p">Relayent routes an app's AI request to a <b>CLI subscription running on a user's own
        machine</b> (Claude Code, Codex, Cursor) instead of a paid API key. This relay is <b>multi-tenant</b>:
        many users, each running their own <b>bridge</b>, each on their own subscription, isolated from one
        another. A job addressed to a user runs only on that user's bridge — never anyone else's.</p>
        <p class="help-p"><b>You are the operator.</b> You manage users, enrol their bridges, and issue app
        credentials. You can see <i>activity</i> — who ran what, when, on which backend — but <b>never the
        prompt or the result</b>. That boundary is built into the relay, not a setting.</p>
      </div>
      </details>

      <details class="help card" id="help-users">
        <summary><span class="caret" aria-hidden="true">▸</span> Users <span class="note muted">— Admin</span></summary>
        <div class="body">
        <dl class="help-dl">
          <dt>What it is</dt><dd>Everyone with an identity on this relay. A user usually appears automatically the first time they sign in; you can also pre-provision one with <b>Add a user</b>.</dd>
          <dt>Roles</dt><dd><b>admin</b> can manage everything, including other users; <b>operator</b> manages app credentials, bridges, and backends, but not users; <b>viewer</b> sees everything read-only; <b>user</b> has no console access at all — just their own jobs and status page. The <b>first person ever to sign in becomes the admin</b>; everyone after is a user until you change their role.</dd>
          <dt>Enrol</dt><dd>Mints a one-time token for that user to pair their bridge — see “Enrol a bridge”.</dd>
          <dt>Disable / Delete</dt><dd><b>Disable</b> blocks a user's jobs immediately but keeps the record; <b>Delete</b> removes it. You can't disable, demote, or delete <b>yourself</b> — a safeguard so the last admin can't be locked out.</dd>
        </dl>
      </div>
      </details>

      <details class="help card" id="help-audit">
        <summary><span class="caret" aria-hidden="true">▸</span> Audit <span class="note muted">— Admin</span></summary>
        <div class="body">
        <p class="help-p">A running history: who did what, when, on which backend, success or failure, and the
        <b>byte counts</b> of the prompt and result. It deliberately holds <b>no content</b> — you see that a job
        ran and how big it was, never what it said. This is the record to check for “is it being used?” and
        “did this user's jobs fail?”.</p>
      </div>
      </details>

      <details class="help card" id="help-status">
        <summary><span class="caret" aria-hidden="true">▸</span> Relay &amp; bridges <span class="note muted">— Configure</span></summary>
        <div class="body">
        <dl class="help-dl">
          <dt>At a glance</dt><dd>Totals across the relay: how many users, how many bridges are online right now, and how many jobs are pending.</dd>
          <dt>Bridge presence</dt><dd>Per user: is their bridge currently connected, how many bridges they've enrolled, and their pending jobs. <b>Online</b> means the bridge polled recently; <b>offline</b> usually means that user's machine is asleep or the bridge isn't running.</dd>
        </dl>
      </div>
      </details>

      <details class="help card" id="help-enroll">
        <summary><span class="caret" aria-hidden="true">▸</span> Enrol a bridge <span class="note muted">— Configure</span></summary>
        <div class="body">
        <p class="help-p">A bridge proves who it is with a credential it earns through <b>enrolment</b>. Pick the
        user, click <b>Mint token</b>, and send them the one-time token out-of-band (chat, email). They run
        <code>relayent-bridge setup</code> and paste it; their bridge redeems it once and is then bound to them.
        The token is shown <b>once</b> and expires — mint a fresh one if it lapses.</p>
      </div>
      </details>

      <details class="help card" id="help-settings">
        <summary><span class="caret" aria-hidden="true">▸</span> Settings <span class="note muted">— Configure</span></summary>
        <div class="body">
        <p class="help-p">A <b>read-only</b> view of how this relay is actually running: version, whether it's
        behind a trusted proxy, whether the multi-tenant store is on, and the OIDC identity settings (issuer,
        client id, redirect). <b>No secret values are ever shown</b> — only whether a pairing key or admin token
        is set. To change any of it, edit the relay's <code>.env</code> and run <code>docker compose up -d</code>;
        editing config from this screen is intentionally not offered.</p>
      </div>
      </details>

      <details class="help card" id="help-creds">
        <summary><span class="caret" aria-hidden="true">▸</span> App credentials <span class="note muted">— Integration</span></summary>
        <div class="body">
        <p class="help-p">A key an <b>app</b> (e.g. EngageHub) uses to enqueue jobs on users' behalf. Issue one
        per app; the secret (<code>&lt;id&gt;.&lt;secret&gt;</code>) is shown <b>once</b> — copy it then. The app sends it as
        a bearer token and names the target user on each job, so a request for Alice runs on Alice's subscription.
        <b>Revoke</b> kills a credential instantly. The relay stores only a hash, never the secret.</p>
      </div>
      </details>

      <details class="help card" id="help-signin">
        <summary><span class="caret" aria-hidden="true">▸</span> Signing in &amp; where you land</summary>
        <div class="body"><p class="help-p">Everyone signs in at <code>/login</code> — “Sign in with your provider”, or the bootstrap
        admin token. Afterwards you're sent to the right place automatically: <b>admins</b> to this console,
        <b>regular users</b> to their own status page at <code>/</code>. Sign out from the bottom of the sidebar.</p></div>
      </details>

      <details class="help card" id="help-learn">
        <summary><span class="caret" aria-hidden="true">▸</span> Learn more</summary>
        <div class="body"><p class="help-p">Full operator setup, migration, and the API live in the project docs:
        <b>INSTALL.md</b> (standing up the relay and onboarding users), <b>SECURITY.md</b> (the multi-tenant
        threat model and what Relayent does <i>not</i> protect against), <b>API.md</b> and <b>openapi.yaml</b>
        (the integration contract). This console never shows prompt or result content — by design.</p></div>
      </details>
    </section>

    <footer class="credits">
      <div class="credits-brand">
        <span class="mark" aria-hidden="true"></span>
        <div>
          <b>Relayent</b>
          <span>Use the AI subscription you already pay for — from anywhere.</span>
        </div>
      </div>
      <nav class="credits-links" aria-label="Project links">
        <a href="https://github.com/ToolTropolis/Relayent" target="_blank" rel="noopener noreferrer">GitHub</a>
        <a href="https://github.com/ToolTropolis/Relayent/blob/main/API.md" target="_blank" rel="noopener noreferrer">API</a>
        <a href="https://github.com/ToolTropolis/Relayent/blob/main/INSTALL.md" target="_blank" rel="noopener noreferrer">Install</a>
        <a href="https://github.com/ToolTropolis/Relayent/blob/main/SECURITY.md" target="_blank" rel="noopener noreferrer">Security</a>
      </nav>
      <div class="credits-legal">
        <span>Relayent v%VERSION% · MIT License</span>
        <span>Open source on <a href="https://github.com/ToolTropolis/Relayent" target="_blank" rel="noopener noreferrer">github.com/ToolTropolis/Relayent</a></span>
      </div>
    </footer>
  </main>
</div>

<script nonce="%NONCE%">
const $ = id => document.getElementById(id);
// Bootstrap admin token. Persisted in sessionStorage so a refresh keeps the
// session; sessionStorage (not localStorage) means it dies when the tab closes,
// which suits a full-admin bootstrap token. Cleared on sign-out.
let token = "";
try { token = sessionStorage.getItem("relayent.admintok") || ""; } catch (e) {}

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

// wireCopy makes btn copy the given value to the clipboard, showing "Copied" briefly.
// navigator.clipboard needs a secure context (https/localhost); on a plain http
// LAN IP it's undefined, so fall back to selecting selectEl's text for a manual
// copy rather than throwing.
function wireCopy(btn, value, selectEl) {
  btn.onclick = async () => {
    let ok = false;
    try { await navigator.clipboard.writeText(value); ok = true; } catch (e) {}
    if (ok) { btn.textContent = "Copied"; setTimeout(() => { btn.textContent = "Copy"; }, 1500); }
    else if (selectEl) {
      const rng = document.createRange(); rng.selectNodeContents(selectEl);
      const sel = window.getSelection(); sel.removeAllRanges(); sel.addRange(rng);
      btn.textContent = "Press ⌘/Ctrl+C";
    }
  };
}
function showSecret(label, value) {
  const b = $("banner");
  b.className = "banner show secret";
  b.replaceChildren();
  const strong = document.createElement("strong");
  strong.textContent = label + " (shown once — copy it now): ";
  const codeEl = document.createElement("code");
  codeEl.textContent = value;   // textContent — never innerHTML
  const copyBtn = document.createElement("button");
  copyBtn.type = "button";
  copyBtn.className = "copybtn";
  copyBtn.textContent = "Copy";
  wireCopy(copyBtn, value, codeEl);
  b.appendChild(strong); b.appendChild(codeEl); b.appendChild(copyBtn);
}

async function api(method, path, body) {
  const opt = {method, headers: headers(), credentials: "same-origin"};
  if (body) opt.body = JSON.stringify(body);
  const r = await fetch(path, opt);
  if (r.status === 401 || r.status === 403) {
    // Not (or no longer) an admin here — drop any stale bootstrap token so the
    // refresh loop can't re-adopt it, then send to /login (the sign-in surface).
    try { sessionStorage.removeItem("relayent.admintok"); } catch (e) {}
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
const VIEWS = ["users","audit","demostats","status","backends","settings","onboard","creds","decommission","help"];
function go(view) {
  if (!VIEWS.includes(view)) view = "users";
  for (const v of VIEWS) $("view-" + v).classList.toggle("active", v === view);
  for (const b of document.querySelectorAll(".navlink"))
    b.classList.toggle("active", b.dataset.view === view);
  // Don't clobber a deep hash like #help/users when routing to its base view.
  const cur = location.hash.slice(1);
  if (cur !== view && !cur.startsWith(view + "/")) location.hash = view;
  loadView(view);
}
async function loadView(view) {
  try {
    if (view === "users")    await loadUsers();
    if (view === "audit")    await loadAudit();
    if (view === "demostats") await loadDemoStats();
    if (view === "status")   await loadStatus();
    if (view === "backends") await loadBackends();
    if (view === "settings") await loadConfig();
    if (view === "creds")    await loadApps();
    if (view === "decommission") await loadDecommission();
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
    const roleSel = document.createElement("select");
    for (const r of ["user", "viewer", "operator", "admin"]) {
      const o = document.createElement("option"); o.value = r; o.textContent = r;
      if (r === u.role) o.selected = true;
      roleSel.appendChild(o);
    }
    roleSel.onchange = () => setRole(u.sub, roleSel.value);
    roleTd.appendChild(roleSel); tr.appendChild(roleTd);

    const bt = document.createElement("td"); bt.appendChild(pill(u.bridge_online, "online", "offline")); tr.appendChild(bt);
    tr.appendChild(cell(String(u.pending_jobs)));
    tr.appendChild(cell(String(u.bridges)));

    const act = document.createElement("td");
    const wrap = document.createElement("div"); wrap.className = "actions";
    wrap.appendChild(btn("Enrol", "ghost sm", () => issueToken(u.sub)));
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
  if (!events.length) { emptyRow(tb, 8, "No activity yet."); return; }
  for (const e of events) {
    const tr = document.createElement("tr");
    tr.appendChild(cell(new Date(e.ts).toLocaleString()));
    tr.appendChild(cell(e.event));
    tr.appendChild(cell(e.target_sub || "—"));
    tr.appendChild(cell(e.backend || "—"));
    tr.appendChild(cell(e.status || "—"));
    tr.appendChild(cell(String((e.prompt_len||0) + (e.result_len||0))));
    tr.appendChild(cell(e.host || "—"));
    tr.appendChild(cell(e.version || "—"));
    tb.appendChild(tr);
  }
}

/* ---- STATUS ---- */
/* ---- DEMO VISITORS ---- */
let demoDaysBound = false;
function fillBreak(id, buckets, emptyText) {
  const tb = $(id); tb.replaceChildren();
  buckets = buckets || [];
  if (!buckets.length) { emptyRow(tb, 2, emptyText); return; }
  for (const b of buckets) {
    const tr = document.createElement("tr");
    tr.appendChild(cell(b.label));
    const c = cell(String(b.count)); c.style.fontVariantNumeric = "tabular-nums"; tr.appendChild(c);
    tb.appendChild(tr);
  }
}
async function loadDemoStats() {
  if (!demoDaysBound) {  // bind the range selector once, on first open
    $("demo-days").addEventListener("change", () => { loadDemoStats(); });
    demoDaysBound = true;
  }
  const days = $("demo-days").value || "30";
  const data = await api("GET", "/v1/admin/demo-stats?days=" + encodeURIComponent(days)) || {};
  $("d-total").textContent = data.total_hits || 0;
  $("d-uniques").textContent = data.uniques || 0;
  $("d-today").textContent = data.today || 0;
  $("demo-range").textContent = data.oldest_ts
    ? "— since " + new Date(data.oldest_ts).toLocaleDateString() : "— no visits yet";

  // Daily bar chart, scaled to the busiest day. Height as a % keeps it lib-free.
  const series = data.series || [];
  const max = series.reduce((m, d) => Math.max(m, d.visits), 0) || 1;
  const bars = $("d-series"); bars.replaceChildren();
  if (!series.length) { bars.innerHTML = '<span class="muted">No visits yet.</span>'; }
  for (const d of series) {
    const bar = document.createElement("div"); bar.className = "bar";
    bar.title = d.date + ": " + d.visits + (d.visits === 1 ? " visit" : " visits");
    const fill = document.createElement("i");
    fill.style.height = Math.round((d.visits / max) * 100) + "%";
    bar.appendChild(fill); bars.appendChild(bar);
  }

  fillBreak("d-countries", data.countries, "No visits yet.");
  fillBreak("d-referrers", data.referrers, "No referred visits — all direct.");
  fillBreak("d-sources", data.sources, "No campaign traffic.");
  fillBreak("d-devices", data.devices, "No visits yet.");
  fillBreak("d-browsers", data.browsers, "No visits yet.");
  fillBreak("d-oses", data.oses, "No visits yet.");
}

// ponytail: must match relay/main.go's onlineWindow const; no API field exposes it.
const BRIDGE_ONLINE_WINDOW_MS = 40 * 1000;
// Guards against overlapping calls (e.g. a double-click on the nav button):
// each call captures its own generation and bails before mutating the DOM if
// a newer call has started since, so only the latest render ever lands.
let loadStatusGen = 0;
async function loadStatus() {
  const gen = ++loadStatusGen;
  const data = await api("GET", "/v1/admin/users");
  if (gen !== loadStatusGen) return;
  const users = (data && data.users) || [];
  usersCache = users;
  const online = users.filter(u => u.bridge_online).length;
  const pending = users.reduce((n, u) => n + (u.pending_jobs||0), 0);
  $("s-users").textContent = String(users.length);
  $("s-online").textContent = String(online);
  $("s-pending").textContent = String(pending);
  const tb = $("presence"); tb.replaceChildren();
  if (!users.length) { emptyRow(tb, 8, "No users yet."); return; }
  let anyRow = false;
  for (const u of users) {
    // Fetch this user's enrolled bridges (one row each). Host/version/presence are
    // per-user (the queue aggregates polls), shown on each of the user's rows.
    let binds = [];
    try { const d = await api("GET", "/v1/admin/users/" + encodeURIComponent(u.sub) + "/bridges"); binds = (d && d.bridges) || []; }
    catch (e) { /* skip on error */ }
    if (gen !== loadStatusGen) return;
    if (!binds.length) {
      // A user with no enrolled bridge still shows once, so they're visible.
      const tr = document.createElement("tr");
      tr.appendChild(cell(u.email || u.sub));
      const idc = cell("—"); idc.className = "muted"; tr.appendChild(idc);
      const pt = document.createElement("td"); pt.appendChild(pill(false, "online", "no bridge")); tr.appendChild(pt);
      tr.appendChild(cell("—")); tr.appendChild(cell("—")); tr.appendChild(cell("—")); tr.appendChild(cell("—")); tr.appendChild(cell(""));
      tb.appendChild(tr); anyRow = true; continue;
    }
    for (const b of binds) {
      anyRow = true;
      const tr = document.createElement("tr");
      tr.appendChild(cell(u.email || u.sub));
      const idc = document.createElement("td"); const code = document.createElement("code"); code.textContent = b.bridge_id; idc.appendChild(code); tr.appendChild(idc);
      const thisOnline = !!b.last_seen && (Date.now() - new Date(b.last_seen).getTime()) <= BRIDGE_ONLINE_WINDOW_MS;
      const pt = document.createElement("td"); pt.appendChild(pill(thisOnline, "online", "offline")); tr.appendChild(pt);
      tr.appendChild(cell((thisOnline && u.bridge_host) || "—"));
      tr.appendChild(cell((thisOnline && u.bridge_version) || "—"));
      tr.appendChild(cell(b.enrolled_at ? new Date(b.enrolled_at).toLocaleDateString() : "—"));
      tr.appendChild(cell(b.last_seen ? new Date(b.last_seen).toLocaleString() : "—"));
      const act = document.createElement("td"); const wrap = document.createElement("div"); wrap.className = "actions";
      wrap.appendChild(btn("Revoke", "danger sm", () => revokeBridge(b.bridge_id, u.email || u.sub)));
      act.appendChild(wrap); tr.appendChild(act);
      tb.appendChild(tr);
    }
  }
  if (!anyRow) emptyRow(tb, 8, "No bridges enrolled yet.");
}
async function revokeBridge(id, who) {
  if (!confirm("Revoke bridge " + id + " for " + who + "? Its credential stops working immediately.")) return;
  try { await api("DELETE", "/v1/admin/bridges/" + encodeURIComponent(id));
    banner("Revoked bridge " + id, "ok"); loadStatus(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
}

/* ---- SETTINGS ---- */
async function loadConfig() {
  const c = await api("GET", "/v1/admin/config");
  kv($("cfg-relay"), [
    ["Version", c.version],
    ["Host", c.hostname || "—"],
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

/* ---- BACKENDS ---- */
// Per-backend CLI install info. Commands are a starting point; the official page
// is the source of truth for the current method and login flow.
const CLI_INFO = {
  claude: { cli:"Claude Code", install:"npm i -g @anthropic-ai/claude-code", login:"claude", url:"https://claude.com/claude-code" },
  codex:  { cli:"Codex CLI",  install:"npm i -g @openai/codex",           login:"codex login",         url:"https://developers.openai.com/codex" },
  cursor: { cli:"Cursor CLI", install:"curl https://cursor.com/install -fsS | bash", login:"cursor-agent login", url:"https://cursor.com/cli" },
  gemini: { cli:"Gemini CLI", install:"npm i -g @google/gemini-cli", login:"gemini", url:"https://github.com/google-gemini/gemini-cli" },
};
// Map a backend's reported state to a readiness label + guidance (kind: how to
// render). Policy (allowed) and readiness are separate: a backend serves jobs
// only when BOTH hold.
function backendReadiness(b) {
  // Name the actual machine when a bridge has reported; otherwise say where to look.
  const hosts = b.hosts || [];
  const where = hosts.length === 1 ? "on " + hosts[0]
              : hosts.length > 1  ? "on the bridge (" + hosts.join(", ") + ")"
              : "on the bridge machine";
  if (!b.supported)
    return { ok:false, label:"Not supported", kind:"text", hint:"This backend is a stub — no working adapter yet. It can't run jobs." };
  if (b.ready_bridges > 0)
    return { ok:true, label:"Ready", kind:"text", hint:"Installed " + where + " on "+b.ready_bridges+" bridge(s). If jobs fail, the CLI is likely logged out — run it there and sign in." };
  if (b.installed_bridges > 0)
    return { ok:false, label:"Installed, not ready", kind:"login", hint:"The CLI is present but not signed in. " + cap1(where) + ", run:" };
  if (!b.reporting_bridges)
    return { ok:false, label:"No bridge reporting", kind:"install", hint:"No bridge has reported yet — see Relay & bridges. On the bridge machine, install and sign in:" };
  return { ok:false, label:"Not installed", kind:"install", hint:"Not installed " + where + ". Install and sign in — it appears on the next poll:" };
}
function cap1(s) { return s.charAt(0).toUpperCase() + s.slice(1); }
// copyCmd renders a command as a click-to-copy chip: the <code> plus a copy
// button. Clicking either copies the exact command to the clipboard.
function copyCmd(command) {
  const wrap = document.createElement("div"); wrap.className = "cmd";
  const code = document.createElement("code"); code.textContent = command;
  const btn = document.createElement("button"); btn.className = "copybtn"; btn.type = "button";
  btn.title = "Copy"; btn.textContent = "Copy";
  const doCopy = () => {
    navigator.clipboard.writeText(command).then(() => {
      const was = btn.textContent; btn.textContent = "Copied"; btn.classList.add("ok");
      setTimeout(() => { btn.textContent = was; btn.classList.remove("ok"); }, 1200);
    }).catch(() => {});
  };
  code.style.cursor = "pointer"; code.onclick = doCopy; btn.onclick = doCopy;
  wrap.appendChild(code); wrap.appendChild(btn);
  return wrap;
}
// buildGuidance renders the What-to-do cell: text + (for install/login) the exact
// commands as copyable chips + a link to the official page.
function buildGuidance(name, r) {
  const td = document.createElement("td");
  td.className = "muted"; td.style.maxWidth = "44ch"; td.style.whiteSpace = "normal";
  td.appendChild(document.createTextNode(r.hint));
  const info = CLI_INFO[name];
  if (info && (r.kind === "install" || r.kind === "login")) {
    if (r.kind === "install" && info.install) td.appendChild(copyCmd(info.install));
    if (info.login) td.appendChild(copyCmd(info.login));
    const a = document.createElement("a"); a.href = info.url; a.target = "_blank"; a.rel = "noopener noreferrer";
    a.textContent = "Official " + info.cli + " docs ↗"; a.style.display = "inline-block"; a.style.marginTop = ".3rem";
    td.appendChild(a);
  }
  return td;
}
// timeAgo renders a short "Ns ago" / "Nm ago" for a timestamp.
function timeAgo(iso) {
  if (!iso) return "";
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return s + "s ago";
  if (s < 3600) return Math.round(s/60) + "m ago";
  return Math.round(s/3600) + "h ago";
}
async function loadBackends() {
  const data = await api("GET", "/v1/admin/backends");
  const tb = $("backends"); tb.replaceChildren();
  const backends = (data && data.backends) || [];
  // Freshness: readiness reflects the last bridge report, so show how old it is.
  $("backends-fresh").textContent = data && data.reported_at
    ? "— readiness reported " + timeAgo(data.reported_at)
    : "— no bridge has reported yet";
  if (!backends.length) { emptyRow(tb, 5, "No backends."); return; }
  for (const b of backends) {
    const tr = document.createElement("tr");
    tr.appendChild(cell(b.name));
    // A stub (unsupported) backend can never run, so its policy is meaningless —
    // show it as unavailable with no allow/block control.
    if (!b.supported) {
      const pol = cell("—"); pol.className = "muted"; tr.appendChild(pol);
      const rd = document.createElement("td"); rd.appendChild(pill(false, "unavailable", "unavailable")); tr.appendChild(rd);
      tr.appendChild(buildGuidance(b.name, {kind:"text", hint:"Not implemented yet — no adapter. Nothing to allow."}));
      tr.appendChild(cell(""));
      tb.appendChild(tr); continue;
    }
    // Policy — whether the admin ALLOWS this backend. Separate from readiness:
    // "Allowed" does not mean the CLI is installed.
    const pol = document.createElement("td"); pol.appendChild(pill(b.enabled, "allowed", "blocked")); tr.appendChild(pol);
    // Readiness — whether a bridge can actually run it.
    const r = backendReadiness(b);
    const rd = document.createElement("td"); rd.appendChild(pill(r.ok, r.label, r.label)); tr.appendChild(rd);
    // Guidance — with real install/login commands + official link when not ready.
    tr.appendChild(buildGuidance(b.name, r));
    // Toggle — Allow / Block.
    const act = document.createElement("td"); const wrap = document.createElement("div"); wrap.className = "actions";
    wrap.appendChild(btn(b.enabled ? "Block" : "Allow", "ghost sm", () => setBackend(b.name, !b.enabled)));
    act.appendChild(wrap); tr.appendChild(act); tb.appendChild(tr);
  }
}
async function setBackend(name, enabled) {
  try { await api("POST", "/v1/admin/backends/" + encodeURIComponent(name), {enabled});
    banner((enabled ? "Allowed " : "Blocked ") + name, "ok"); loadBackends(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
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
    // Active -> Revoke (disable, keep record). Revoked -> Delete (remove the row).
    // No un-revoke by design: issue a new credential to restore access.
    if (!c.revoked) wrap.appendChild(btn("Revoke", "ghost sm", () => revokeApp(c.id)));
    else wrap.appendChild(btn("Delete", "danger sm", () => deleteApp(c.id, c.app_id)));
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
    banner("Set " + sub + " to " + role, "ok"); loadUsers(); }
  catch (e) { banner("Error: " + e.message, "bad"); loadUsers(); }
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
async function deleteApp(id, appId) {
  if (!confirm("Delete the revoked credential for " + appId + "? This removes the record permanently.")) return;
  try { await api("DELETE", "/v1/admin/app-creds/" + encodeURIComponent(id));
    banner("Deleted credential", "ok"); loadApps(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
}

$("adduser").onclick = async () => {
  const sub = $("nsub").value.trim(), email = $("nemail").value.trim(), role = $("nrole").value;
  if (!sub || !email) { banner("user id and email are required", "bad"); return; }
  try { await api("POST", "/v1/admin/users", {sub, email, role});
    $("nsub").value = ""; $("nemail").value = ""; $("nrole").value = "user"; banner("User added", "ok"); loadUsers(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
};
$("addapp").onclick = async () => {
  const app_id = $("appid").value.trim();
  if (!app_id) { banner("app id is required", "bad"); return; }
  try { const r = await api("POST", "/v1/admin/app-creds", {app_id});
    $("appid").value = ""; showSecret("App credential for " + app_id, r.credential); loadApps(); }
  catch (e) { banner("Error: " + e.message, "bad"); }
};

/* ---- Onboard-an-app wizard: a one-step-at-a-time flow over the existing
   app-cred / user / enrol-token APIs. ob.step is the visible panel; ob.done[i]
   gates whether Next is allowed to leave step i. ---- */
const ob = { step: 0, cred: "", credId: "", sub: "", appId: "", done: [false, false, false, true] };
const OB_LAST = 3;

function obShow() {
  for (const p of document.querySelectorAll("#view-onboard .wpanel"))
    p.hidden = Number(p.dataset.step) !== ob.step;
  for (const li of document.querySelectorAll("#ob-stepper li")) {
    const s = Number(li.dataset.step);
    li.classList.toggle("active", s === ob.step);
    li.classList.toggle("done", s < ob.step && ob.done[s]);
  }
  $("ob-back").disabled = ob.step === 0;
  $("ob-next").hidden = ob.step === OB_LAST;
  $("ob-next").disabled = !ob.done[ob.step];
  $("ob-restart").hidden = ob.step !== OB_LAST;
  $("ob-done").hidden = ob.step !== OB_LAST;
}
function obReset() {
  ob.step = 0; ob.cred = ""; ob.credId = ""; ob.sub = ""; ob.appId = ""; ob.done = [false, false, false, true];
  $("ob-appid").value = ""; $("ob-sub").value = "";
  $("ob-cred-box").hidden = true; $("ob-user-done").hidden = true;
  $("ob-bridgecmd").hidden = true; $("ob-tokenhint").textContent = "";
  obShow();
}
// Roll back a half-finished onboard: delete whatever was created (credential,
// user) so an abandoned wizard leaves nothing behind — all-or-none. The enrol
// token (step 3) needs no cleanup: it is one-time and simply expires unused.
async function obRollback() {
  const undo = [];
  if (ob.sub)    undo.push(api("DELETE", "/v1/admin/users/" + encodeURIComponent(ob.sub)).catch(() => {}));
  if (ob.credId) undo.push(api("DELETE", "/v1/admin/app-creds/" + encodeURIComponent(ob.credId)).catch(() => {}));
  await Promise.all(undo);
  loadApps(); loadUsers();
}
function obRenderSummary() {
  $("ob-config").textContent =
    "relay_url   = " + location.origin + "\n" +
    "credential  = " + ob.cred + "\n" +
    "target_user = " + ob.sub +
    "\n\n# From a Docker container, use host.docker.internal instead of 127.0.0.1/localhost in relay_url.";
}

// Close the modal (× or backdrop click). If the onboard was started but not
// finished (something created, Done not reached), offer to roll it back so no
// partial app is left behind.
async function obClose() {
  const partial = ob.step < OB_LAST && (ob.credId || ob.sub);
  if (partial) {
    if (confirm("This onboard isn't finished. Delete what was created (credential" + (ob.sub ? " and user" : "") + ") so nothing partial is left?")) {
      await obRollback();
      banner("Rolled back the unfinished onboard", "ok");
    }
  }
  obReset();
  go("status");
}
$("ob-x").onclick = obClose;
$("view-onboard").addEventListener("click", (e) => { if (e.target.id === "view-onboard") obClose(); });
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && $("view-onboard").classList.contains("active")) obClose();
});

$("ob-next").onclick = () => { if (ob.done[ob.step] && ob.step < OB_LAST) { ob.step++; if (ob.step === OB_LAST) obRenderSummary(); obShow(); } };
$("ob-back").onclick = () => { if (ob.step > 0) { ob.step--; obShow(); } };
$("ob-restart").onclick = obReset;
$("ob-done").onclick = () => { obReset(); go("status"); };

$("ob-mkcred").onclick = async () => {
  const app_id = $("ob-appid").value.trim();
  if (!app_id) { banner("app id is required", "bad"); return; }
  try {
    const r = await api("POST", "/v1/admin/app-creds", {app_id});
    // Credential is "<id>.<secret>"; keep the id so an aborted onboard can roll
    // the credential back.
    ob.cred = r.credential; ob.credId = r.credential.split(".")[0]; ob.appId = app_id; ob.done[0] = true;
    // Show the secret inside the dialog (the page banner sits behind the modal
    // backdrop, so it would be hidden). textContent — never innerHTML.
    const val = $("ob-cred-val");
    val.textContent = r.credential;
    wireCopy($("ob-cred-copy"), r.credential, val);
    $("ob-cred-box").hidden = false;
    obShow(); loadApps();
  } catch (e) { banner("Error: " + e.message, "bad"); }
};
$("ob-mkuser").onclick = async () => {
  const sub = $("ob-sub").value.trim();
  if (!sub) { banner("user id is required", "bad"); return; }
  try {
    await api("POST", "/v1/admin/users", {sub, email: sub});
    ob.sub = sub; ob.done[1] = true;
    $("ob-user-done").textContent = "✓ User " + sub + " created.";
    $("ob-user-done").hidden = false;
    $("ob-tokenhint").textContent = "For user " + sub + ".";
    obShow(); loadUsers();
  } catch (e) { banner("Error: " + e.message, "bad"); }
};
$("ob-mktoken").onclick = async () => {
  if (!ob.sub) { banner("create the user first", "bad"); return; }
  try {
    const r = await api("POST", "/v1/admin/enroll-tokens", {user_sub: ob.sub});
    // On the user's machine: install the bridge, then run it with the enrolment
    // token as the key. The token is one-time; the bridge swaps it for a bound
    // credential on first start, so no secret is stored after that.
    $("ob-bridgecmd").textContent =
      "# On " + ob.sub + "'s machine (token is one-time, expires ~15 min):\n\n" +
      "# 1. install the bridge\n" +
      "curl -fsSL https://raw.githubusercontent.com/navjyotnishant/relayent/main/install.sh \\\n" +
      "  | RELAYENT_RELAY_URL=" + location.origin + " RELAYENT_PAIRING_KEY=" + r.token + " sh\n\n" +
      "# 2. start it\n" +
      "RELAYENT_RELAY_URL=" + location.origin + " \\\n" +
      "RELAYENT_PAIRING_KEY=" + r.token + " \\\n" +
      "  relayent-bridge";
    $("ob-bridgecmd").hidden = false;
    ob.done[2] = true; obShow();
    banner("Enrolment token minted", "ok");
  } catch (e) { banner("Error: " + e.message, "bad"); }
};
obShow();

/* ---- Decommission: delete a user + their bridge bindings + selected app-creds.
   All over existing DELETE endpoints. Type-to-confirm gates the delete. ---- */
const dc = { sub: "", bridges: [] };
async function loadDecommission() {
  $("dc-review").hidden = true;
  const data = await api("GET", "/v1/admin/users");
  const users = (data && data.users) || [];
  const sel = $("dc-user"); sel.replaceChildren();
  if (!users.length) { const o=document.createElement("option"); o.value=""; o.textContent="No users"; sel.appendChild(o); return; }
  const o0=document.createElement("option"); o0.value=""; o0.textContent="Select a user…"; sel.appendChild(o0);
  for (const u of users) { const o=document.createElement("option"); o.value=u.sub; o.textContent=(u.email||u.sub); sel.appendChild(o); }
}
$("dc-load").onclick = async () => {
  const sub = $("dc-user").value;
  if (!sub) { banner("pick a user", "bad"); return; }
  dc.sub = sub;
  // Bridges bound to this user.
  const bd = await api("GET", "/v1/admin/users/" + encodeURIComponent(sub) + "/bridges");
  dc.bridges = (bd && bd.bridges) || [];
  $("dc-userline").textContent = sub;
  $("dc-bridges").textContent = dc.bridges.length
    ? dc.bridges.map(b => b.bridge_id + (b.last_seen ? "  (last seen " + b.last_seen + ")" : "")).join("\n")
    : "(none)";
  // App-creds — unlinked, so list all and let the admin tick.
  const cd = await api("GET", "/v1/admin/app-creds");
  const creds = (cd && cd.app_creds) || [];
  const box = $("dc-creds"); box.replaceChildren();
  if (!creds.length) { const s=document.createElement("span"); s.className="muted"; s.textContent="(no app credentials)"; box.appendChild(s); }
  for (const c of creds) {
    const lab=document.createElement("label");
    const cb=document.createElement("input"); cb.type="checkbox"; cb.value=c.id; cb.className="dc-credcb";
    const t=document.createTextNode(c.app_id + "  ·  " + c.id + (c.revoked ? "  (revoked)" : ""));
    lab.appendChild(cb); lab.appendChild(t); box.appendChild(lab);
  }
  $("dc-echo").textContent = sub;
  $("dc-confirm").value = ""; $("dc-delete").disabled = true;
  $("dc-review").hidden = false;
};
$("dc-confirm").oninput = () => { $("dc-delete").disabled = $("dc-confirm").value.trim() !== dc.sub; };
$("dc-delete").onclick = async () => {
  if ($("dc-confirm").value.trim() !== dc.sub) return;
  const credIds = Array.from(document.querySelectorAll(".dc-credcb:checked")).map(cb => cb.value);
  $("dc-delete").disabled = true;
  const fail = [];
  // Bindings first, then chosen creds, then the user last.
  for (const b of dc.bridges) {
    try { await api("DELETE", "/v1/admin/bridges/" + encodeURIComponent(b.bridge_id)); }
    catch (e) { fail.push("bridge " + b.bridge_id + ": " + e.message); }
  }
  for (const id of credIds) {
    try { await api("DELETE", "/v1/admin/app-creds/" + encodeURIComponent(id)); }
    catch (e) { fail.push("cred " + id + ": " + e.message); }
  }
  try { await api("DELETE", "/v1/admin/users/" + encodeURIComponent(dc.sub)); }
  catch (e) { fail.push("user " + dc.sub + ": " + e.message); }
  if (fail.length) banner("Partly done — failures: " + fail.join("; "), "bad");
  else banner("Decommissioned " + dc.sub, "ok");
  loadDecommission(); loadUsers(); loadApps();
};

for (const b of document.querySelectorAll(".navlink"))
  b.onclick = () => go(b.dataset.view);

// Sign out: drop the bootstrap token before following the server logout link,
// so a bootstrap-only session actually ends instead of surviving in the tab.
$("logout").addEventListener("click", () => {
  try { sessionStorage.removeItem("relayent.admintok"); } catch (e) {}
});

/* Collapsible sidebar groups: each header folds/unfolds its items; the state is
   remembered per group in localStorage so it sticks across reloads. */
function setGroup(name, open) {
  const hdr = document.querySelector('.navgroup[data-group="' + name + '"]');
  const items = document.querySelector('.navgroup-items[data-group-items="' + name + '"]');
  if (!hdr || !items) return;
  items.hidden = !open;
  hdr.setAttribute("aria-expanded", open ? "true" : "false");
  try { localStorage.setItem("nav.grp." + name, open ? "1" : "0"); } catch (e) {}
}
for (const hdr of document.querySelectorAll(".navgroup")) {
  const name = hdr.dataset.group;
  let open = name !== "help"; // Help starts folded; other groups start expanded.
  try { const v = localStorage.getItem("nav.grp." + name); if (v !== null) open = v === "1"; } catch (e) {}
  setGroup(name, open);
  hdr.addEventListener("click", () => setGroup(name, hdr.getAttribute("aria-expanded") !== "true"));
}

/* Guide is a collapsible tree: the parent toggles the sub-tree AND opens the
   Guide view; each child opens the Guide and scrolls to that topic's card. */
function setGuideOpen(open) {
  $("guide-sub").hidden = !open;
  $("guide-toggle").setAttribute("aria-expanded", open ? "true" : "false");
  $("guide-caret").textContent = open ? "▾" : "▸";
}
$("guide-toggle").addEventListener("click", () => {
  const opening = $("guide-sub").hidden;
  setGuideOpen(opening);
  go("help");
});
function showTopic(topic) {
  setGuideOpen(true);
  go("help");
  for (const s of document.querySelectorAll(".subnavlink"))
    s.classList.toggle("active", s.dataset.topic === topic);
  const el = $("help-" + topic);
  if (el) {
    if (el.tagName === "DETAILS") el.open = true;   // expand the collapsed topic
    el.scrollIntoView({behavior: "smooth", block: "start"});
    el.classList.remove("flash"); void el.offsetWidth; el.classList.add("flash");
  }
}
for (const s of document.querySelectorAll(".subnavlink"))
  s.onclick = () => { showTopic(s.dataset.topic); location.hash = "help/" + s.dataset.topic; };

window.addEventListener("hashchange", () => routeHash());
function routeHash() {
  const h = location.hash.slice(1);
  if (h.startsWith("help/")) { showTopic(h.slice(5)); return; }
  go(h);
}

/* Pick up a bootstrap token handed over from /login via the URL fragment
   (#token=...). The fragment is never sent to the server; we read it, keep the
   token in memory only, and strip it from the address bar immediately. */
function adoptTokenFromHash() {
  const h = location.hash || "";
  const m = h.match(/(?:^#|&)token=([^&]+)/);
  if (m) {
    token = decodeURIComponent(m[1]);
    try { sessionStorage.setItem("relayent.admintok", token); } catch (e) {}
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
    $("whoami-label").textContent = "Signed in";
    // Reveal the "View demo" link only when a demo URL is configured (RELAYENT_DEMO_URL).
    // Admin -> demo only; the public demo never links back here.
    try {
      const cfg = await api("GET", "/v1/admin/config");
      // Only accept an http(s) URL (guards against a javascript: href even though
      // the value is operator-set server-side).
      if (cfg && /^https?:\/\//i.test(cfg.demo_url || "")) {
        const d = $("demolink"); d.href = cfg.demo_url; d.style.display = "";
      }
    } catch (e) { /* non-fatal: no demo link if config can't be read */ }
    if (location.hash.slice(1)) routeHash(); else go("users");
  } catch (e) { if (e.message !== "unauthorized") banner("Error: " + e.message, "bad"); }
}
boot();
</script>
</body>
</html>`
