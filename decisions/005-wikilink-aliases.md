# Decision: `aliases:` frontmatter field for wikilink resolution

**Date:** 2026-07-10
**Status:** Decided

## Context

[[004-wikilinks-by-title.md]] lets `[[X]]` resolve against an entry's slug or
its exact `Title`. That breaks when a title mixes scripts, e.g.
`インドゥティオマルス（Indutiomarus）`: a natural `[[インドゥティオマルス]]`
reference (the Japanese name alone, no romanization or parentheses) matches
neither the ASCII slug (`indutiomarus`) nor the full title, so it falls
through to the "not yet created" case.

004's own "Future plan" section already anticipated this and sketched the
fix: an optional `aliases:` frontmatter field, comma-separated, parsed
alongside `tags:`/`title:`.

## Decision

Add `Aliases []string` to `kb.Entry`, parsed from an `aliases:` frontmatter
line exactly like `tags:` (comma-separated, trimmed) and round-tripped by
`Marshal`. `buildWikiLinkIndex` (`cmd/serve.go`) gains an `aliases` map
alongside `slugs`/`titles`, and `resolve` checks it as a third tier:

1. Slug — exact match (unchanged).
2. Title — exact match (unchanged, from 004).
3. **Alias — exact match (new).**
4. Fallback — target used verbatim (unchanged).

So `title: インドゥティオマルス（Indutiomarus）` plus
`aliases: インドゥティオマルス, Indutiomarus` makes both
`[[インドゥティオマルス]]` and `[[Indutiomarus]]` resolve to the entry.

Same collision handling as titles: first-seen wins in `buildWikiLinkIndex`,
and since `kb.List` sorts by `ModTime` descending, that means most-recently-
modified wins for an alias shared by multiple entries. No separate warning
for duplicate aliases was added — `gkb status`'s existing
`warnDuplicateTitles` covers the common case (title collisions), and alias
collisions are rare enough not to warrant new machinery yet.

No persisted alias table, for the same reason 004 rejected one: the `.md`
file stays the sole source of truth, editable directly via `$EDITOR`.

## Consequences

- Existing entries are unaffected (no `aliases:` line ⇒ empty `Aliases` ⇒ no
  behavior change).
- Adding an alias, to a new or existing entry, is a manual frontmatter edit
  (`gkb add` seeds a body via stdin but has no per-field flags for anything
  but `--tag`/`--slug`; `gkb edit` replaces the whole raw file). No new CLI
  flag was added for `aliases:` — out of scope for this fix, and easy to add
  later if it turns out to matter.
