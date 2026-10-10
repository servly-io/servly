import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { menuApi } from '../api/admin';
import type { Category, MenuItem } from '../api/types';
import { formatMoney } from '../shared/money';
import { useSession } from '../shared/session';
import { ErrorBanner, Spinner, Toggle, useAction, useLoad } from '../shared/ui';
import { useAdmin } from './AdminLayout';

export function MenuPage() {
  const { settings } = useAdmin();
  const { can } = useSession();
  const canEdit = can('menu.manage');
  const [params, setParams] = useSearchParams();
  const cats = useLoad(() => menuApi.categories().then((r) => r.items));
  const items = useLoad(() => menuApi.items().then((r) => r.items));
  const act = useAction();
  const [newCat, setNewCat] = useState('');

  if (!cats.data || !items.data) {
    const error = cats.error ?? items.error;
    return error ? <ErrorBanner error={error} onRetry={() => void Promise.all([cats.reload(), items.reload()])} /> : <Spinner />;
  }
  const categories = cats.data;
  const selectedId = params.get('category') ?? categories[0]?.id;
  const selected = categories.find((c) => c.id === selectedId);
  const shown = items.data.filter((i) => i.category_id === selectedId);

  const reloadAll = async () => {
    await Promise.all([cats.reload(), items.reload()]);
  };
  const patchCategory = (c: Category, body: Partial<Category>) => act.run(() => menuApi.updateCategory(c.id, body).then(reloadAll));
  const patchItem = (i: MenuItem, body: Parameters<typeof menuApi.updateItem>[1]) =>
    act.run(() => menuApi.updateItem(i.id, body).then(() => items.reload()));

  const addCategory = async (e: React.FormEvent) => {
    e.preventDefault();
    const c = await act.run(() => menuApi.createCategory({ name: newCat }));
    if (c) {
      setNewCat('');
      await cats.reload();
      setParams({ category: c.id });
    }
  };

  return (
    <section>
      <h1>Menu</h1>
      <ErrorBanner error={act.error} />
      <div className="split">
        <aside className="split-side" aria-label="Categories">
          <h2>Categories</h2>
          <ul className="list-plain stack-tight">
            {categories.map((c, idx) => (
              <li key={c.id} className={'row' + (c.id === selectedId ? ' row-selected' : '')}>
                <button type="button" className="link-button row-grow" aria-current={c.id === selectedId} onClick={() => setParams({ category: c.id })}>
                  {c.name}
                  {!c.is_enabled && <span className="badge">hidden</span>}
                </button>
                {canEdit && (
                  <span className="row-actions">
                    <button type="button" className="btn btn-icon" aria-label={`Move ${c.name} up`} disabled={idx === 0 || act.busy}
                      onClick={() => patchCategory(c, { position: idx - 1 })}>↑</button>
                    <button type="button" className="btn btn-icon" aria-label={`Move ${c.name} down`} disabled={idx === categories.length - 1 || act.busy}
                      onClick={() => patchCategory(c, { position: idx + 1 })}>↓</button>
                  </span>
                )}
              </li>
            ))}
          </ul>
          {canEdit && (
            <form onSubmit={addCategory} className="row">
              <input aria-label="New category name" placeholder="New category" value={newCat} onChange={(e) => setNewCat(e.target.value)} required />
              <button type="submit" className="btn" disabled={act.busy}>Add</button>
            </form>
          )}
        </aside>

        <div className="split-main">
          {!selected ? (
            <p className="muted">Create a category to start your menu.</p>
          ) : (
            <>
              <CategoryHeader category={selected} canEdit={canEdit} busy={act.busy} onPatch={(b) => patchCategory(selected, b)} />
              {shown.length === 0 && <p className="muted">No items in this category yet.</p>}
              <ul className="list-plain stack-tight">
                {shown.map((it, idx) => (
                  <li key={it.id} className="card item-row">
                    {it.image_url && <img src={it.image_url} alt="" className="thumb" />}
                    <div className="row-grow">
                      <strong>{it.name}</strong> {!it.is_enabled && <span className="badge">hidden</span>}
                      <div className="muted small">{formatMoney(it.price_minor, settings.currency)}
                        {it.option_groups.length > 0 && ` · ${it.option_groups.map((g) => g.name).join(', ')}`}</div>
                    </div>
                    {canEdit ? (
                      <div className="row-actions">
                        <Toggle label="Available" checked={it.is_available} disabled={act.busy} onChange={(v) => patchItem(it, { is_available: v })} />
                        <button type="button" className="btn btn-icon" aria-label={`Move ${it.name} up`} disabled={idx === 0 || act.busy}
                          onClick={() => patchItem(it, { position: idx - 1 })}>↑</button>
                        <button type="button" className="btn btn-icon" aria-label={`Move ${it.name} down`} disabled={idx === shown.length - 1 || act.busy}
                          onClick={() => patchItem(it, { position: idx + 1 })}>↓</button>
                        <Link className="btn" to={`/admin/menu/items/${it.id}`}>Edit</Link>
                      </div>
                    ) : (
                      <span className="badge">{it.is_available ? 'available' : 'sold out'}</span>
                    )}
                  </li>
                ))}
              </ul>
              {canEdit && (
                <p>
                  <Link className="btn btn-primary" to={`/admin/menu/items/new?category=${selected.id}`}>Add item</Link>
                </p>
              )}
            </>
          )}
        </div>
      </div>
    </section>
  );
}

function CategoryHeader({ category, canEdit, busy, onPatch }: {
  category: Category; canEdit: boolean; busy: boolean; onPatch: (b: Partial<Category>) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(category.name);
  if (editing) {
    return (
      <form className="row" onSubmit={(e) => { e.preventDefault(); onPatch({ name }); setEditing(false); }}>
        <input aria-label="Category name" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        <button type="submit" className="btn btn-primary" disabled={busy}>Save</button>
        <button type="button" className="btn" onClick={() => { setName(category.name); setEditing(false); }}>Cancel</button>
      </form>
    );
  }
  return (
    <div className="row">
      <h2 className="row-grow">{category.name}</h2>
      {canEdit && (
        <>
          <Toggle label="Shown to guests" checked={category.is_enabled} disabled={busy} onChange={(v) => onPatch({ is_enabled: v })} />
          <button type="button" className="btn" onClick={() => { setName(category.name); setEditing(true); }}>Rename</button>
        </>
      )}
    </div>
  );
}
