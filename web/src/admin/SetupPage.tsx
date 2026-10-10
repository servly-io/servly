import { Link } from 'react-router';
import { diningApi, menuApi } from '../api/admin';
import { Spinner, useLoad } from '../shared/ui';
import { useAdmin } from './AdminLayout';

/** Guided setup: what is done, what is next. */
export function SetupPage() {
  const { settings } = useAdmin();
  const state = useLoad(async () => {
    const [cats, items, tables] = await Promise.all([menuApi.categories(), menuApi.items(), diningApi.list()]);
    return { cats: cats.items, items: items.items, tables: tables.items };
  });
  if (!state.data) return <Spinner />;
  const { cats, items, tables } = state.data;
  const firstTable = tables.find((t) => t.is_enabled);

  const steps = [
    { done: true, label: `Restaurant details: ${settings.name}, ${settings.currency}, ${settings.timezone}`, to: '/admin/settings', action: 'Edit settings' },
    { done: cats.length > 0, label: 'Create menu categories, such as Starters or Drinks', to: '/admin/menu', action: 'Open menu' },
    { done: items.length > 0, label: 'Add items with prices and options', to: '/admin/menu', action: 'Open menu' },
    { done: tables.length > 0, label: 'Add tables or a pickup point', to: '/admin/tables', action: 'Open tables' },
    { done: false, label: 'Print the QR codes and put them on the tables', to: '/admin/tables/print', action: 'Print QR codes' },
  ];

  return (
    <section>
      <h1>Setup</h1>
      <ol className="checklist">
        {steps.map((s) => (
          <li key={s.label} className={s.done ? 'done' : ''}>
            <span aria-hidden="true">{s.done ? '✓' : '○'}</span>
            <span className="checklist-label">
              {s.label}
              {s.done && <span className="visually-hidden"> (done)</span>}
            </span>
            <Link to={s.to}>{s.action}</Link>
          </li>
        ))}
      </ol>
      {firstTable && (
        <p>
          <a href={firstTable.menu_url} target="_blank" rel="noreferrer">
            Preview the guest menu ({firstTable.name})
          </a>
        </p>
      )}
    </section>
  );
}
