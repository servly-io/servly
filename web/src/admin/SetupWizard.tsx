import { useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { settingsApi, setupApi } from '../api/admin';
import { ApiError } from '../api/client';
import { CurrencySelect, TimezoneSelect } from '../shared/inputs';
import { useSession } from '../shared/session';
import { ErrorBanner, Field, Spinner, Toggle, useAction } from '../shared/ui';

const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone ?? '';

const steps = ['Welcome', 'Your account', 'Restaurant', 'Ordering', 'Review'] as const;

// The step that holds the field a server error code is about.
const stepOfError: Record<string, number> = {
  setup_token_invalid: 0,
  invalid_email: 1,
  email_taken: 1,
  password_too_short: 1,
  password_too_long: 1,
  name_required: 2,
  invalid_slug: 2,
  invalid_currency: 2,
  invalid_timezone: 2,
};

/** First-run setup: claims a fresh installation and creates its first restaurant and owner. */
export function SetupWizard() {
  const { setupRequired, completeSetup } = useSession();
  const navigate = useNavigate();
  const act = useAction();
  const [step, setStep] = useState(0);
  const [f, setF] = useState({
    setup_token: '',
    owner_name: '',
    owner_email: '',
    owner_password: '',
    confirm: '',
    restaurant_name: '',
    currency: 'EUR',
    timezone: browserZone,
    order_notes_enabled: true,
    public_status_enabled: true,
  });
  const heading = useRef<HTMLHeadingElement>(null);
  const set = (patch: Partial<typeof f>) => setF({ ...f, ...patch });

  if (setupRequired === undefined) return <Spinner />;
  if (!setupRequired && !act.busy) {
    return (
      <main className="auth">
        <div className="auth-card">
          <h1>servly is already set up</h1>
          <p>Sign in with your account. If nobody can sign in, ask the person who runs the server to reset the owner’s password.</p>
          <Link className="btn btn-primary btn-large" to="/login">Sign in</Link>
        </div>
      </main>
    );
  }

  const go = (to: number) => {
    setStep(to);
    // Move focus to the new step's heading for screen-reader and keyboard users.
    requestAnimationFrame(() => heading.current?.focus());
  };

  const next = (e: React.FormEvent) => {
    e.preventDefault();
    act.setError(null);
    if (step === 0) {
      // Check the token now, not after the owner has filled in every step.
      void act.run(async () => {
        await setupApi.verify(f.setup_token);
        go(1);
      });
      return;
    }
    if (step === 1 && f.owner_password !== f.confirm) {
      act.setError(new Error('The two passwords are not the same.'));
      return;
    }
    if (step < steps.length - 1) {
      go(step + 1);
      return;
    }
    void act.run(async () => {
      try {
        await completeSetup({
          setup_token: f.setup_token,
          restaurant_name: f.restaurant_name,
          currency: f.currency,
          timezone: f.timezone,
          owner_name: f.owner_name,
          owner_email: f.owner_email,
          owner_password: f.owner_password,
        });
      } catch (err) {
        if (err instanceof ApiError && stepOfError[err.code] !== undefined) go(stepOfError[err.code]!);
        throw err;
      }
      if (!f.order_notes_enabled || !f.public_status_enabled) {
        // The restaurant exists now; if this fails, Settings shows the switches to set by hand.
        const saved = await settingsApi
          .update({ order_notes_enabled: f.order_notes_enabled, public_status_enabled: f.public_status_enabled })
          .catch(() => null);
        if (!saved) return navigate('/admin/settings', { replace: true });
      }
      navigate('/admin', { replace: true, state: { welcome: true } });
    });
  };

  return (
    <main className="wizard">
      <p className="wizard-brand">
        <span className="logo">s</span>servly <span className="eyebrow">Setup</span>
      </p>
      <form className="wizard-card stack" onSubmit={next}>
        <div>
          <p className="eyebrow">
            Step {step + 1} of {steps.length} · {steps[step]}
          </p>
          <ol className="wizard-steps" aria-label="Setup progress">
            {steps.map((s, i) => (
              <li key={s} className={i <= step ? 'reached' : ''} aria-current={i === step ? 'step' : undefined}>
                <span className="visually-hidden">
                  {s}
                  {i < step ? ' (done)' : ''}
                </span>
              </li>
            ))}
          </ol>
        </div>

        {step === 0 && (
          <>
            <h1 ref={heading} tabIndex={-1}>Welcome to servly</h1>
            <p>Let's get your restaurant ready. This takes about five minutes. You can add the menu, tables and staff afterwards.</p>
            <ol className="wizard-intro">
              <li>Create your owner account</li>
              <li>Name your restaurant</li>
              <li>Choose how guests order</li>
            </ol>
            <Field
              label="Setup token"
              hint="This proves you run the server. Find it in the server log, in the line that starts the first-run setup: podman logs servly, or journalctl -u servly."
            >
              <input
                required
                autoFocus
                autoComplete="off"
                spellCheck={false}
                placeholder="xxxxx-xxxxx-xxxxx-xxxxx"
                value={f.setup_token}
                onChange={(e) => set({ setup_token: e.target.value })}
              />
            </Field>
          </>
        )}

        {step === 1 && (
          <>
            <h1 ref={heading} tabIndex={-1}>Create your owner account</h1>
            <p className="muted">The owner manages the menu, tables, staff and settings. You sign in with this email.</p>
            <Field label="Your name">
              <input required maxLength={120} autoComplete="name" value={f.owner_name} onChange={(e) => set({ owner_name: e.target.value })} />
            </Field>
            <Field label="Email">
              <input type="email" required autoComplete="username" value={f.owner_email} onChange={(e) => set({ owner_email: e.target.value })} />
            </Field>
            <Field label="Password" hint="At least 10 characters. A few unrelated words work well.">
              <input
                type="password"
                required
                minLength={10}
                autoComplete="new-password"
                value={f.owner_password}
                onChange={(e) => set({ owner_password: e.target.value })}
              />
            </Field>
            <Field label="Repeat password">
              <input type="password" required minLength={10} autoComplete="new-password" value={f.confirm} onChange={(e) => set({ confirm: e.target.value })} />
            </Field>
          </>
        )}

        {step === 2 && (
          <>
            <h1 ref={heading} tabIndex={-1}>Your restaurant</h1>
            <Field label="Restaurant name" hint="Guests see it on the menu.">
              <input required maxLength={120} autoComplete="organization" value={f.restaurant_name} onChange={(e) => set({ restaurant_name: e.target.value })} />
            </Field>
            <div className="row">
              <Field label="Currency" hint="All prices use it. It can change until the first order.">
                <CurrencySelect value={f.currency} onChange={(v) => set({ currency: v })} />
              </Field>
              <Field label="Timezone" hint="Order numbers restart at midnight here.">
                <TimezoneSelect value={f.timezone} onChange={(v) => set({ timezone: v })} />
              </Field>
            </div>
          </>
        )}

        {step === 3 && (
          <>
            <h1 ref={heading} tabIndex={-1}>How guests order</h1>
            <p className="muted">Guests scan a QR code on the table, choose from the menu and send the order to your staff. They pay at the counter or at the table, as you do today.</p>
            <Toggle label="Guests can add notes to orders, such as “no onions”" checked={f.order_notes_enabled} onChange={(v) => set({ order_notes_enabled: v })} />
            <Toggle label="Guests can follow their order status on their phone" checked={f.public_status_enabled} onChange={(v) => set({ public_status_enabled: v })} />
            <p className="muted small">You can change both later in Settings.</p>
          </>
        )}

        {step === 4 && (
          <>
            <h1 ref={heading} tabIndex={-1}>Ready to create your restaurant</h1>
            <dl className="summary">
              <SummaryRow label="Owner" value={`${f.owner_name} · ${f.owner_email}`} onEdit={() => go(1)} />
              <SummaryRow label="Restaurant" value={f.restaurant_name} onEdit={() => go(2)} />
              <SummaryRow label="Currency and timezone" value={`${f.currency} · ${f.timezone}`} onEdit={() => go(2)} />
              <SummaryRow
                label="Ordering"
                value={[f.order_notes_enabled ? 'notes on' : 'notes off', f.public_status_enabled ? 'status page on' : 'status page off'].join(' · ')}
                onEdit={() => go(3)}
              />
            </dl>
            <p className="muted small">Next you add the menu and tables. A checklist guides you.</p>
          </>
        )}

        <ErrorBanner error={act.error} />
        <div className="wizard-actions">
          {step > 0 ? (
            <button type="button" className="btn" disabled={act.busy} onClick={() => go(step - 1)}>
              Back
            </button>
          ) : (
            <span />
          )}
          <button type="submit" className="btn btn-primary btn-large" disabled={act.busy}>
            {step === 0 ? (act.busy ? 'Checking…' : 'Get started') : step < steps.length - 1 ? 'Continue' : act.busy ? 'Creating…' : 'Create restaurant'}
          </button>
        </div>
      </form>
    </main>
  );
}

function SummaryRow({ label, value, onEdit }: { label: string; value: string; onEdit: () => void }) {
  return (
    <div className="summary-row">
      <dt>{label}</dt>
      <dd>{value}</dd>
      <button type="button" className="btn btn-small" onClick={onEdit} aria-label={`Edit ${label.toLowerCase()}`}>
        Edit
      </button>
    </div>
  );
}
