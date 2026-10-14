import { useState } from 'react';
import { paymentsApi } from '../api/admin';
import type { PaymentAccount, PaymentMarket, PaymentsOverview, PickupFlow, TableFlow } from '../api/types';
import { Icon } from '../shared/icons';
import { EmptyState, ErrorBanner, Field, PageHeader, Spinner, useAction, useLoad } from '../shared/ui';
import { useAdmin } from './AdminLayout';

const providerNames: Record<string, string> = { stripe: 'Stripe', xendit: 'Xendit', midtrans: 'Midtrans', sandbox: 'Sandbox' };

const tableFlows: { value: TableFlow; title: string; text: string }[] = [
  { value: 'after', title: 'Pay from the phone when finished', text: 'Guests order as now and pay on their phone whenever they like. Staff can still take cash.' },
  { value: 'before', title: 'Pay when ordering', text: 'The order reaches the kitchen only once it is paid.' },
  { value: 'staff', title: 'Staff only', text: 'No online payment at tables.' },
];
const pickupFlows: { value: PickupFlow; title: string; text: string }[] = [
  { value: 'before', title: 'Pay when ordering', text: 'Recommended for pickup: the kitchen only sees paid orders.' },
  { value: 'either', title: 'Guest chooses', text: 'Pay now on the phone, or at the counter when collecting.' },
  { value: 'counter', title: 'At the counter', text: 'No online payment for pickup.' },
];

/** Settings > Payments: a setup wizard, then the connected account. */
export function PaymentsPage() {
  const data = useLoad(() => paymentsApi.overview());
  if (!data.data) return data.error ? <ErrorBanner error={data.error} onRetry={data.reload} /> : <Spinner />;
  const o = data.data;
  return (
    <section className="stack">
      <PageHeader title="Payments" subtitle="Let guests pay on their phone: cards, Apple Pay, Google Pay and the methods your country uses most." />
      {!o.available ? (
        <div className="card">
          <EmptyState icon="wallet" title="Online payments are switched off on this server">
            The person who runs servly sets a secret key first (<code>SERVLY_SECRET_KEY</code>, for example from <code>openssl rand -base64 32</code>) and restarts it. Cash and card-terminal payments work without it.
          </EmptyState>
        </div>
      ) : o.account ? (
        <Connected account={o.account} market={marketFor(o, o.account.country)} onChange={data.reload} />
      ) : (
        <Wizard overview={o} onDone={data.reload} />
      )}
    </section>
  );
}

function marketFor(o: PaymentsOverview, country: string): PaymentMarket | undefined {
  const eu = ['AT', 'BE', 'BG', 'HR', 'CY', 'CZ', 'DK', 'EE', 'FI', 'FR', 'DE', 'GR', 'HU', 'IE', 'IT', 'LV', 'LT', 'LU', 'MT', 'NL', 'PL', 'PT', 'RO', 'SK', 'SI', 'ES', 'SE'];
  return o.markets.find((m) => m.code === country) ?? (eu.includes(country) ? o.markets.find((m) => m.code === 'EU') : undefined);
}

