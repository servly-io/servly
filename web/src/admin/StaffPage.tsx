import { useState } from 'react';
import { staffApi } from '../api/admin';
import type { Role, StaffMember } from '../api/types';
import { useSession } from '../shared/session';
import { ErrorBanner, Field, Spinner, useAction, useLoad } from '../shared/ui';

const roles: { value: Role; label: string }[] = [
  { value: 'staff', label: 'Staff: orders and payments' },
  { value: 'manager', label: 'Manager: also menu, tables, late cancels, refunds' },
  { value: 'owner', label: 'Owner: also staff and settings' },
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
    if (pw) void act.run(() => staffApi.update(m.user_id, { password: pw }).then(() => window.alert('Password changed. They are signed out everywhere.')));
  };
  const remove = (m: StaffMember) => {
    if (window.confirm(`Remove ${m.display_name} from this restaurant?`)) void act.run(() => staffApi.remove(m.user_id).then(() => list.reload()));
  };

  return (
    <section>
      <h1>Staff</h1>
      <ErrorBanner error={act.error} />
      <table className="table">
        <thead><tr><th>Name</th><th>Email</th><th>Role</th><th><span className="visually-hidden">Actions</span></th></tr></thead>
        <tbody>
          {list.data.map((m) => (
            <tr key={m.user_id}>
              <td>{m.display_name}{m.user_id === session?.user.id && <span className="badge">you</span>}</td>
              <td>{m.email}</td>
              <td>
                <select aria-label={`Role of ${m.display_name}`} value={m.role} disabled={act.busy} onChange={(e) => void setRole(m, e.target.value as Role)}>
                  {roles.map((r) => <option key={r.value} value={r.value}>{r.value}</option>)}
                </select>
              </td>
              <td className="row-actions">
                <button type="button" className="btn btn-small" disabled={act.busy} onClick={() => resetPassword(m)}>Set password</button>
                <button type="button" className="btn btn-small btn-danger" disabled={act.busy} onClick={() => remove(m)}>Remove</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <form onSubmit={add} className="card stack">
        <h2>Add a person</h2>
        <div className="grid-2">
          <Field label="Email"><input type="email" required value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} /></Field>
          <Field label="Name"><input value={form.display_name} onChange={(e) => setForm({ ...form, display_name: e.target.value })} /></Field>
          <Field label="Role">
            <select value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as Role })}>
              {roles.map((r) => <option key={r.value} value={r.value}>{r.label}</option>)}
            </select>
          </Field>
          <Field label="Password" hint="At least 10 characters. Leave empty if they already sign in to servly for another restaurant.">
            <input type="password" autoComplete="new-password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
          </Field>
        </div>
        <button type="submit" className="btn btn-primary" disabled={act.busy}>Add</button>
      </form>
    </section>
  );
}
