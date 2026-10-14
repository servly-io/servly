# servly - Master Plan for Agentic Development

**Document:** `MASTER_PLAN.md`
**Status:** Authoritative implementation specification
**Product:** servly, a lightweight Restaurant Order Management System (OMS)
**Primary objective:** Deliver a simple, low-maintenance restaurant ordering system that can run locally or as a centrally managed, multi-tenant cloud service.

---

## 1. Mission and product vision

Build a lightweight order management system for restaurants.

A restaurant owner must be able to configure their business and menu without technical knowledge. Customers must be able to open the menu from a QR code, select products, customize their order, and submit it. Restaurant staff must be able to process orders from an Android device, tablet, or desktop browser.

The system must support two deployment models using the same application codebase:

1. **Self-hosted:** A restaurant operates its own instance, potentially without internet connectivity.
2. **Managed cloud:** The platform operator hosts and manages restaurant tenants on shared or dedicated infrastructure.

The product must be designed for agentic implementation: clear boundaries, explicit contracts, testable acceptance criteria, limited coupling, and independently verifiable work.

### Guiding principles

1. Simplicity over feature count.
2. One application codebase and one business-domain model.
3. Local operation must not require a cloud account or cloud control plane.
4. Cloud hosting must not require a separate fork of the application.
5. Tenant isolation is a security requirement, not an optional feature.
6. The server is authoritative for pricing, order state, authorization, and payment state.
7. Persistent business data must survive restarts and upgrades.
8. Avoid distributed systems unless a demonstrated requirement justifies them.
9. Prefer boring, maintained technology with a small operational footprint.
10. Every implemented feature must include tests and documentation appropriate to its risk.
11. Restaurant business happens in the web application; the command line is for operating the installation (§4.5).

---

## 2. Product scope

### 2.1 MVP capabilities

#### Restaurant administration

* Set up a fresh installation in the browser: claim it, create the owner account and the first restaurant.
* Configure restaurant name, currency, timezone, and basic settings.
* Create, edit, reorder, enable, and disable menu categories.
* Create, edit, duplicate, and disable menu items.
* Configure descriptions, prices, and optional images.
* Configure product options and extras with price adjustments.
* Mark items temporarily unavailable.
* Create tables or pickup endpoints.
* Generate QR codes for public menu entry points.
* Manage staff users and basic roles.
* Preview the customer-facing menu.

#### Customer ordering

* Open the menu in a normal mobile browser.
* Browse categories and products.
* View product descriptions and prices.
* Select permitted options and quantities.
* Add order notes where enabled.
* Review the cart and server-confirmed pricing.
* Submit an order without creating a customer account.
* Receive a confirmation and a customer-safe order reference.
* View limited order status where enabled.

#### Staff operations

* View new and active orders in a live order board.
* Inspect order items, options, quantities, and notes.
* Accept, prepare, mark ready, and complete orders.
* Cancel orders through controlled transitions.
* Record payments made through external terminals or cash.
* Filter orders by status and order mode.
* Identify connectivity failures and recover by reloading persisted state.

#### Operations

* Support SQLite for local deployments.
* Support PostgreSQL for managed cloud deployments.
* Provide database migrations and health endpoints.
* Persist uploaded assets.
* Support automated backups and documented restoration.
* Run as one application process in a straightforward deployment.
* Serve frontend assets from the Go application.
* Provide container and/or systemd deployment instructions.

### 2.2 Explicitly out of scope for the initial MVP

Do not implement these unless the product owner explicitly changes this document:

* Native Android, iOS, Windows, or macOS applications. (A Tap to Pay counter app is considered in Phase 7.6 and needs its own decision record.)
* Online payment processing in the MVP. It follows as Phase 7 (§8.1, ADR 0007).
* Fiscal cash-register replacement or TSE integration.
* Inventory, purchasing, and supplier management.
* Delivery fleet management.
* Loyalty programs and customer accounts.
* Subscription billing.
* Advanced analytics.
* Complex kitchen station routing.
* Distributed microservices.
* Kubernetes-based deployment as a prerequisite.
* Mandatory external authentication or cloud dependencies.
* Mandatory internet connectivity for local operations.

Design extension points where appropriate, but do not build speculative infrastructure.

---

## 3. Technology decisions

These choices are authoritative unless a documented technical blocker is approved.

### Backend

* Language: Go.
* HTTP: standard `net/http` and a lightweight router such as `chi`.
* API: REST and JSON.
* Database access: `database/sql`, explicit SQL, and optionally `sqlc`.
* Migrations: versioned SQL migrations.
* Logging: structured `log/slog`.
* Live updates: Server-Sent Events (SSE).
* Authentication: secure, server-managed sessions for staff.
* Testing: standard Go testing, HTTP integration tests, and database integration tests.

### Frontend

* TypeScript.
* React.
* Vite.
* Responsive web application with PWA support.
* One codebase for customer, staff, and administration experiences.
* Accessible forms, large operational buttons, and mobile-friendly layouts.

Use a small, maintained UI component library if useful. Do not introduce a large frontend framework stack without a concrete need.

### Persistence

