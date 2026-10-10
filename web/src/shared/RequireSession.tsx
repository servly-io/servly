import { Navigate, useLocation } from 'react-router';
import { useSession } from './session';
import { ErrorBanner, Spinner, useAction } from './ui';

/** Renders children for a signed-in user with an active restaurant. */
export function RequireSession({ children }: { children: React.ReactNode }) {
  const { session, switchTenant } = useSession();
  const location = useLocation();
  const { busy, error, run } = useAction();

  if (session === undefined) return <Spinner />;
  if (session === null) {
    return <Navigate to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`} replace />;
  }
  if (!session.active_tenant) {
    return (
      <main className="page page-narrow">
        <h1>Choose a restaurant</h1>
        {session.memberships.length === 0 && <p>Your sign-in does not belong to any restaurant yet. Ask an owner to add you.</p>}
        <ul className="stack list-plain">
          {session.memberships.map((m) => (
            <li key={m.tenant_id}>
              <button type="button" className="btn btn-large btn-block" disabled={busy} onClick={() => run(() => switchTenant(m.tenant_id))}>
                {m.tenant_name} <span className="muted">({m.role})</span>
              </button>
            </li>
          ))}
        </ul>
        <ErrorBanner error={error} />
      </main>
    );
  }
  return <>{children}</>;
}
