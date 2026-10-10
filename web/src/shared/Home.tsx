import { useEffect, useState } from 'react';
import { Link, Navigate } from 'react-router';
import { Icon } from './icons';
import { useSession } from './session';

export function Home() {
  const { setupRequired } = useSession();
  const [server, setServer] = useState<string>('checking…');

  useEffect(() => {
    const ctrl = new AbortController();
    fetch('/healthz', { signal: ctrl.signal })
      .then((r) => r.json() as Promise<{ status: string; version: string }>)
      .then((h) => setServer(`${h.status}, version ${h.version}`))
      .catch((err: unknown) => {
        if (!(err instanceof DOMException && err.name === 'AbortError')) setServer('unreachable');
      });
    return () => ctrl.abort();
  }, []);

  if (setupRequired) return <Navigate to="/setup" replace />;
  return (
    <main className="auth">
      <span className="brand"><span className="logo">s</span>servly</span>
      <div className="auth-card">
        <div className="stack-tight">
          <h1>Where to?</h1>
          <p className="muted">Pick the screen for this device.</p>
        </div>
        <nav className="launch" aria-label="Screens">
          <Link to="/staff">
            <span className="launch-icon"><Icon name="receipt" /></span>
            <span className="row-grow"><strong>Order board</strong><br /><span className="muted small">For the kitchen, bar and counter</span></span>
            <Icon name="chevronRight" />
          </Link>
          <Link to="/admin">
            <span className="launch-icon"><Icon name="store" /></span>
            <span className="row-grow"><strong>Administration</strong><br /><span className="muted small">Menu, tables, staff and settings</span></span>
            <Icon name="chevronRight" />
          </Link>
        </nav>
      </div>
      <p className="auth-foot">Server: {server}</p>
    </main>
  );
}
