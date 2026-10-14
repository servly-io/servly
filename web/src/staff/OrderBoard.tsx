import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router';
import { ApiError } from '../api/client';
import { settingsApi } from '../api/admin';
import { ordersApi } from '../api/staff';
import type { OrderMode, OrderStatus, PaymentStatus, StaffOrder } from '../api/types';
import { formatMoney } from '../shared/money';
import { useSession } from '../shared/session';
import { Icon } from '../shared/icons';
import { EmptyState, ErrorBanner, Spinner, useAction, useLoad } from '../shared/ui';
import { OrderDrawer } from './OrderDrawer';
import { useLiveOrders } from './useLiveOrders';

const columns: { status: OrderStatus; title: string; empty: string; next: OrderStatus; action: string }[] = [
  { status: 'new', title: 'New', empty: 'New orders appear here.', next: 'accepted', action: 'Accept' },
  { status: 'accepted', title: 'Accepted', empty: 'Nothing waiting for the kitchen.', next: 'preparing', action: 'Start preparing' },
  { status: 'preparing', title: 'Preparing', empty: 'Nothing on the stove.', next: 'ready', action: 'Mark ready' },
  { status: 'ready', title: 'Ready', empty: 'Nothing waiting to be served or collected.', next: 'completed', action: 'Served' },
];
const active: OrderStatus[] = ['new', 'accepted', 'preparing', 'ready'];
const done: OrderStatus[] = ['completed', 'cancelled'];
// Money still to collect: any order not cancelled whose payment is not settled.
const notCancelled: OrderStatus[] = ['new', 'accepted', 'preparing', 'ready', 'completed'];
const owing: PaymentStatus[] = ['unpaid', 'pending'];

type View = 'kitchen' | 'pay' | 'history';

