// Wire types of the API contract (docs/docs/api.md). Keep in step with the
// contract; the server is authoritative for every value here.

/** Integer minor units (cents). */
export type MinorUnits = number;

/** ISO 4217 code, such as "EUR". */
export type Currency = string;

export type OrderStatus = 'new' | 'accepted' | 'preparing' | 'ready' | 'completed' | 'cancelled';

export type PaymentStatus = 'unpaid' | 'pending' | 'paid' | 'failed' | 'partially_refunded' | 'refunded';

export type PaymentMethod = 'cash' | 'external_terminal' | 'other';

export type OrderMode = 'dine_in' | 'pickup';

export type Role = 'owner' | 'manager' | 'staff';

/** The body of every error response. */
export interface ErrorBody {
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
    request_id?: string;
  };
}

/** A page of a collection; pass next_cursor back as ?cursor= for the next one. */
export interface Page<T> {
  items: T[];
  next_cursor: string | null;
}
