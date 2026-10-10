---
id: contribution
title: Contributing
---

## Building and testing

```bash
make build             # frontend (web/dist), then dist/servly with it embedded
make run               # build and serve on :8080 with SQLite in ./data
make test              # Go tests: unit and SQLite integration, race detector, coverage
make test-integration  # every Go test on SQLite and PostgreSQL (starts a PostgreSQL container)
make check             # gofmt, vet, golangci-lint, tests, frontend lint, typecheck and build
make image             # container image
```

Frontend development with live reload:

```bash
make build-go && SERVLY_LOG_FORMAT=text dist/servly serve   # API on :8080
cd web && npm run dev                                        # app on :5173, proxies /api
```

Toolchain: Go (version in `go.mod`), Node 18.18+ for `web/`, Node 20+ for `docs/`, and podman or docker for `make test-integration` and `make image`.

## Working on servly

* [MASTER_PLAN.md](https://github.com/servly-io/servly/blob/main/MASTER_PLAN.md) is authoritative. Its §14 sets the rules for tasks, ownership and completion reports.
* Contracts first: the schema, the [domain model](../domain-model.md) and the [API](../api.md) change together with the code, in the same pull request.
* Every persistence test uses `dbtest.Each`, so it runs on both databases.
* Every tenant-owned endpoint gets cross-tenant tests. See [Security](../security.md#tenant-isolation).

## Pull requests

The PR title is a [Conventional Commit](https://www.conventionalcommits.org/) line (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`). Add `!` or a `BREAKING CHANGE:` footer if it breaks the API, the configuration or the database upgrade path.

The title becomes the squash commit and drives the release notes and the version bump.

Use the [PR template](https://github.com/servly-io/servly/blob/main/.github/PULL_REQUEST_TEMPLATE.md).
