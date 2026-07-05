# Decision: Resolve wikilinks by title for non-ASCII page names

**Date:** 2026-07-04
**Status:** Decided

## Context

Slugs (and thus filenames) are kept ASCII by design, but titles can be in any
language. For a Japanese-titled entry (e.g. title `ローマ`, slug `rome`), the
current wikilink syntax — `[[slug]]` or `[[slug|text]]` — forces every link to
either show the ugly ASCII slug as link text or repeat the title manually as
the pipe override (`[[rome|ローマ]]`) on every reference.

The KB already has real wikilink usage: 56 links across 22 files, all in
`[[slug|text]]` order (slug first), e.g.
`[[avenger-9-8-c-stand-30-kit|9.8' C-Stand 30 Kit]]`. Any change must not
break these.

## Decision

**Extend bare `[[X]]` resolution, keep the pipe form as-is:**

1. If `X` is an existing slug → unchanged behavior (backward compatible).
2. Else if `X` exactly matches some entry's `Title` → resolve to that entry's
   slug, with display text `X` itself. This is what makes `[[ローマ]]` just
   work with no pipe needed.
3. Else → today's fallback (renders a link to a not-yet-created page).

The pipe form keeps its existing `[[slug|text]]` order (slug first, matching
every link already in the KB and MediaWiki's convention). It already covers
"custom display text pointing to a specific page" — e.g.
`[[rome|ローマの休日のロケ地]]` — with no code change needed.

**No persisted mapping/alias table.** The Title→Slug index is built
dynamically at render time from existing frontmatter (`kb.List` already
parses title + slug for every file). This mirrors what the `/` list page
already does — a full scan of `kb_dir` on every request — rather than
introducing a second artifact that can drift out of sync with the `.md`
files, which remain the sole source of truth.

## Consequences

- Rendering a single entry (`/entry/<slug>`) now reads every `.md` file's
  frontmatter on every request, not just the one requested. At current scale
  (~56 files) this is inconsequential, and stays so well into the low
  thousands of entries.
- `gkb rename` (slug rename) now leaves title-based wikilinks working
  automatically, since resolution follows the live title rather than a
  stored slug. Existing slug-based `[[old-slug]]` links still break on
  rename, same as today — unaffected by this change.
- Title collisions (two entries sharing the exact same `Title`) resolve as
  "last one wins," with a warning surfaced (e.g. via `gkb status` or at
  `gkb serve` startup) rather than a hard error.

## Rejected alternative

A persisted mapping/alias table maintained by `add`/`edit`/`rename`/web-save.
Rejected because the primary editing path is `$EDITOR` on the raw `.md`
file — a title change made there would silently desync the table, requiring
a "reindex" recovery command anyway. At that point you're back to a dynamic
scan, just with extra machinery in between.

## Future plan (not yet decided)

- **Surviving a title rename** (not just a slug rename): if `[[old-title]]`
  references need to keep resolving after retitling a page, add an optional
  `aliases:` frontmatter field (comma-separated former titles), parsed
  alongside `tags:`/`title:`. Keeps the single `.md` file as the sole source
  of truth instead of a separate table.
- **Scaling the per-render scan**: if a full-KB scan on every entry render
  ever becomes a real bottleneck, cache the title index in memory for the
  lifetime of the `gkb serve` process, invalidated by a periodic re-scan or
  file-mtime check. Deferred until it's an actual problem.