* SQLite for single-instance local deployments.
* PostgreSQL for managed shared-tenancy deployments.
* UUID identifiers for internal entities.
* Integer minor units for monetary amounts.
* Explicit repository interfaces and transaction boundaries.
* Separate migration sets where database syntax or semantics differ.

Do not assume that SQLite and PostgreSQL are interchangeable. Both implementations must satisfy the same application-level contracts and behavior.

### Packaging

* One Go application process.
* Frontend assets embedded or packaged with the server.
* Persistent data directory for local deployments.
* Environment-based configuration.
* Container deployment for reproducible installation.
* Optional systemd deployment for minimal Linux installations.

Avoid Redis, message brokers, separate frontend servers, and separate database services for local deployments unless a demonstrated requirement warrants them.

---

## 4. Architectural boundaries

Use a modular monolith. Modules represent business and infrastructure boundaries, not independently deployed services.

Suggested repository layout:

```
servly/
├── cmd/
│   └── servly/            one binary: `serve`, `migrate`, `version` (ADR 0002)
├── internal/
│   ├── auth/
│   ├── tenant/
│   ├── menu/
│   ├── orders/
│   ├── payments/
│   ├── money/
│   ├── ids/
│   ├── storage/
│   ├── notifications/
│   ├── platform/
│   └── httpapi/
├── web/
│   ├── src/
│   │   ├── customer/
│   │   ├── staff/
│   │   ├── admin/
│   │   ├── shared/
│   │   └── api/
│   └── package.json
├── migrations/
│   ├── sqlite/
│   └── postgres/
├── deploy/
│   └── systemd/
├── docs/                  Docusaurus site; pages live in docs/docs/ (ADR 0003)
├── Containerfile
├── Makefile
├── MASTER_PLAN.md
└── README.md
```

Agents may adjust the layout to suit implementation details, but must preserve the architectural boundaries below.

### 4.1 Domain layer

Contains business rules and domain types. It must not depend on HTTP, React, a particular database driver, or deployment infrastructure.

Responsibilities include:

* Menu validation.
* Price calculations.
* Order creation rules.
* Order status transitions.
* Payment state transitions.
* Tenant-scoped authorization rules.
* Validation of product options and quantities.

### 4.2 Application and persistence layer

Coordinates use cases, database transactions, repositories, and external interfaces.

Responsibilities include:

* Creating orders atomically.
* Enforcing idempotency.
* Persisting state changes.
* Retrieving tenant-scoped records.
* Storing audit events.
* Managing migrations and database-specific implementation details.

### 4.3 HTTP/API layer

Handles:

* Routing.
* Authentication and session handling.
* Request validation.
* Tenant resolution.
* Authorization.
* JSON serialization.
* Error responses.
* SSE connections.
* Rate limits and request-size limits.

Do not place business logic directly in HTTP handlers.

### 4.4 Frontend

Consumes documented API contracts. It must not calculate authoritative totals, determine authorization, or independently enforce order state transitions.

The UI can calculate provisional cart totals for convenience, but the server must recompute and validate all prices before accepting an order.

### 4.5 Command-line interface

The `servly` command line serves the operator of an installation, never the restaurant's business.

The CLI may:

* Run the server (`serve`).
* Manage the schema (`migrate`).
* Show and check configuration (`config`, `version`).
* Back up and restore data.
* Recover access, such as resetting a password when nobody can sign in (`user set-password`).
* Provision and suspend tenants on a managed platform (`tenant`). This is platform administration, not restaurant setup.

The CLI must not:

* Be a required step for a restaurant owner. A fresh installation is set up in the browser (§12, first-run setup).
* Configure menus, tables, staff, settings, orders or payments. These belong to the web application and its API.

Business rules stay in the application layer for both interfaces. A CLI command calls the same use cases as the API and adds no business logic of its own.

---

## 5. Tenancy and deployment architecture

Tenancy must be supported from the beginning, even when the initial local deployment contains only one tenant.

### 5.1 Deployment profiles

**Profile A - Local**

* One restaurant tenant.
* SQLite.
* Local asset storage.
* No required external services.
* No required platform account.
* Core operations work without internet connectivity.
* One configured local tenant.

**Profile B - Managed shared cloud**

* Multiple tenants.
* PostgreSQL.
* Tenant-aware access control.
* HTTPS.
* Centralized monitoring, upgrades, and backups.
* Tenant-aware public domains or routing.
* Strict isolation of restaurant data.

**Profile C - Dedicated managed deployment**

* One or more explicitly assigned tenants.
* Dedicated application and/or database resources.
* Operated by the platform provider.
* Same application artifact and domain logic.

Profile C is an architectural target, not an MVP deliverable.

### 5.2 Tenant model

Introduce at least:

* `Tenant`
* `User`
* `TenantMembership`
* `RestaurantLocation`
* `TenantDomain`

A tenant represents the customer/business account. A restaurant location represents a physical operating location. The MVP may limit each tenant to one location.

All tenant-owned records must be attributable to a tenant, directly or through a clearly defined parent relationship.

Tenant scope must apply to:

* Menu categories and items.
* Tables and public ordering tokens.
* Orders and order items.
* Payments and payment records.
* Tenant users and memberships.
* Uploaded assets.
* Settings.
* Audit records.
* Exports and reports.

