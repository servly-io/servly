import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { ApiError } from '../api/client';
import { publicApi } from '../api/public';
import type { Quote } from '../api/types';
import { Icon } from '../shared/icons';
import { formatMoney } from '../shared/money';
import { EmptyState, ErrorBanner, Spinner, useAction, useLoad } from '../shared/ui';
import { rememberCheckout, rememberOrder, useCart } from './cart';

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

  if (!menu.data) return <main className="guest">{menu.error ? <div className="checkout"><ErrorBanner error={menu.error} onRetry={menu.reload} /></div> : <Spinner />}</main>;
  const cur = menu.data.restaurant.currency;
  const pickup = menu.data.dining_location.kind === 'pickup';

  const place = async () => {
    const order = await submit.run(() => publicApi.place(request, cart.checkoutKey));
    if (order) {
      rememberOrder(order, token);
      cart.clear();
      navigate(`/o/${order.order_token}`, { replace: true, state: { placed: true } });
    }
  };

  // Pay first: the provider's page takes the money, and the order is created
  // once servly confirms the payment.
  const payNow = async () => {
    const start = await submit.run(() => publicApi.prepay(request, cart.checkoutKey));
    if (start) {
      rememberCheckout(start.checkout_token, { menu: token });
      window.location.assign(start.redirect_url);
    }
  };

  const offline = submit.error instanceof ApiError && submit.error.isNetwork;
  const total = quote ? formatMoney(quote.total_minor, cur) : '';
  const online = menu.data.payment;
  const payText =
    online === 'before'
      ? 'You pay online now: card, Apple Pay, Google Pay or your local method. The order goes to the kitchen once it is paid.'
      : online === 'either'
        ? 'Pay online now, or at the counter when you collect.'
        : online === 'after'
          ? `You can pay on your phone after ordering, or ${pickup ? 'when you collect' : 'at the table'}.`
          : `You pay ${pickup ? 'when you collect' : 'at the table'}: cash or card. Nothing is charged now.`;

  return (
    <div className="guest">
      <header className="topnav-guest">
        <Link to={`/m/${token}`} className="btn btn-ghost btn-icon btn-round" aria-label="Back to the menu"><Icon name="chevronLeft" /></Link>
        <div className="row-grow">
          <h1>Your order</h1>
          <span className="muted small">{menu.data.restaurant.name}</span>
        </div>
      </header>
      {cart.lines.length === 0 ? (
        <main className="checkout">
          <EmptyState icon="bag" title="Your order is empty" action={<Link className="btn btn-primary btn-large btn-round" to={`/m/${token}`}>Browse the menu</Link>}>
            Add something from the menu and it shows up here.
          </EmptyState>
        </main>
      ) : (
        <main className="checkout">
          <section className="card info-row">
            <span className="info-icon"><Icon name={pickup ? 'bag' : 'pin'} /></span>
            <div className="row-grow">
              <div className="strong">{menu.data.dining_location.name}</div>
              <div className="muted small">{pickup ? 'Pickup: we tell you when it is ready' : 'Dine-in: we bring it to your table'}</div>
            </div>
          </section>

          <section className="card">
            <ul className="cart-lines">
              {cart.lines.map((l, i) => (
                <li key={l.key} className="cart-line">
                  <span className="stepper stepper-sm" role="group" aria-label={`Quantity of ${l.name}`}>
                    <button type="button" aria-label={l.quantity === 1 ? `Remove ${l.name}` : 'One less'} onClick={() => cart.setQuantity(l.key, l.quantity - 1)}>
                      <Icon name={l.quantity === 1 ? 'trash' : 'minus'} size={16} />
                    </button>
                    <output>{l.quantity}</output>
                    <button type="button" aria-label="One more" disabled={l.quantity >= 99} onClick={() => cart.setQuantity(l.key, l.quantity + 1)}>
                      <Icon name="plus" size={16} />
                    </button>
                  </span>
                  <div className="row-grow">
                    <div className="strong">{l.name}</div>
                    {l.optionLabels.length > 0 && <div className="muted small">{l.optionLabels.join(', ')}</div>}
                    {l.notes && <div className="muted small">“{l.notes}”</div>}
                  </div>
                  <span className="cart-line-price">{quote?.items[i] ? formatMoney(quote.items[i]!.line_total_minor, cur) : '…'}</span>
                </li>
              ))}
            </ul>
            <Link to={`/m/${token}`} className="text-link row small" style={{ marginTop: 12, gap: 6 }}><Icon name="plus" size={16} />Add more items</Link>
          </section>

          {menu.data.restaurant.order_notes_enabled && (
            <label className="card field">
              <span className="field-label">Note for the restaurant</span>
              <textarea rows={2} maxLength={500} placeholder="Allergies, timing, anything we should know" value={cart.notes} onChange={(e) => cart.setNotes(e.target.value)} />
            </label>
          )}

          <section className="card stack-tight">
            <ErrorBanner error={quoteError} />
            {quote && quote.subtotal_minor !== quote.total_minor && (
              <div className="totals-sub"><span>Subtotal</span><span className="num">{formatMoney(quote.subtotal_minor, cur)}</span></div>
            )}
            <div className="totals" style={{ borderTop: 'none', marginTop: 0, paddingTop: 0 }}>
              <span>Total</span>
              <span className="num">{quote ? total : <Spinner inline label="Checking prices…" />}</span>
            </div>
            <div className="info-row muted small" style={{ gap: 10 }}>
              <Icon name="wallet" size={18} />
              <span>{payText}</span>
            </div>
          </section>

          {offline ? (
            <div className="banner banner-warn" role="alert">
              <Icon name="alert" />
              <span>No connection. Your order is not confirmed yet. Trying again is safe: it cannot be placed twice.</span>
            </div>
          ) : (
            <ErrorBanner error={submit.error} />
          )}
          {!menu.data.restaurant.accepting_orders && (
            <p className="banner banner-warn" role="status"><Icon name="clock" /><span>The restaurant is not taking orders right now.</span></p>
          )}
          <div className="checkout-cta stack-tight" hidden={!menu.data.restaurant.accepting_orders}>
            {online === 'before' || online === 'either' ? (
              <button type="button" className="btn btn-primary btn-large btn-block" disabled={!quote || submit.busy} onClick={() => void payNow()}>
                <span>{submit.busy ? 'Opening payment…' : online === 'before' ? 'Pay and order' : 'Pay now'}</span>
                <span className="num">{total}</span>
              </button>
            ) : null}
            {online !== 'before' && (
              <button type="button" className={`btn btn-large btn-block ${online === 'either' ? '' : 'btn-primary'}`} disabled={!quote || submit.busy} onClick={() => void place()}>
                <span>{submit.busy ? 'Sending…' : offline ? 'Try again' : online === 'either' ? 'Order, pay at the counter' : 'Place order'}</span>
                <span className="num">{online === 'either' ? '' : total}</span>
              </button>
            )}
          </div>
        </main>
      )}
    </div>
  );
}
