import { Link } from 'react-router';
import { EmptyState } from './ui';

export function Placeholder({ title, phase, children }: { title: string; phase: string; children?: React.ReactNode }) {
  return (
    <main className="auth">
      <div className="auth-card">
        <EmptyState icon="alert" title={title} action={<Link className="btn btn-primary" to="/">Go to the start page</Link>}>
          {phase.charAt(0).toUpperCase() + phase.slice(1)}.
        </EmptyState>
        {children}
      </div>
    </main>
  );
}
