import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router';
import { ApiError } from '../api/client';
import { ordersApi } from '../api/staff';
import type { OrderMode, OrderStatus, StaffOrder } from '../api/types';
import { formatMoney } from '../shared/money';
import { useSession } from '../shared/session';
import { ErrorBanner, Spinner, Toggle, useAction } from '../shared/ui';
import { OrderDrawer } from './OrderDrawer';
import { useLiveOrders } from './useLiveOrders';

const columns: { status: OrderStatus; title: string; next?: OrderStatus; action?: string }[] = [
  { status: 'new', title: 'New', next: 'accepted', action: 'Accept' },
  { status: 'accepted', title: 'Accepted', next: 'preparing', action: 'Start preparing' },
  { status: 'preparing', title: 'Preparing', next: 'ready', action: 'Mark ready' },
  { status: 'ready', title: 'Ready', next: 'completed', action: 'Complete' },
];
const active: OrderStatus[] = ['new', 'accepted', 'preparing', 'ready'];

/** Live order board for staff: /staff. */
export function OrderBoard() {
  const { session, can, logout } = useSession();
  const [mode, setMode] = useState<OrderMode | ''>('');
  const [showDone, setShowDone] = useState(false);
  const live = useLiveOrders(showDone ? ['completed', 'cancelled'] : active, mode);
  const [open, setOpen] = useState<string | null>(null);
  const [sound, setSound] = useState(false);
  const act = useAction();
  const now = useNow();

  const newCount = live.orders?.filter((o) => o.status === 'new').length ?? 0;
  useNewOrderAlert(newCount, sound && !showDone);

  const byStatus = useMemo(() => {
    const m = new Map<OrderStatus, StaffOrder[]>();
    for (const o of [...(live.orders ?? [])].reverse()) m.set(o.status, [...(m.get(o.status) ?? []), o]); // oldest first
    return m;
  }, [live.orders]);

  // Always refetch afterwards: on success to show the move, on a 409 to show
  // what the other person did.
  const advance = async (o: StaffOrder, to: OrderStatus) => {
    await act.run(() => ordersApi.setStatus(o.id, to, o.version));
    await live.refetch();
  };

  return (
    <div className="board-page">
      <header className="topbar">
        <strong className="topbar-title">{session?.active_tenant?.name} · Orders</strong>
        <span className={`conn conn-${live.connection}`} role="status" aria-live="polite">
          {live.connection === 'live' ? 'Live' : live.connection === 'connecting' ? 'Connecting…' : 'Connection lost: reconnecting…'}
        </span>
        <span className="topbar-end">
          <select aria-label="Order type" value={mode} onChange={(e) => setMode(e.target.value as OrderMode | '')}>
            <option value="">All orders</option>
            <option value="dine_in">Tables</option>
            <option value="pickup">Pickup</option>
          </select>
          <Toggle label="Completed" checked={showDone} onChange={setShowDone} />
          <Toggle label="Sound" checked={sound} onChange={setSound} />
          {can('menu.manage') || can('settings.manage') || can('dining_locations.manage') ? <Link className="btn btn-small" to="/admin">Admin</Link> : null}
          <button type="button" className="btn btn-small" onClick={() => void logout()}>Sign out</button>
        </span>
      </header>
      <ErrorBanner error={live.error} onRetry={live.refetch} />
      {act.error instanceof ApiError && act.error.status === 409 ? (
        <div className="banner banner-error" role="alert">Someone else changed that order just now. The board is up to date; try again.</div>
      ) : (
        <ErrorBanner error={act.error} />
      )}
      {!live.orders ? (
        <Spinner />
      ) : showDone ? (
        <section className="page">
          <h2>Completed and cancelled</h2>
          <ul className="list-plain stack-tight">
            {live.orders.map((o) => <OrderCard key={o.id} order={o} now={now} onOpen={() => setOpen(o.id)} />)}
          </ul>
        </section>
      ) : (
        <div className="board">
          {columns.map((c) => (
            <section key={c.status} className="board-col" aria-labelledby={`col-${c.status}`}>
              <h2 id={`col-${c.status}`}>{c.title} <span className="count">{byStatus.get(c.status)?.length ?? 0}</span></h2>
              <ul className="list-plain stack-tight">
                {(byStatus.get(c.status) ?? []).map((o) => (
                  <OrderCard key={o.id} order={o} now={now} onOpen={() => setOpen(o.id)}>
                    {c.next && can('orders.update_status') && (
                      <button type="button" className="btn btn-primary btn-large btn-block" disabled={act.busy} onClick={() => void advance(o, c.next!)}>
                        {c.action}
                      </button>
                    )}
                  </OrderCard>
                ))}
              </ul>
            </section>
          ))}
        </div>
      )}
      {open && <OrderDrawer id={open} onClose={() => setOpen(null)} onChanged={() => void live.refetch()} />}
    </div>
  );
}

function OrderCard({ order: o, now, onOpen, children }: { order: StaffOrder; now: number; onOpen: () => void; children?: React.ReactNode }) {
  const mins = Math.max(0, Math.floor((now - new Date(o.created_at).getTime()) / 60_000));
  return (
    <li className={`card order-card status-${o.status}`}>
      <button type="button" className="order-card-head" onClick={onOpen} aria-label={`Order ${o.number}, details`}>
        <span className="order-no">#{o.number}</span>
        <span className="row-grow">{o.dining_location?.name ?? (o.mode === 'pickup' ? 'Pickup' : '')}</span>
        <span className={mins >= 15 && o.status !== 'completed' ? 'age age-late' : 'age'}>{mins} min</span>
      </button>
      <ul className="list-plain order-lines">
        {o.items.map((l, i) => (
          <li key={i}>
            <strong>{l.quantity}×</strong> {l.name}
            {l.options.length > 0 && <span className="muted"> · {l.options.map((x) => x.option_name).join(', ')}</span>}
            {l.notes && <div className="note">“{l.notes}”</div>}
          </li>
        ))}
      </ul>
      {o.notes && <div className="note">Order note: “{o.notes}”</div>}
      <div className="row small">
        <span className="row-grow">{formatMoney(o.total_minor, o.currency)}</span>
        <span className={`badge pay-${o.payment_status}`}>{o.payment_status.replace('_', ' ')}</span>
        {o.status === 'cancelled' && <span className="badge">cancelled</span>}
      </div>
      {children}
    </li>
  );
}

/** Re-render every 30 s so order ages stay current. */
function useNow(): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(t);
  }, []);
  return now;
}

/** Title badge always; a short tone when enabled and new orders arrive. */
function useNewOrderAlert(count: number, sound: boolean) {
  const prev = useRef(count);
  useEffect(() => {
    document.title = count > 0 ? `(${count}) New orders · servly` : 'Orders · servly';
    if (sound && count > prev.current) beep();
    prev.current = count;
  }, [count, sound]);
  useEffect(() => () => { document.title = 'servly'; }, []);
}

function beep() {
  try {
    const ctx = new AudioContext();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.frequency.value = 880;
    gain.gain.setValueAtTime(0.2, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.6);
    osc.connect(gain).connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + 0.6);
  } catch {
    /* audio unavailable */
  }
}
