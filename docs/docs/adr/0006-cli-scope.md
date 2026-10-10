---
id: 0006-cli-scope
title: "0006: The CLI operates the installation, not the restaurant"
---

**Status:** accepted, 2026-10-10

## Context

ADR 0002 made `servly` one binary with operator commands, and it expected more of them later (`tenant create`, `backup`). Until ADR 0005, a restaurant could start only after its owner ran `servly tenant create`, a step that is part of the business. That blurs two audiences. The **operator** runs the installation. The **restaurant** (owner, managers, staff) runs the business. Restaurant owners are not expected to use a shell (MASTER_PLAN.md §1).

## Decision

The command line serves the operator only (MASTER_PLAN.md §4.5).

| The CLI does | The CLI does not |
|---|---|
| `serve`, `migrate`, `config`, `version`, `options` | set up a restaurant for its owner |
| backup and restore (Phase 5) | change the menu, tables, staff or settings |
| `user set-password`: recover access when nobody can sign in | take, change or pay orders |
| `tenant create`: provision a tenant on a managed platform | serve as a required step in any owner-facing guide |

* A fresh installation is set up in the browser ([ADR 0005](0005-first-run-setup.md)).
* Owner-facing documentation (setup guide, local deployment, README) never sends the reader to the CLI for business tasks. Recovery is the one exception, and the guide labels it as an operator task.
* A CLI command calls the same `internal/app` use cases as the API. It adds no business rules of its own.
* Developer tooling (`make demo`, tests) may use the CLI to seed data. It is not a product path.

## Alternatives

* **Full admin CLI** (menu import, staff, orders): it would be a second interface to every business rule, to keep in step and to secure. No owner asked for it.
* **No tenant command at all:** a managed platform then needs the browser setup on every new tenant, or direct database access. Provisioning stays a platform-administration command until a control plane exists (MASTER_PLAN.md §5.5).

## Consequences

* New business features ship in the web app and the API only.
* `servly tenant create` still takes the restaurant's currency and timezone, because a provisioned tenant needs a valid location. The owner can change both in Settings.
* A future bulk feature, such as menu import, goes into the admin UI. It does not become a CLI command.
