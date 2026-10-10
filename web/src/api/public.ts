// Typed calls of the guest API (docs/docs/api.md#public-api).
import { api } from './client';
import type { OrderRequest, PublicMenu, PublicOrder, Quote } from './types';

export const publicApi = {
  menu: (token: string) => api<PublicMenu>('GET', `/api/public/menu/${encodeURIComponent(token)}`),
  quote: (body: OrderRequest) => api<Quote>('POST', '/api/public/orders/quote', { body }),
  /** Retries with the same key return the same order: never a duplicate. */
  place: (body: OrderRequest, idempotencyKey: string) =>
    api<PublicOrder>('POST', '/api/public/orders', { body, idempotencyKey }),
  order: (token: string) => api<PublicOrder>('GET', `/api/public/orders/${encodeURIComponent(token)}`),
};