### 5.3 Tenant resolution

For staff endpoints:

1. Authenticate the user.
2. Load the user's permitted tenant memberships.
3. Resolve the requested tenant context.
4. Verify that the user is authorized for that tenant.
5. Scope every operation to that tenant.

For public endpoints:

1. Resolve the public restaurant through a configured domain or unguessable public token.
2. Allow only explicitly public operations.
3. Avoid exposing internal identifiers or private customer data.
4. Never trust a client-supplied tenant ID as proof of access.

A hostname is a routing hint, not an authorization mechanism.

### 5.4 Tenant isolation

* Add tenant-aware indexes and uniqueness constraints.
* Use repository APIs that make tenant scope explicit.
* Never use unrestricted tenant-owned lookups in application handlers.
* Add PostgreSQL row-level security where appropriate as defense in depth.
* Test cross-tenant read, write, delete, and enumeration attempts.
* Scope caches, background tasks, uploaded files, and SSE events by tenant.
* Audit privileged cross-tenant platform access.

The same application-level authorization contract must hold for SQLite and PostgreSQL.

### 5.5 Control plane

A platform control plane may later manage:

* Tenant provisioning and suspension.
* Deployment assignment.
* Domains and TLS configuration.
* Application versions.
* Backup policies.
* Health and operational status.

The control plane is not required to serve local restaurant orders. Local operation must remain independent of the control plane.

Do not implement a Kubernetes operator or distributed provisioning system in the MVP.

---

## 6. Core data model

The initial domain model includes:

### Tenant and access

* `tenants`
* `users`
* `tenant_memberships`
* `restaurant_locations`
* `tenant_domains`

### Menu

* `menu_categories`
* `menu_items`
* `option_groups`
* `options`

### Ordering

* `dining_locations`
* `orders`
* `order_items`
* `order_item_options`
* `order_status_events`

### Payments

* `payments`
* `payment_accounts`, `payment_checkouts`, `payment_events` (Phase 7.0), `payment_readers` (Phase 7.4)

### Operations

* `audit_events`
* `idempotency_records`, or an equivalent durable idempotency mechanism

Exact columns and relationships must be defined in the schema design deliverable before feature implementation begins.

### Data invariants

1. Internal identifiers are stable and collision-resistant.
2. Monetary amounts use integer minor units.
3. Currency is explicit.
4. Order numbers are unique within the appropriate tenant/location/business-date scope.
5. Order items retain historical names, options, and prices.
6. Editing a menu item does not mutate historical orders.
7. Order creation and its line items are committed atomically.
8. Tenant-owned queries enforce tenant scope.
9. Duplicate order submissions cannot create duplicate orders.
10. Deletion must not silently erase completed financial or operational history.

For money, do not use binary floating-point types. Use an explicit currency and a documented rounding policy.

---

## 7. Order lifecycle

Use an explicit state machine.

Initial order states:

* `new`
* `accepted`
* `preparing`
* `ready`
* `completed`
* `cancelled`

Expected normal transition:

```
new -> accepted -> preparing -> ready -> completed
```

Cancellation is allowed only through defined transitions and authorization rules.

Requirements:

* Reject invalid state transitions.
* Record actor, timestamp, old state, and new state.
* Persist state changes before broadcasting notifications.
* Do not silently delete an order that staff has begun processing.
* Handle simultaneous staff actions safely.
* Provide a defined correction or cancellation workflow.
* Keep payment status separate from order status.

Do not assume that every restaurant needs every intermediate state. However, the MVP must have one documented, predictable workflow.

