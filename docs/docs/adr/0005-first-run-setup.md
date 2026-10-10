---
id: 0005-first-run-setup
title: "0005: First-run setup in the browser"
---

**Status:** accepted, 2026-10-10

## Context

MASTER_PLAN.md §2.1 and §10 ask for a guided setup that an owner without technical knowledge can follow. The first restaurant and its owner were created only with `servly tenant create` on the server host. That kept every account-creating path behind shell access, but the owner had to run a command (`podman exec … --password-stdin` in a container) before seeing the product.

A browser setup adds an unauthenticated endpoint that creates the first owner. Whoever reaches it first owns the installation, so it must not be open to anyone on the network, must succeed only once, and must close for good.

## Decision

* **Installation state is stored:** the `installation` table holds one row (`id = 1`) once the installation is set up. Both `servly tenant create` and the browser setup write it, in the same transaction that creates the tenant. Migration 00003 adds the row for databases that already have a tenant.
* **Atomic claim:** the browser setup inserts the row without `ON CONFLICT`, as the first write of its transaction. A competing claim fails on the primary key (`409 setup_complete`). `servly tenant create`, the platform provisioning command ([ADR 0006](0006-cli-scope.md)), uses `ON CONFLICT DO NOTHING`, so it keeps working for more tenants.
* **Setup token:** at startup, if the installation is not set up, `servly serve` generates a random 80-bit token and logs it once at `WARN`. `POST /api/setup` requires it. Only someone who can read the server log, and so has access to the host, can claim the installation. The token lives only in that process, and every restart prints a new one. After setup, no token is generated and the endpoint answers `409`.
* **Scope:** the setup creates only the first tenant, location and owner, and signs the owner in. Menu, tables, staff and ordering options use the normal admin API afterwards, guided by the checklist on the admin home page. Nothing on the server needs to be resumed: until the one setup transaction commits, nothing exists, and after it commits, the checklist shows what is still missing.

## Alternatives

* **CLI only (the previous state):** no unauthenticated endpoint, but the first step for every owner is a shell command. That puts a business step on the operator's interface, which ADR 0006 rules out.
* **Infer the state from "no users exist":** if every user were ever removed, setup would open again on a live installation. It is also not atomic without a lock.
* **No token, open on first start:** anyone on the restaurant or guest Wi-Fi who opens the page first owns the installation.
* **Token in a file in the data directory:** it survives restarts, but `podman logs` is easier for the reader of this guide than `podman exec … cat`, and the token stays valid until it is used. Revisit this if several replicas share one fresh database.

## Consequences

* An installation can be set up entirely in the browser. The operator only copies one line from the log.
* The token is in the log until setup ends. It is useless after that, because the endpoint is closed for good.
* On a managed platform, tenants are provisioned with `servly tenant create`. The first one closes the browser setup.
* Recovery (a lost owner password) stays a host operation: `servly user set-password`.
