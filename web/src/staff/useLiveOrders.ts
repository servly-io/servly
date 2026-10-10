import { useCallback, useEffect, useRef, useState } from 'react';
import { ordersApi } from '../api/staff';
import type { OrderMode, OrderStatus, PaymentStatus, StaffOrder } from '../api/types';

export type Connection = 'connecting' | 'live' | 'reconnecting';

/**
 * The staff board's data: the orders in the given statuses, refetched from
 * the server whenever the SSE stream says something changed, and in full
 * after every (re)connect. Notifications only trigger fetches; the server's
 * answer is the state. Missed or duplicate events are harmless.
 */
export function useLiveOrders(statuses: OrderStatus[], mode: OrderMode | '', payment: PaymentStatus[] = []) {
  const [orders, setOrders] = useState<StaffOrder[] | undefined>(undefined);
  const [error, setError] = useState<unknown>(null);
  const [connection, setConnection] = useState<Connection>('connecting');
  // A stable string, so a new array with the same statuses is the same filter.
  const statusKey = statuses.join(',');
  const paymentKey = payment.join(',');
  const pending = useRef<ReturnType<typeof setTimeout> | null>(null);

  const refetch = useCallback(async () => {
    try {
      const status = statusKey.split(',') as OrderStatus[];
      const page = await ordersApi.list({
        status,
        payment: paymentKey ? (paymentKey.split(',') as PaymentStatus[]) : undefined,
        mode: mode || undefined,
        limit: 200,
      });
      setOrders(page.items);
      setError(null);
    } catch (err) {
      setError(err);
    }
  }, [statusKey, paymentKey, mode]);

  // Coalesce bursts of events into one fetch.
  const soon = useCallback(() => {
    if (pending.current) clearTimeout(pending.current);
    pending.current = setTimeout(() => void refetch(), 250);
  }, [refetch]);

  useEffect(() => {
    void refetch();
    const es = new EventSource('/api/admin/events');
    es.addEventListener('ready', () => {
      setConnection('live');
      void refetch();
    });
    es.addEventListener('resync', soon);
    es.addEventListener('order.created', soon);
    es.addEventListener('order.updated', soon);
    // The browser reconnects by itself; until "ready" arrives again the
    // board may be stale, and staff should see that.
    es.onerror = () => setConnection('reconnecting');
    // A stream the browser gave up on (for example after the session
    // expired) is reopened by reloading, which also re-checks the session.
    const watchdog = setInterval(() => {
      if (es.readyState === EventSource.CLOSED) window.location.reload();
    }, 15_000);
    const online = () => void refetch();
    window.addEventListener('online', online);
    return () => {
      es.close();
      clearInterval(watchdog);
      window.removeEventListener('online', online);
      if (pending.current) clearTimeout(pending.current);
    };
  }, [refetch, soon]);

  return { orders, error, connection, refetch };
}
