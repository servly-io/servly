import { useState } from 'react';
import { Link } from 'react-router';
import { diningApi } from '../api/admin';
import type { DiningKind, DiningLocation } from '../api/types';
import { useSession } from '../shared/session';
import { ErrorBanner, Field, Spinner, Toggle, useAction, useLoad } from '../shared/ui';

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
    <section>
      <div className="row">
        <h1 className="row-grow">Tables and pickup points</h1>
        <Link className="btn btn-primary" to="/admin/tables/print">Print QR codes</Link>
      </div>
      <p className="muted">Each table gets its own QR code. Guests scan it to open the menu and order to that table.</p>
      <ErrorBanner error={act.error} />
      {list.data.length === 0 && <p className="muted">No tables yet.</p>}
      <ul className="list-plain qr-grid">
        {list.data.map((l) => (
          <TableCard key={l.id} table={l} canEdit={canEdit} busy={act.busy} onPatch={(b) => patch(l, b)} onRotate={() => rotate(l)} />
        ))}
      </ul>
      {canEdit && (
        <form onSubmit={add} className="card row">
          <Field label="Name"><input value={name} required maxLength={60} placeholder="Table 1" onChange={(e) => setName(e.target.value)} /></Field>
          <Field label="Kind">
            <select value={kind} onChange={(e) => setKind(e.target.value as DiningKind)}>
              <option value="table">Table</option>
              <option value="pickup">Pickup point</option>
            </select>
          </Field>
          <button type="submit" className="btn btn-primary" disabled={act.busy}>Add</button>
        </form>
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
    <li className="card qr-card stack-tight">
      {/* the token in the query busts the browser cache when the code is rotated */}
      <img src={`${diningApi.qrURL(table.id)}?v=${table.public_token}`} alt={`QR code for ${table.name}`} className="qr" />
      {canEdit ? (
        <form className="row" onSubmit={(e) => { e.preventDefault(); onPatch({ name }); }}>
          <input aria-label="Name" value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
          {name !== table.name && <button type="submit" className="btn btn-small" disabled={busy}>Save</button>}
        </form>
      ) : (
        <strong>{table.name}</strong>
      )}
      <span className="muted small">{table.kind === 'pickup' ? 'Pickup point' : 'Table'}</span>
      <a className="small break" href={table.menu_url} target="_blank" rel="noreferrer">{table.menu_url}</a>
      {canEdit && (
        <div className="row">
          <Toggle label="Accepting orders" checked={table.is_enabled} disabled={busy} onChange={(v) => onPatch({ is_enabled: v })} />
          <button type="button" className="btn btn-small" disabled={busy} onClick={onRotate}>New code</button>
          <a className="btn btn-small" href={diningApi.qrURL(table.id, 'png')} download={`${table.name}.png`}>PNG</a>
        </div>
      )}
    </li>
  );
}

/** One printable card per enabled table: name, QR code, short instruction. */
export function TablesPrintPage() {
  const list = useLoad(() => diningApi.list().then((r) => r.items.filter((l) => l.is_enabled)));
  if (!list.data) return list.error ? <ErrorBanner error={list.error} /> : <Spinner />;
  return (
    <section>
      <div className="row no-print">
        <Link to="/admin/tables" className="btn">Back</Link>
        <button type="button" className="btn btn-primary" onClick={() => window.print()}>Print</button>
      </div>
      <div className="print-sheet">
        {list.data.map((l) => (
          <div key={l.id} className="print-card">
            <img src={diningApi.qrURL(l.id)} alt={`QR code for ${l.name}`} />
            <strong>{l.name}</strong>
            <span>Scan to see the menu and order</span>
          </div>
        ))}
      </div>
    </section>
  );
}
