---
id: index
title: Decision records
---

Architecture decision records (ADRs) document the decisions MASTER_PLAN.md §18 requires, and other choices an agent would otherwise re-open. Each ADR has a context, the decision, alternatives, consequences and compatibility notes. ADRs are not edited after acceptance. A new ADR supersedes an old one.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-persistence-stack.md) | modernc SQLite, pgx, goose; shared SQL with `$N` placeholders | accepted |
| [0002](0002-single-binary-cli.md) | one `servly` binary; cobra commands, viper configuration | accepted |
| [0003](0003-docs-site.md) | documentation stays a Docusaurus site under `docs/docs/` | accepted |
| [0004](0004-tenant-context-and-roles.md) | active tenant in the session; three roles mapped to permissions | accepted |
| [0005](0005-first-run-setup.md) | first-run setup in the browser, claimed once with a setup token from the server log | accepted |
| [0006](0006-cli-scope.md) | the CLI operates the installation; restaurant business happens in the web app | accepted |
| [0007](0007-online-payments.md) | online payments through provider adapters, hosted checkout, polling plus webhooks, `awaiting_payment` | accepted |

New ADR: copy the most recent one, take the next number, add a row here and to `docs/sidebars.js`.