/** Live order board for staff: /staff. */
export function OrderBoard() {
  const { session, can, logout } = useSession();
  const [mode, setMode] = useState<OrderMode | ''>('');
  const [view, setView] = useState<View>('kitchen');
  const live = useLiveOrders(view === 'history' ? done : active, mode);
  // Reloaded on every reconnect, so the board catches up with a change in Settings.
  const closed = useLoad(() => settingsApi.get(), [live.connection]);
  // Always on, so the "To pay" count is current in every view.
  const toPay = useLiveOrders(notCancelled, mode, owing);
  const [open, setOpen] = useState<{ id: string; handover?: boolean } | null>(null);
  const [sound, setSound] = useState(false);
  const act = useAction();
  const now = useNow();
  const canPay = can('payments.record');

  const newCount = view === 'history' ? 0 : (live.orders?.filter((o) => o.status === 'new').length ?? 0);
  useNewOrderAlert(newCount, sound && view !== 'history');

  const byStatus = useMemo(() => {
    const m = new Map<OrderStatus, StaffOrder[]>();
    for (const o of [...(live.orders ?? [])].reverse()) m.set(o.status, [...(m.get(o.status) ?? []), o]); // oldest first
    return m;
  }, [live.orders]);
  const refetchAll = () => void Promise.all([live.refetch(), toPay.refetch()]);

  // Always refetch afterwards: on success to show the move, on a 409 to show
  // what the other person did.
  const advance = async (o: StaffOrder, to: OrderStatus) => {
    await act.run(() => ordersApi.setStatus(o.id, to, o.version));
    refetchAll();
  };
  const unpaid = (o: StaffOrder) => o.payment_status === 'unpaid' || o.payment_status === 'pending';

  const conn = live.connection === 'live' ? 'Live' : live.connection === 'connecting' ? 'Connecting…' : 'Offline: reconnecting…';
  const payCount = toPay.orders?.length ?? 0;
  const shown = view === 'pay' ? toPay : live;

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
          {closed.data?.accepting_orders === false && (
            <span className="conn conn-reconnecting" title="Turned off in Settings">Not taking orders</span>
          )}
        </div>
        <div className="kds-tools">
          <div className="seg" role="group" aria-label="View">
            <button type="button" aria-pressed={view === 'kitchen'} onClick={() => setView('kitchen')}>Kitchen</button>
            <button type="button" aria-pressed={view === 'pay'} onClick={() => setView('pay')}>
              To pay
              {payCount > 0 && <span className="seg-count">{payCount}</span>}
            </button>
            <button type="button" aria-pressed={view === 'history'} onClick={() => setView('history')}>History</button>
          </div>
          <div className="seg" role="group" aria-label="Order type">
            {([['', 'All'], ['dine_in', 'Tables'], ['pickup', 'Pickup']] as const).map(([v, label]) => (
              <button key={v} type="button" aria-pressed={mode === v} onClick={() => setMode(v)}>{label}</button>
            ))}
          </div>
          <button type="button" className="btn btn-icon" aria-pressed={sound} aria-label={sound ? 'Sound on' : 'Sound off'} title={sound ? 'Sound on' : 'Sound off'} onClick={() => setSound(!sound)}>
            <Icon name={sound ? 'volume' : 'volumeOff'} />
          </button>
          <a className="btn btn-icon" href="/staff/display" target="_blank" rel="noreferrer" aria-label="Open the pickup screen for guests" title="Pickup screen for guests">
            <Icon name="monitor" size={18} />
          </a>
          {can('menu.manage') || can('settings.manage') || can('dining_locations.manage') ? (
            <Link className="btn" to="/admin" aria-label="Admin"><Icon name="sliders" size={18} /><span className="hide-sm">Admin</span></Link>
          ) : null}
          <button type="button" className="btn btn-icon" aria-label="Sign out" title="Sign out" onClick={() => void logout()}>
            <Icon name="logout" size={18} />
          </button>
        </div>
      </header>
      <div style={{ padding: '0 16px' }} className="stack-tight">
        {shown.error ? <div style={{ marginTop: 12 }}><ErrorBanner error={shown.error} onRetry={shown.refetch} /></div> : null}
        {act.error instanceof ApiError && act.error.status === 409 ? (
          <div className="banner banner-warn" role="alert" style={{ marginTop: 12 }}>
            <Icon name="refresh" />
            <span>Someone else changed that order just now. The board is up to date; try again.</span>
          </div>
        ) : act.error ? (
          <div style={{ marginTop: 12 }}><ErrorBanner error={act.error} /></div>
        ) : null}
      </div>
      {!shown.orders ? (
        <Spinner label="Loading orders…" />
      ) : view === 'pay' ? (
        shown.orders.length === 0 ? (
          <EmptyState icon="checkCircle" title="Everything is paid">Orders that still owe money appear here, also after they are served or collected.</EmptyState>
        ) : (
          <>
            <p className="muted small" style={{ padding: '12px 16px 0' }}>
              {payCount} {payCount === 1 ? 'order' : 'orders'} still to pay · {formatMoney(shown.orders.reduce((s, o) => s + o.total_minor, 0), shown.orders[0]!.currency)} in total
            </p>
            <ul className="list-plain done-grid">
              {[...shown.orders].reverse().map((o) => (
                <Ticket key={o.id} order={o} now={now} showStatus onOpen={() => setOpen({ id: o.id })}>
                  {canPay && (
                    <div className="ticket-action">
                      <button type="button" className="btn btn-st" onClick={() => setOpen({ id: o.id })}>
                        <Icon name="wallet" size={18} />Take payment · {formatMoney(o.total_minor, o.currency)}
                      </button>
                    </div>
                  )}
                </Ticket>
              ))}
            </ul>
          </>
        )
      ) : view === 'history' ? (
        shown.orders.length === 0 ? (
          <EmptyState icon="receipt" title="No completed orders yet">Completed and cancelled orders appear here.</EmptyState>
        ) : (
          <ul className="list-plain done-grid">
            {shown.orders.map((o) => <Ticket key={o.id} order={o} now={now} showStatus onOpen={() => setOpen({ id: o.id })} />)}
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
                  {list.map((o) => {
                    // A pickup order is handed over at the counter: that is where its money is taken.
                    const handover = c.status === 'ready' && o.mode === 'pickup';
                    const takeMoney = handover && unpaid(o) && canPay;
                    return (
                      <Ticket key={o.id} order={o} now={now} onOpen={() => setOpen({ id: o.id })}>
                        {can('orders.update_status') && (
                          <div className="ticket-action">
                            <button type="button" className="btn btn-st" disabled={act.busy}
                              onClick={() => (takeMoney ? setOpen({ id: o.id, handover: true }) : void advance(o, c.next))}>
                              {takeMoney ? <><Icon name="wallet" size={18} />Take payment & hand over</> : handover ? 'Handed over' : c.action}
                            </button>
                          </div>
                        )}
                      </Ticket>
                    );
                  })}
                </ul>
              </section>
            );
          })}
        </div>
      )}
      {open && <OrderDrawer id={open.id} handover={open.handover} onClose={() => setOpen(null)} onChanged={refetchAll} />}
    </div>
  );
}

