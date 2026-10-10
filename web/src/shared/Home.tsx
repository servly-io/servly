import { useEffect, useState } from 'react';
import { Link } from 'react-router';

export function Home() {
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

  return (
    <main className="page">
      <h1>servly</h1>
      <nav className="tiles">
        <Link className="tile" to="/staff">
          Orders
        </Link>
        <Link className="tile" to="/admin">
          Administration
        </Link>
      </nav>
      <p className="muted">Server: {server}</p>
    </main>
  );
}
