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
    background:linear-gradient(150deg,#818cf8,var(--accent) 55%,#4f46e5);
    box-shadow:0 4px 14px -4px color-mix(in srgb,var(--accent) 60%,transparent); }
  .brand .mark::after { content:""; position:absolute; inset:0; border-radius:9px;
    box-shadow:inset 0 1px 0 rgba(255,255,255,.35); }
  .brand b { font-size:1.06rem; letter-spacing:-.02em; font-weight:650; }
  .brand span { color:var(--faint); font-size:.7rem; letter-spacing:.02em;
    text-transform:uppercase; }
  nav { padding:.6rem .7rem; overflow-y:auto; flex:1; }
  .navgroup { color:var(--faint); font-size:.66rem; text-transform:uppercase;
    letter-spacing:.11em; font-weight:700; padding:1rem .7rem .3rem; }
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
  .tag { font-size:.7rem; font-weight:600; padding:.13rem .55rem; border-radius:999px;
    border:1px solid var(--line); color:var(--muted); letter-spacing:.01em; }
  .tag.admin { color:var(--accent-fg); border-color:var(--accent-soft); background:var(--accent-soft); }
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

  /* Credits / footer — sits at the bottom of every view (main is a flex column). */
  .credits { margin-top:auto; padding-top:1.75rem; border-top:1px solid var(--line-soft);
    display:grid; grid-template-columns:1fr auto; gap:1rem 2rem; align-items:start;
    color:var(--muted); }
  .credits-brand { display:flex; align-items:center; gap:.7rem; }
  .credits-brand .mark { width:26px; height:26px; border-radius:8px; flex:none;
    background:linear-gradient(150deg,#818cf8,var(--accent) 55%,#4f46e5); }
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
      <div class="navgroup">Admin</div>
      <button class="navlink" data-view="users"><span class="ic">◱</span> Users</button>
      <button class="navlink" data-view="audit"><span class="ic">≣</span> Audit</button>
      <div class="navgroup">Configure</div>
      <button class="navlink" data-view="status"><span class="ic">◈</span> Relay &amp; bridges</button>
      <button class="navlink" data-view="enroll"><span class="ic">＋</span> Enrol a bridge</button>
      <button class="navlink" data-view="backends"><span class="ic">◧</span> Backends</button>
      <button class="navlink" data-view="settings"><span class="ic">⚙</span> Settings</button>
      <div class="navgroup">Integration</div>
      <button class="navlink" data-view="creds"><span class="ic">⚿</span> App credentials</button>
      <div class="navgroup">Help</div>
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

    <!-- BACKENDS -->
    <section id="view-backends" class="view">
      <div class="head"><h1>Backends</h1><p>Control which AI backends this relay exposes. A disabled backend is hidden from apps and refused at enqueue — use it to keep a public surface off paid subscriptions.</p></div>
      <div class="card">
        <h2>Exposure policy</h2>
        <div class="tablewrap"><table>
          <thead><tr><th>Backend</th><th>Status</th><th></th></tr></thead>
          <tbody id="backends"><tr><td colspan="3" class="muted">Loading…</td></tr></tbody>
        </table></div>
        <p class="hint">Enabled backends still only run when a user's bridge actually has that CLI installed and ready — this policy narrows what's offered, it doesn't add capability.</p>
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

    <!-- HELP -->
    <section id="view-help" class="view">
      <div class="head"><h1>Guide</h1><p>What each section does, and the ideas behind them. For full setup and the API, see the docs linked at the bottom.</p></div>

      <div class="card" id="help-overview">
        <h2>The big picture</h2>
        <p class="help-p">Relayent routes an app's AI request to a <b>CLI subscription running on a user's own
        machine</b> (Claude Code, Codex, Cursor) instead of a paid API key. This relay is <b>multi-tenant</b>:
        many users, each running their own <b>bridge</b>, each on their own subscription, isolated from one
        another. A job addressed to a user runs only on that user's bridge — never anyone else's.</p>
        <p class="help-p"><b>You are the operator.</b> You manage users, enrol their bridges, and issue app
        credentials. You can see <i>activity</i> — who ran what, when, on which backend — but <b>never the
        prompt or the result</b>. That boundary is built into the relay, not a setting.</p>
      </div>

      <div class="card" id="help-users">
        <h2>Users <span class="note muted">— Admin</span></h2>
        <dl class="help-dl">
          <dt>What it is</dt><dd>Everyone with an identity on this relay. A user usually appears automatically the first time they sign in; you can also pre-provision one with <b>Add a user</b>.</dd>
          <dt>Roles</dt><dd><b>admin</b> can manage everything here; <b>user</b> can only run their own jobs and see their own status. The <b>first person ever to sign in becomes the admin</b>; everyone after is a user until you promote them (<b>Make admin</b> / <b>Demote</b>).</dd>
          <dt>Enrol</dt><dd>Mints a one-time token for that user to pair their bridge — see “Enrol a bridge”.</dd>
          <dt>Disable / Delete</dt><dd><b>Disable</b> blocks a user's jobs immediately but keeps the record; <b>Delete</b> removes it. You can't disable, demote, or delete <b>yourself</b> — a safeguard so the last admin can't be locked out.</dd>
        </dl>
      </div>

      <div class="card" id="help-audit">
        <h2>Audit <span class="note muted">— Admin</span></h2>
        <p class="help-p">A running history: who did what, when, on which backend, success or failure, and the
        <b>byte counts</b> of the prompt and result. It deliberately holds <b>no content</b> — you see that a job
        ran and how big it was, never what it said. This is the record to check for “is it being used?” and
        “did this user's jobs fail?”.</p>
      </div>

      <div class="card" id="help-status">
        <h2>Relay &amp; bridges <span class="note muted">— Configure</span></h2>
        <dl class="help-dl">
          <dt>At a glance</dt><dd>Totals across the relay: how many users, how many bridges are online right now, and how many jobs are pending.</dd>
          <dt>Bridge presence</dt><dd>Per user: is their bridge currently connected, how many bridges they've enrolled, and their pending jobs. <b>Online</b> means the bridge polled recently; <b>offline</b> usually means that user's machine is asleep or the bridge isn't running.</dd>
        </dl>
      </div>

      <div class="card" id="help-enroll">
        <h2>Enrol a bridge <span class="note muted">— Configure</span></h2>
        <p class="help-p">A bridge proves who it is with a credential it earns through <b>enrolment</b>. Pick the
        user, click <b>Mint token</b>, and send them the one-time token out-of-band (chat, email). They run
        <code>relayent-bridge setup</code> and paste it; their bridge redeems it once and is then bound to them.
        The token is shown <b>once</b> and expires — mint a fresh one if it lapses.</p>
      </div>

      <div class="card" id="help-settings">
        <h2>Settings <span class="note muted">— Configure</span></h2>
        <p class="help-p">A <b>read-only</b> view of how this relay is actually running: version, whether it's
        behind a trusted proxy, whether the multi-tenant store is on, and the OIDC identity settings (issuer,
        client id, redirect). <b>No secret values are ever shown</b> — only whether a pairing key or admin token
        is set. To change any of it, edit the relay's <code>.env</code> and run <code>docker compose up -d</code>;
        editing config from this screen is intentionally not offered.</p>
      </div>

      <div class="card" id="help-creds">
        <h2>App credentials <span class="note muted">— Integration</span></h2>
        <p class="help-p">A key an <b>app</b> (e.g. EngageHub) uses to enqueue jobs on users' behalf. Issue one
        per app; the secret (<code>&lt;id&gt;.&lt;secret&gt;</code>) is shown <b>once</b> — copy it then. The app sends it as
        a bearer token and names the target user on each job, so a request for Alice runs on Alice's subscription.
        <b>Revoke</b> kills a credential instantly. The relay stores only a hash, never the secret.</p>
      </div>

      <div class="card" id="help-signin">
        <h2>Signing in &amp; where you land</h2>
        <p class="help-p">Everyone signs in at <code>/login</code> — “Sign in with your provider”, or the bootstrap
        admin token. Afterwards you're sent to the right place automatically: <b>admins</b> to this console,
        <b>regular users</b> to their own status page at <code>/</code>. Sign out from the bottom of the sidebar.</p>
      </div>

      <div class="card">
        <h2>Learn more</h2>
        <p class="help-p">Full operator setup, migration, and the API live in the project docs:
        <b>INSTALL.md</b> (standing up the relay and onboarding users), <b>SECURITY.md</b> (the multi-tenant
        threat model and what Relayent does <i>not</i> protect against), <b>API.md</b> and <b>openapi.yaml</b>
        (the integration contract). This console never shows prompt or result content — by design.</p>
      </div>
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
const VIEWS = ["users","audit","status","enroll","backends","settings","creds","help"];
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
    if (view === "status")   await loadStatus();
    if (view === "enroll")   await loadEnrollUsers();
    if (view === "backends") await loadBackends();
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
  if (!users.length) { emptyRow(tb, 8, "No users yet."); return; }
  let anyRow = false;
  for (const u of users) {
    // Fetch this user's enrolled bridges (one row each). Host/version/presence are
    // per-user (the queue aggregates polls), shown on each of the user's rows.
    let binds = [];
    try { const d = await api("GET", "/v1/admin/users/" + encodeURIComponent(u.sub) + "/bridges"); binds = (d && d.bridges) || []; }
    catch (e) { /* skip on error */ }
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
      const pt = document.createElement("td"); pt.appendChild(pill(u.bridge_online, "online", "offline")); tr.appendChild(pt);
      tr.appendChild(cell(u.bridge_host || "—"));
      tr.appendChild(cell(u.bridge_version || "—"));
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
async function loadBackends() {
  const data = await api("GET", "/v1/admin/backends");
  const tb = $("backends"); tb.replaceChildren();
  const backends = (data && data.backends) || [];
  if (!backends.length) { emptyRow(tb, 3, "No backends."); return; }
  for (const b of backends) {
    const tr = document.createElement("tr");
    tr.appendChild(cell(b.name));
    const st = document.createElement("td"); st.appendChild(pill(b.enabled, "enabled", "disabled")); tr.appendChild(st);
    const act = document.createElement("td"); const wrap = document.createElement("div"); wrap.className = "actions";
    wrap.appendChild(btn(b.enabled ? "Disable" : "Enable", "ghost sm", () => setBackend(b.name, !b.enabled)));
    act.appendChild(wrap); tr.appendChild(act); tb.appendChild(tr);
  }
}
async function setBackend(name, enabled) {
  try { await api("POST", "/v1/admin/backends/" + encodeURIComponent(name), {enabled});
    banner((enabled ? "Enabled " : "Disabled ") + name, "ok"); loadBackends(); }
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
