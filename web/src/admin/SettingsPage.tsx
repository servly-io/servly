import { useEffect, useState } from 'react';
import { settingsApi } from '../api/admin';
import type { Settings } from '../api/types';
import { useSession } from '../shared/session';
import { ErrorBanner, Field, Toggle, useAction } from '../shared/ui';
import { useAdmin } from './AdminLayout';

const zones = typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf('timeZone') : [];

export function SettingsPage() {
  const { settings, reloadSettings } = useAdmin();
  const { can } = useSession();
  const canEdit = can('settings.manage');
  const [form, setForm] = useState<Settings>(settings);
  const [saved, setSaved] = useState(false);
  const act = useAction();
  useEffect(() => setForm(settings), [settings]);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaved(false);
    if (await act.run(() => settingsApi.update(form))) {
      await reloadSettings();
      setSaved(true);
    }
  };

  return (
    <form onSubmit={save} className="stack page-narrow">
      <h1>Settings</h1>
      <fieldset disabled={!canEdit} className="stack">
        <Field label="Restaurant name"><input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
        <Field label="Currency" hint="Three-letter code such as EUR. It cannot change after the first order.">
          <input required maxLength={3} value={form.currency} onChange={(e) => setForm({ ...form, currency: e.target.value.toUpperCase() })} />
        </Field>
        <Field label="Timezone" hint="Order numbers restart every day at midnight in this timezone.">
          <input list="timezones" required value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} />
          <datalist id="timezones">{zones.map((z) => <option key={z} value={z} />)}</datalist>
        </Field>
        <Toggle label="Guests can add notes to orders" checked={form.order_notes_enabled} onChange={(v) => setForm({ ...form, order_notes_enabled: v })} />
        <Toggle label="Guests can follow their order status" checked={form.public_status_enabled} onChange={(v) => setForm({ ...form, public_status_enabled: v })} />
      </fieldset>
      <ErrorBanner error={act.error} />
      {saved && <p role="status" className="banner banner-ok">Saved.</p>}
      {canEdit ? (
        <button type="submit" className="btn btn-primary btn-large" disabled={act.busy}>Save</button>
      ) : (
        <p className="muted">Only owners change settings.</p>
      )}
    </form>
  );
}