function Wizard({ overview, onDone }: { overview: PaymentsOverview; onDone: () => void }) {
  const { settings } = useAdmin();
  const suggested = overview.markets.find((m) => m.currencies?.includes(settings.currency)) ?? overview.markets[overview.markets.length - 1]!;
  const [step, setStep] = useState(0);
  const [market, setMarket] = useState<PaymentMarket>(suggested);
  const [providerId, setProviderId] = useState('');
  const [secretKey, setSecretKey] = useState('');
  const [webhookSecret, setWebhookSecret] = useState('');
  const [tableFlow, setTableFlow] = useState<TableFlow>('after');
  const [pickupFlow, setPickupFlow] = useState<PickupFlow>('before');
  const act = useAction();
  const steps = ['Country', 'Provider', 'Connect', 'Guests'];

  const pick = (id: string) => {
    setProviderId(id);
    setSecretKey(id === 'sandbox' ? 'sandbox' : '');
    setStep(2);
  };
  const connect = async (e: React.FormEvent) => {
    e.preventDefault();
    if (step === 2) return setStep(3);
    const ok = await act.run(() =>
      paymentsApi.connect({ provider: providerId, secret_key: secretKey, webhook_secret: webhookSecret || undefined, table_flow: tableFlow, pickup_flow: pickupFlow }),
    );
    if (ok) onDone();
  };

  return (
    <form className="card stack" onSubmit={connect}>
      <div>
        <p className="eyebrow">Step {step + 1} of {steps.length} · {steps[step]}</p>
        <ol className="wizard-steps" aria-label="Setup progress">
          {steps.map((s, i) => <li key={s} className={i <= step ? 'reached' : ''} aria-current={i === step ? 'step' : undefined}><span className="visually-hidden">{s}</span></li>)}
        </ol>
      </div>

      {step === 0 && (
        <>
          <div>
            <h2>Where is the restaurant?</h2>
            <p className="muted small card-sub">Guests in every country pay differently. servly suggests the providers that cover what your guests use most.</p>
          </div>
          <div className="pick-grid" role="radiogroup" aria-label="Country or region">
            {overview.markets.map((m) => (
              <label key={m.code} className={`pick${market.code === m.code ? ' picked' : ''}`}>
                <input type="radio" name="market" className="visually-hidden" checked={market.code === m.code} onChange={() => setMarket(m)} />
                <strong>{m.name}</strong>
                <span className="muted small">{m.methods.slice(0, 2).join(' · ')}</span>
              </label>
            ))}
          </div>
          <div className="wizard-actions"><span /><button type="button" className="btn btn-primary" onClick={() => setStep(1)}>Continue</button></div>
        </>
      )}

      {step === 1 && (
        <>
          <div>
            <h2>Choose a payment provider</h2>
            <p className="muted small card-sub">
              What guests in {market.name} use most: {market.methods.join(', ')}. The provider pays the money out to your bank; servly never holds it.
            </p>
          </div>
          <ul className="list-plain stack-tight">
            {market.providers.map((p) => (
              <li key={p.id} className={`provider${p.available ? '' : ' unavailable'}`}>
                <div className="row-grow stack-tight" style={{ gap: 4 }}>
                  <div className="row" style={{ gap: 8 }}>
                    <strong>{p.name}</strong>
                    {p.recommended && <span className="badge badge-ok">Recommended</span>}
                    {!p.available && <span className="badge">Coming later</span>}
                  </div>
                  <span className="muted small">{p.methods.join(' · ')}</span>
                  {p.note && <span className="small">{p.note}</span>}
                </div>
                <button type="button" className={`btn ${p.recommended ? 'btn-primary' : ''}`} disabled={!p.available} onClick={() => pick(p.id)}>
                  Connect
                </button>
              </li>
            ))}
            {overview.sandbox && (
              <li className="provider">
                <div className="row-grow stack-tight" style={{ gap: 4 }}>
                  <div className="row" style={{ gap: 8 }}><strong>Sandbox</strong><span className="badge badge-warn">No real money</span></div>
                  <span className="muted small">A fake checkout page to try the guest flow. For demos and testing only.</span>
                </div>
                <button type="button" className="btn" onClick={() => pick('sandbox')}>Use sandbox</button>
              </li>
            )}
          </ul>
          <div className="wizard-actions"><button type="button" className="btn" onClick={() => setStep(0)}>Back</button><span /></div>
        </>
      )}

      {step === 2 && (
        <>
          <h2>Connect {providerNames[providerId] ?? providerId}</h2>
          {providerId === 'stripe' && (
            <>
              <ol className="steps-list">
                <li>Sign in at <a className="text-link" href="https://dashboard.stripe.com/test/apikeys" target="_blank" rel="noreferrer">dashboard.stripe.com</a>. Start in <strong>test mode</strong>: you can switch to live keys later.</li>
                <li>Under <strong>Developers → API keys</strong>, create a <strong>restricted key</strong> with write access to <em>Checkout Sessions</em> and <em>Refunds</em>, and read access to <em>Payment Intents</em> (and <em>Account</em>, to show your business name).</li>
                <li>Paste the key here. It is stored encrypted and never shown again.</li>
              </ol>
              <Field label="Restricted key" hint="Starts with rk_test_ (test) or rk_live_ (live). A secret key (sk_) works too.">
                <input required autoComplete="off" spellCheck={false} placeholder="rk_test_…" value={secretKey} onChange={(e) => setSecretKey(e.target.value.trim())} />
              </Field>
              <Field label="Webhook signing secret (optional)" hint="Makes payments show up instantly. You can add it later; until then servly checks open payments every few seconds.">
                <input autoComplete="off" spellCheck={false} placeholder="whsec_…" value={webhookSecret} onChange={(e) => setWebhookSecret(e.target.value.trim())} />
              </Field>
              <p className="muted small">Payment methods (cards, Apple Pay, Google Pay, {market.methods.slice(1, 3).join(', ')}) are switched on in your Stripe dashboard under <strong>Settings → Payment methods</strong>; the checkout offers what fits each guest.</p>
            </>
          )}
          {providerId === 'xendit' && (
            <>
              <ol className="steps-list">
                <li>Sign in at <a className="text-link" href="https://dashboard.xendit.co/settings/developers#api-keys" target="_blank" rel="noreferrer">dashboard.xendit.co</a> in <strong>test mode</strong>: you can switch to the live key later.</li>
                <li>Under <strong>Settings → API keys</strong>, generate a <strong>secret key</strong> with <em>write</em> permission for <em>Payment Sessions</em> (Money-in) and <em>Refunds</em>.</li>
                <li>Paste it here. It is stored encrypted and never shown again.</li>
              </ol>
              <Field label="Secret key" hint="Starts with xnd_development_ (test) or xnd_production_ (live).">
                <input required autoComplete="off" spellCheck={false} placeholder="xnd_development_…" value={secretKey} onChange={(e) => setSecretKey(e.target.value.trim())} />
              </Field>
              <Field label="Webhook verification token (optional)" hint="Settings → Webhooks. Makes payments show up instantly; until then servly checks open payments every few seconds.">
                <input autoComplete="off" spellCheck={false} value={webhookSecret} onChange={(e) => setWebhookSecret(e.target.value.trim())} />
              </Field>
              <p className="muted small">QRIS, e-wallets (OVO, DANA, ShopeePay, LinkAja), virtual accounts and cards are switched on in your Xendit dashboard under <strong>Payment channels</strong>; the checkout offers what is active.</p>
            </>
          )}
          {providerId === 'midtrans' && (
            <>
              <ol className="steps-list">
                <li>Sign in at <a className="text-link" href="https://dashboard.sandbox.midtrans.com/settings/config_info" target="_blank" rel="noreferrer">dashboard.sandbox.midtrans.com</a> (sandbox) first: you can switch to production later.</li>
                <li>Under <strong>Settings → Access keys</strong>, copy the <strong>server key</strong> (not the client key).</li>
                <li>Paste it here. It is stored encrypted and never shown again.</li>
              </ol>
              <Field label="Server key" hint="Starts with SB-Mid-server- (sandbox) or Mid-server- (production).">
                <input required autoComplete="off" spellCheck={false} placeholder="SB-Mid-server-…" value={secretKey} onChange={(e) => setSecretKey(e.target.value.trim())} />
              </Field>
              <p className="muted small">QRIS, GoPay, ShopeePay, virtual accounts and cards are switched on in your Midtrans dashboard under <strong>Settings → Snap preferences</strong>; the payment page offers what is active.</p>
            </>
          )}
          {providerId === 'sandbox' && <p className="muted">Nothing to connect: the sandbox runs inside servly. Guests see a test page with Pay and Decline buttons.</p>}
          <div className="wizard-actions">
            <button type="button" className="btn" onClick={() => setStep(1)}>Back</button>
            <button type="submit" className="btn btn-primary" disabled={!secretKey}>Continue</button>
          </div>
        </>
      )}

      {step === 3 && (
        <>
          <h2>When do guests pay?</h2>
          <FlowPicker legend="At tables" options={tableFlows} value={tableFlow} onChange={setTableFlow} />
          <FlowPicker legend="At pickup points" options={pickupFlows} value={pickupFlow} onChange={setPickupFlow} />
          <ErrorBanner error={act.error} />
          <div className="wizard-actions">
            <button type="button" className="btn" onClick={() => setStep(2)} disabled={act.busy}>Back</button>
            <button type="submit" className="btn btn-primary" disabled={act.busy}>{act.busy ? 'Checking with the provider…' : 'Connect'}</button>
          </div>
        </>
      )}
    </form>
  );
}

