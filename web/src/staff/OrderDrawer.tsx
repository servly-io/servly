import { useEffect, useRef, useState } from 'react';
import { ordersApi } from '../api/staff';
import type { OrderDetail, OrderStatus, PaymentMethod } from '../api/types';
import { formatMoney, parseMoney } from '../shared/money';
import { useSession } from '../shared/session';
import { Icon } from '../shared/icons';
import { MoneyInput } from '../shared/inputs';
import { ErrorBanner, Field, Spinner, useAction, useLoad } from '../shared/ui';

const late: OrderStatus[] = ['preparing', 'ready'];

/** Order details, history, cancellation and payments. */
export function OrderDrawer({ id, onClose, onChanged }: { id: string; onClose: () => void; onChanged: () => void }) {
  const { can } = useSession();
  const detail = useLoad(() => ordersApi.get(id), [id]);
  const act = useAction();
  const dialog = useRef<HTMLDialogElement>(null);
  const [method, setMethod] = useState<PaymentMethod>('cash');
  const [reference, setReference] = useState('');

  useEffect(() => {
    const d = dialog.current;
    d?.showModal();
    return () => d?.close();
  }, []);

  const apply = async (fn: () => Promise<OrderDetail>) => {
    const d = await act.run(fn);
    if (d) {
      detail.setData(d);
      onChanged();
    } else {
      await detail.reload();
    }
  };

  const o = detail.data;
  const canCancel = o && !['completed', 'cancelled'].includes(o.status) &&
    (late.includes(o.status) ? can('orders.cancel_started') : can('orders.update_status'));

  const statusTone: Record<OrderStatus, string> = {
    new: 'badge-warn', accepted: 'badge-ink', preparing: 'badge-info', ready: 'badge-ok', completed: '', cancelled: 'badge-danger',
  };
  const methods: { value: PaymentMethod; label: string; icon: 'cash' | 'card' | 'wallet' }[] = [
    { value: 'cash', label: 'Cash', icon: 'cash' },
    { value: 'external_terminal', label: 'Card terminal', icon: 'card' },
    { value: 'other', label: 'Other', icon: 'wallet' },
  ];

  return (
    <dialog ref={dialog} className="drawer" onClose={onClose} aria-labelledby="drawer-title">
      <div className="drawer-head">
        <div className="row-grow">
          <h2 id="drawer-title">{o ? `Order #${o.number}` : 'Order'}</h2>
          {o && <span className="muted small">{o.dining_location?.name ?? (o.mode === 'pickup' ? 'Pickup' : '')} · {new Date(o.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>}
        </div>
        {o && <span className={`badge ${statusTone[o.status]}`} style={{ textTransform: 'capitalize' }}>{o.status}</span>}
        <button type="button" className="btn btn-ghost btn-icon" aria-label="Close" onClick={onClose}><Icon name="x" /></button>
      </div>
      <div className="drawer-body">
        {!o ? (
          detail.error ? <ErrorBanner error={detail.error} /> : <Spinner />
        ) : (
          <>
            <ErrorBanner error={act.error} />
            <section className="card stack-tight">
              <ul className="lines">
                {o.items.map((l, i) => (
                  <li key={i}>
                    <span className="qty">{l.quantity}×</span>
                    <span className="row-grow">
                      {l.name}
                      {l.options.length > 0 && <span className="ticket-opts">{l.options.map((x) => x.option_name).join(', ')}</span>}
                      {l.notes && <span className="note"><Icon name="note" size={14} />{l.notes}</span>}
                    </span>
                    <span className="num">{formatMoney(l.line_total_minor, o.currency)}</span>
                  </li>
                ))}
              </ul>
              {o.notes && <p className="note"><Icon name="note" size={14} />{o.notes}</p>}
              <div className="totals"><span>Total</span><span className="num">{formatMoney(o.total_minor, o.currency)}</span></div>
            </section>

            {can('payments.record') && o.outstanding_minor > 0 && (
              <form className="card stack" onSubmit={(e) => { e.preventDefault(); void apply(() => ordersApi.pay(o.id, { method, status: 'paid', amount_minor: o.outstanding_minor, reference })); }}>
                <div>
                  <h3>Record payment</h3>
                  <p className="muted small card-sub">{formatMoney(o.outstanding_minor, o.currency)} open. This records money taken at the till or on the card terminal; servly does not check the terminal.</p>
                </div>
                <div className="method-picker" role="group" aria-label="Payment method">
                  {methods.map((m) => (
                    <button key={m.value} type="button" aria-pressed={method === m.value} onClick={() => setMethod(m.value)}>
                      <Icon name={m.icon} />
                      {m.label}
                    </button>
                  ))}
                </div>
                <input aria-label="Terminal receipt number (optional, never card numbers)" placeholder="Receipt no. (optional)" maxLength={100}
                  value={reference} onChange={(e) => setReference(e.target.value)} />
                <div className="row">
                  <button type="submit" className="btn btn-accent btn-large row-grow" disabled={act.busy}>
                    <Icon name="check" />Paid {formatMoney(o.outstanding_minor, o.currency)}
                  </button>
                  {method === 'external_terminal' && (
                    <button type="button" className="btn btn-large" disabled={act.busy}
                      onClick={() => void apply(() => ordersApi.pay(o.id, { method, status: 'pending', amount_minor: o.outstanding_minor, reference }))}>
                      Waiting for terminal
                    </button>
                  )}
                </div>
              </form>
            )}

            {o.payments.length > 0 && (
              <section className="card stack-tight">
                <h3>Payments</h3>
                <ul className="list-plain stack-tight">
                  {o.payments.map((p) => (
                    <li key={p.id} className="stack-tight" style={{ paddingTop: 8, borderTop: '1px solid var(--border)' }}>
                      <div className="row">
                        <strong className="num">{formatMoney(p.amount_minor, p.currency)}</strong>
                        <span className="muted small row-grow">{p.method === 'external_terminal' ? 'Card terminal' : p.method === 'cash' ? 'Cash' : 'Other'}{p.recorded_by && ` · ${p.recorded_by.name}`}</span>
                        <span className={`badge ${p.status === 'paid' ? 'badge-ok' : p.status === 'pending' ? 'badge-warn' : p.status === 'failed' ? 'badge-danger' : 'badge-info'}`}>{p.status.replace('_', ' ')}</span>
                      </div>
                      {p.refunded_minor > 0 && <span className="muted small">Refunded {formatMoney(p.refunded_minor, p.currency)}</span>}
                      {p.reference && <span className="muted small">Ref: {p.reference}</span>}
                      {p.status === 'pending' && can('payments.record') && (
                        <div className="row">
                          <button type="button" className="btn btn-accent" disabled={act.busy} onClick={() => void apply(() => ordersApi.settle(p.id, 'paid'))}>Terminal approved</button>
                          <button type="button" className="btn btn-danger" disabled={act.busy} onClick={() => void apply(() => ordersApi.settle(p.id, 'failed'))}>Terminal declined</button>
                        </div>
                      )}
                      {(p.status === 'paid' || p.status === 'partially_refunded') && can('payments.refund') && (
                        <RefundForm currency={p.currency} max={p.amount_minor - p.refunded_minor} busy={act.busy}
                          onRefund={(amount, reason) => void apply(() => ordersApi.refund(p.id, amount, reason))} />
                      )}
                    </li>
                  ))}
                </ul>
              </section>
            )}

            <section className="card stack-tight">
              <h3>History</h3>
              <ol className="timeline">
                {o.events.map((e, i) => (
                  <li key={i}>
                    <span className="strong" style={{ textTransform: 'capitalize' }}>{e.from ? e.to : 'Placed'}</span>
                    <span className="muted"> · {new Date(e.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}{e.actor && ` · ${e.actor.name}`}</span>
                    {e.reason && <div className="muted small">“{e.reason}”</div>}
                  </li>
                ))}
              </ol>
            </section>

            {canCancel && (
              <button type="button" className="btn btn-danger" disabled={act.busy} onClick={() => {
                const reason = window.prompt(`Cancel order #${o.number}? Reason:`);
                if (reason?.trim()) void apply(() => ordersApi.setStatus(o.id, 'cancelled', o.version, reason));
              }}><Icon name="xCircle" size={18} />Cancel order</button>
            )}
          </>
        )}
      </div>
    </dialog>
  );
}

function RefundForm({ currency, max, busy, onRefund }: { currency: string; max: number; busy: boolean; onRefund: (amount: number, reason: string) => void }) {
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const minor = parseMoney(amount, currency);
  return (
    <form className="row" style={{ alignItems: 'end' }} onSubmit={(e) => { e.preventDefault(); if (minor) onRefund(minor, reason); }}>
      <Field label={`Refund (max ${formatMoney(max, currency)})`} error={minor !== null && minor > max ? `At most ${formatMoney(max, currency)}` : undefined}>
        <MoneyInput currency={currency} value={amount} required placeholder="0.00" onChange={setAmount} />
      </Field>
      <Field label="Reason"><input value={reason} required maxLength={200} onChange={(e) => setReason(e.target.value)} /></Field>
      <button type="submit" className="btn" style={{ minHeight: 44 }} disabled={busy || !minor || minor <= 0 || minor > max}>Refund</button>
    </form>
  );
}
