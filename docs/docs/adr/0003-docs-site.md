---
id: 0003-docs-site
title: "0003: Documentation site"
---

**Status:** accepted, 2026-10-10

## Context

MASTER_PLAN.md §19 lists `docs/*.md` files. The repository already had a Docusaurus site under `docs/`, with search, Mermaid and GitHub Pages deployment.

## Decision

Keep Docusaurus. The §19 documents live in `docs/docs/` as pages of the site (`architecture.md`, `domain-model.md`, `api.md`, `security.md`, `local-deployment.md`, `cloud-deployment.md`, `backup-restore.md`, `adr/`). MASTER_PLAN.md §19 is updated to these paths.

## Consequences

* Docs are readable as Markdown in the repository and published as a site.
* Broken links fail the docs build (`onBrokenLinks: 'throw'`), and CI builds the site on every change under `docs/`.
* The docs site needs Node 20+, and its dependencies are separate from `web/`.
