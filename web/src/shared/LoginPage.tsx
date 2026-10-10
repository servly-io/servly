import { useState } from 'react';
import { Navigate, useNavigate, useSearchParams } from 'react-router';
import { useSession } from './session';
import { ErrorBanner, Field, useAction } from './ui';

export function LoginPage() {
  const { session, setupRequired, login } = useSession();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const { busy, error, run } = useAction();
  const next = safeNext(params.get('next'));

  if (setupRequired) return <Navigate to="/setup" replace />;
  if (session) return <Navigate to={next} replace />;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    await run(async () => {
      await login(email, password);
      navigate(next, { replace: true });
    });
  };

  return (
    <main className="auth">
      <span className="brand"><span className="logo">s</span>servly</span>
      <form onSubmit={submit} className="auth-card">
        <div className="stack-tight">
          <h1>Sign in</h1>
          <p className="muted">Welcome back. Sign in to run the restaurant.</p>
        </div>
        <Field label="Email">
          <input type="email" autoComplete="username" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
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
        <button type="submit" className="btn btn-primary btn-large btn-block" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
      <p className="auth-foot">Forgot your password? Ask the owner. If the owner is locked out, ask the person who runs the server.</p>
    </main>
  );
}

/** Only same-site paths are allowed as redirect targets after sign-in. */
function safeNext(next: string | null): string {
  if (!next || !next.startsWith('/') || next.startsWith('//') || next.includes('\\')) return '/staff';
  return next;
}
