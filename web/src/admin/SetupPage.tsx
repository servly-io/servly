import { Link, useLocation } from 'react-router';
import { diningApi, menuApi, staffApi } from '../api/admin';
import { ordersApi } from '../api/staff';
import type { OrderStatus } from '../api/types';
import { useSession } from '../shared/session';
import { Icon } from '../shared/icons';
import { ErrorBanner, PageHeader, Spinner, useLoad } from '../shared/ui';
import { useLiveOrders } from '../staff/useLiveOrders';
import { useAdmin } from './AdminLayout';

const active: OrderStatus[] = ['new', 'accepted', 'preparing', 'ready'];

/** Admin home: a launch checklist until the restaurant is ready, then today's queue at a glance. */
export function SetupPage() {
  const { settings } = useAdmin();
  const { session, can } = useSession();
  const welcome = (useLocation().state as { welcome?: boolean } | null)?.welcome;
  const canStaff = can('staff.manage');
  // The same live stream as the order board: counts move as orders arrive and advance.
  const live = useLiveOrders(active, '');
  const state = useLoad(async () => {
    const [cats, items, tables, staff, anyOrder] = await Promise.all([
      menuApi.categories(),
      menuApi.items(),
      diningApi.list(),
      canStaff ? staffApi.list() : null,
      ordersApi.list({ limit: 1 }),
    ]);
    return {
      cats: cats.items,
      items: items.items,
      tables: tables.items,
      staffCount: staff?.items.length,
      hasOrder: anyOrder.items.length > 0,
    };
  }, [canStaff]);
  if (!state.data) return state.error ? <ErrorBanner error={state.error} onRetry={state.reload} /> : <Spinner />;
  const { cats, items, tables, staffCount } = state.data;
  const open = live.orders ?? [];
  const hasOrder = state.data.hasOrder || open.length > 0;
  const firstTable = tables.find((t) => t.is_enabled);

  const steps = [
    { done: true, label: 'Restaurant details', detail: `${settings.name} · ${settings.currency} · ${settings.timezone}`, to: '/admin/settings', action: 'Edit settings' },
    { done: cats.length > 0, label: 'Create menu categories', detail: 'Such as Starters, Mains and Drinks.', to: '/admin/menu', action: 'Open menu' },
    { done: items.length > 0, label: 'Add menu items', detail: 'Names, prices, photos and options such as sizes or extras.', to: '/admin/menu', action: 'Add items' },
    { done: tables.length > 0, label: 'Add tables and print their QR codes', detail: 'One QR code per table, or a pickup point for takeaway.', to: '/admin/tables', action: 'Open tables' },
    ...(staffCount === undefined
      ? []
      : [{ done: staffCount > 1, label: 'Invite your team', detail: 'Give each person their own sign-in and role.', to: '/admin/staff', action: 'Add staff' }]),
    { done: hasOrder, label: 'Place a test order', detail: 'Scan a QR code with a phone and order, then process it on the order board.', to: firstTable ? firstTable.menu_url : '/admin/tables', action: firstTable ? 'Open guest menu' : 'Add a table first', external: !!firstTable },
  ];
  const doneCount = steps.filter((s) => s.done).length;
  const nextStep = steps.find((s) => !s.done);
  const firstName = session?.user.display_name.split(' ')[0];

  const tiles = [
    { label: 'New', tone: 'tone-new', count: open.filter((o) => o.status === 'new').length },
    { label: 'In progress', tone: 'tone-progress', count: open.filter((o) => o.status === 'accepted' || o.status === 'preparing').length },
    { label: 'Ready', tone: 'tone-ready', count: open.filter((o) => o.status === 'ready').length },
  ];

  return (
    <section className="stack-loose">
      {welcome && (
        <p className="banner banner-ok" role="status">
          <Icon name="sparkles" />
          <span>{settings.name} is created and you are signed in as its owner.</span>
        </p>
      )}
      <PageHeader
        title={`${greeting()}${firstName ? `, ${firstName}` : ''}`}
        subtitle={nextStep ? 'Let’s get your restaurant ready for service.' : `${settings.name} is ready for service.`}
        actions={
          <>
            {firstTable && (
              <a className="btn" href={firstTable.menu_url} target="_blank" rel="noreferrer">
                <Icon name="eye" size={18} />
                Guest menu
              </a>
            )}
            <Link className="btn btn-primary" to="/staff">
              <Icon name="receipt" size={18} />
              Order board
            </Link>
          </>
        }
      />

      {nextStep && (
        <div className="card">
          <div className="card-head">
            <div className="row-grow">
              <h2>Finish setting up</h2>
              <p className="muted small card-sub">Do these once and you are ready for your first guests.</p>
            </div>
            <span className="badge">{doneCount} of {steps.length} done</span>
          </div>
          <div className="meter" role="progressbar" aria-label="Setup progress" aria-valuemin={0} aria-valuemax={steps.length} aria-valuenow={doneCount}>
            <span style={{ width: `${(doneCount / steps.length) * 100}%` }} />
          </div>
          <ol className="checklist" style={{ marginTop: 12 }}>
            {steps.map((s) => (
              <li key={s.label} className={s.done ? 'done' : s === nextStep ? 'current' : ''}>
                <span className="check" aria-hidden="true">{s.done && <Icon name="check" size={16} />}</span>
                <span className="checklist-label">
                  <strong>{s.label}</strong>
                  {s.done && <span className="visually-hidden"> (done)</span>}
                  <span className="muted small">{s.detail}</span>
                </span>
                {s.external ? (
                  <a className={`btn btn-small ${s === nextStep ? 'btn-primary' : ''}`} href={s.to} target="_blank" rel="noreferrer">
                    {s.action}
                  </a>
                ) : (
                  <Link className={`btn btn-small ${s === nextStep ? 'btn-primary' : ''}`} to={s.to}>
                    {s.action}
                  </Link>
                )}
              </li>
            ))}
          </ol>
        </div>
      )}

      <section className="stack-tight" aria-labelledby="today">
        <div className="row">
          <h2 id="today" className="row-grow">Open orders</h2>
          <span className={`conn conn-${live.connection}`} role="status">
            <span className="dot" />
            {live.connection === 'live' ? 'Live' : live.connection === 'connecting' ? 'Connecting…' : 'Reconnecting…'}
          </span>
        </div>
        <div className="stats">
          {tiles.map((t) => (
            <Link key={t.label} to="/staff" className="card stat">
              <span className={`stat-label ${t.tone}`}>
                <span className="dot" />
                {t.label}
              </span>
              <span className="stat-value">{live.orders ? t.count : '-'}</span>
            </Link>
          ))}
        </div>
      </section>
    </section>
  );
}

function greeting(): string {
  const h = new Date().getHours();
  return h < 12 ? 'Good morning' : h < 18 ? 'Good afternoon' : 'Good evening';
}
