---
id: 0007-online-payments
title: "0007: Online payments through provider adapters"
---

**Status:** accepted, 2026-10-10

## Context

MASTER_PLAN.md §2.2 kept online payment processing out of the MVP, and §18 requires a decision record before adding it. The MVP is feature-complete. Restaurants in the target markets (Indonesia, Singapore, Malaysia, the EU and the US) expect guests to pay from their phone and staff to charge cards without retyping amounts. Each market has a different everyday method: QRIS, PayNow, DuitNow QR, cards with Apple Pay and Google Pay, iDEAL / Wero, PayPal.

Two things are fixed. servly must not handle card data. And a local install has no public address for webhooks.

## Decision

* **Payments go through providers, behind one adapter interface** (`internal/payments/provider`). One adapter per provider, not per method. The first adapters, in order: **Stripe** (EU, US, SG, MY cards), **Xendit** (ID, also MY and more SEA), **HitPay** (SG and MY national QR codes), **PayPal** (where Stripe cannot offer it, and for platform accounts). Others follow demand. The full design and the per-market ranking are in [Payment integrations](../payments.md).
* **Hosted checkout and server-driven readers only** in the web app. servly stays out of card data (PCI DSS SAQ A). Embedded card fields and Tap to Pay need SDKs and native apps, which are separate later decisions.
* **The restaurant is the merchant.** It connects its own provider account: with its own restricted API keys when self-hosted, or through the provider's hosted platform onboarding on the managed cloud. servly never holds funds.
* **A payment is settled only from the provider's API or a verified webhook**, with amount and currency checked against the order, in one idempotent transaction. A reconciler polls open checkouts, so local installs work without webhooks.
* **The order lifecycle gains one state for prepaid orders:** `awaiting_payment`. It allows `awaiting_payment → new` once the payment is confirmed, and `awaiting_payment → cancelled` when the checkout expires or fails. The kitchen does not see such orders. Orders paid after serving (the default for tables) do not use it.
* **Provider credentials are encrypted at rest** with a key from `SERVLY_SECRET_KEY`. Without that key, online payments cannot be set up.
* **Owners set it up in a wizard** in Settings: country, then a ranked provider, then connect, then methods, then when guests pay, then test and go live. There is no CLI path (ADR 0006).

## Alternatives

* **One aggregator for everything** (for example only Adyen): one integration, but its contracts and minimums do not fit small restaurants, and the local QR methods of Indonesia and Malaysia are better served by local providers.
* **Stripe only:** simplest, but no DuitNow QR in Malaysia, only preview access in Indonesia, and no PayPal for platform accounts. The adapter interface keeps Stripe first without locking servly to it.
* **Embedded card fields on servly's own pages:** a smoother look, but a larger PCI scope (SAQ A-EP), a Content Security Policy for provider scripts, and Apple Pay domain verification, which a LAN install cannot do.
* **Webhooks only:** simpler, but they never reach a local install. Polling is the floor, and webhooks are the optimisation.
* **Creating the prepaid order only after payment:** no new state, but the guest's cart would have to live in the checkout session, and a paid checkout whose order creation failed would need its own recovery. One extra state is simpler.

## Consequences

* New tables: `payment_accounts`, `payment_checkouts`, `payment_events` and `payment_readers`. `payments` gains provider columns. `orders.status` allows `awaiting_payment` on both dialects (a migration changes the CHECK constraint).
* New public endpoints (checkout start, webhooks) with rate limits, plus a background reconciler.
* Markets that add service charges or taxes at the bill (SG, MY, ID, US) need an "order charges" feature before online payment launches there. MASTER_PLAN.md Phase 7.0 lists it.
* Online payments need internet. Manual methods remain the fallback, and the board keeps working offline.

## Compatibility

Existing orders and manual payments are unchanged. `payment_status` keeps its values. Clients that do not know `awaiting_payment` never receive it from staff listings, because the board filters by status, and public order pages show it as "waiting for payment".
