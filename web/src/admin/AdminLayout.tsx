import { createContext, useContext } from 'react';
import { NavLink, Outlet } from 'react-router';
import { settingsApi } from '../api/admin';
import type { Settings } from '../api/types';
import { useSession } from '../shared/session';
import { ErrorBanner, Spinner, useLoad } from '../shared/ui';

interface AdminCtx {
  settings: Settings;
  reloadSettings: () => Promise<void>;
}

const Ctx = createContext<AdminCtx | null>(null);

export function useAdmin(): AdminCtx {
  const c = useContext(Ctx);
  if (!c) throw new Error('useAdmin outside AdminLayout');
  return c;
}

export function AdminLayout() {
  const { session, logout, can, switchTenant } = useSession();
  const settings = useLoad(() => settingsApi.get());

  return (
    <div className="admin">
      <header className="topbar">
        <strong className="topbar-title">{session?.active_tenant?.name}</strong>
        <nav aria-label="Administration" className="topnav">
          <NavLink to="/admin" end>
            Setup
          </NavLink>
          <NavLink to="/admin/menu">Menu</NavLink>
          <NavLink to="/admin/tables">Tables</NavLink>
          {can('staff.manage') && <NavLink to="/admin/staff">Staff</NavLink>}
          <NavLink to="/admin/settings">Settings</NavLink>
          <NavLink to="/staff">Orders</NavLink>
        </nav>
        <span className="topbar-end">
          {session && session.memberships.length > 1 && (
            <select
              aria-label="Restaurant"
              value={session.active_tenant?.id}
              onChange={(e) => void switchTenant(e.target.value).then(() => settings.reload())}
            >
              {session.memberships.map((m) => (
                <option key={m.tenant_id} value={m.tenant_id}>
                  {m.tenant_name}
                </option>
              ))}
            </select>
          )}
          <button type="button" className="btn btn-small" onClick={() => void logout()}>
            Sign out
          </button>
        </span>
      </header>
      <main className="page">
        <ErrorBanner error={settings.error} onRetry={settings.reload} />
        {settings.data ? (
          <Ctx.Provider value={{ settings: settings.data, reloadSettings: settings.reload }}>
            <Outlet />
          </Ctx.Provider>
        ) : (
          !settings.error && <Spinner />
        )}
      </main>
    </div>
  );
}
