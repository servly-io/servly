import { createContext, useContext, useEffect, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router';
import { settingsApi } from '../api/admin';
import type { Settings } from '../api/types';
import { Icon, type IconName } from '../shared/icons';
import { useSession } from '../shared/session';
import { ErrorBanner, Spinner, initials, useLoad } from '../shared/ui';

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

/** The administration shell: a sidebar on desktop, a drawer on phones. */
export function AdminLayout() {
  const { session, logout, can, switchTenant } = useSession();
  const settings = useLoad(() => settingsApi.get());
  const [navOpen, setNavOpen] = useState(false);
  const { pathname } = useLocation();
  useEffect(() => setNavOpen(false), [pathname]);

  const nav: { to: string; label: string; icon: IconName; end?: boolean; external?: boolean; show?: boolean }[] = [
    { to: '/admin', label: 'Home', icon: 'home', end: true },
    { to: '/staff', label: 'Orders', icon: 'receipt', external: true },
    { to: '/admin/menu', label: 'Menu', icon: 'utensils' },
    { to: '/admin/tables', label: 'Tables & QR codes', icon: 'qr' },
    { to: '/admin/staff', label: 'Staff', icon: 'users', show: can('staff.manage') },
    { to: '/admin/settings', label: 'Settings', icon: 'sliders' },
  ];
  const name = session?.active_tenant?.name ?? '';
  const user = session?.user;

  return (
    <div className={`shell${navOpen ? ' nav-open' : ''}`}>
      <header className="shell-top">
        <button type="button" className="btn btn-ghost btn-icon" aria-label="Open navigation" aria-expanded={navOpen} onClick={() => setNavOpen(true)}>
          <Icon name="menu" />
        </button>
        <span className="logo" aria-hidden="true">{name.charAt(0)}</span>
        <strong className="truncate">{name}</strong>
      </header>

      <aside className="sidebar" aria-label="Administration">
        <div className="sidebar-brand">
          <span className="logo" aria-hidden="true">{name.charAt(0)}</span>
          <div className="row-grow">
            <strong className="truncate">{name}</strong>
            <span className="muted tiny">servly</span>
          </div>
          <button type="button" className="btn btn-ghost btn-icon btn-small shell-close" aria-label="Close navigation" hidden={!navOpen} onClick={() => setNavOpen(false)}>
            <Icon name="x" size={18} />
          </button>
        </div>
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
        <nav className="nav">
          {nav
            .filter((n) => n.show !== false)
            .map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} className="nav-item">
                <Icon name={n.icon} />
                {n.label}
                {n.external && <span className="nav-end"><Icon name="chevronRight" size={16} /></span>}
              </NavLink>
            ))}
        </nav>
        <div className="sidebar-foot">
          <span className="avatar" aria-hidden="true">{initials(user?.display_name || user?.email || '')}</span>
          <div className="row-grow">
            <div className="small strong truncate">{user?.display_name}</div>
            <div className="tiny muted truncate">{session?.active_tenant?.role}</div>
          </div>
          <button type="button" className="btn btn-ghost btn-icon btn-small" aria-label="Sign out" title="Sign out" onClick={() => void logout()}>
            <Icon name="logout" size={18} />
          </button>
        </div>
      </aside>
      {navOpen && <div className="scrim" aria-hidden="true" onClick={() => setNavOpen(false)} />}

      <main className="shell-main">
        <div className="content">
          <ErrorBanner error={settings.error} onRetry={settings.reload} />
          {settings.data ? (
            <Ctx.Provider value={{ settings: settings.data, reloadSettings: settings.reload }}>
              <Outlet />
            </Ctx.Provider>
          ) : (
            !settings.error && <Spinner />
          )}
        </div>
      </main>
    </div>
  );
}
