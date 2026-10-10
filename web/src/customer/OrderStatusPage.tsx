import { useEffect, useState } from 'react';
import { Link, useLocation, useParams } from 'react-router';
import { ApiError } from '../api/client';
import { publicApi } from '../api/public';
import type { OrderStatus, PublicOrder } from '../api/types';
import { formatMoney } from '../shared/money';
import { Icon, type IconName } from '../shared/icons';
import { EmptyState, ErrorBanner, Spinner } from '../shared/ui';
import { recallMenuToken, recallOrder } from './cart';

const statusView: Record<OrderStatus, { title: string; text: string; icon: IconName }> = {
  new: { title: 'Order sent', text: 'Waiting for the restaurant to accept it.', icon: 'clock' },
  accepted: { title: 'Order accepted', text: 'The kitchen has your order and starts soon.', icon: 'check' },
  preparing: { title: 'Being prepared', text: 'Your food is being made right now.', icon: 'flame' },
  ready: { title: 'Ready!', text: 'Your order is ready.', icon: 'bag' },
  completed: { title: 'Enjoy your meal', text: 'This order is complete. Thank you!', icon: 'checkCircle' },
  cancelled: { title: 'Order cancelled', text: 'Please ask the staff.', icon: 'xCircle' },
};

const steps: OrderStatus[] = ['new', 'accepted', 'preparing', 'ready', 'completed'];
const stepLabels = ['Sent', 'Accepted', 'Preparing', 'Ready'];
const POLL_MS = 15_000;

/** Confirmation and live status of a guest's order: /o/:token. */
export function OrderStatusPage() {
  const { token = '' } = useParams();
  const placed = (useLocation().state as { placed?: boolean } | null)?.placed;
  const [order, setOrder] = useState<PublicOrder | null>(() => recallOrder(token));
  const [statusHidden, setStatusHidden] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [menuToken] = useState(() => recallMenuToken(token));

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
    if (statusHidden) {
      return (
        <main className="auth">
          <div className="auth-card"><EmptyState icon="receipt" title="Order not found">Please ask the staff.</EmptyState></div>
        </main>
      );
    }
    return <main className="guest"><Spinner /></main>;
  }
  const current = steps.indexOf(order.status);
  const view = statusView[order.status];
  const live = !statusHidden && order.status !== 'completed' && order.status !== 'cancelled';
  const pickup = order.mode === 'pickup';
  const readyText = pickup ? 'Collect it at the counter.' : 'It is on its way to your table.';

  return (
    <div className="guest">
      <main className="checkout">
        <section className={`track-hero st-${statusHidden ? 'completed' : order.status === 'new' ? 'new' : order.status === 'cancelled' ? 'cancelled' : order.status === 'preparing' ? 'preparing' : 'ready'}`} aria-live="polite">
          <span className={`track-icon${live ? ' live' : ''}`}><Icon name={statusHidden ? 'checkCircle' : view.icon} size={36} /></span>
          <h1>{statusHidden ? 'Thank you!' : view.title}</h1>
          <p className="muted">{statusHidden ? 'Your order was sent. Show this number to the staff if you have questions.' : order.status === 'ready' ? `${view.text} ${readyText}` : view.text}</p>
          <span className="track-number"><span className="muted small">Order</span><strong>{order.number}</strong></span>
        </section>

        {placed && !statusHidden && order.status === 'new' && (
          <p className="banner banner-ok" role="status"><Icon name="checkCircle" /><span>Thank you! Your order was sent to the kitchen.</span></p>
        )}

        {!statusHidden && order.status !== 'cancelled' && (
          <section className="card stack-tight">
            <ol className="track" aria-label="Progress">
              {stepLabels.map((label, i) => (
                <li key={label} className={i < current || current >= 4 ? 'reached' : i === current ? 'reached current' : ''}>{label}</li>
              ))}
            </ol>
            {live && <p className="muted tiny">This page updates by itself.</p>}
          </section>
        )}
        <ErrorBanner error={error} />

        <section className="card stack-tight">
          <div className="row">
            <h2 className="row-grow">Your order</h2>
            <span className="muted small">{order.dining_location} · {new Date(order.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
          </div>
          <ul className="lines">
            {order.items.map((l, i) => (
              <li key={i}>
                <span className="qty">{l.quantity}×</span>
                <span className="row-grow">
                  {l.name}
                  {l.options.length > 0 && <span className="ticket-opts">{l.options.map((o) => o.option_name).join(', ')}</span>}
                </span>
                <span className="num">{formatMoney(l.line_total_minor, order.currency)}</span>
              </li>
            ))}
          </ul>
          <div className="totals"><span>Total</span><span className="num">{formatMoney(order.total_minor, order.currency)}</span></div>
          <div className="info-row muted small" style={{ gap: 10 }}>
            <Icon name="wallet" size={18} />
            <span>{order.payment_status === 'paid' ? 'Paid. Thank you!' : `Pay ${pickup ? 'when you collect' : 'at the table'}.`}</span>
          </div>
        </section>

        {menuToken && (
          <Link className="btn btn-large btn-block btn-round" to={`/m/${menuToken}`}>
            <Icon name="plus" size={18} />Order more
          </Link>
        )}
      </main>
    </div>
  );
}
