<!--- Title: a Conventional Commit line, e.g. "feat(orders): reject quantity above 99". -->

## Description
<!--- What changed and why. Link the issue if any: Fixes #123 -->

## Testing
<!--- What you ran and what you observed. Say if a test could not run here and why. -->

## Checklist
- [ ] Tests added or updated; `make test` passes; `make test-integration` if SQL, migrations or repositories changed.
- [ ] Every tenant-owned query takes the tenant id; cross-tenant access is tested where data access changed.
- [ ] `make check` clean (gofmt, vet, lint, frontend lint/typecheck/build).
- [ ] Docs in `docs/docs/` updated if a contract, behavior or deployment step changed; ADR added if MASTER_PLAN.md §18 requires one.
- [ ] No new runtime dependency or external service without an ADR.
- [ ] No password, session token, payment detail or connection secret reaches a log line or an API response.
