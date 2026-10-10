import { useEffect, useRef, useState } from 'react';
import { ordersApi } from '../api/staff';
import type { OrderDetail, OrderStatus, PaymentMethod } from '../api/types';
import { formatMoney, parseMoney } from '../shared/money';
import { useSession } from '../shared/session';
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

  return (
    <dialog ref={dialog} className="sheet drawer" onClose={onClose} aria-labelledby="drawer-title">
      {!o ? (
        detail.error ? <ErrorBanner error={detail.error} /> : <Spinner />
      ) : (
        <div className="stack">
          <div className="row">
            <h2 id="drawer-title" className="row-grow">Order #{o.number} · {o.dining_location?.name ?? o.mode}</h2>
            <button type="button" className="btn" onClick={onClose}>Close</button>
          </div>
          <p className="muted">Status: <strong>{o.status}</strong> · Payment: <strong>{o.payment_status.replace('_', ' ')}</strong></p>
          <ul className="list-plain stack-tight">
            {o.items.map((l, i) => (
              <li key={i} className="row">
                <span className="row-grow">{l.quantity}× {l.name}{l.options.length > 0 && <span className="muted"> · {l.options.map((x) => x.option_name).join(', ')}</span>}
                  {l.notes && <div className="note">“{l.notes}”</div>}</span>
                <span>{formatMoney(l.line_total_minor, o.currency)}</span>
              </li>
            ))}
          </ul>
          {o.notes && <p className="note">Order note: “{o.notes}”</p>}
          <p className="total row"><span className="row-grow">Total</span><strong>{formatMoney(o.total_minor, o.currency)}</strong></p>
          <ErrorBanner error={act.error} />

          {can('payments.record') && o.outstanding_minor > 0 && (
            <form className="card stack-tight" onSubmit={(e) => { e.preventDefault(); void apply(() => ordersApi.pay(o.id, { method, status: 'paid', amount_minor: o.outstanding_minor, reference })); }}>
              <h3>Record payment of {formatMoney(o.outstanding_minor, o.currency)}</h3>
              <p className="muted small">Records money taken at the till or on the card terminal. servly does not check the terminal.</p>
              <div className="row">
                <select aria-label="Method" value={method} onChange={(e) => setMethod(e.target.value as PaymentMethod)}>
                  <option value="cash">Cash</option>
                  <option value="external_terminal">Card terminal</option>
                  <option value="other">Other</option>
                </select>
                <input aria-label="Terminal receipt number (optional, never card numbers)" placeholder="Receipt no. (optional)" maxLength={100}
                  value={reference} onChange={(e) => setReference(e.target.value)} />
              </div>
              <div className="row">
                <button type="submit" className="btn btn-primary btn-large" disabled={act.busy}>Paid</button>
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
            <section>
              <h3>Payments</h3>
              <ul className="list-plain stack-tight">
                {o.payments.map((p) => (
                  <li key={p.id} className="card stack-tight">
                    <div className="row">
                      <span className="row-grow">{formatMoney(p.amount_minor, p.currency)} · {p.method.replace('_', ' ')} · <strong>{p.status.replace('_', ' ')}</strong>
                        {p.refunded_minor > 0 && ` · refunded ${formatMoney(p.refunded_minor, p.currency)}`}</span>
                      <span className="muted small">{p.recorded_by?.name}</span>
                    </div>
                    {p.reference && <span className="muted small">Ref: {p.reference}</span>}
                    {p.status === 'pending' && can('payments.record') && (
                      <div className="row">
                        <button type="button" className="btn" disabled={act.busy} onClick={() => void apply(() => ordersApi.settle(p.id, 'paid'))}>Terminal approved</button>
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

          {canCancel && (
            <button type="button" className="btn btn-danger" disabled={act.busy} onClick={() => {
              const reason = window.prompt(`Cancel order #${o.number}? Reason:`);
              if (reason?.trim()) void apply(() => ordersApi.setStatus(o.id, 'cancelled', o.version, reason));
            }}>Cancel order</button>
          )}

          <section>
            <h3>History</h3>
            <ol className="history">
              {o.events.map((e, i) => (
                <li key={i}>
                  {new Date(e.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} · {e.from ? `${e.from} → ${e.to}` : 'placed'}
                  {e.actor && ` by ${e.actor.name}`}{e.reason && ` (${e.reason})`}
                </li>
              ))}
            </ol>
          </section>
        </div>
      )}
    </dialog>
  );
}

function RefundForm({ currency, max, busy, onRefund }: { currency: string; max: number; busy: boolean; onRefund: (amount: number, reason: string) => void }) {
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const minor = parseMoney(amount, currency);
  return (
    <form className="row" onSubmit={(e) => { e.preventDefault(); if (minor) onRefund(minor, reason); }}>
      <Field label={`Refund (max ${formatMoney(max, currency)})`}><input inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} /></Field>
      <Field label="Reason"><input value={reason} required onChange={(e) => setReason(e.target.value)} /></Field>
      <button type="submit" className="btn" disabled={busy || !minor || minor <= 0 || minor > max}>Refund</button>
    </form>
  );
}
