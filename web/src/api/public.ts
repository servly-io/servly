// Typed calls of the guest API (docs/docs/api.md#public-api).
import { api } from './client';
import type { CheckoutStart, CheckoutState, OrderRequest, PublicMenu, PublicOrder, Quote } from './types';

export const publicApi = {
  menu: (token: string) => api<PublicMenu>('GET', `/api/public/menu/${encodeURIComponent(token)}`),
  quote: (body: OrderRequest) => api<Quote>('POST', '/api/public/orders/quote', { body }),
  /** Retries with the same key return the same order: never a duplicate. */
  place: (body: OrderRequest, idempotencyKey: string) =>
    api<PublicOrder>('POST', '/api/public/orders', { body, idempotencyKey }),
  order: (token: string) => api<PublicOrder>('GET', `/api/public/orders/${encodeURIComponent(token)}`),
  /** Pay first: the order is created once the provider confirms the payment. */
  prepay: (body: OrderRequest, idempotencyKey: string) =>
    api<CheckoutStart>('POST', '/api/public/checkouts', { body, idempotencyKey }),
  /** Pay an existing order's outstanding amount. */
  payOrder: (orderToken: string) => api<CheckoutStart>('POST', `/api/public/orders/${encodeURIComponent(orderToken)}/checkout`),
  checkout: (token: string) => api<CheckoutState>('GET', `/api/public/checkouts/${encodeURIComponent(token)}`),
};
