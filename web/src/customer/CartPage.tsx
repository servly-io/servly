import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { ApiError } from '../api/client';
import { publicApi } from '../api/public';
import type { Quote } from '../api/types';
import { formatMoney } from '../shared/money';
import { ErrorBanner, Spinner, useAction, useLoad } from '../shared/ui';
import { rememberOrder, useCart } from './cart';

/** Review the cart with server-confirmed prices, then submit: /m/:token/cart. */
export function CartPage() {
  const { token = '' } = useParams();
  const navigate = useNavigate();
  const cart = useCart(token);
  const menu = useLoad(() => publicApi.menu(token), [token]);
  const [quote, setQuote] = useState<Quote | null>(null);
  const [quoteError, setQuoteError] = useState<unknown>(null);
  const submit = useAction();
  const request = cart.request();
  const requestKey = JSON.stringify(request);

  // Ask the server for the real prices whenever the cart changes.
  useEffect(() => {
    if (cart.lines.length === 0) return;
    let live = true;
    setQuote(null);
    publicApi
      .quote(JSON.parse(requestKey) as typeof request)
      .then((q) => live && (setQuote(q), setQuoteError(null)))
      .catch((err: unknown) => live && setQuoteError(err));
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [requestKey]);

  if (!menu.data) return menu.error ? <main className="page"><ErrorBanner error={menu.error} onRetry={menu.reload} /></main> : <main className="page"><Spinner /></main>;
  const cur = menu.data.restaurant.currency;

  const place = async () => {
    const order = await submit.run(() => publicApi.place(request, cart.checkoutKey));
    if (order) {
      rememberOrder(order);
      cart.clear();
      navigate(`/o/${order.order_token}`, { replace: true, state: { placed: true } });
    }
  };

  const offline = submit.error instanceof ApiError && submit.error.isNetwork;

  return (
    <main className="page page-narrow guest stack">
      <Link to={`/m/${token}`} className="btn">← Back to the menu</Link>
      <h1>Your order</h1>
      <p className="muted">{menu.data.restaurant.name} · {menu.data.dining_location.name}</p>
      {cart.lines.length === 0 ? (
        <p>Your order is empty.</p>
      ) : (
        <>
          <ul className="list-plain stack-tight">
            {cart.lines.map((l, i) => (
              <li key={l.key} className="card row">
                <div className="row-grow">
                  <strong>{l.name}</strong>
                  {l.optionLabels.length > 0 && <div className="muted small">{l.optionLabels.join(', ')}</div>}
                  {l.notes && <div className="muted small">“{l.notes}”</div>}
                </div>
                <span className="stepper" role="group" aria-label={`Quantity of ${l.name}`}>
                  <button type="button" className="btn btn-icon" aria-label={l.quantity === 1 ? `Remove ${l.name}` : 'One less'}
                    onClick={() => cart.setQuantity(l.key, l.quantity - 1)}>−</button>
                  <output>{l.quantity}</output>
                  <button type="button" className="btn btn-icon" aria-label="One more" disabled={l.quantity >= 99}
                    onClick={() => cart.setQuantity(l.key, l.quantity + 1)}>+</button>
                </span>
                <span className="price">{quote?.items[i] ? formatMoney(quote.items[i]!.line_total_minor, cur) : '…'}</span>
              </li>
            ))}
          </ul>
          {menu.data.restaurant.order_notes_enabled && (
            <label className="field">
              <span className="field-label">Note for the whole order</span>
              <textarea rows={2} maxLength={500} value={cart.notes} onChange={(e) => cart.setNotes(e.target.value)} />
            </label>
          )}
          <ErrorBanner error={quoteError} />
          <p className="total row">
            <span className="row-grow">Total</span>
            <strong>{quote ? formatMoney(quote.total_minor, cur) : <Spinner label="Checking prices…" />}</strong>
          </p>
          {offline ? (
            <div className="banner banner-error" role="alert">
              <span>No connection. Your order was not confirmed yet. Trying again is safe: it cannot be placed twice.</span>
            </div>
          ) : (
            <ErrorBanner error={submit.error} />
          )}
          <button type="button" className="btn btn-primary btn-large btn-block" disabled={!quote || submit.busy} onClick={() => void place()}>
            {submit.busy ? 'Sending…' : offline ? 'Try again' : `Place order · ${quote ? formatMoney(quote.total_minor, cur) : ''}`}
          </button>
          <p className="muted small">You pay at the table or the counter. No account needed.</p>
        </>
      )}
    </main>
  );
}
