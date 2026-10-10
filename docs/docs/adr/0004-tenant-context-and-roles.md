---
id: 0004-tenant-context-and-roles
title: "0004: Tenant context and roles"
---

**Status:** accepted, 2026-10-10

## Context

MASTER_PLAN.md §5.3 requires staff requests to resolve and verify a tenant context, and §12 separates platform administration, tenant owners/managers and staff. The proposed staff paths (`/api/admin/...`) carry no tenant segment.

## Decision

* The **active tenant is part of the server-side session**. Login selects it when the user has one membership. `PUT /api/auth/session/tenant` switches it after checking the membership. Each request re-reads the membership, so revocation takes effect immediately.
* A client-supplied tenant id is never used for authorization.
* **Roles** per membership: `owner`, `manager`, `staff`. **Permissions** are checked in handlers through `tenant.Role.Can`. The matrix is in `internal/tenant/roles.go` and [Domain model](../domain-model.md#roles-and-permissions).
* **Platform admin** is a user flag outside the role system. It grants no implicit tenant membership.
* MVP: one location per tenant. Admin endpoints act on it.

## Alternatives

* Tenant in the path (`/api/t/{tenant}/admin/...`): explicit, but every client call must carry it, and it invites trusting the path. It is still possible later, as a version bump.
* A tenant header: same trust problem, and easy to forget in the client.

## Consequences

* One browser session works in one tenant at a time. Switching tenants reloads the admin UI.
* Adding multiple locations later adds an active location to the session and the same check against it.
