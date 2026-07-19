# Relayent nginx vhosts

These are Relayent's reverse-proxy vhosts. They live **here, in the Relayent repo** — not in
whatever app happens to run the host's nginx. On a shared host, the host's nginx (e.g. an app's)
mounts these files from `/opt/relayent/nginx/`, so the proxy fronts Relayent **without any Relayent
config living in the app's repo**.

| File | Serves | Proxies to |
|---|---|---|
| `relayent.conf` | `relayent.ignorelist.com` | `relayent-relay:8787` |
| `relayent-demo.conf` | `relayent-demo.ignorelist.com` | `relayent-demo:8080` |

## Why they're mounted, not owned by the proxy's repo

Only one process can bind host `:443`, so on a single host every public site shares one nginx. That
nginx is a **shared host component**; it should not carry another product's config in its repo. So:

- Relayent owns these files (here).
- Deployed to the host at `/opt/relayent/nginx/`.
- The host nginx's compose mounts them read-only from that path:

  ```yaml
  # in the host nginx service's volumes:
  - /opt/relayent/nginx/relayent.conf:/etc/nginx/conf.d/relayent.conf:ro
  - /opt/relayent/nginx/relayent-demo.conf:/etc/nginx/conf.d/relayent-demo.conf:ro
  ```

The app's repo keeps only those two mount lines — a declaration that "this host also fronts
Relayent" — and none of the vhost content. Update a vhost here → copy to `/opt/relayent/nginx/` →
`nginx -t && nginx -s reload`.

## Requirements

- The host nginx container must join the external `relayent-network` (so `proxy_pass` resolves the
  `relayent-relay` / `relayent-demo` container names) and mount `/etc/letsencrypt` + a
  `/var/www/certbot` webroot.
- A cert per hostname. Issue without downtime via webroot:

  ```bash
  sudo certbot certonly --webroot -w /var/www/certbot -d relayent.ignorelist.com
  sudo certbot certonly --webroot -w /var/www/certbot -d relayent-demo.ignorelist.com
  ```

- Both vhosts pass the relay/demo's own per-request-nonce CSP straight through (they set none of
  their own); don't add `proxy_hide_header Content-Security-Policy`.

## Fresh host (Relayent owns the proxy too)

If Relayent is the only thing on the host, use the bundled Caddy stack in `deploy/` instead — it
fetches and renews certs automatically and needs none of the above. These nginx vhosts are for the
**shared-host** case, where another app's nginx is already the host proxy.
