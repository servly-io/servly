import { Link } from 'react-router';

export function Placeholder({ title, phase, children }: { title: string; phase: string; children?: React.ReactNode }) {
  return (
    <main className="page">
      <h1>{title}</h1>
      <p className="muted">Not built yet: {phase}.</p>
      {children}
      <p>
        <Link to="/">Back</Link>
      </p>
    </main>
  );
}
