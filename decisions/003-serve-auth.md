# Decision: HTTPS via reverse proxy + Basic Auth for web editing

**Date:** 2026-07-03  
**Status:** Decided

## Context

`gkb serve` was a read-only web view of the knowledge base bound to `0.0.0.0`
over plain HTTP. We want two things:

1. **HTTPS**, so the KB can be reached securely from other devices.
2. **Editing from the browser** (`add`/`edit` without dropping to `$EDITOR`),
   which turns the server into a *writable* surface and therefore needs
   authentication.

## Decision

**TLS is terminated by a reverse proxy, not by `gkb`.** `gkb serve` stays plain
HTTP and something in front (Tailscale `serve`, or Caddy) provides the trusted
certificate.

**`gkb` owns authentication** via a whole-site form login backed by a signed
session cookie, so editing is gated regardless of what the proxy does.
Credentials live in `~/.gkb`:

```toml
kb_dir     = "~/self/kb"
serve_user = "gaku"
serve_pass = "..."
```

If either field is empty the server runs open and prints a warning at startup.

## Rationale

- **No cert management in the tool.** Let's Encrypt via `autocert` needs a public
  domain and open ports 80/443 — too much for a personal KB behind NAT.
  Self-signed certs mean permanent browser warnings. A proxy sidesteps both.
- **Tailscale is the sweet spot.** `tailscale serve https / http://127.0.0.1:8086`
  gives a browser-trusted cert on the tailnet with zero code and no port
  forwarding. Caddy is the equivalent for a public domain.
- **Form login over Basic Auth — for password managers.** Basic Auth's native
  browser dialog is not an HTML form, so password managers can't save or autofill
  it. A real `/login` form with `autocomplete="username"` /
  `autocomplete="current-password"` fields fixes that. The tradeoff is a small
  amount of session-cookie plumbing.
- **Stateless signed cookie.** On success the server sets `gkb_session` =
  `<expiry>.<HMAC-SHA256(expiry)>`, keyed by the password. No server-side session
  store, and cookies survive restarts (important because `make install` bounces
  the process) — yet changing the password invalidates every session. Credential
  and signature checks are constant-time (`crypto/subtle`, `hmac.Equal`).
- **Auth composes cleanly with a TLS-terminating proxy.** The password crosses
  only the encrypted browser→proxy hop and the loopback proxy→gkb hop.
- **Whole-site (not writes-only) auth** was chosen for the simplest mental model:
  one password protects everything.

## Binding

Behind a proxy, run with `--bind 127.0.0.1` so the plaintext port is not
reachable on the LAN (which would let a client send the password in cleartext
to the raw HTTP port). The default stays `0.0.0.0` to preserve the original
LAN-browsing behavior for users not fronting it with a proxy.

```
gkb serve --bind 127.0.0.1        # then: tailscale serve https / http://127.0.0.1:8086
```

## Web editor

- `/new` — form to create an entry (slug derived from the title via `Slugify`).
- `/edit/<slug>` — edit title, tags, and raw markdown body.
- `POST /save` — writes the entry. Empty `slug` field ⇒ new entry; otherwise the
  existing entry is loaded so its **creation date is preserved** and title / tags
  / body are updated. Redirects to `/entry/<slug>` (303).
- The slug is fixed on edit (it is the file identity); renaming stays a CLI
  operation (`gkb rename`), consistent with 001.
- New write path in the `kb` package: `kb.Save` / `kb.Marshal` serialize
  frontmatter **plus body** (the original `marshal` emitted frontmatter only,
  since the CLI filled the body in via `$EDITOR`).

## Open Questions

- **Delete from the web** — deliberately omitted for now; deletion stays a CLI
  operation to avoid destructive one-click actions.
- **CSRF** — the session cookie is `SameSite=Lax`, which blocks it on cross-site
  POSTs (a basic defense). No per-form CSRF token yet; acceptable for a
  single-user tool on a private tailnet, revisit if exposed more broadly.
- **Cookie `Secure` flag** — omitted so plain-HTTP localhost testing works. Behind
  a TLS proxy the browser↔proxy hop is encrypted anyway; set it if the server is
  ever reached over HTTP on an untrusted network.
- **Password storage** — plaintext in `~/.gkb`. A bcrypt hash would be stronger
  but adds a hashing/setup step; deferred as over-engineering for a personal KB.
- **Proxy-provided identity** — Tailscale can pass `Tailscale-User-Login`
  headers; we could trust those instead of Basic Auth, but that couples auth to
  one proxy. Kept app-level auth for portability.
