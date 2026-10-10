// Typed calls of the staff order API (docs/docs/api.md#staff-api-orders).
import { api } from './client';
import type { MinorUnits, OrderDetail, OrderMode, OrderStatus, Page, PaymentMethod, PaymentStatus, StaffOrder } from './types';

export const ordersApi = {
  list: (q: { status?: OrderStatus[]; payment?: PaymentStatus[]; mode?: OrderMode; limit?: number; cursor?: string }) => {
    const p = new URLSearchParams();
    q.status?.forEach((s) => p.append('status', s));
    q.payment?.forEach((s) => p.append('payment_status', s));
    if (q.mode) p.set('mode', q.mode);
    if (q.limit) p.set('limit', String(q.limit));
    if (q.cursor) p.set('cursor', q.cursor);
    return api<Page<StaffOrder>>('GET', `/api/admin/orders?${p}`);
  },
  get: (id: string) => api<OrderDetail>('GET', `/api/admin/orders/${id}`),
  setStatus: (id: string, status: OrderStatus, version: number, reason = '') =>
    api<OrderDetail>('PATCH', `/api/admin/orders/${id}/status`, { body: { status, version, reason } }),
  pay: (id: string, body: { method: PaymentMethod; status: 'paid' | 'pending'; amount_minor: MinorUnits; reference: string }) =>
    api<OrderDetail>('POST', `/api/admin/orders/${id}/payments`, { body }),
  settle: (paymentId: string, status: 'paid' | 'failed') =>
    api<OrderDetail>('PATCH', `/api/admin/payments/${paymentId}`, { body: { status } }),
  refund: (paymentId: string, amount_minor: MinorUnits, reason: string) =>
    api<OrderDetail>('POST', `/api/admin/payments/${paymentId}/refund`, { body: { amount_minor, reason } }),
};
