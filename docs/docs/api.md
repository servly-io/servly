---
id: api
title: API
---

This page is the HTTP contract between the server, the web app and any other client. MASTER_PLAN.md §9 proposed the starting paths; this page is now authoritative, and [Changes from the plan](#changes-from-the-plan) lists where it departs from §9. Agents implement against this page. Change the page first, in the same change set as the code.

Status: **implemented contract**. Implemented: `/healthz`, `/readyz`, error format, request ids, body limit, [Auth](#auth) with CSRF, session tenant context and login rate limits, [menu](#staff-api-menu) with image upload, [settings and staff](#staff-api-restaurant-settings-and-staff), [dining locations and QR codes](#staff-api-dining-locations-tables-pickup-points), the [public API](#public-api): menu, quote, idempotent order placement, order status, and the [staff order API](#staff-api-orders) with payments and live events. Everything in this contract is implemented.

## Conventions

* **Base path** `/api`. JSON in and out, `Content-Type: application/json`. Field names are `snake_case`.
* **Ids** are UUID strings. **Public tokens** are 26 lowercase base32 characters.
* **Money** is an integer field ending in `_minor` (minor units), with a `currency` (ISO 4217) on the enclosing object. Floats never appear.
* **Timestamps** are RFC 3339 in UTC with microseconds: `"2026-10-10T18:30:01.123456Z"`.
* **Unknown JSON fields are rejected** (`malformed_json`). Absent optional fields keep their current value on `PATCH`.
* **Body limit:** 1 MiB by default, 64 KiB on `/api/public/*`, the image limit plus 64 KiB on asset upload (1 MB images by default, `uploads.max_image_bytes`).
* **Request id:** every response has `X-Request-Id`, and error bodies repeat it. A proxy may send a well-formed `X-Request-Id` (≤ 64 characters of `[A-Za-z0-9_-]`), and the server keeps it.
* **Pagination:** collections that can grow take `?limit=` (default 50, max 200) and `?cursor=`, and return:

  ```json
  { "items": [ ... ], "next_cursor": "opaque-or-null" }
  ```

## Errors

Every error has one shape:

```json
{
  "error": {
    "code": "invalid_transition",
    "message": "order status change not allowed",
    "details": { "from": "completed", "to": "accepted" },
    "request_id": "9f1c2e…"
  }
}
```

| HTTP | Kind | When |
|---|---|---|
| 401 | unauthenticated | no or expired session (`unauthenticated`) |
| 403 | forbidden | role lacks the permission (`forbidden`), CSRF check failed (`csrf_failed`) |
| 404 | not found | absent, or belongs to another tenant: the two are indistinguishable |
| 409 | conflict | stale `version` (`order_conflict`), `idempotency_key_reused`, duplicate name/slug |
| 413 | invalid | `request_too_large` |
| 422 | invalid | validation and rule failures |
| 429 | - | `rate_limited`, with `Retry-After` |
| 500 | - | `internal_error`; details are logged, never returned |

Codes defined so far: `route_not_found`, `method_not_allowed`, `malformed_json`, `request_too_large`, `internal_error`, `invalid_currency`, `amount_out_of_range`, `menu_not_found`, `invalid_price`, `invalid_option_group`, `name_required`, `order_not_found`, `order_conflict`, `idempotency_key_reused`, `invalid_transition`, `reason_required`, `forbidden`, `payment_not_found`, `invalid_payment_transition`, `invalid_payment_status`, `invalid_payment_method`, `invalid_payment_amount`, `invalid_refund_amount`, `unauthenticated`, `invalid_credentials`, `csrf_failed`, `no_active_tenant`, `rate_limited`, `not_found`, `slug_taken`, `email_taken`, `already_member`, `invalid_slug`, `invalid_email`, `invalid_timezone`, `invalid_role`, `last_owner`, `currency_fixed`, `password_too_short`, `password_too_long`, `text_too_long`, `too_many_options`, `category_required`, `existing_user`, `shared_user`, `invalid_kind`, `file_required`, `unsupported_media_type`, `asset_too_large`, `idempotency_key_required`, `empty_order`, `too_many_lines`, `invalid_quantity`, `item_unavailable`, `option_unavailable`, `invalid_option_selection`, `notes_too_long`, `notes_disabled`, `negative_line_price`, `invalid_cursor`, `payment_conflict`, `reference_too_long`, `reference_looks_like_card`. A new code goes into this list in the same change.

## Authentication and tenant context

* Staff sign in with email and password. The server sets the `servly_session` cookie: random 256-bit token, `HttpOnly`, `SameSite=Lax`, `Path=/`, and `Secure` behind HTTPS. Only the SHA-256 of the token is stored.
* Every state-changing request (`POST`, `PUT`, `PATCH`, `DELETE`) under `/api/auth` and `/api/admin` must send `X-CSRF-Token` with the value from `GET /api/auth/session`. `POST /api/auth/login` is exempt.
* The session holds the **active tenant**. Every `/api/admin/*` call acts in that tenant, after the server checks the membership again. A user with one membership has it selected at login. A user with several memberships selects one with `PUT /api/auth/session/tenant`. A tenant id sent in a body or query is never trusted.
* MVP: one location per tenant, and `/api/admin/*` acts on it. Multiple locations will add a location selector to the session.
* Public endpoints need no session. The dining-location token or order token in the path is the only access check.

## Health

| | |
|---|---|
| `GET /healthz` | liveness: `200 {"status":"ok","version":"…"}` whenever the process serves HTTP |
| `GET /readyz` | readiness: `200 {"status":"ok"}` when the database answers and the schema is current, else `503 {"status":"unavailable"}` |

## Auth

### `POST /api/auth/login`

```json
{ "email": "anna@example.com", "password": "…" }
```

`200` returns the session (same body as `GET /api/auth/session`) and sets the cookie. A wrong email and a wrong password give the same `401 invalid_credentials`. Attempts are rate-limited per IP and per email.

### `POST /api/auth/logout`

`204`. Deletes the session and clears the cookie.

### `GET /api/auth/session`

```json
{
  "user": { "id": "…", "email": "anna@example.com", "display_name": "Anna", "is_platform_admin": false },
  "csrf_token": "…",
  "active_tenant": { "id": "…", "name": "Trattoria Roma", "role": "manager", "permissions": ["orders.view", "…"] },
  "memberships": [ { "tenant_id": "…", "tenant_name": "Trattoria Roma", "role": "manager" } ],
  "expires_at": "…"
}
```

`401` without a session. `permissions` let the UI hide controls it may not use. The server still checks every call.

### `PUT /api/auth/session/tenant`

```json
{ "tenant_id": "…" }
```

`200` returns the updated session. `404` when the user has no membership in that tenant.

## Public API

### `GET /api/public/menu/{token}`

`token` is a dining location's public token. The response contains only enabled categories and items that are enabled and not archived. Sold-out items are included with `available: false`.

```json
{
  "restaurant": { "name": "Trattoria Roma", "currency": "EUR", "order_notes_enabled": true, "accepting_orders": true },
  "dining_location": { "name": "Table 4", "kind": "table" },
  "payment": "after",
  "categories": [
    {
      "id": "…", "name": "Pizza", "description": "",
      "items": [
        {
          "id": "…", "name": "Margherita", "description": "Tomato, mozzarella",
          "price_minor": 950, "image_url": "/api/public/assets/…", "available": true,
          "option_groups": [
            { "id": "…", "name": "Size", "min_select": 1, "max_select": 1,
              "options": [ { "id": "…", "name": "Large", "price_delta_minor": 300, "available": true } ] }
          ]
        }
      ]
    }
  ]
}
```

`404 menu_not_found` for an unknown or disabled token, a suspended tenant, or a closed location.

`payment` says how guests here can pay online (see [Online payments](#online-payments)): `none`, `before` (pay when ordering: use `POST /api/public/checkouts`), `after` (order, then pay from the order page), or `either` (the guest chooses).

### `POST /api/public/orders`

Header `Idempotency-Key` is required. See [idempotent order creation](domain-model.md#idempotent-order-creation).

```json
{
  "dining_location_token": "…",
  "notes": "",
  "items": [
    { "menu_item_id": "…", "quantity": 2, "option_ids": ["…"], "notes": "no basil" }
  ]
}
```

The order's `mode` follows the dining location: a table gives `dine_in`, a pickup point gives `pickup`.

Refused with `422 not_accepting_orders` while the restaurant has ordering switched off (the public menu shows `"accepting_orders": false`), and with `409 too_many_waiting_orders` when 3 orders already wait at this table, or 60 at the restaurant, for staff to accept them. Both apply to `POST /api/public/checkouts` too.

`201` creates the order. A replay with the same key and body returns `200` with the same order. The body:

```json
{
  "order_token": "…",
  "number": 17,
  "status": "new",
  "payment_status": "unpaid",
  "mode": "dine_in",
  "dining_location": "Table 4",
  "currency": "EUR",
  "subtotal_minor": 2500,
  "total_minor": 2500,
  "items": [
    { "name": "Margherita", "quantity": 2, "unit_price_minor": 950, "options_total_minor": 300,
      "line_total_minor": 2500, "notes": "no basil",
      "options": [ { "group_name": "Size", "option_name": "Large", "price_delta_minor": 300 } ] }
  ],
  "notes": "",
  "created_at": "…"
}
```

`422` for any rule in [Pricing](domain-model.md#pricing). `details` names the offending line index and option. Client-sent prices are not accepted, because there is no field for them. Rate-limited per IP (10 at once, then 1 per 6 s) and per dining location (30, then 1 per 2 s).

### `POST /api/public/orders/quote`

The same body as order placement, without `Idempotency-Key`. Prices the cart against the live menu and returns `{mode, dining_location, currency, subtotal_minor, total_minor, items}` as order placement would; nothing is stored. The same `422` errors apply. The cart review page uses it to show server-confirmed totals.

### `GET /api/public/orders/{orderToken}`

Returns the same body as order creation. It contains no internal ids, no staff identities and no payment details beyond `payment_status`. `404` if the location disabled public status, or the token is unknown. Rate-limited per IP.

### `GET /api/public/assets/{id}`

Serves an image that a non-archived menu item uses; an upload no item uses is `404`. Served with the stored `Content-Type`, `X-Content-Type-Options: nosniff`, `Content-Disposition: inline` and a sandboxing `Content-Security-Policy`.

## Staff API: orders

Permission `orders.view` unless stated otherwise.

### `GET /api/admin/orders`

Query: `status` (repeatable), `payment_status` (repeatable: `unpaid`, `pending`, `paid`, `partially_refunded`, `refunded`), `mode`, `limit`, `cursor`. Newest first. Each item is an [order](#order-object). The board's "To pay" view asks for `payment_status=unpaid&payment_status=pending` in every status except `cancelled`.

### `GET /api/admin/orders/{id}`

Returns an [order](#order-object) with `outstanding_minor`, `events` (status history: `{from, to, actor: {id, name} | null, reason, created_at}`; the first event has `from: null` and no actor) and `payments` (`{id, method, status, amount_minor, refunded_minor, currency, reference, recorded_by, created_at}`).

### `PATCH /api/admin/orders/{id}/status`

Permission `orders.update_status`, or `orders.cancel_started` to cancel a `preparing` or `ready` order.

```json
{ "status": "accepted", "version": 3, "reason": "" }
```

`200` returns the order detail. Errors: `422 invalid_transition` or `reason_required`, `403 forbidden`, `409 order_conflict` when `version` is not the current one (refetch and retry if still intended).

### `POST /api/admin/orders/{id}/payments`

Permission `payments.record`.

```json
{ "method": "external_terminal", "status": "paid", "amount_minor": 2500, "reference": "TID 4411 #0193" }
```

`200` returns the order detail with the new `payment_status`. `status` is `paid` or `pending` (waiting for the terminal). The amount must equal the outstanding amount (`422 invalid_payment_amount`). `reference` is free text of at most 100 characters; anything shaped like a card number (13-19 digits passing the Luhn check) is refused with `422 reference_looks_like_card`.

### `PATCH /api/admin/payments/{id}`

Permission `payments.record`. It resolves a pending payment as `paid` or `failed`; returns the order detail.

```json
{ "status": "paid" }
```

### `POST /api/admin/payments/{id}/refund`

Permission `payments.refund`.

```json
{ "amount_minor": 500, "reason": "cold pizza" }
```

Returns the order detail. The reason is required. For an online payment the refund goes through the provider first. If the provider refuses, nothing is recorded, and the error says why: for example `404 payments_not_connected` when the account was disconnected.

### `GET /api/admin/events`

Server-Sent Events for the active tenant and location. Permission `orders.view`.

```
event: ready
data: {}

event: order.created
data: {"order_id":"…","version":1}

event: order.updated
data: {"order_id":"…","version":4}

: keepalive
```

Event names: `order.created`, `order.updated` (status or payment change), and `resync` when this connection missed events (a slow client). A keepalive comment is sent every 25 s, and `retry: 3000` sets the reconnect delay. Events are hints: on any event, fetch the order. On (re)connect, after `ready`, and on `resync`, refetch the board. There is no replay and no `Last-Event-ID` support. The stream ends when the server shuts down; clients reconnect.

### Order object

```json
{
  "id": "…", "number": 17, "business_date": "2026-10-10", "mode": "dine_in",
  "dining_location": { "id": "…", "name": "Table 4" },
  "status": "preparing", "payment_status": "paid", "version": 4,
  "currency": "EUR", "subtotal_minor": 2500, "total_minor": 2500, "notes": "",
  "items": [ "…as in the public order…" ],
  "created_at": "…", "updated_at": "…"
}
```

## Staff API: menu

Permission `menu.manage` for writes. Every member may read.

| | |
|---|---|
| `GET /api/admin/menu/categories` | all categories, by `position` |
| `POST /api/admin/menu/categories` | `{name, description?, is_enabled?}`; appended at the end |
| `PATCH /api/admin/menu/categories/{id}` | `{name?, description?, is_enabled?, position?}`; setting `position` moves it and renumbers the others |
| `GET /api/admin/menu/items` | `?category_id=`; items with option groups |
| `POST /api/admin/menu/items` | item with nested `option_groups[].options[]` |
| `GET /api/admin/menu/items/{id}` | one item with its option groups |
| `PATCH /api/admin/menu/items/{id}` | partial; when present, `option_groups` is the complete new list: a group or option with its `id` is updated and keeps the id, one without `id` is added, one left out is removed (archived, as past orders may reference it). An `id` from another item is `404` |
| `POST /api/admin/menu/items/{id}/duplicate` | copy named "… (copy)", disabled |
| `DELETE /api/admin/menu/items/{id}` | `204`; archives instead when an order references the item, and `200 {"archived": true}` says so |
| `POST /api/admin/assets` | `multipart/form-data` field `file`; JPEG, PNG or WebP up to 5 MiB, the type detected from the content; returns `{id, url, admin_url, content_type, size_bytes}`. Set `image_asset_id` on an item to show it; `null` removes it |
| `GET /api/admin/assets/{id}` | the image, attached or not, for previews in the admin UI |

Item body:

```json
{
  "category_id": "…", "name": "Margherita", "description": "…", "price_minor": 950,
  "image_asset_id": null, "is_enabled": true, "is_available": true, "position": 3,
  "option_groups": [
    { "name": "Size", "min_select": 1, "max_select": 1,
      "options": [ { "name": "Large", "price_delta_minor": 300, "is_available": true } ] }
  ]
}
```

## Staff API: dining locations (tables, pickup points)

Permission `dining_locations.manage` for writes.

| | |
|---|---|
| `GET /api/admin/dining-locations` | all; each `{id, name, kind, public_token, menu_url, is_enabled}` |
| `POST /api/admin/dining-locations` | `{name, kind: "table"\|"pickup"}`; the server generates `public_token` |
| `PATCH /api/admin/dining-locations/{id}` | `{name?, is_enabled?}` |
| `POST /api/admin/dining-locations/{id}/rotate-token` | new token; old QR codes stop working |
| `GET /api/admin/dining-locations/{id}/qr` | `image/svg+xml` QR code of `menu_url` = `<public base URL>/m/<token>`, where the base is `http.public_base_url` or else the request's scheme and host; `?format=png` for PNG |

## Staff API: restaurant settings and staff

| | Permission |
|---|---|
| `GET /api/admin/settings` | any member |
| `PATCH /api/admin/settings` - `{name?, currency?, timezone?, order_notes_enabled?, public_status_enabled?, accepting_orders?}`; `GET` also returns the read-only `max_image_bytes`; the currency is fixed once orders exist (`409 currency_fixed`) | `settings.manage` |
| `GET /api/admin/staff` - `{items: [{user_id, email, display_name, role, status}]}` | `staff.manage` |
| `POST /api/admin/staff` - `{email, display_name, role, password}`; for a person who already has a servly sign-in (another restaurant), leave `password` out: sending one is `422 existing_user` | `staff.manage` |
| `PATCH /api/admin/staff/{user_id}` - `{role?, password?}`; a password can be set only for people who work for this restaurant alone (`422 shared_user`), and it ends their sessions | `staff.manage` |
| `DELETE /api/admin/staff/{user_id}` - `204`; removes the membership, keeps the sign-in for other restaurants | `staff.manage` |

The last active owner can be neither demoted nor removed (`409 last_owner`).

## First-run setup

A fresh installation has no users. Its owner sets it up in the browser at `/setup`, with the setup token that `servly serve` logs on start. From then on, the installation counts as set up ([ADR 0005](adr/0005-first-run-setup.md)).

On a managed platform, the operator provisions tenants with `servly tenant create` instead. This is platform administration, not part of the API ([ADR 0006](adr/0006-cli-scope.md)). The first provisioned tenant also closes the browser setup.

### `GET /api/setup`

`200 {"required": true|false}`. `required` stays true until the first restaurant exists. No session is needed.

### `POST /api/setup/verify`

`{"setup_token": "…"}`. Checks the token without creating anything, so the wizard can reject a wrong one on its first step. It answers `204` when the token is right, `403 setup_token_invalid` when it is wrong, `409 setup_complete` once the installation is set up, and `429` beyond the per-IP sign-in limit. The token is checked again by `POST /api/setup`.

### `POST /api/setup`

```json
{
  "setup_token": "2133e-b5bc8-97896-1be3c",
  "restaurant_name": "Trattoria Roma",
  "currency": "EUR",
  "timezone": "Europe/Berlin",
  "owner_name": "Anna Rossi",
  "owner_email": "anna@example.com",
  "owner_password": "at least 10 characters"
}
```

The request creates the tenant, its location and the owner, and signs the owner in. It answers `201` with the session body of `GET /api/auth/session` and sets the session cookie. No session or CSRF token is needed. The setup token authorizes the request. Case and whitespace in the token are ignored.

| Error | When |
|---|---|
| `403 setup_token_invalid` | wrong token, or the server has no token (it started with migrations pending) |
| `409 setup_complete` | the installation is already set up; this is final |
| `409 email_taken` | the owner email already has a sign-in |
| `422` validation codes | `name_required`, `invalid_slug`, `invalid_currency`, `invalid_timezone`, `invalid_email`, `password_too_short`, `password_too_long` |
| `429 rate_limited` | the per-IP sign-in limit applies |

## Online payments

Through payment providers ([Payment integrations](payments.md), ADR 0007, ADR 0008). All need the operator's `secret_key`; without it `GET /api/admin/payments` reports `"available": false`, and connecting fails with `422 secret_key_missing`.

### `GET /api/admin/payments`

Permission `settings.manage`. It returns:
- `available`;
- `sandbox`, whether the fake provider is offered;
- `markets`, the catalog: per country or region, the methods guests use most and the providers, each with `recommended` and `available` (servly can connect it today);
- `account`, the connected account or `null`.

### `PUT /api/admin/payments/account`

Permission `settings.manage`. Connects, or replaces, the location's account:

```json
{ "provider": "stripe", "secret_key": "rk_test_…", "webhook_secret": "whsec_…", "table_flow": "after", "pickup_flow": "before" }
```

servly verifies the key with the provider (`422 payment_credentials_invalid`), seals it, and returns the account:

```json
{ "provider": "stripe", "mode": "test", "account_name": "Roma GmbH", "account_ref": "acct_…", "country": "DE",
  "table_flow": "after", "pickup_flow": "before", "webhooks": true,
  "webhook_url": "https://…/api/payments/webhooks/stripe/<account id>", "updated_at": "…" }
```

The key is never returned. `table_flow` is `after`, `before` or `staff`. `pickup_flow` is `before`, `either` or `counter`. `409 payments_in_progress` while guests of the current account are still paying.

### `PATCH /api/admin/payments/account`

Any of `table_flow`, `pickup_flow`, `webhook_secret`.

### `DELETE /api/admin/payments/account`

`204`. The credentials are forgotten; payment records stay. `409 payments_in_progress` while checkouts are open (at most about 30 minutes).

### `POST /api/public/checkouts`

Pay first. The body and `Idempotency-Key` header are as for `POST /api/public/orders`. It answers `200 {"checkout_token": "…", "redirect_url": "https://checkout.stripe.com/…"}`. The same key returns the same checkout. No order exists until the payment is confirmed; then the order is created as `new`, already paid. `422 online_payment_not_offered` where the dining location does not take online payment first.

Where a dining location's flow is `before`, `POST /api/public/orders` answers `422 payment_required`.

### `POST /api/public/orders/{orderToken}/checkout`

Pay an existing order's outstanding amount. It answers like the endpoint above. Tapping twice returns the same open checkout. Errors: `422 nothing_to_pay`, `422 online_payment_not_offered`. The public order shows `"pay_online": true` while this is possible.

### `GET /api/public/checkouts/{token}`

The page the provider sends the guest back to (`/pay/{token}`) polls this:

```json
{ "status": "paid", "kind": "prepay", "order_token": "…", "redirect_url": "…" }
```

`status` is `open`, `paid`, `failed` or `expired`. While `open`, each call (at most every 2 s per checkout) also asks the provider, so the guest does not wait for the reconciler.

### `POST /api/payments/webhooks/{provider}/{accountId}`

For the provider, not for servly's frontend: there is no session or CSRF token. The body is verified with the account's webhook signing secret. A wrong or old signature, an unknown account, or a provider mismatch answers `422 payment_webhook_invalid`, and nothing about the account is revealed. Each event is applied once, however often it is delivered. Stripe events: `checkout.session.completed`, `…async_payment_succeeded`, `…async_payment_failed`, `…expired`.

### Payment object additions

Order detail payments carry `"method": "online"`, `"provider": "stripe"` and `"detail": "Apple Pay · Visa •••• 4242"` for online payments. `recorded_by` is `null` for them.

## Changes from the plan

| MASTER_PLAN.md §9 | This contract | Why |
|---|---|---|
| `/api/admin/locations`, `…/{id}/qr` | `/api/admin/dining-locations`, `…/{id}/qr` | "location" is the restaurant (`restaurant_locations`); tables and pickup points are dining locations |
| - | `PUT /api/auth/session/tenant` | explicit tenant context for users with several memberships (ADR 0004) |
| - | `PATCH /api/admin/payments/{id}`, `POST …/refund` | the payment lifecycle needs them |
| - | `/api/admin/settings`, `/api/admin/staff`, `/api/admin/assets`, `…/duplicate`, `…/rotate-token`, `/api/public/assets/{id}` | MVP capabilities of §2.1 |
| - | `GET /api/setup`, `POST /api/setup` | first-run setup in the browser (ADR 0005); the CLI provisions tenants only on a managed platform (ADR 0006) |
| - | `/api/admin/payments…`, `/api/public/checkouts…`, `/api/payments/webhooks/…` | online payments (MASTER_PLAN.md §8.1, Phase 7) |
