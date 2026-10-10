import { useState } from 'react';
import { staffApi } from '../api/admin';
import type { Role, StaffMember } from '../api/types';
import { useSession } from '../shared/session';
import { Icon } from '../shared/icons';
import { ErrorBanner, Field, PageHeader, Spinner, initials, useAction, useLoad } from '../shared/ui';

const roles: { value: Role; title: string; can: string }[] = [
  { value: 'staff', title: 'Staff', can: 'Takes orders through to completion and records payments.' },
  { value: 'manager', title: 'Manager', can: 'Also edits the menu and tables, cancels late and refunds.' },
  { value: 'owner', title: 'Owner', can: 'Also manages staff and restaurant settings.' },
];

export function StaffPage() {
  const { session } = useSession();
  const list = useLoad(() => staffApi.list().then((r) => r.items));
  const act = useAction();
  const [form, setForm] = useState({ email: '', display_name: '', role: 'staff' as Role, password: '' });

  if (!list.data) return list.error ? <ErrorBanner error={list.error} onRetry={list.reload} /> : <Spinner />;

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    const body = { ...form, password: form.password || undefined };
    if (await act.run(() => staffApi.add(body))) {
      setForm({ email: '', display_name: '', role: 'staff', password: '' });
      await list.reload();
    }
  };
  const setRole = (m: StaffMember, role: Role) => act.run(() => staffApi.update(m.user_id, { role }).then(() => list.reload()));
  const resetPassword = (m: StaffMember) => {
    const pw = window.prompt(`New password for ${m.display_name} (at least 10 characters):`);
    if (pw && pw.length < 10) {
      act.setError(new Error('The password needs at least 10 characters.'));
      return;
    }
    if (pw) void act.run(() => staffApi.update(m.user_id, { password: pw }).then(() => window.alert('Password changed. They are signed out everywhere.')));
  };
  const remove = (m: StaffMember) => {
    if (window.confirm(`Remove ${m.display_name} from this restaurant?`)) void act.run(() => staffApi.remove(m.user_id).then(() => list.reload()));
  };

  return (
    <section className="stack">
      <PageHeader title="Staff" subtitle="Everyone who works here gets their own sign-in. Their role decides what they can change." />
      <ErrorBanner error={act.error} />
      <div className="card card-flush">
        <div className="card-head" style={{ padding: '16px 20px 0', marginBottom: 12 }}>
          <h2>Team</h2>
          <span className="badge">{list.data.length}</span>
        </div>
        <ul className="people">
          {list.data.map((m) => (
            <li key={m.user_id} className="person">
              <span className="avatar" aria-hidden="true">{initials(m.display_name || m.email)}</span>
              <div className="row-grow">
                <div className="strong">
                  {m.display_name}
                  {m.user_id === session?.user.id && <span className="badge" style={{ marginLeft: 8 }}>You</span>}
                </div>
                <div className="muted small truncate">{m.email}</div>
              </div>
              <select aria-label={`Role of ${m.display_name}`} value={m.role} disabled={act.busy} onChange={(e) => void setRole(m, e.target.value as Role)}>
                {roles.map((r) => <option key={r.value} value={r.value}>{r.title}</option>)}
              </select>
              <span className="row-actions">
                <button type="button" className="btn btn-small" disabled={act.busy} onClick={() => resetPassword(m)}>Set password</button>
                <button type="button" className="btn btn-small btn-ghost btn-icon" aria-label={`Remove ${m.display_name}`} title="Remove" disabled={act.busy} onClick={() => remove(m)}>
                  <Icon name="trash" size={18} />
                </button>
              </span>
            </li>
          ))}
        </ul>
      </div>

      <form onSubmit={add} className="card stack">
        <div>
          <h2>Add a person</h2>
          <p className="muted small card-sub">They sign in at this address with their email and the password you set.</p>
        </div>
        <div className="grid-2">
          <Field label="Email"><input type="email" required value={form.email} placeholder="name@example.com" onChange={(e) => setForm({ ...form, email: e.target.value })} /></Field>
          <Field label="Name"><input maxLength={120} value={form.display_name} placeholder="Marco" onChange={(e) => setForm({ ...form, display_name: e.target.value })} /></Field>
          <Field label="Role">
            <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as Role })}>
              {roles.map((r) => <option key={r.value} value={r.value}>{r.title}</option>)}
            </select>
          </Field>
          <Field label="Password" hint="At least 10 characters. Leave empty if they already sign in to servly for another restaurant.">
            <input type="password" autoComplete="new-password" minLength={10} maxLength={256} value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
          </Field>
        </div>
        <div className="role-legend">
          {roles.map((r) => (
            <div key={r.value}>
              <span className="strong small">{r.title}</span>
              <span className="muted small">{r.can}</span>
            </div>
          ))}
        </div>
        <div>
          <button type="submit" className="btn btn-primary" disabled={act.busy}><Icon name="plus" size={18} />Add person</button>
        </div>
      </form>
    </section>
  );
}
