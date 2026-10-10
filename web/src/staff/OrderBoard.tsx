import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router';
import { ApiError } from '../api/client';
import { ordersApi } from '../api/staff';
import type { OrderMode, OrderStatus, StaffOrder } from '../api/types';
import { formatMoney } from '../shared/money';
import { useSession } from '../shared/session';
import { Icon } from '../shared/icons';
import { EmptyState, ErrorBanner, Spinner, useAction } from '../shared/ui';
import { OrderDrawer } from './OrderDrawer';
import { useLiveOrders } from './useLiveOrders';

const columns: { status: OrderStatus; title: string; empty: string; next?: OrderStatus; action?: string }[] = [
  { status: 'new', title: 'New', empty: 'New orders appear here.', next: 'accepted', action: 'Accept' },
  { status: 'accepted', title: 'Accepted', empty: 'Nothing waiting for the kitchen.', next: 'preparing', action: 'Start preparing' },
  { status: 'preparing', title: 'Preparing', empty: 'Nothing on the stove.', next: 'ready', action: 'Mark ready' },
  { status: 'ready', title: 'Ready', empty: 'Nothing waiting to be served.', next: 'completed', action: 'Complete' },
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

  const conn = live.connection === 'live' ? 'Live' : live.connection === 'connecting' ? 'Connecting…' : 'Offline: reconnecting…';

  return (
    <div className="kds">
      <header className="kds-bar">
        <div className="kds-title">
          <span className="logo" aria-hidden="true">{session?.active_tenant?.name.charAt(0)}</span>
          <strong className="truncate">{session?.active_tenant?.name}</strong>
          <span className={`conn conn-${live.connection}`} role="status" aria-live="polite">
            <span className="dot" />
            {conn}
          </span>
        </div>
        <div className="kds-tools">
          <div className="seg" role="group" aria-label="Order type">
            {([['', 'All'], ['dine_in', 'Tables'], ['pickup', 'Pickup']] as const).map(([v, label]) => (
              <button key={v} type="button" aria-pressed={mode === v} onClick={() => setMode(v)}>{label}</button>
            ))}
          </div>
          <div className="seg" role="group" aria-label="Show">
            <button type="button" aria-pressed={!showDone} onClick={() => setShowDone(false)}>Active</button>
            <button type="button" aria-pressed={showDone} onClick={() => setShowDone(true)}>Done</button>
          </div>
          <button type="button" className="btn btn-icon" aria-pressed={sound} aria-label={sound ? 'Sound on' : 'Sound off'} title={sound ? 'Sound on' : 'Sound off'} onClick={() => setSound(!sound)}>
            <Icon name={sound ? 'volume' : 'volumeOff'} />
          </button>
          {can('menu.manage') || can('settings.manage') || can('dining_locations.manage') ? (
            <Link className="btn" to="/admin" aria-label="Admin"><Icon name="sliders" size={18} /><span className="hide-sm">Admin</span></Link>
          ) : null}
          <button type="button" className="btn btn-icon" aria-label="Sign out" title="Sign out" onClick={() => void logout()}>
            <Icon name="logout" size={18} />
          </button>
        </div>
      </header>
      <div style={{ padding: '0 16px' }} className="stack-tight">
        {live.error ? <div style={{ marginTop: 12 }}><ErrorBanner error={live.error} onRetry={live.refetch} /></div> : null}
        {act.error instanceof ApiError && act.error.status === 409 ? (
          <div className="banner banner-warn" role="alert" style={{ marginTop: 12 }}>
            <Icon name="refresh" />
            <span>Someone else changed that order just now. The board is up to date; try again.</span>
          </div>
        ) : act.error ? (
          <div style={{ marginTop: 12 }}><ErrorBanner error={act.error} /></div>
        ) : null}
      </div>
      {!live.orders ? (
        <Spinner label="Loading orders…" />
      ) : showDone ? (
        live.orders.length === 0 ? (
          <EmptyState icon="receipt" title="No completed orders yet">Completed and cancelled orders of today appear here.</EmptyState>
        ) : (
          <ul className="list-plain done-grid">
            {live.orders.map((o) => <Ticket key={o.id} order={o} now={now} onOpen={() => setOpen(o.id)} />)}
          </ul>
        )
      ) : (
        <div className="board">
          {columns.map((c) => {
            const list = byStatus.get(c.status) ?? [];
            return (
              <section key={c.status} className={`board-col st-${c.status}`} aria-labelledby={`col-${c.status}`}>
                <h2 id={`col-${c.status}`} className="col-head">
                  <span className="dot" />
                  {c.title}
                  <span className="count">{list.length}</span>
                </h2>
                {list.length === 0 && <p className="col-empty">{c.empty}</p>}
                <ul className="list-plain stack-tight">
                  {list.map((o) => (
                    <Ticket key={o.id} order={o} now={now} onOpen={() => setOpen(o.id)}>
                      {c.next && can('orders.update_status') && (
                        <div className="ticket-action">
                          <button type="button" className="btn btn-st" disabled={act.busy} onClick={() => void advance(o, c.next!)}>
                            {c.action}
                          </button>
                        </div>
                      )}
                    </Ticket>
                  ))}
                </ul>
              </section>
            );
          })}
        </div>
      )}
      {open && <OrderDrawer id={open} onClose={() => setOpen(null)} onChanged={() => void live.refetch()} />}
    </div>
  );
}

