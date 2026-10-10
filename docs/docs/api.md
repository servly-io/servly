---
id: api
title: API
---

This page is the HTTP contract between the server, the web app and any other client. MASTER_PLAN.md §9 proposed the starting paths; this page is now authoritative, and [Changes from the plan](#changes-from-the-plan) lists where it departs from §9. Agents implement against this page. Change the page first, in the same change set as the code.

Status: **contract (Phase 0)**. Implemented so far: `/healthz`, `/readyz`, error format, request ids, body limit, [Auth](#auth) with CSRF, session tenant context and login rate limits, [menu](#staff-api-menu) with image upload, [settings and staff](#staff-api-restaurant-settings-and-staff), [dining locations and QR codes](#staff-api-dining-locations-tables-pickup-points), and the [public API](#public-api): menu, idempotent order placement, order status.

## Conventions

* **Base path** `/api`. JSON in and out, `Content-Type: application/json`. Field names are `snake_case`.
* **Ids** are UUID strings. **Public tokens** are 26 lowercase base32 characters.
* **Money** is an integer field ending in `_minor` (minor units), with a `currency` (ISO 4217) on the enclosing object. Floats never appear.
* **Timestamps** are RFC 3339 in UTC with microseconds: `"2026-10-10T18:30:01.123456Z"`.
* **Unknown JSON fields are rejected** (`malformed_json`). Absent optional fields keep their current value on `PATCH`.
* **Body limit:** 1 MiB by default, 64 KiB on `/api/public/*`, 5 MiB on asset upload.
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

Codes defined so far: `route_not_found`, `method_not_allowed`, `malformed_json`, `request_too_large`, `internal_error`, `invalid_currency`, `amount_out_of_range`, `menu_not_found`, `invalid_price`, `invalid_option_group`, `name_required`, `order_not_found`, `order_conflict`, `idempotency_key_reused`, `invalid_transition`, `reason_required`, `forbidden`, `payment_not_found`, `invalid_payment_transition`, `invalid_payment_status`, `invalid_payment_method`, `invalid_payment_amount`, `invalid_refund_amount`, `unauthenticated`, `invalid_credentials`, `csrf_failed`, `no_active_tenant`, `rate_limited`, `not_found`, `slug_taken`, `email_taken`, `already_member`, `invalid_slug`, `invalid_email`, `invalid_timezone`, `invalid_role`, `last_owner`, `currency_fixed`, `password_too_short`, `password_too_long`, `text_too_long`, `too_many_options`, `category_required`, `existing_user`, `shared_user`, `invalid_kind`, `file_required`, `unsupported_media_type`, `asset_too_large`, `idempotency_key_required`, `empty_order`, `too_many_lines`, `invalid_quantity`, `item_unavailable`, `option_unavailable`, `invalid_option_selection`, `notes_too_long`, `notes_disabled`, `negative_line_price`, `invalid_cursor`. A new code goes into this list in the same change.

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
  "restaurant": { "name": "Trattoria Roma", "currency": "EUR", "order_notes_enabled": true },
  "dining_location": { "name": "Table 4", "kind": "table" },
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

Query: `status` (repeatable), `mode`, `limit`, `cursor`. Newest first. Each item is an [order](#order-object).

### `GET /api/admin/orders/{id}`

Returns an [order](#order-object) with `events` (status history) and `payments`.

### `PATCH /api/admin/orders/{id}/status`

Permission `orders.update_status`, or `orders.cancel_started` to cancel a `preparing` or `ready` order.

```json
{ "status": "accepted", "version": 3, "reason": "" }
```

`200` returns the updated order. Errors: `422 invalid_transition` or `reason_required`, `403 forbidden`, `409 order_conflict` (refetch and retry if still intended).

### `POST /api/admin/orders/{id}/payments`

Permission `payments.record`.

```json
{ "method": "external_terminal", "status": "paid", "amount_minor": 2500, "reference": "TID 4411 #0193" }
```

`201` returns the payment and the order's new `payment_status`. The amount must equal the outstanding amount. `reference` is free text of at most 100 characters. It must never contain card data, and the UI says so.

### `PATCH /api/admin/payments/{id}`

Permission `payments.record`. It resolves a pending payment.

```json
{ "status": "paid" }
```

### `POST /api/admin/payments/{id}/refund`

Permission `payments.refund`.

```json
{ "amount_minor": 500, "reason": "cold pizza" }
```

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

Event names: `order.created`, `order.updated` (status or payment change). A keepalive comment is sent every 25 s. Events are hints: on any event, fetch the order. On (re)connect, after `ready`, refetch the board. There is no replay and no `Last-Event-ID` support.

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
| `PATCH /api/admin/settings` - `{name?, currency?, timezone?, order_notes_enabled?, public_status_enabled?}`; the currency is fixed once orders exist (`409 currency_fixed`) | `settings.manage` |
| `GET /api/admin/staff` - `{items: [{user_id, email, display_name, role, status}]}` | `staff.manage` |
| `POST /api/admin/staff` - `{email, display_name, role, password}`; for a person who already has a servly sign-in (another restaurant), leave `password` out: sending one is `422 existing_user` | `staff.manage` |
| `PATCH /api/admin/staff/{user_id}` - `{role?, password?}`; a password can be set only for people who work for this restaurant alone (`422 shared_user`), and it ends their sessions | `staff.manage` |
| `DELETE /api/admin/staff/{user_id}` - `204`; removes the membership, keeps the sign-in for other restaurants | `staff.manage` |

The last active owner can be neither demoted nor removed (`409 last_owner`).

## Bootstrap

A fresh install has no users. The first tenant and owner are created on the server host with the CLI, not over HTTP. This means no unauthenticated setup endpoint exists to race for:

```bash
servly tenant create --name "Trattoria Roma" --currency EUR --timezone Europe/Berlin --owner-email anna@example.com
# prompts for the owner's password
```

The guided setup in the admin UI starts after that first sign-in.

## Changes from the plan

| MASTER_PLAN.md §9 | This contract | Why |
|---|---|---|
| `/api/admin/locations`, `…/{id}/qr` | `/api/admin/dining-locations`, `…/{id}/qr` | "location" is the restaurant (`restaurant_locations`); tables and pickup points are dining locations |
| - | `PUT /api/auth/session/tenant` | explicit tenant context for users with several memberships (ADR 0004) |
| - | `PATCH /api/admin/payments/{id}`, `POST …/refund` | the payment lifecycle needs them |
| - | `/api/admin/settings`, `/api/admin/staff`, `/api/admin/assets`, `…/duplicate`, `…/rotate-token`, `/api/public/assets/{id}` | MVP capabilities of §2.1 |
| - | `servly tenant create` | first-owner bootstrap without a public setup endpoint |
