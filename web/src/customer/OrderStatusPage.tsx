import { useEffect, useState } from 'react';
import { useLocation, useParams } from 'react-router';
import { ApiError } from '../api/client';
import { publicApi } from '../api/public';
import type { OrderStatus, PublicOrder } from '../api/types';
import { formatMoney } from '../shared/money';
import { ErrorBanner, Spinner } from '../shared/ui';
import { recallOrder } from './cart';

const statusText: Record<OrderStatus, string> = {
  new: 'Received: waiting for the staff to accept it.',
  accepted: 'Accepted.',
  preparing: 'Being prepared.',
  ready: 'Ready!',
  completed: 'Completed. Enjoy!',
  cancelled: 'Cancelled. Please ask the staff.',
};

const steps: OrderStatus[] = ['new', 'accepted', 'preparing', 'ready', 'completed'];
const POLL_MS = 15_000;

/** Confirmation and live status of a guest's order: /o/:token. */
export function OrderStatusPage() {
  const { token = '' } = useParams();
  const placed = (useLocation().state as { placed?: boolean } | null)?.placed;
  const [order, setOrder] = useState<PublicOrder | null>(() => recallOrder(token));
  const [statusHidden, setStatusHidden] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const o = await publicApi.order(token);
        if (!live) return;
        setOrder(o);
        setError(null);
        if (o.status === 'completed' || o.status === 'cancelled') return;
      } catch (err) {
        if (!live) return;
        if (err instanceof ApiError && err.status === 404) {
          setStatusHidden(true);
          return;
        }
        setError(err);
      }
      timer = setTimeout(() => void poll(), POLL_MS);
    };
    void poll();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [token]);

  if (!order) {
    if (statusHidden) return <main className="page page-narrow"><h1>Order not found</h1><p>Please ask the staff.</p></main>;
    return <main className="page"><Spinner /></main>;
  }
  const current = steps.indexOf(order.status);

  return (
    <main className="page page-narrow guest stack">
      {placed && <p className="banner banner-ok" role="status">Thank you! Your order was sent.</p>}
      <h1>Order {order.number}</h1>
      <p className="muted">{order.dining_location} · {new Date(order.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</p>
      {statusHidden ? (
        <p>Show this number to the staff if you have questions.</p>
      ) : (
        <>
          <p className="status-text" aria-live="polite">{statusText[order.status]}</p>
          {order.status !== 'cancelled' && (
            <ol className="progress" aria-label="Progress">
              {steps.slice(0, 4).map((s, i) => <li key={s} className={i <= current ? 'reached' : ''}>{s}</li>)}
            </ol>
          )}
        </>
      )}
      <ErrorBanner error={error} />
      <ul className="list-plain stack-tight">
        {order.items.map((l, i) => (
          <li key={i} className="row">
            <span className="row-grow">{l.quantity} × {l.name}{l.options.length > 0 && <span className="muted small"> ({l.options.map((o) => o.option_name).join(', ')})</span>}</span>
            <span>{formatMoney(l.line_total_minor, order.currency)}</span>
          </li>
        ))}
      </ul>
      <p className="total row"><span className="row-grow">Total</span><strong>{formatMoney(order.total_minor, order.currency)}</strong></p>
    </main>
  );
}