const payLabel: Record<StaffOrder['payment_status'], { text: string; tone: string }> = {
  unpaid: { text: 'Unpaid', tone: '' },
  pending: { text: 'Payment pending', tone: 'badge-warn' },
  paid: { text: 'Paid', tone: 'badge-ok' },
  failed: { text: 'Payment failed', tone: 'badge-danger' },
  partially_refunded: { text: 'Part refunded', tone: 'badge-info' },
  refunded: { text: 'Refunded', tone: 'badge-info' },
};

function Ticket({ order: o, now, onOpen, children }: { order: StaffOrder; now: number; onOpen: () => void; children?: React.ReactNode }) {
  const mins = Math.max(0, Math.floor((now - new Date(o.created_at).getTime()) / 60_000));
  const open = o.status !== 'completed' && o.status !== 'cancelled';
  const timer = !open ? 'timer' : mins >= 15 ? 'timer timer-late' : mins >= 10 ? 'timer timer-warn' : 'timer';
  const where = o.dining_location?.name ?? (o.mode === 'pickup' ? 'Pickup' : 'Table');
  const pay = payLabel[o.payment_status];
  return (
    <li className={`ticket st-${o.status}`}>
      <button type="button" className="ticket-head" onClick={onOpen} aria-label={`Order ${o.number}, ${where}, details`}>
        <span className="ticket-no">#{o.number}</span>
        <span className="ticket-where">
          <strong className="truncate">{where}</strong>
          <span className="muted tiny">{o.mode === 'pickup' ? 'Pickup' : 'Dine-in'} · {new Date(o.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
        </span>
        <span className={timer}>
          <Icon name="clock" size={14} />
          {mins === 0 ? 'now' : `${mins} min`}
        </span>
      </button>
      <ul className="ticket-lines">
        {o.items.map((l, i) => (
          <li key={i}>
            <span className="qty">{l.quantity}×</span>
            <span>
              {l.name}
              {l.options.length > 0 && <span className="ticket-opts">{l.options.map((x) => x.option_name).join(', ')}</span>}
              {l.notes && <span className="note"><Icon name="note" size={14} />{l.notes}</span>}
            </span>
          </li>
        ))}
      </ul>
      {o.notes && <div className="note ticket-note"><Icon name="note" size={14} />{o.notes}</div>}
      <div className="ticket-foot">
        <span className="row-grow strong num">{formatMoney(o.total_minor, o.currency)}</span>
        <span className={`badge ${pay.tone}`}>{pay.text}</span>
        {o.status === 'cancelled' && <span className="badge badge-danger">Cancelled</span>}
        {o.status === 'completed' && <span className="badge">Completed</span>}
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
