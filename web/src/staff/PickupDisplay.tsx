import { useEffect, useRef } from 'react';
import { useSession } from '../shared/session';
import { Icon } from '../shared/icons';
import { useLiveOrders } from './useLiveOrders';

/**
 * The guest-facing pickup screen, for a TV or tablet at the counter:
 * /staff/display. Order numbers only, no names or items. It runs on a staff
 * sign-in, like the board, and follows the same live stream.
 */
export function PickupDisplay() {
  const { session } = useSession();
  const live = useLiveOrders(['accepted', 'preparing', 'ready'], 'pickup');
  const orders = [...(live.orders ?? [])].reverse(); // oldest first
  const preparing = orders.filter((o) => o.status !== 'ready');
  const ready = orders.filter((o) => o.status === 'ready');
  const newest = useRef<Set<string>>(new Set());

  // Numbers that just turned ready flash once; remember which ones were already shown.
  const fresh = new Set(ready.filter((o) => !newest.current.has(o.id)).map((o) => o.id));
  useEffect(() => {
    newest.current = new Set(ready.map((o) => o.id));
  });

  return (
    <main className="display">
      <header className="display-head">
        <span className="logo logo-lg" aria-hidden="true">{session?.active_tenant?.name.charAt(0)}</span>
        <div className="row-grow">
          <h1>{session?.active_tenant?.name}</h1>
          <p>Collect your order at the counter when your number shows under Ready.</p>
        </div>
        {live.connection !== 'live' && <span className="conn conn-reconnecting"><span className="dot" />Reconnecting…</span>}
        <button type="button" className="btn btn-ghost btn-icon display-full" aria-label="Full screen" title="Full screen"
          onClick={() => void document.documentElement.requestFullscreen?.().catch(() => undefined)}>
          <Icon name="monitor" />
        </button>
      </header>
      <div className="display-cols">
        <section aria-labelledby="d-prep">
          <h2 id="d-prep"><Icon name="flame" size={28} />Being prepared</h2>
          <ul className="display-nums">
            {preparing.map((o) => <li key={o.id}>{o.number}</li>)}
          </ul>
          {preparing.length === 0 && <p className="display-empty">-</p>}
        </section>
        <section aria-labelledby="d-ready" className="display-ready">
          <h2 id="d-ready"><Icon name="bag" size={28} />Ready to collect</h2>
          <ul className="display-nums" aria-live="polite">
            {ready.map((o) => <li key={o.id} className={fresh.has(o.id) ? 'fresh' : ''}>{o.number}</li>)}
          </ul>
          {ready.length === 0 && <p className="display-empty">-</p>}
        </section>
      </div>
    </main>
  );
}
