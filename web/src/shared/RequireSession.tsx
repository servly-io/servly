import { Navigate, useLocation } from 'react-router';
import { useSession } from './session';
import { ErrorBanner, Spinner, useAction } from './ui';

/** Renders children for a signed-in user with an active restaurant. */
export function RequireSession({ children }: { children: React.ReactNode }) {
  const { session, setupRequired, switchTenant } = useSession();
  const location = useLocation();
  const { busy, error, run } = useAction();

  if (session === undefined || setupRequired === undefined) return <Spinner />;
  if (setupRequired) return <Navigate to="/setup" replace />;
  if (session === null) {
    return <Navigate to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`} replace />;
  }
  if (!session.active_tenant) {
    return (
      <main className="auth">
        <div className="auth-card">
          <div className="stack-tight">
            <h1>Choose a restaurant</h1>
            {session.memberships.length === 0 && <p className="muted">Your sign-in does not belong to any restaurant yet. Ask an owner to add you.</p>}
          </div>
          <ul className="list-plain launch">
            {session.memberships.map((m) => (
              <li key={m.tenant_id}>
                <button type="button" className="btn btn-large btn-block" style={{ justifyContent: 'space-between' }} disabled={busy} onClick={() => run(() => switchTenant(m.tenant_id))}>
                  <span>{m.tenant_name}</span>
                  <span className="badge">{m.role}</span>
                </button>
              </li>
            ))}
          </ul>
          <ErrorBanner error={error} />
        </div>
      </main>
    );
  }
  return <>{children}</>;
}
