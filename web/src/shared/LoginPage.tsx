import { useState } from 'react';
import { Navigate, useNavigate, useSearchParams } from 'react-router';
import { useSession } from './session';
import { ErrorBanner, Field, useAction } from './ui';

export function LoginPage() {
  const { session, login } = useSession();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const { busy, error, run } = useAction();
  const next = safeNext(params.get('next'));

  if (session) return <Navigate to={next} replace />;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    await run(async () => {
      await login(email, password);
      navigate(next, { replace: true });
    });
  };

  return (
    <main className="page page-narrow">
      <h1>Sign in</h1>
      <form onSubmit={submit} className="stack">
        <Field label="Email">
          <input type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label="Password">
          <input
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        <ErrorBanner error={error} />
        <button type="submit" className="btn btn-primary btn-large" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
      <p className="muted small">Forgot your password? Ask the owner, or run `servly user set-password` on the server.</p>
    </main>
  );
}

/** Only same-site paths are allowed as redirect targets after sign-in. */
function safeNext(next: string | null): string {
  if (!next || !next.startsWith('/') || next.startsWith('//') || next.includes('\\')) return '/staff';
  return next;
}
