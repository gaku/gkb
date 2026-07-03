# gkb — gaku's knowledge base

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

This sets `kb_dir` in `~/.gkb` (TOML format). The directory is created automatically on first `add`.

## Usage

```
gkb add <title> [-t tag1,tag2]   create a new entry
gkb show <slug>                   display an entry
gkb edit <slug>                   open entry in $EDITOR
gkb delete <slug>                 remove an entry
gkb list                          list all entries
gkb search <query>                full-text search
gkb search --tag <tag>            filter by tag
gkb status                        show kb_dir and entry count
```

### Examples

```bash
gkb add "auth strategy" -t auth,infra
gkb list
gkb show auth-strategy
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
kb_dir = "~/self/kb"
```
