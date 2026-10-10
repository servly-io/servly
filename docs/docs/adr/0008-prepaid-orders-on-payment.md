---
id: 0008-prepaid-orders-on-payment
title: "0008: Prepaid orders are created when the payment is confirmed"
---

**Status:** accepted, 2026-10-10. It replaces the `awaiting_payment` order state of [ADR 0007](0007-online-payments.md). The rest of ADR 0007 stands.

## Context

ADR 0007 planned to add an `awaiting_payment` order state for orders paid when ordering, and to hide those orders from the kitchen. While building it, two problems showed:

* **SQLite cannot change a `CHECK` constraint in place.** Allowing a new value in `orders.status` means rebuilding `orders`. Every order table references `orders` (items, options, status events, payments, checkouts). The safe rebuild needs foreign keys switched off, which a migration running in a transaction cannot do.
* **The state touches more than the kitchen.** Every listing, count, status filter and the order-number sequence would have to skip unpaid orders. Abandoned checkouts would also use up order numbers.

## Decision

* A prepaid checkout stores the **priced cart** (lines, notes, total), not an order. Pricing happens when the guest starts paying, and that total is the amount charged.
* The order is created **in the same transaction that records the confirmed payment**, from that priced cart. It gets the next order number at that moment, starts as `new`, and is already paid.
* A checkout that fails or expires leaves nothing behind except its checkout record.
* The order lifecycle stays as MASTER_PLAN.md §7 defines it. No new state.
* Where a dining location is set to "pay when ordering", `POST /api/public/orders` answers `422 payment_required`, so guests cannot skip the payment.

## Alternatives

* **`awaiting_payment` (ADR 0007):** rejected for the two reasons above.
* **Create the order, then cancel it if unpaid:** the kitchen would see orders that may never be paid, and the history would fill with cancellations.

## Consequences

* The prepaid order is created from a stored price snapshot. A menu change while the guest is paying does not change what they paid for.
* If creating the order fails after a confirmed payment, for example because the database is unavailable, the checkout stays `open`. The reconciler retries until the order exists. The payment can never be recorded twice: a unique index on the provider's payment ID and a unique index on the checkout prevent it.
* Order numbers have no gaps from abandoned payments.
