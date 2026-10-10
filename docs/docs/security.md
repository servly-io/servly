---
id: security
title: Security
---

This page covers how servly authenticates, authorizes and isolates tenants, and the threats it is designed against. Report a vulnerability privately through [GitHub's private vulnerability reporting](https://github.com/servly-io/servly/security/advisories/new).

## Threat model

| Actor | Can | Must not be able to |
|---|---|---|
| Anyone on the internet / restaurant Wi-Fi | open a menu with a valid table token; submit orders; view an order with its token | enumerate tokens, orders or tenants; set prices; read staff data; exhaust the server |
| Customer of tenant A | see own order by token | see other orders, other tenants, internal ids, staff identities |
| Staff of tenant A | operate orders and payments of A | touch tenant B (by id substitution, enumeration, or crafted tenant ids); escalate role |
| Manager / owner of A | manage menu, tables, staff, settings of A | the same cross-tenant actions |
| Platform admin | provision and suspend tenants | read tenant business data without an audited action |
| Someone with the host or database | everything | - out of scope; protect the host and backups |

## Authentication

* Passwords are hashed with Argon2id (`golang.org/x/crypto/argon2`). Each hash has its own random salt, and the parameters are stored with the hash. Minimum password length is 10 characters. Upper length limit: 256 bytes, which keeps the hashing cost bounded.
* Sessions are server-side. The cookie carries a 256-bit random token, and the database stores only its SHA-256. Cookie flags: `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` when served over HTTPS. Idle expiry is 12 h and absolute expiry is 7 days. Logout deletes the row. Disabling a user or removing a membership takes effect on the next request.
* **CSRF:** cookie authentication combined with a synchronizer token. State-changing `/api/auth` and `/api/admin` requests must carry `X-CSRF-Token` equal to the session's token. A cross-site form cannot set the header, and `SameSite=Lax` blocks cross-site cookies on `POST`.
* **Login rate limits:** per IP and per email. Unknown email and wrong password give the same response and take the same time.

## Authorization

* Every `/api/admin/*` handler resolves the context in this order: session, then user (must be active), then active tenant (must be active), then membership (re-read, not cached in the cookie), then role, then the required permission (`tenant.Role.Can`). Anything missing ends the request with `401` or `403`.
* The [permission matrix](domain-model.md#roles-and-permissions) lives in `internal/tenant/roles.go`. A handler asks for a permission, never for a role name.
* A hidden button in the UI is a convenience, not a control. The server checks every call.
* Platform admins are not tenant members by default. A platform action that reads tenant data writes an `audit_events` row with the admin as actor.

## Tenant isolation

Several layers enforce isolation:

1. **Resolution:** the tenant comes from the session (staff) or from an unguessable token (public). It never comes from a client-supplied tenant id. A hostname is only a routing hint.
2. **Repositories:** every method on tenant-owned data takes `tenantID` and puts it in the `WHERE` clause. A lookup that matches in another tenant returns `404`, the same response as an unknown id.
3. **Schema:** composite `(tenant_id, id)` foreign keys. A row cannot reference a parent of another tenant, even through a bug in the application (see `TestSchemaRejectsCrossTenantReferences`).
4. **Side channels:** SSE subscriptions, caches, asset storage keys and background jobs are keyed by tenant.
5. **PostgreSQL row-level security** is planned as defense in depth for the managed cloud (Phase 6). Each transaction would set `servly.tenant_id`, and policies would compare against it. This needs an ADR before implementation.

Required tests for every tenant-owned endpoint: read, update and delete another tenant's record by id (expect `404`); list (expect only own records); reference another tenant's child in a write (expect `404`/`422`).

## Public endpoints

* Table and order tokens carry 128 random bits (`ids.PublicToken`). Malformed tokens are rejected before a database lookup.
* Rate limits apply per IP and per token to menu reads, order submission and status reads. In-process token buckets are enough for the single-process MVP.
* Body limit on `/api/public/*` is 64 KiB. The server enforces the order limits: 50 lines, quantity 99, notes 500 characters.
* Prices are always recomputed. The request has no price fields, and unknown fields are rejected.
* Public responses never contain internal ids, staff names, payment references or other orders.

## Uploaded files

* JPEG, PNG and WebP only. The type is detected from the content, not from the name or the client's header. Maximum size is 5 MiB.
* Files are stored under a server-generated key (`<tenant>/<uuid>`). Client file names never become paths, so path traversal is not possible.
* Served with the stored type, `X-Content-Type-Options: nosniff` and `Content-Disposition: inline`.

## Logging and secrets

* The request log records method, path, status, size, duration and request id. It never records bodies, cookies, query strings, `Authorization` headers or tokens.
* Never logged: passwords, session tokens, CSRF tokens, database URLs with credentials, or payment references.
* `servly config view` redacts passwords in connection URLs.
* Errors that are not `apperr` errors reach the client only as `internal_error` with a request id.
* Secrets come from the environment or a root-readable file, such as `EnvironmentFile=/etc/servly-io/servly.env` with mode `0600`. They never live in Git.

## Payment data

servly records that a payment happened. It never handles card data: there are no fields for card number, expiry or CVC. The free-text `reference` is labelled "terminal receipt number" in the UI. Marking a payment as paid is a staff statement, and servly does not verify it with the terminal.

## Transport

* Hosted deployments run behind HTTPS (see [Cloud deployment](cloud-deployment.md)). The session cookie is `Secure` there.
* A local deployment on the restaurant LAN may use plain HTTP. Anyone on that network can then observe staff sessions. The local deployment guide recommends a separate staff network or TLS through a reverse proxy.
