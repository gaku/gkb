# gkb — Gaku's knowledge base

A minimal CLI for managing a personal knowledge base in plain markdown files.

## Build & Install

**Requirements:** Go 1.21+

```bash
git clone <repo>
cd gkb
go build -o gkb .
sudo mv gkb /usr/local/bin/
```

Or install directly with Go:

```bash
go install github.com/gaku/gkb@latest
```

## Setup

Create `~/.gkb` config by running:

```bash
gkb init ~/self/kb
```

This sets `kb_dir` in `~/.gkb` (TOML format). The content directory is created automatically on first `add`.

## Usage

```
gkb add <title> [-t tag1,tag2]   create a new entry (reads body from stdin if redirected)
gkb show <slug>                   print an entry's raw Markdown file
gkb edit <slug>                   open entry in $EDITOR (overwrites from stdin if redirected)
gkb attach <slug> <image-file|->  attach an image (or - for stdin) and print its Markdown
gkb delete <slug>                 remove an entry
gkb list                          list all entries
gkb search <query>                full-text search
gkb search --tag <tag>            filter by tag
gkb status                        show kb_dir and entry count
gkb serve [-p port] [-b bind]     browse & edit over the web
```

### Web server (`gkb serve`)

`gkb serve` starts a local web UI to browse, search, **and edit** entries
(create at `/new`, edit via the "edit" link on any entry). Existing entries can
also accept images by drag-and-drop; uploads are stored flat in `attachments/`
and shown as copyable Markdown on the edit page.

```bash
gkb serve                     # http://0.0.0.0:8086
gkb serve -p 9000             # custom port
gkb serve -b 127.0.0.1        # loopback only (use behind a TLS proxy)
```

**Authentication.** Because the web UI can write, set credentials in `~/.gkb` to
gate the whole site. Requests without a valid session are redirected to a `/login`
form (password-manager friendly) that sets a signed session cookie; "sign out"
clears it. If unset, the server runs open and warns at startup.

```toml
serve_user = "gaku"
serve_pass = "your-password"
```

**HTTPS.** `gkb` speaks plain HTTP; terminate TLS with a reverse proxy. With
[Tailscale](https://tailscale.com) you get a trusted cert on your tailnet with
no port forwarding:

```bash
gkb serve -b 127.0.0.1                         # bind loopback
tailscale serve https / http://127.0.0.1:8086  # trusted HTTPS on your tailnet
```

Caddy works the same way for a public domain (`reverse_proxy 127.0.0.1:8086`).
Always bind `127.0.0.1` behind a proxy so the plaintext port isn't reachable on
the LAN.

### Examples

```bash
gkb add "auth strategy" -t auth,infra
gkb add "auth strategy" -t auth,infra < notes.md   # seed the body from a file
gkb list
gkb show auth-strategy
gkb show auth-strategy | sed 's/foo/bar/' | gkb edit auth-strategy   # read-modify-write the raw file
gkb attach auth-strategy ./diagram.png
wl-paste --type image/png | gkb attach auth-strategy -   # attach a clipboard image
gkb search auth
gkb search --tag infra
gkb delete auth-strategy
```

## Entry format

Each entry is a markdown file with YAML frontmatter stored in `kb_dir`:

```markdown
---
title: auth strategy
date: 2026-07-01
tags: auth, infra
---

Entry body goes here.
```

## Config

`~/.gkb` is a TOML file:

```toml
kb_dir     = "~/self/kb"
serve_user = "gaku"          # optional — enables Basic Auth for `gkb serve`
serve_pass = "your-password" # optional
```