function FlowPicker<T extends string>({ legend, options, value, onChange }: { legend: string; options: { value: T; title: string; text: string }[]; value: T; onChange: (v: T) => void }) {
  return (
    <fieldset className="stack-tight">
      <legend className="field-label" style={{ marginBottom: 8 }}>{legend}</legend>
      {options.map((o) => (
        <label key={o.value} className={`pick pick-row${value === o.value ? ' picked' : ''}`}>
          <input type="radio" name={legend} checked={value === o.value} onChange={() => onChange(o.value)} />
          <span className="stack-tight" style={{ gap: 2 }}>
            <strong>{o.title}</strong>
            <span className="muted small">{o.text}</span>
          </span>
        </label>
      ))}
    </fieldset>
  );
}

function Connected({ account: a, market, onChange }: { account: PaymentAccount; market?: PaymentMarket; onChange: () => void }) {
  const [tableFlow, setTableFlow] = useState(a.table_flow);
  const [pickupFlow, setPickupFlow] = useState(a.pickup_flow);
  const [secret, setSecret] = useState('');
  const flows = useAction();
  const hook = useAction();
  const off = useAction();
  const dirty = tableFlow !== a.table_flow || pickupFlow !== a.pickup_flow;

  return (
    <>
      <section className="card">
        <div className="row" style={{ gap: 14 }}>
          <span className="launch-icon"><Icon name="wallet" /></span>
          <div className="row-grow">
            <div className="row" style={{ gap: 8 }}>
              <strong>{providerNames[a.provider] ?? a.provider}</strong>
              <span className={`badge ${a.mode === 'live' ? 'badge-ok' : 'badge-warn'}`}>{a.mode === 'live' ? 'Live' : 'Test mode'}</span>
            </div>
            <span className="muted small">{[a.account_name, a.country, a.account_ref].filter(Boolean).join(' · ')}</span>
          </div>
        </div>
        {a.mode === 'test' && a.provider !== 'sandbox' && (
          <p className="banner banner-warn" style={{ marginTop: 16 }}><Icon name="alert" /><span>Test mode: guests cannot pay real money yet. When the test order works, reconnect with your live key.</span></p>
        )}
      </section>

      <form className="card stack" onSubmit={(e) => { e.preventDefault(); void flows.run(async () => { await paymentsApi.update({ table_flow: tableFlow, pickup_flow: pickupFlow }); onChange(); }); }}>
        <h2>When guests pay</h2>
        <FlowPicker legend="At tables" options={tableFlows} value={tableFlow} onChange={setTableFlow} />
        <FlowPicker legend="At pickup points" options={pickupFlows} value={pickupFlow} onChange={setPickupFlow} />
        <ErrorBanner error={flows.error} />
        {dirty && <div><button type="submit" className="btn btn-primary" disabled={flows.busy}>Save</button></div>}
      </form>

      {a.provider !== 'sandbox' && (
        <form className="card stack" onSubmit={(e) => { e.preventDefault(); void hook.run(async () => { await paymentsApi.update({ webhook_secret: secret }); setSecret(''); onChange(); }); }}>
          <div className="row">
            <h2 className="row-grow">Instant confirmation (webhooks)</h2>
            {a.provider !== 'midtrans' && <span className={`badge ${a.webhooks ? 'badge-ok' : ''}`}>{a.webhooks ? 'On' : 'Off'}</span>}
          </div>
          <p className="muted small">
            {a.webhooks
              ? 'The provider tells servly about each payment the moment it happens.'
              : 'servly checks open payments every few seconds, so this works without webhooks. Webhooks make payments appear instantly, but need servly to be reachable from the internet over HTTPS.'}
          </p>
          <ol className="steps-list">
            {a.provider === 'stripe' && <li>In Stripe, open <strong>Developers → Webhooks → Add endpoint</strong> and paste this address:</li>}
            {a.provider === 'xendit' && <li>In Xendit, open <strong>Settings → Webhooks</strong> and paste this address for <strong>Payment Session</strong> events:</li>}
            {a.provider === 'midtrans' && <li>In Midtrans, open <strong>Settings → Payment → Notification URL</strong> and paste this address as the payment notification URL:</li>}
          </ol>
          <code className="copy-box break">{a.webhook_url}</code>
          {a.provider === 'midtrans' ? (
            <p className="muted small">Midtrans signs notifications with your server key, so there is no secret to copy.</p>
          ) : (
            <>
              <ol className="steps-list" start={2}>
                {a.provider === 'stripe' && <li>Select the events <code>checkout.session.completed</code>, <code>checkout.session.async_payment_succeeded</code>, <code>checkout.session.async_payment_failed</code> and <code>checkout.session.expired</code>.</li>}
                <li>{a.provider === 'xendit' ? 'Copy the webhook verification token on the same page here.' : "Copy the endpoint's signing secret here."}</li>
              </ol>
              <div className="add-bar">
                <Field label={a.provider === 'xendit' ? 'Verification token' : 'Signing secret'}>
                  <input autoComplete="off" spellCheck={false} placeholder={a.provider === 'stripe' ? 'whsec_…' : ''} value={secret} onChange={(e) => setSecret(e.target.value.trim())} />
                </Field>
                <button type="submit" className="btn" style={{ minHeight: 44 }} disabled={hook.busy || !secret}>Save</button>
              </div>
            </>
          )}
          <ErrorBanner error={hook.error} />
        </form>
      )}

      {market && a.provider !== 'sandbox' && (
        <section className="card stack-tight">
          <h2>Payment methods</h2>
          <p className="muted small">Guests in {market.name} pay most with: {market.methods.join(', ')}. Switch methods on or off in your {providerNames[a.provider]} dashboard; the checkout shows each guest what fits them.</p>
        </section>
      )}

      <section className="card stack-tight">
        <h2>Disconnect</h2>
        <p className="muted small">Guests can no longer pay online. Past payments stay recorded, and refunds of online payments need the account connected again.</p>
        <ErrorBanner error={off.error} />
        <div>
          <button type="button" className="btn btn-danger" disabled={off.busy} onClick={() => {
            if (window.confirm(`Disconnect ${providerNames[a.provider] ?? a.provider}? Guests can no longer pay online.`)) void off.run(async () => { await paymentsApi.disconnect(); onChange(); });
          }}>Disconnect</button>
        </div>
      </section>
    </>
  );
}
