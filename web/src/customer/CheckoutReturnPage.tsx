import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { publicApi } from '../api/public';
import type { CheckoutState } from '../api/types';
import { Icon } from '../shared/icons';
import { ErrorBanner } from '../shared/ui';
import { clearCart, recallCheckout, rememberOrder, renewCheckoutKey } from './cart';

const POLL_MS = 2_000;
const SLOW_MS = 45_000;

/**
 * Where the payment provider sends the guest back: /pay/:token. The server
 * confirms the payment with the provider; this page waits for that, then
 * moves on to the order.
 */
export function CheckoutReturnPage() {
  const { token = '' } = useParams();
  const navigate = useNavigate();
  const place = recallCheckout(token);
  const [state, setState] = useState<CheckoutState | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [slow, setSlow] = useState(false);

  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout>;
    const started = Date.now();
    const poll = async () => {
      try {
        const s = await publicApi.checkout(token);
        if (!live) return;
        setState(s);
        setError(null);
        if (s.status === 'paid' && s.order_token) {
          if (s.kind === 'prepay' && place.menu) clearCart(place.menu);
          const order = await publicApi.order(s.order_token).catch(() => null);
          if (order && place.menu) rememberOrder(order, place.menu);
          navigate(`/o/${s.order_token}`, { replace: true, state: { placed: s.kind === 'prepay', paid: true } });
          return;
        }
        if (s.status !== 'open') {
          if (s.kind === 'prepay' && place.menu) renewCheckoutKey(place.menu);
          return;
        }
      } catch (err) {
        if (live) setError(err);
      }
      if (live) {
        setSlow(Date.now() - started > SLOW_MS);
        timer = setTimeout(() => void poll(), POLL_MS);
      }
    };
    void poll();
    return () => {
      live = false;
      clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  const failed = state && (state.status === 'failed' || state.status === 'expired');
  const back = state?.kind === 'prepay' && place.menu ? `/m/${place.menu}/cart` : place.order ? `/o/${place.order}` : place.menu ? `/m/${place.menu}` : null;

  return (
    <div className="guest">
      <main className="checkout">
        <section className={`track-hero ${failed ? 'st-cancelled' : 'st-preparing'}`} aria-live="polite">
          <span className={`track-icon${failed ? '' : ' live'}`}><Icon name={failed ? 'xCircle' : 'wallet'} size={36} /></span>
          <h1>{failed ? 'Payment not completed' : 'Confirming your payment'}</h1>
          <p className="muted">
            {failed
              ? state?.status === 'expired'
                ? 'The payment timed out. Nothing was charged.'
                : 'The payment did not go through. Nothing was charged.'
              : 'This takes a few seconds. Please keep this page open.'}
          </p>
        </section>
        <ErrorBanner error={error} />
        {!failed && slow && (
          <p className="banner banner-info"><Icon name="clock" /><span>Still waiting for the payment provider. If you paid, this page updates by itself.</span></p>
        )}
        {!failed && state?.redirect_url && slow && (
          <a className="btn btn-large btn-block btn-round" href={state.redirect_url}>Back to the payment page</a>
        )}
        {failed && back && (
          <Link className="btn btn-primary btn-large btn-block btn-round" to={back} replace>
            {state?.kind === 'prepay' ? 'Back to your order' : 'Back'}
          </Link>
        )}
      </main>
    </div>
  );
}