const payLabel: Record<StaffOrder['payment_status'], { text: string; tone: string; icon: 'wallet' | 'checkCircle' | 'clock' | 'refresh' }> = {
  unpaid: { text: 'Not paid', tone: 'badge-warn', icon: 'wallet' },
  pending: { text: 'Terminal pending', tone: 'badge-warn', icon: 'clock' },
  paid: { text: 'Paid', tone: 'badge-ok', icon: 'checkCircle' },
  failed: { text: 'Payment failed', tone: 'badge-danger', icon: 'wallet' },
  partially_refunded: { text: 'Part refunded', tone: 'badge-info', icon: 'refresh' },
  refunded: { text: 'Refunded', tone: 'badge-info', icon: 'refresh' },
};
const statusLabel: Record<OrderStatus, string> = {
  new: 'New', accepted: 'Accepted', preparing: 'Preparing', ready: 'Ready', completed: 'Completed', cancelled: 'Cancelled',
};

function Ticket({ order: o, now, onOpen, showStatus, children }: { order: StaffOrder; now: number; onOpen: () => void; showStatus?: boolean; children?: React.ReactNode }) {
  const mins = Math.max(0, Math.floor((now - new Date(o.created_at).getTime()) / 60_000));
  const open = o.status !== 'completed' && o.status !== 'cancelled';
  const timer = !open ? 'timer' : mins >= 15 ? 'timer timer-late' : mins >= 10 ? 'timer timer-warn' : 'timer';
  const where = o.dining_location?.name ?? (o.mode === 'pickup' ? 'Pickup' : 'Table');
  const pay = payLabel[o.payment_status];
  return (
    <li className={`ticket st-${o.status}`}>
      <button type="button" className="ticket-head" onClick={onOpen} aria-label={`Order ${o.number}, ${where}, ${pay.text}, details`}>
        <span className="ticket-no">#{o.number}</span>
        <span className="ticket-where">
          <strong className="truncate row-nowrap" style={{ gap: 6 }}>
            {o.mode === 'pickup' && <Icon name="bag" size={16} />}
            {where}
          </strong>
          <span className="muted tiny">{o.mode === 'pickup' ? 'Pickup' : 'Dine-in'} · {new Date(o.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
        </span>
        {open ? (
          <span className={timer}>
            <Icon name="clock" size={14} />
            {mins === 0 ? 'now' : `${mins} min`}
          </span>
        ) : (
          showStatus && <span className={`badge ${o.status === 'cancelled' ? 'badge-danger' : ''}`}>{statusLabel[o.status]}</span>
        )}
      </button>
      {showStatus && open && <div className="ticket-state"><span className="dot" />{statusLabel[o.status]}</div>}
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
      <div className={`ticket-foot pay-${o.payment_status}`}>
        <span className="row-grow strong num">{formatMoney(o.total_minor, o.currency)}</span>
        {o.status !== 'cancelled' && (
          <span className={`badge badge-pay ${pay.tone}`}><Icon name={pay.icon} size={14} />{pay.text}</span>
        )}
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
