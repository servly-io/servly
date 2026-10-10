import { useState } from 'react';
import { Link } from 'react-router';
import { diningApi } from '../api/admin';
import type { DiningKind, DiningLocation } from '../api/types';
import { Icon } from '../shared/icons';
import { useSession } from '../shared/session';
import { EmptyState, ErrorBanner, Field, PageHeader, Spinner, Toggle, useAction, useLoad } from '../shared/ui';
import { useAdmin } from './AdminLayout';

export function TablesPage() {
  const { can } = useSession();
  const canEdit = can('dining_locations.manage');
  const list = useLoad(() => diningApi.list().then((r) => r.items));
  const act = useAction();
  const [name, setName] = useState('');
  const [kind, setKind] = useState<DiningKind>('table');

  if (!list.data) return list.error ? <ErrorBanner error={list.error} onRetry={list.reload} /> : <Spinner />;

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    if (await act.run(() => diningApi.create(name, kind))) {
      setName('');
      await list.reload();
    }
  };
  const patch = (l: DiningLocation, body: { name?: string; is_enabled?: boolean }) =>
    act.run(() => diningApi.update(l.id, body).then(() => list.reload()));
  const rotate = (l: DiningLocation) => {
    if (!window.confirm(`Issue a new QR code for ${l.name}? The printed code stops working.`)) return;
    void act.run(() => diningApi.rotate(l.id).then(() => list.reload()));
  };

  return (
    <section className="stack">
      <PageHeader
        title="Tables & QR codes"
        subtitle="Each table or pickup point has its own QR code. Guests scan it to open the menu and order."
        actions={list.data.length > 0 && (
          <Link className="btn btn-primary" to="/admin/tables/print">
            <Icon name="printer" size={18} />
            Print QR codes
          </Link>
        )}
      />
      {canEdit && (
        <form onSubmit={add} className="card add-bar">
          <Field label="Name"><input value={name} required maxLength={60} placeholder={kind === 'table' ? 'Table 1' : 'Counter'} onChange={(e) => setName(e.target.value)} /></Field>
          <div className="field" style={{ flex: '0 0 auto' }}>
            <span className="field-label" id="kind-label">Kind</span>
            <div className="seg" role="group" aria-labelledby="kind-label">
              <button type="button" aria-pressed={kind === 'table'} onClick={() => setKind('table')}>Table</button>
              <button type="button" aria-pressed={kind === 'pickup'} onClick={() => setKind('pickup')}>Pickup point</button>
            </div>
          </div>
          <button type="submit" className="btn btn-primary" style={{ minHeight: 44 }} disabled={act.busy}>
            <Icon name="plus" size={18} />Add
          </button>
        </form>
      )}
      <ErrorBanner error={act.error} />
      {list.data.length === 0 ? (
        <div className="card">
          <EmptyState icon="qr" title="No tables yet">Add one entry per table, such as “Table 1”. Add a pickup point if you sell takeaway.</EmptyState>
        </div>
      ) : (
        <ul className="list-plain qr-grid">
          {list.data.map((l) => (
            <TableCard key={l.id} table={l} canEdit={canEdit} busy={act.busy} onPatch={(b) => patch(l, b)} onRotate={() => rotate(l)} />
          ))}
        </ul>
      )}
    </section>
  );
}

function TableCard({ table, canEdit, busy, onPatch, onRotate }: {
  table: DiningLocation; canEdit: boolean; busy: boolean;
  onPatch: (b: { name?: string; is_enabled?: boolean }) => void; onRotate: () => void;
}) {
  const [name, setName] = useState(table.name);
  return (
    <li className={`card qr-card${table.is_enabled ? '' : ' paused'}`}>
      <div className="qr-frame">
        {/* the token in the query busts the browser cache when the code is rotated */}
        <img src={`${diningApi.qrURL(table.id)}?v=${table.public_token}`} alt={`QR code for ${table.name}`} className="qr" />
      </div>
      <div className="stack-tight">
        {canEdit ? (
          <form className="row-nowrap" onSubmit={(e) => { e.preventDefault(); onPatch({ name }); }}>
            <input aria-label="Name" className="qr-name-input" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
            {name !== table.name && <button type="submit" className="btn btn-small btn-primary" disabled={busy}>Save</button>}
          </form>
        ) : (
          <span className="qr-name">{table.name}</span>
        )}
        <span className="row" style={{ gap: 6 }}>
          <span className="badge">{table.kind === 'pickup' ? 'Pickup point' : 'Table'}</span>
          <span className={`badge ${table.is_enabled ? 'badge-ok' : 'badge-warn'}`}>{table.is_enabled ? 'Taking orders' : 'Paused'}</span>
        </span>
      </div>
      <div className="qr-foot">
        {canEdit && <Toggle label={`${table.name} taking orders`} hideLabel checked={table.is_enabled} disabled={busy} onChange={(v) => onPatch({ is_enabled: v })} />}
        <span className="row-grow" />
        <a className="btn btn-ghost btn-icon btn-small" href={table.menu_url} target="_blank" rel="noreferrer" aria-label={`Open the guest menu of ${table.name}`} title="Open guest menu">
          <Icon name="external" size={18} />
        </a>
        <a className="btn btn-ghost btn-icon btn-small" href={diningApi.qrURL(table.id, 'png')} download={`${table.name}.png`} aria-label={`Download the QR code of ${table.name}`} title="Download PNG">
          <Icon name="arrowDown" size={18} />
        </a>
        {canEdit && (
          <button type="button" className="btn btn-ghost btn-icon btn-small" disabled={busy} onClick={onRotate} aria-label={`New QR code for ${table.name}`} title="New code">
            <Icon name="refresh" size={18} />
          </button>
        )}
      </div>
    </li>
  );
}

/** One printable card per enabled table: restaurant, QR code, table name, short instruction. */
export function TablesPrintPage() {
  const { settings } = useAdmin();
  const list = useLoad(() => diningApi.list().then((r) => r.items.filter((l) => l.is_enabled)));
  if (!list.data) return list.error ? <ErrorBanner error={list.error} /> : <Spinner />;
  return (
    <section>
      <div className="no-print">
        <PageHeader
          back={{ to: '/admin/tables', label: 'Tables & QR codes' }}
          title="Print QR codes"
          subtitle="One card per table that takes orders. Cut along the border and place each card on its table."
          actions={<button type="button" className="btn btn-primary" onClick={() => window.print()}><Icon name="printer" size={18} />Print</button>}
        />
      </div>
      <div className="print-sheet">
        {list.data.map((l) => (
          <div key={l.id} className="print-card">
            <span className="print-rest">{settings.name}</span>
            <img src={diningApi.qrURL(l.id)} alt={`QR code for ${l.name}`} />
            <strong>{l.name}</strong>
            <span>Scan to see the menu and order</span>
          </div>
        ))}
      </div>
    </section>
  );
}
