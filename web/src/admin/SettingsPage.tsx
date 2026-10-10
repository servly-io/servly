import { useEffect, useState } from 'react';
import { settingsApi } from '../api/admin';
import type { Settings } from '../api/types';
import { useSession } from '../shared/session';
import { Icon } from '../shared/icons';
import { CurrencySelect, TimezoneSelect } from '../shared/inputs';
import { ErrorBanner, Field, PageHeader, Toggle, useAction } from '../shared/ui';
import { useAdmin } from './AdminLayout';

export function SettingsPage() {
  const { settings, reloadSettings } = useAdmin();
  const { can } = useSession();
  const canEdit = can('settings.manage');
  const [form, setForm] = useState<Settings>(settings);
  const [saved, setSaved] = useState(false);
  const act = useAction();
  useEffect(() => setForm(settings), [settings]);
  const dirty = JSON.stringify(form) !== JSON.stringify(settings);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaved(false);
    if (await act.run(() => settingsApi.update(form))) {
      await reloadSettings();
      setSaved(true);
    }
  };

  return (
    <form onSubmit={save} className="stack" style={{ maxWidth: 720 }}>
      <PageHeader title="Settings" subtitle={canEdit ? undefined : 'Only owners change settings.'} />
      <fieldset disabled={!canEdit} className="stack">
        <section className="card stack">
          <div>
            <h2>Restaurant</h2>
            <p className="muted small card-sub">Guests see the name on the menu.</p>
          </div>
          <Field label="Restaurant name"><input required maxLength={120} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
          <div className="grid-2">
            <Field label="Currency" hint="Every price uses it. It cannot change after the first order.">
              <CurrencySelect value={form.currency} onChange={(v) => setForm({ ...form, currency: v })} />
            </Field>
            <Field label="Timezone" hint="Order numbers restart every day at midnight in this timezone.">
              <TimezoneSelect value={form.timezone} onChange={(v) => setForm({ ...form, timezone: v })} />
            </Field>
          </div>
        </section>
        <section className="card stack-tight">
          <h2>Guest ordering</h2>
          <Toggle label="Notes on orders" description="Guests can add a note for the kitchen, such as “no onions”." checked={form.order_notes_enabled} onChange={(v) => setForm({ ...form, order_notes_enabled: v })} />
          <Toggle label="Order status page" description="After ordering, guests can follow their order on their phone." checked={form.public_status_enabled} onChange={(v) => setForm({ ...form, public_status_enabled: v })} />
        </section>
      </fieldset>
      <ErrorBanner error={act.error} />
      {saved && !dirty && <p role="status" className="banner banner-ok"><Icon name="checkCircle" /><span>Saved.</span></p>}
      {canEdit && dirty && (
        <div className="savebar" role="region" aria-label="Save changes">
          <span className="savebar-text"><Icon name="alert" size={18} />Unsaved changes</span>
          <button type="button" className="btn" onClick={() => setForm(settings)}>Discard</button>
          <button type="submit" className="btn btn-accent" disabled={act.busy}>{act.busy ? 'Saving…' : 'Save'}</button>
        </div>
      )}
    </form>
  );
}
