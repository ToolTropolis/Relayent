// Primary author: Navjyot Nishant
// Created on: 2026-07-19
// Last updated: 2026-07-19
// Description: The demo's single chat page. Self-contained and CSP-nonce'd like
//
//	the relay's own pages; it shares their visual identity. The page holds no
//	secret — it calls only the demo server's own /api/* endpoints, which proxy
//	to the relay with the credential attached server-side. All model output is
//	rendered with textContent (never innerHTML), so a model can't inject markup.
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

func (s *server) handlePage(w http.ResponseWriter, r *http.Request) {
	// Ignore anything but the root so /favicon.ico etc. don't render the app.
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	n := base64.RawStdEncoding.EncodeToString(nonce)
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'nonce-"+n+"'; style-src 'unsafe-inline'; img-src data:; "+
			"connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	page := strings.ReplaceAll(demoHTML, "%NONCE%", n)
	page = strings.ReplaceAll(page, "%TITLE%", htmlEscape(s.cfg.title))
	_, _ = w.Write([]byte(page))
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;").Replace(s)
}

const demoHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%TITLE%</title>
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,%3Csvg%20width%3D%22512%22%20height%3D%22512%22%20viewBox%3D%220%200%20512%20512%22%20fill%3D%22none%22%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20role%3D%22img%22%20aria-label%3D%22Relayent%22%3E%3Ctitle%3ERelayent%3C%2Ftitle%3E%3Cdefs%3E%3ClinearGradient%20id%3D%22rl-bg%22%20x1%3D%2296%22%20y1%3D%2272%22%20x2%3D%22416%22%20y2%3D%22440%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23818cf8%22%2F%3E%3Cstop%20offset%3D%220.55%22%20stop-color%3D%22%236366f1%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%234f46e5%22%2F%3E%3C%2FlinearGradient%3E%3ClinearGradient%20id%3D%22rl-path%22%20x1%3D%22150%22%20y1%3D%22330%22%20x2%3D%22362%22%20y2%3D%22182%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.55%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.95%22%2F%3E%3C%2FlinearGradient%3E%3C%2Fdefs%3E%3Crect%20x%3D%2248%22%20y%3D%2248%22%20width%3D%22416%22%20height%3D%22416%22%20rx%3D%22104%22%20fill%3D%22url%28%23rl-bg%29%22%2F%3E%3Crect%20x%3D%2248.5%22%20y%3D%2248.5%22%20width%3D%22415%22%20height%3D%22415%22%20rx%3D%22103.5%22%20fill%3D%22none%22%20stroke%3D%22%23ffffff%22%20stroke-opacity%3D%220.18%22%20stroke-width%3D%221%22%2F%3E%3Cpath%20d%3D%22M150%20330%20L256%20214%20L362%20182%22%20fill%3D%22none%22%20stroke%3D%22url%28%23rl-path%29%22%20stroke-width%3D%2226%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%2F%3E%3Ccircle%20cx%3D%22150%22%20cy%3D%22330%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22362%22%20cy%3D%22182%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2266%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.14%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2246%22%20fill%3D%22%23ffffff%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2221%22%20fill%3D%22%234f46e5%22%2F%3E%3C%2Fsvg%3E">
<style>
  :root {
    --bg:#0b0d12; --panel:#0f1218; --card:#141821; --card-2:#181d27; --line:#222836;
    --line-soft:#1a1f2b; --fg:#e8eaf0; --fg-dim:#c2c7d4; --muted:#8790a2; --faint:#5b6478;
    --accent:#6366f1; --accent-fg:#c7cbff; --accent-soft:color-mix(in srgb,var(--accent) 18%,transparent);
    --ok:#10b981; --bad:#ef4444; color-scheme:dark;
  }
  @media (prefers-color-scheme: light) {
    :root { --bg:#f7f8fa; --panel:#fbfcfe; --card:#fff; --card-2:#f7f8fb; --line:#e5e8ef;
      --line-soft:#eef0f5; --fg:#141824; --fg-dim:#3a4152; --muted:#5b6373; --faint:#98a0b0;
      --accent:#5457ee; --accent-fg:#3f43d6; --accent-soft:color-mix(in srgb,var(--accent) 12%,transparent);
      color-scheme:light; }
  }
  * { box-sizing:border-box; }
  html,body { height:100%; margin:0; }
  body { background:var(--bg); color:var(--fg);
    font:15px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
    -webkit-font-smoothing:antialiased; display:flex; flex-direction:column; }
  ::selection { background:var(--accent-soft); }
  a { color:var(--accent-fg); }

  header { border-bottom:1px solid var(--line); background:var(--panel);
    padding:.9rem 1.15rem; display:flex; align-items:center; gap:.7rem; flex-wrap:wrap; }
  .mark { width:26px; height:26px; border-radius:8px; flex:none;
    background:center/contain no-repeat url("data:image/svg+xml,%3Csvg%20width%3D%22512%22%20height%3D%22512%22%20viewBox%3D%220%200%20512%20512%22%20fill%3D%22none%22%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20role%3D%22img%22%20aria-label%3D%22Relayent%22%3E%3Ctitle%3ERelayent%3C%2Ftitle%3E%3Cdefs%3E%3ClinearGradient%20id%3D%22rl-bg%22%20x1%3D%2296%22%20y1%3D%2272%22%20x2%3D%22416%22%20y2%3D%22440%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23818cf8%22%2F%3E%3Cstop%20offset%3D%220.55%22%20stop-color%3D%22%236366f1%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%234f46e5%22%2F%3E%3C%2FlinearGradient%3E%3ClinearGradient%20id%3D%22rl-path%22%20x1%3D%22150%22%20y1%3D%22330%22%20x2%3D%22362%22%20y2%3D%22182%22%20gradientUnits%3D%22userSpaceOnUse%22%3E%3Cstop%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.55%22%2F%3E%3Cstop%20offset%3D%221%22%20stop-color%3D%22%23ffffff%22%20stop-opacity%3D%220.95%22%2F%3E%3C%2FlinearGradient%3E%3C%2Fdefs%3E%3Crect%20x%3D%2248%22%20y%3D%2248%22%20width%3D%22416%22%20height%3D%22416%22%20rx%3D%22104%22%20fill%3D%22url%28%23rl-bg%29%22%2F%3E%3Crect%20x%3D%2248.5%22%20y%3D%2248.5%22%20width%3D%22415%22%20height%3D%22415%22%20rx%3D%22103.5%22%20fill%3D%22none%22%20stroke%3D%22%23ffffff%22%20stroke-opacity%3D%220.18%22%20stroke-width%3D%221%22%2F%3E%3Cpath%20d%3D%22M150%20330%20L256%20214%20L362%20182%22%20fill%3D%22none%22%20stroke%3D%22url%28%23rl-path%29%22%20stroke-width%3D%2226%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%2F%3E%3Ccircle%20cx%3D%22150%22%20cy%3D%22330%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22362%22%20cy%3D%22182%22%20r%3D%2230%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.92%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2266%22%20fill%3D%22%23ffffff%22%20fill-opacity%3D%220.14%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2246%22%20fill%3D%22%23ffffff%22%2F%3E%3Ccircle%20cx%3D%22256%22%20cy%3D%22214%22%20r%3D%2221%22%20fill%3D%22%234f46e5%22%2F%3E%3C%2Fsvg%3E"); }
  header b { font-size:1rem; letter-spacing:-.01em; }
  header .grow { flex:1; }
  .sel { display:flex; align-items:center; gap:.5rem; }
  .sel label { color:var(--muted); font-size:.82rem; }
  select { background:var(--bg); border:1px solid var(--line); color:var(--fg);
    padding:.4rem .6rem; border-radius:9px; font:inherit; max-width:60vw; }
  select:focus { outline:none; border-color:var(--accent); box-shadow:0 0 0 3px var(--accent-soft); }
  .badge { font-size:.72rem; font-weight:600; padding:.15rem .55rem; border-radius:999px;
    border:1px solid var(--line); color:var(--muted); display:inline-flex; align-items:center; gap:.35rem; }
  .badge .dot { width:7px; height:7px; border-radius:50%; background:var(--faint); }
  .badge.on { color:var(--ok); border-color:color-mix(in srgb,var(--ok) 35%,transparent); }
  .badge.on .dot { background:var(--ok); }

  main { flex:1; display:flex; flex-direction:column; max-width:820px; width:100%;
    margin:0 auto; padding:1.15rem; min-height:0; }
  #log { flex:1; overflow-y:auto; display:flex; flex-direction:column; gap:.9rem; padding-bottom:1rem; }
  .msg { display:flex; gap:.7rem; max-width:100%; }
  .msg .avatar { width:26px; height:26px; border-radius:7px; flex:none; display:grid; place-items:center;
    font-size:.72rem; font-weight:700; }
  .msg.user .avatar { background:var(--accent); color:#fff; }
  .msg.bot .avatar { background:var(--card-2); border:1px solid var(--line); color:var(--muted); }
  .bubble { border:1px solid var(--line); border-radius:12px; padding:.7rem .9rem;
    background:var(--card); white-space:pre-wrap; word-break:break-word; min-width:0; }
  .msg.user .bubble { background:var(--accent-soft); border-color:var(--accent-soft); }
  .bubble.err { border-color:color-mix(in srgb,var(--bad) 40%,transparent);
    background:color-mix(in srgb,var(--bad) 10%,transparent); color:var(--fg); }
  .bubble.think { color:var(--muted); }
  .empty { color:var(--faint); text-align:center; margin:auto; max-width:36ch; }

  form { display:flex; gap:.6rem; align-items:flex-end; border-top:1px solid var(--line-soft);
    padding-top:.9rem; }
  textarea { flex:1; resize:none; background:var(--bg); border:1px solid var(--line); color:var(--fg);
    padding:.65rem .8rem; border-radius:11px; font:inherit; max-height:160px; }
  textarea:focus { outline:none; border-color:var(--accent); box-shadow:0 0 0 3px var(--accent-soft); }
  button { background:var(--accent); color:#fff; border:0; padding:.65rem 1.1rem; border-radius:11px;
    font:inherit; font-weight:650; cursor:pointer; }
  button:hover { filter:brightness(1.08); }
  button:disabled { opacity:.5; cursor:not-allowed; }
  .ghlink { display:inline-flex; align-items:center; gap:.4rem; color:var(--muted);
    text-decoration:none; font-size:.84rem; border:1px solid var(--line); padding:.35rem .6rem;
    border-radius:9px; }
  .ghlink:hover { color:var(--fg); border-color:var(--accent); }
  .ghlink svg { width:15px; height:15px; fill:currentColor; }
  footer { display:flex; flex-wrap:wrap; align-items:center; justify-content:center;
    gap:.4rem 1rem; color:var(--faint); font-size:.78rem; padding:.9rem .6rem;
    border-top:1px solid var(--line-soft); }
  footer .tagline { color:var(--faint); }
  footer nav { display:flex; flex-wrap:wrap; gap:.9rem; align-items:center; }
  footer a { color:var(--muted); text-decoration:none; }
  footer a:hover { color:var(--fg); }
  footer .sep { color:var(--line); }
  @media (prefers-reduced-motion:reduce) { *{animation:none !important; transition:none !important;} }
</style>
</head>
<body>
  <header>
    <span class="mark"></span>
    <b>%TITLE%</b>
    <span class="badge" id="status"><span class="dot"></span><span id="status-t">connecting…</span></span>
    <span class="grow"></span>
    <a class="ghlink" href="https://github.com/ToolTropolis/Relayent" target="_blank" rel="noopener noreferrer" title="Relayent on GitHub">
      <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
      GitHub
    </a>
    <span class="sel">
      <label for="model">Model</label>
      <select id="model"><option>loading…</option></select>
    </span>
  </header>

  <main>
    <div id="log">
      <div class="empty" id="empty">Pick a model and say hello. Responses run on a real CLI
      subscription via Relayent — there's no API key behind this.</div>
    </div>
    <form id="form">
      <textarea id="prompt" rows="1" placeholder="Message…" autocomplete="off"></textarea>
      <button id="send" type="submit" disabled>Send</button>
    </form>
  </main>
  <footer>
    <span class="tagline">Powered by <a href="https://github.com/ToolTropolis/Relayent" target="_blank" rel="noopener noreferrer">Relayent</a> — your subscription, from anywhere.</span>
    <nav aria-label="Project links">
      <a href="https://github.com/ToolTropolis/Relayent" target="_blank" rel="noopener noreferrer">GitHub</a>
      <span class="sep">·</span>
      <a href="https://github.com/ToolTropolis/Relayent/blob/main/README.md" target="_blank" rel="noopener noreferrer">Docs</a>
      <a href="https://github.com/ToolTropolis/Relayent/blob/main/API.md" target="_blank" rel="noopener noreferrer">API</a>
      <a href="https://github.com/ToolTropolis/Relayent/blob/main/INSTALL.md" target="_blank" rel="noopener noreferrer">Install</a>
      <a href="https://github.com/ToolTropolis/Relayent/blob/main/SECURITY.md" target="_blank" rel="noopener noreferrer">Security</a>
    </nav>
  </footer>

<script nonce="%NONCE%">
const $ = id => document.getElementById(id);
let models = {};          // backend -> {models:[], default}
let busy = false;

function setStatus(online) {
  const b = $("status"), t = $("status-t");
  b.className = "badge" + (online ? " on" : "");
  t.textContent = online ? "bridge online" : "no bridge online";
}

function addMsg(who, text, cls) {
  $("empty")?.remove();
  const wrap = document.createElement("div"); wrap.className = "msg " + who;
  const av = document.createElement("div"); av.className = "avatar";
  av.textContent = who === "user" ? "You" : "AI";
  const bub = document.createElement("div"); bub.className = "bubble" + (cls ? " " + cls : "");
  bub.textContent = text;                    // textContent — never innerHTML
  wrap.appendChild(av); wrap.appendChild(bub);
  $("log").appendChild(wrap);
  $("log").scrollTop = $("log").scrollHeight;
  return bub;
}

async function loadModels() {
  try {
    const r = await fetch("/api/models");
    const d = await r.json();
    setStatus(!!d.online);
    const sel = $("model"); sel.replaceChildren(); models = {};
    if (!d.backends || !d.backends.length) {
      const o = document.createElement("option"); o.textContent = "no models available"; o.value = "";
      sel.appendChild(o); $("send").disabled = true; return;
    }
    for (const b of d.backends) {
      models[b.name] = { list: b.models || [], def: b.default_model || (b.models||[])[0] || "" };
      const list = (b.models && b.models.length) ? b.models : [""];
      for (const m of list) {
        const o = document.createElement("option");
        o.value = b.name + "|" + m;
        o.textContent = m ? (b.name + " · " + m) : b.name;
        sel.appendChild(o);
      }
    }
    // Pre-select the configured default backend's default model.
    const pref = d.default_backend;
    if (models[pref]) {
      sel.value = pref + "|" + (models[pref].def || (models[pref].list[0] || ""));
    }
    $("send").disabled = !d.online;
  } catch (e) {
    setStatus(false);
    const sel = $("model"); sel.replaceChildren();
    const o = document.createElement("option"); o.textContent = "relay unreachable"; sel.appendChild(o);
    $("send").disabled = true;
  }
}

async function send(prompt) {
  const [backend, model] = $("model").value.split("|");
  if (!backend) { return; }
  busy = true; $("send").disabled = true;
  addMsg("user", prompt);
  const thinking = addMsg("bot", "thinking…", "think");
  try {
    const r = await fetch("/api/chat", {
      method: "POST", headers: {"Content-Type":"application/json"},
      body: JSON.stringify({backend, model, prompt})
    });
    const d = await r.json();
    if (d.status === "done") { thinking.className = "bubble"; thinking.textContent = d.reply || "(empty response)"; }
    else { thinking.className = "bubble err"; thinking.textContent = d.error || "something went wrong"; }
  } catch (e) {
    thinking.className = "bubble err"; thinking.textContent = "network error — try again";
  } finally {
    busy = false; $("send").disabled = false; $("prompt").focus();
  }
}

$("form").addEventListener("submit", e => {
  e.preventDefault();
  const t = $("prompt").value.trim();
  if (!t || busy) return;
  $("prompt").value = ""; autosize();
  send(t);
});
// Enter to send, Shift+Enter for newline.
$("prompt").addEventListener("keydown", e => {
  if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); $("form").requestSubmit(); }
});
function autosize() { const t = $("prompt"); t.style.height = "auto"; t.style.height = Math.min(t.scrollHeight, 160) + "px"; }
$("prompt").addEventListener("input", autosize);

loadModels();
setInterval(loadModels, 20000);   // refresh presence/models periodically
</script>
</body>
</html>`