Orders that are paid when ordering do not add a state (ADR 0008, which replaces ADR 0007's `awaiting_payment`). The checkout stores the priced cart, and the order is created as `new` in the same transaction that records the confirmed payment. The kitchen never sees an unpaid prepaid order. A failed or expired checkout leaves no order behind.

---

## 8. Payment architecture

Payment integration is out of scope for the MVP, but payment concepts must exist in the domain model.

Initial payment methods:

* `cash`
* `external_terminal`
* `other`

Initial payment statuses:

* `unpaid`
* `pending`
* `paid`
* `failed`
* `partially_refunded`
* `refunded`

Requirements:

* Record payment method, amount, currency, status, timestamps, and actor where relevant.
* Permit payment to be recorded independently of order completion.
* Prevent invalid payment-state transitions.
* Preserve an audit trail of manual changes.
* Do not store card numbers, security codes, or sensitive payment credentials.
* Do not pretend that manual marking verifies an external terminal transaction.

Fiscal and cash-register compliance must be assessed separately before positioning the product as a replacement for a POS system.

### 8.1 Online payments (Phase 7)

Online payments go through established payment providers, following their recommended integrations. servly does not build a payment stack. The decision is ADR 0007, and the full design is `docs/docs/payments.md`. This section is the binding summary.

**Principles**

1. Hosted checkout pages and server-driven card readers only. servly never handles card data (PCI DSS SAQ A).
2. A payment is settled only from the provider's API or a verified webhook, with amount and currency checked against the order, in one idempotent transaction. A guest returning from checkout proves nothing.
3. A reconciler polls open checkouts, so local installs without a public address work. Webhooks make results faster where they can arrive.
4. One adapter per provider (`internal/payments/provider`), not per payment method. The adapter translates only; rules, idempotency and state changes stay in the application layer. Each adapter owns its minor-unit conversion per currency.
5. The restaurant is the merchant and connects its own provider account. servly never holds funds.
6. Manual payment methods remain available, also offline.

**What to support first, by market** (most widely used methods first)

| Market | Methods, in order | Recommended provider | Alternatives |
|---|---|---|---|
| Indonesia | QRIS, e-wallets (GoPay, OVO, DANA, ShopeePay), virtual accounts, cards | Xendit | Midtrans |
| Singapore | cards with Apple Pay and Google Pay, PayNow, GrabPay | Stripe | HitPay, Adyen |
| Malaysia | DuitNow QR, e-wallets (Touch 'n Go, GrabPay, ShopeePay, Boost), cards, FPX | HitPay | Stripe (no DuitNow), Fiuu, Xendit |
| EU | cards with Apple Pay and Google Pay, iDEAL / Wero, Bancontact, BLIK, PayPal | Stripe | Mollie, Adyen; SumUp for readers |
| US | cards with Apple Pay and Google Pay, tips, PayPal and Venmo, Cash App Pay | Stripe | Square, PayPal |

Adapters are built in this order: **Stripe, Xendit and Midtrans, HitPay, PayPal**. Mollie, Adyen, Square, SumUp and Fiuu follow demand.

**Connecting.** Self-hosted installs connect with the restaurant's own restricted API keys. The managed cloud uses the providers' hosted platform onboarding: Stripe Connect Standard with Account Links, Xendit xenPlatform, PayPal partner onboarding. Provider credentials are encrypted at rest (AES-256-GCM, key from `SERVLY_SECRET_KEY`), write-only, and never logged.

**Setup wizard** (Settings → Payments), one decision per step, resumable:
1. country, suggested from the currency and timezone;
2. a provider, ranked for that country, with the recommended one first;
3. connect: test mode first;
4. payment methods the account can accept, with the recommended ones on;
5. when guests pay, for tables and for pickup points, and tips where the locale expects them;
6. optional card readers and in-person QR;
7. a test payment and the go-live checklist.

**Guest flows.**
* **Tables:** pay from the phone when finished (default).
* **Pickup:** pay when ordering (default once online payments work). The order is created once the payment is confirmed (ADR 0008).
* On phones, QR methods use the provider's app-switch flow. The checkout page uses the guest's language.

**Staff flows.**
* "Charge on reader": server-driven readers such as Stripe smart readers and SumUp Solo through its Cloud API.
* "Show payment QR": a dynamic QRIS, PayNow or DuitNow code on the staff screen.
* Refunds through the provider.
* Tap to Pay on phones requires a native app with the provider's SDK. It is Phase 7.6 and needs its own decision record.

**Dependencies and limits.**
* Markets that add charges at the bill (service charge and GST in SG, service charge and service tax in MY, service charge and PB1/PBJT in ID, sales tax and tips in the US) need the order-charges feature (Phase 7.0) before online payments launch there.
* Online payments need internet; the board and manual payments do not.
* Fiscal obligations (TSE, RKSV, e-invoicing) remain outside servly.

---

## 9. API contracts

Use REST and JSON. Prefix application endpoints with `/api`.

Initial endpoint contract:

### Public API

* `GET /api/public/menu/{token}`
* `POST /api/public/orders`
* `GET /api/public/orders/{publicToken}`

### Staff API

* `GET /api/admin/orders`
* `GET /api/admin/orders/{id}`
* `PATCH /api/admin/orders/{id}/status`
* `POST /api/admin/orders/{id}/payments`
* `GET /api/admin/events`

### Menu administration

* `GET /api/admin/menu/categories`
* `POST /api/admin/menu/categories`
* `PATCH /api/admin/menu/categories/{id}`
* `GET /api/admin/menu/items`
* `POST /api/admin/menu/items`
* `PATCH /api/admin/menu/items/{id}`
* `DELETE /api/admin/menu/items/{id}`, subject to historical reference rules

### Location administration

* `GET /api/admin/locations`
* `POST /api/admin/locations`
* `GET /api/admin/locations/{id}/qr`

### Payments (Phase 7)

* `GET /api/admin/payments`, `PUT`/`PATCH`/`DELETE /api/admin/payments/account`
* `POST /api/public/checkouts`, `POST /api/public/orders/{orderToken}/checkout`, `GET /api/public/checkouts/{token}`
* `POST /api/payments/webhooks/{provider}/{accountId}`
* Later: `POST /api/admin/orders/{id}/payments/reader` (7.4), `POST /api/admin/orders/{id}/payments/qr` (7.2)

### Access and health

* `GET /api/setup`
* `POST /api/setup`
* `POST /api/auth/login`
* `POST /api/auth/logout`
* `GET /api/auth/session`
* `GET /healthz`
* `GET /readyz`

These paths are the proposed starting contract. Agents must document any required changes before implementing incompatible variants.

### API requirements

* Use consistent JSON error structures.
* Validate all incoming data.
* Use pagination for potentially large collections.
* Enforce authentication and tenant authorization.
* Require idempotency for order creation.
* Avoid exposing database implementation details.
* Ensure public order references are unguessable.
* Version the API if breaking changes become necessary.
* Keep OpenAPI documentation or an equivalent contract artifact current.

Do not expose sensitive internal error messages to customers.

---

## 10. Frontend requirements

Use a single responsive React/TypeScript application with distinct customer, staff, and administration routes.

### Customer interface

* Fast loading.
* Mobile-first layout.
* Clear category navigation.
* Accessible product options.
* Persistent cart during normal navigation.
* Clear price and order summary.
* Submission progress and recoverable errors.
* Confirmation after successful order creation.
* No mandatory customer account.

### Staff interface

* Large, touch-friendly controls.
* Readable order cards.
* Prominent new-order indicators.
* Clear status transitions.
* Visible connectivity state.
* Automatic updates through SSE.
* Refetch current state after reconnection.
* No dependence on notifications arriving exactly once.

### Administration interface

* A first-run setup wizard for a fresh installation: owner account, restaurant, ordering options, review.
* A launch checklist on the admin home page until the restaurant is ready: menu, tables and QR codes, staff, test order.
* Straightforward category and item editing.
* Simple price and availability controls.
* Product duplication.
* Reordering through accessible drag-and-drop or equivalent controls.
* QR generation and printable output.
* Clear validation errors and unsaved-change handling.
* Phase 7: a payments setup wizard that ranks providers and methods for the restaurant's country (§8.1).

### Frontend quality

* Use shared API client types.
* Avoid duplicating backend business rules.
* Support common mobile and desktop browser sizes.
* Provide keyboard-accessible controls.
* Include loading, empty, error, and offline states.
* Do not introduce native app projects for the MVP.

---

## 11. Real-time updates

Use SSE for staff-facing live order updates.

Required behavior:

1. An order transaction commits successfully.
2. The application publishes a corresponding event.
3. Authorized staff clients receive the event.
4. The UI updates the order board.
5. If the connection drops, the UI reconnects and fetches authoritative state.

SSE is a notification mechanism, not the system of record.

The database is authoritative. Agents must not build a message broker, distributed event bus, or event-sourcing framework for this feature.

For the MVP, a process-local event broadcaster is sufficient. It must not be treated as a durable queue.

---

## 12. Security requirements

Security must be considered in every implementation phase.

### Authentication

* Store password hashes using a suitable, maintained password-hashing implementation.
* Use secure session cookies.
* Apply appropriate cookie flags, including `HttpOnly` and `SameSite`.
* Use `Secure` cookies in HTTPS deployments.
* Implement logout and session expiration.
* Protect state-changing requests against CSRF when cookie authentication is used.
* Rate-limit authentication attempts.

### First-run setup

* The browser setup is available only while the installation is not set up.
* Claiming requires a one-time setup token that only the operator can read, such as from the server log.
* The claim is atomic: exactly one claim succeeds, also under concurrent requests.
* The installation state is stored in the database. It is not inferred from whether users exist.
* After setup, the setup endpoint is closed permanently. Recovery uses authenticated settings or a deliberate operator procedure.
* Never ship default administrator passwords.

### Payments (Phase 7)

* Never accept, log, or store card numbers or security codes; use hosted pages and server-driven readers.
* Encrypt provider credentials at rest and never return them in full.
* Verify webhook signatures and timestamps, deduplicate events by provider event ID, and check amount and currency against the order.
* Rate-limit and size-limit checkout and webhook endpoints.
* Scope every payment account, checkout and event to its tenant. A webhook must match the account its path names.
* Audit connects, disconnects, method changes, going live, refunds, and reader pairing.

### Authorization

* Separate platform administration, tenant ownership/management, and ordinary staff roles.
* Check permissions on the server for every protected operation.
* Enforce tenant scope in all tenant-owned data access.
* Avoid trusting hidden frontend controls as access control.

### Public ordering

* Use unguessable public tokens.
* Rate-limit order submission and public status lookups.
* Validate quantity limits and option selections.
* Recompute prices and totals server-side.
* Enforce request-size limits.
* Do not reveal unrelated orders or private customer information.

### Infrastructure

* Keep secrets out of source control.
* Use least-privilege database credentials.
* Avoid exposing admin endpoints unnecessarily.
* Use HTTPS for hosted deployments.
* Restrict uploaded file types and sizes.
* Avoid path traversal in asset storage.
* Do not log passwords, session secrets, or sensitive payment data.

Security-sensitive changes require explicit tests.

---

## 13. Local-first reliability and operations

Local deployment must work without an internet connection after installation.

Required capabilities:

* Application starts after a host reboot.
* Orders survive application restarts.
* Database migrations are versioned and repeatable.
* Health endpoints are available.
* Logs are structured and useful.
* Errors are visible to operators.
* Database backups are supported.
* Restore procedures are documented and tested.
* Persistent assets are included in backup and restore planning.
* Application upgrades preserve existing business data.

SQLite backup must use a safe mechanism, such as the SQLite backup API or a correctly managed snapshot. Do not assume copying an actively written database file is always safe.

Define retention and storage requirements for backups. Do not silently overwrite the only valid backup during an upgrade.

Cloud deployments must additionally document:

* PostgreSQL backup and restore.
* Tenant isolation.
* TLS and domain management.
* Health monitoring.
* Resource limits.
* Upgrade and rollback procedures.
* Operational access controls.

---

## 14. Agentic development operating model

The coordinating agent is the technical lead and owns consistency across the project.

Agents are contributors, not independent architects. They must follow this master plan.

### 14.1 Required agent roles

**Agent 1 - Architecture and contracts**

Owns:

* Domain model.
* Database schema.
* Repository interfaces.
* API contract.
* Architecture decision records.
* Cross-agent consistency.

This agent produces the initial contracts before parallel feature development.

**Agent 2 - Backend foundation**

Owns:

* Go application bootstrap.
* Configuration.
* Logging.
* Database initialization.
* Migrations.
* Health endpoints.
* Persistence infrastructure.
* Deployment integration.

**Agent 3 - Menu and administration**

Owns:

* Menu domain functionality.
* Menu persistence.
* Administration endpoints.
* Restaurant setup UI.
* Category and product editing.

**Agent 4 - Ordering and staff operations**

Owns:

* Order creation and validation.
* Idempotency.
* State machine.
* Staff endpoints.
* SSE updates.
* Staff order board.

**Agent 5 - Customer experience**

Owns:

* Public menu UI.
* Cart.
* Product options.
* Order submission.
* Confirmation and limited public order status.

**Agent 6 - Quality and operations**

Owns:

* Cross-module tests.
* Tenant-isolation tests.
* End-to-end scenarios.
* Backup and restore tests.
* Container validation.
* Deployment documentation.
* Security regression checks.

For a small team, combine roles as appropriate. The responsibilities must still be covered.

### 14.2 Work ownership

Every task must have:

* A single accountable owner.
* Explicit inputs and outputs.
* Dependencies.
* Acceptance criteria.
* Files or modules it may modify.
* Tests it must add or update.

Avoid assigning two agents overlapping ownership of the same files.

If parallel agents cannot work independently because a contract is missing, stop and establish the contract rather than allowing them to invent separate interfaces.

### 14.3 Contract-first parallelism

The following artifacts must exist before major parallel implementation begins:

1. Domain model.
2. Database schema and migration conventions.
3. Repository interface definitions.
4. API request and response contracts.
5. Authentication and tenant-scope rules.
6. Order and payment state machines.
7. Frontend route and API client conventions.

Agents must implement against these contracts.

Changes to shared contracts require a documented decision and coordinated updates to dependent work.

### 14.4 Agent completion report

Every agent must report:

* What was implemented.
* Files or modules changed.
* Tests executed and their results.
* Known limitations.
* Contract changes, if any.
* Follow-up tasks.
* Whether the work is ready for integration.

Never accept "implemented" as evidence that code compiles, tests pass, or the feature works.

---

## 15. Execution plan and phase gates

### Phase 0 - Repository reconnaissance and architecture

Tasks:

* Inspect the repository and existing conventions.
* Identify existing code and reusable components.
* Confirm build tooling and supported Go version.
* Produce the domain model.
* Produce the database schema.
* Define the persistence interfaces.
* Define API contracts and error format.
* Define tenant isolation rules.
* Document architecture decisions.

Exit criteria:

* The architecture is documented.
* Core types and interfaces are agreed upon.
* Migration strategy is established for SQLite and PostgreSQL.
* No major unresolved design decision blocks implementation.

Do not overwrite or replace existing project work before inspecting it.

### Phase 1 - Foundation

Tasks:

* Initialize Go application.
* Initialize frontend.
* Add configuration and logging.
* Implement database connection and migration handling.
* Implement health endpoints.
* Add local deployment configuration.
* Add PostgreSQL integration configuration.
* Establish automated test commands.

Exit criteria:

* Application starts using the documented configuration.
* Both database backends can apply their initial migrations.
* Health checks behave correctly.
* CI or equivalent automated checks run successfully.

### Phase 2 - Tenant, access, and menu

Tasks:

* Implement tenant and user models.
* Implement authentication and memberships.
* Implement authorization middleware.
* Implement menu categories, items, options, and pricing.
* Implement administration UI.
* Add tenant isolation tests.

Exit criteria:

* An owner can create and edit a menu.
* Unauthorized users cannot access administration.
* Tenant-owned data cannot be accessed across tenants.
* Menu changes persist across restarts.

### Phase 3 - Customer ordering

Tasks:

* Implement public menu resolution.
* Implement cart and option selection.
* Implement server-side price calculation.
* Implement atomic order creation.
* Implement durable idempotency.
* Implement public confirmation and status access.

Exit criteria:

* A customer can place an order from a mobile browser.
* Prices and totals are computed and validated server-side.
* Repeated submissions with the same idempotency key do not create duplicate orders.
* Orders and line items persist atomically.
* Public tokens do not reveal unrelated orders.

### Phase 4 - Staff operations

Tasks:

* Implement the order state machine.
* Implement staff order listing and status changes.
* Implement the order board.
* Implement SSE updates and reconnect recovery.
* Implement manual payment recording.
* Add audit events for important state changes.

Exit criteria:

* Staff can process orders through the documented lifecycle.
* Invalid state transitions are rejected.
* Two simultaneous staff actions do not corrupt order state.
* Disconnected clients recover the current state.
* Payment state remains independent of order status.

### Phase 5 - Deployment and hardening

Tasks:

* Complete container or binary packaging.
* Add backup and restore procedures.
* Add deployment and upgrade documentation.
* Run security and tenant-isolation tests.
* Test clean installation and upgrade paths.
* Validate common Android and desktop browsers.
* Test the core workflow with internet access disabled.

Exit criteria:

* A clean installation can be deployed from documentation.
* Committed orders survive restart.
* Backups can be restored successfully.
* Tenant-isolation tests pass.
* The end-to-end MVP acceptance checklist passes.

### Phase 6 - Managed cloud pilot

This phase follows a successful local MVP.

Tasks:

* Deploy PostgreSQL-backed application instances.
* Configure HTTPS and tenant domains.
* Validate tenant routing and authorization.
* Establish centralized monitoring.
* Establish backup, restoration, and upgrade procedures.
* Decide how tenants are provisioned and suspended.
* Test shared deployment and dedicated deployment configurations.

Exit criteria:

* Multiple test tenants can operate concurrently.
* Cross-tenant access attempts are denied.
* Tenant-specific backups and operational recovery are documented.
* Deployments can be upgraded without losing business data.

Do not add subscription billing or automated provisioning until the operational model is proven.

### Phase 7 - Payment integrations

Design: §8.1, ADR 0007, `docs/docs/payments.md`. Each step ships on its own and is usable on its own.

**7.0 - Contracts and prerequisites** (done, except order charges)

* Done: the adapter interface, the market catalog, the data model (`payment_accounts`, `payment_checkouts`, `payment_events`, provider columns on `payments`), and credential encryption (`secret_key`).
* Done: prepaid orders created on payment (ADR 0008).
* Done: the reconciler (polling with backoff, restart-safe, safe with two processes), and the sandbox provider for demos and tests.
* Open: order charges (service charge and taxes per location), required before SG, MY, ID and US launches.

**7.1 - Stripe, own keys** (done)

* The wizard: country, provider, connect, when guests pay. Stripe's dynamic payment methods are managed in the Stripe dashboard.
* Tables pay when finished; pickup pays when ordering; or the guest chooses.
* Webhooks and polling; refunds through the provider.
* Markets: EU, US and SG; MY for cards, FPX and GrabPay.
* Still to prove with a real Stripe test account: one end-to-end payment per listed method. The adapter is tested against recorded responses only.

**7.2 - Indonesia: Xendit and Midtrans** (done, except the staff-screen QR)

* Done: Xendit through Payment Sessions (its hosted checkout, which replaced the Invoice API), and Midtrans through Snap, both with own keys. QRIS, Indonesian e-wallets, virtual accounts and cards, as the restaurant enables them in the provider's dashboard. Webhooks: Xendit's verification token, Midtrans' signature_key. Refunds through the provider.
* Open: dynamic QR on the staff screen ("Show payment QR").
* Rupiah only for now; Xendit for MY follows with HitPay (7.3).
* Still to prove with real sandbox accounts: one payment per method.

**7.3 - HitPay**

* DuitNow QR, FPX and Malaysian e-wallets; PayNow.
* Markets: MY and SG.

**7.4 - Card readers**

* Server-driven readers: Stripe smart readers and SumUp Solo through its Cloud API. Wizard step 6.

**7.5 - Managed cloud and PayPal**

* Platform connect: Stripe Connect Standard with Account Links, Xendit xenPlatform, PayPal partner onboarding.
* The PayPal adapter (PayPal, and Venmo in the US).

**7.6 - Later, each with its own decision**

* A native "servly Counter" app for Tap to Pay on iPhone and Android.
* Tips beyond presets, split bills, and further adapters (Mollie, Adyen, Square, Fiuu) as restaurants ask for them.

Exit criteria (for every step that adds an adapter):

* A test payment completes end to end, with the provider's sandbox, for every method the step lists.
* A paid order is marked paid exactly once, whether the webhook, the reconciler or both deliver the result, and also after a restart in between.
* A forged or replayed webhook, a wrong amount, a wrong currency, and another tenant's account are all rejected.
* An unpaid prepay checkout never reaches the kitchen, and a declined or expired one leaves no order.
* A full and a partial refund through the provider update the order's payment status.
* No card data or credentials appear in logs, API responses, or exports.
* A local install without a public address settles payments by polling alone.

---

## 16. Testing strategy

Every phase must add automated tests at the appropriate level.

### Unit tests

* Price calculations and rounding.
* Option validation.
* Order state transitions.
* Payment state transitions.
* Authorization decisions.
* Tenant resolution.

### Integration tests

* Migrations against SQLite and PostgreSQL.
* Repository operations.
* Atomic order creation.
* Idempotency under retries.
* Concurrent order status changes.
* Authentication and session behavior.
* Tenant-scoped CRUD operations.
* SSE reconnect and state refresh.

### End-to-end tests

1. Complete the first-run setup in the browser: owner and restaurant.
2. Configure a menu.
3. Create a table and public QR token.
4. Submit a customer order.
5. Observe it on the staff board.
6. Accept and prepare the order.
7. Mark it ready and complete.
8. Record an external payment.
9. Restart the application.
10. Verify that the order and payment history remain intact.

### Security tests

* Unauthenticated administration requests.
* Unauthorized tenant access.
* Cross-tenant ID substitution.
* Public order enumeration.
* Invalid prices and options.
* Oversized requests.
* Duplicate order submission.
* Invalid state transitions.
* Session and CSRF behavior.
* Asset access and path traversal.

### Payment tests (Phase 7)

* Adapter contract tests against recorded provider responses, per adapter.
* Minor-unit conversion per adapter and currency, including IDR and zero-decimal currencies.
* Webhook signature, timestamp tolerance, and deduplication.
* Webhook and reconciler racing on the same checkout.
* A declined or expired prepay checkout leaves no order; a paid one creates exactly one.
* Sandbox end-to-end runs, kept out of the default test run behind an environment variable.

### Reliability tests

* Database unavailable at startup.
* Database failure during an operation.
* Restart after a committed order.
* Interrupted SSE connection.
* Failed migration.
* Backup restoration on a clean instance.

Tests must verify outcomes, not implementation details alone.

---

## 17. CI and definition of done

Provide documented commands for:

* Backend formatting.
* Backend linting.
* Backend unit tests.
* Backend integration tests.
* Frontend linting.
* Frontend type checking.
* Frontend build.
* Migration tests.
* End-to-end tests.

A pull request or agent task is complete only when:

1. The change meets its acceptance criteria.
2. Relevant tests have been added or updated.
3. Formatting and static checks pass.
4. No unrelated modules or contracts were changed unnecessarily.
5. Documentation reflects relevant behavior.
6. Tenant isolation has been preserved.
7. The change does not introduce an undocumented runtime dependency.
8. Known limitations are reported.

Do not claim tests passed unless they were actually run and the results were observed. If the environment prevents a test from running, report the limitation explicitly.

---

## 18. Scope control and architectural decision process

The coordinating agent may make local implementation decisions within this document.

The following require an explicit decision record before implementation:

* Changing the primary framework or database strategy.
* Introducing a new external service.
* Adding a distributed component.
* Changing tenant boundaries.
* Changing authentication architecture.
* Adding online payment processing.
* Changing the order lifecycle.
* Introducing a new deployment prerequisite.
* Making cloud services mandatory for local operation.
* Adding a feature that materially expands the MVP.

Each decision record should contain:

* Context.
* Decision.
* Alternatives considered.
* Consequences.
* Migration or compatibility implications.

When requirements are ambiguous, prefer the smallest implementation that preserves the stated architecture and leaves a clean extension point. Do not invent large features based on assumptions.

---

## 19. Required documentation

The repository must contain:

* `MASTER_PLAN.md` - this specification.
* `README.md` - project overview and quick start.
* `docs/docs/architecture.md` - component and dependency boundaries.
* `docs/docs/domain-model.md` - entities, relationships, and database schema.
* `docs/docs/api.md` - API contracts.
* `docs/docs/security.md` - authentication, authorization, tenant isolation, and threat considerations.
* `docs/docs/local-deployment.md` - self-hosted setup and recovery.
* `docs/docs/cloud-deployment.md` - managed deployment architecture.
* `docs/docs/backup-restore.md` - backup, restore, and data-loss considerations.
* `docs/docs/payments.md` - payment providers, markets, flows, and security (Phase 7).
* `docs/docs/adr/` - architecture decision records.

The documentation is a Docusaurus site (`docs/`), published to GitHub Pages. See ADR 0003.

Keep documentation close to the implementation. Update it when behavior or contracts change.

---

## 20. First assignment to the coordinating agent

Do not begin by generating the entire application in one pass.

Execute these steps in order:

1. Inspect the repository, existing code, available tooling, and current tests.
2. Report what already exists and what must be added.
3. Create or update `MASTER_PLAN.md` with this document.
4. Produce the domain model and database schema.
5. Define repository interfaces and API contracts.
6. Establish configuration for local SQLite and hosted PostgreSQL.
7. Implement the smallest working vertical slice: create a tenant, configure a menu, submit an order, and retrieve it through the staff interface.
8. Run the tests and validate persistence.
9. Only then delegate independent menu, customer, and staff features to implementation agents.
10. Integrate and verify every phase against its exit criteria.

The coordinating agent is responsible for resolving interface conflicts, reviewing security-sensitive changes, keeping the master plan authoritative, and ensuring that parallel contributions form one coherent application.

---

## 21. Final acceptance objective

The MVP succeeds when a restaurant owner can install the application, set it up in the browser without a command line, configure a menu, print a QR code, accept customer orders, process them from a tablet or PC, and retain a reliable history of orders and payments.

The same codebase must be capable of running in a managed cloud environment with multiple isolated restaurant tenants, without changing the fundamental ordering workflow.

The application should be understandable, maintainable, and operable by a small engineering team. Its simplicity is a product requirement, not merely an implementation preference.
