---
name: gkb
description: Use gaku's `gkb` CLI to look up, search, create, or update personal notes/research/wiki. Trigger when gaku asks to save/note/remember something to his knowledge base, gkb, or "kb", or asks what he has recorded about a topic, or asks to list/search/tag/rename/delete entries.
---

# gkb — gaku's knowledge-base CLI

This is a command-line interface for managing a personal knowledge base stored as Markdown files. All the accesses to the knowledge base should go through this CLI.

Each entry starts with a small frontmatter block:

```
---
title: <title>
date: YYYY-MM-DD
tags: tag1, tag2
---

<body markdown>
```

**Always use the `gkb` subcommands below — NEVER read or write entry files
directly with a general-purpose file tool (Read/Edit/Write/cat/etc.), even
though you technically have filesystem access.** Going through the CLI keeps
frontmatter well-formed and slugs consistent, and it's what `gkb --help`
itself tells agents to do. You don't need to know `kb_dir` at all; every
command below takes a slug or file path, not the kb directory.

## Read operations (safe, run freely)

- `gkb status` — entry count (and a warning if any title is ambiguous).
- `gkb list` — all entries with dates/tags.
- `gkb search [query]` / `gkb search --tag <tag>` — text search over title+body, or exact tag filter.
- `gkb show <slug>` — print one entry in full, raw frontmatter included. Always do this before editing an existing entry.

## Creating a new entry

`gkb add <title> [--tag <tag> ...] [--slug <slug>]` creates `<kb_dir>/<slug>.md`.
Slug is auto-derived from the title by lowercasing and hyphenating; pass
`--slug` for non-ASCII titles.

To seed the body at creation time, pipe it in on stdin — this is the normal,
agent-safe way to use `add`:

```bash
gkb add "<title>" --tag foo --tag bar <<'EOF'
body markdown goes here
EOF
```

If you don't pipe anything, the entry is created with an empty body — follow
up with `gkb edit <slug>` (see below) to fill it in.

Before creating, run `gkb search "<key terms>"` (or `gkb list`) to check whether a
relevant entry already exists — prefer updating it over creating a near-duplicate.

## Linking

You can link pages using double brackets: `[[title]]`. When the content is non-English, it might be better to use page title instead of slug.

To show different display text, use `[[slug|display text]]` — slug first, then a
literal pipe, then the text. Do **not** escape the pipe (`\|`); a plain `|` is
correct as-is.

## Tables

`{{table filename.tsv}}` renders a tab-separated file as a Markdown table
(first row = header) when the page is viewed via `gkb serve`. The `.tsv` file
must already exist in `kb_dir/attachments/` — there's no CLI command to
create one, so write it there directly with a file tool rather than through
`gkb`.

## Updating an existing entry

`gkb edit <slug>` reads stdin and overwrites the entry's raw Markdown file
verbatim — it errors out if stdin isn't redirected (no `$EDITOR`, so it's
always safe to run from an agent context). Typical read-modify-write:

```bash
gkb show <slug> | sed 's/foo/bar/' | gkb edit <slug>
```

Or reconstruct the whole file yourself (frontmatter + body) and pipe it in
with a heredoc. Either way, preserve the existing frontmatter (title/date/tags)
unless the change specifically calls for updating it, then run `gkb show <slug>`
again to verify.

## Serving and URL

`gkb serve` starts a local web UI to browse/search/edit entries — only run it
if the owner explicitly asks. If he's set `serve_url` in `~/.gkb` (the externally
reachable Tailscale/Caddy address, since gkb only knows its local bind
address), `gkb status` prints it as `serving at: <url>` — use that to hand
the owner a link instead of guessing one.

It is good habit to share the serving URL when you create/modify entries.

## Other mutations

- `gkb rename <old-slug> <new-slug>` — only when explicitly requested.
- `gkb attach <slug> <image-file|->` — attaches an image to an entry (pass `-` to read image bytes from stdin).
- `gkb delete <slug>` — destructive; confirm with gaku before running.
- `gkb serve` — starts a persistent local web server; only run if explicitly asked.
- `gkb init` — (re)initializes the kb directory; only run if explicitly asked.

## Command reference

```text
gkb status
gkb list
gkb search [query] [--tag <tag>]
gkb show <slug>
gkb add <title> [--slug <slug>] [--tag <tag> ...]   # pipe body on stdin
gkb edit <slug>                                     # pipe new raw file on stdin
gkb rename <old-slug> <new-slug>
gkb attach <slug> <image-file|->
gkb delete <slug>
gkb serve [--bind <addr>] [--port <n>]
gkb init
```
