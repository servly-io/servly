import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import { diningApi, menuApi } from '../api/admin';
import type { Category, MenuItem } from '../api/types';
import { Icon } from '../shared/icons';
import { formatMoney } from '../shared/money';
import { useSession } from '../shared/session';
import { EmptyState, ErrorBanner, PageHeader, Spinner, Toggle, useAction, useLoad } from '../shared/ui';
import { useAdmin } from './AdminLayout';

export function MenuPage() {
  const { settings } = useAdmin();
  const { can } = useSession();
  const canEdit = can('menu.manage');
  const [params, setParams] = useSearchParams();
  const cats = useLoad(() => menuApi.categories().then((r) => r.items));
  const items = useLoad(() => menuApi.items().then((r) => r.items));
  const preview = useLoad(() => diningApi.list().then((r) => r.items.find((t) => t.is_enabled)?.menu_url ?? null));
  const act = useAction();
  const [newCat, setNewCat] = useState('');
  const [query, setQuery] = useState('');

  if (!cats.data || !items.data) {
    const error = cats.error ?? items.error;
    return error ? <ErrorBanner error={error} onRetry={() => void Promise.all([cats.reload(), items.reload()])} /> : <Spinner />;
  }
  const categories = cats.data;
  const selectedId = params.get('category') ?? categories[0]?.id;
  const selected = categories.find((c) => c.id === selectedId);
  const q = query.trim().toLowerCase();
  const shown = q
    ? items.data.filter((i) => i.name.toLowerCase().includes(q) || i.description.toLowerCase().includes(q))
    : items.data.filter((i) => i.category_id === selectedId);
  const countIn = (c: Category) => items.data!.filter((i) => i.category_id === c.id).length;
  const catName = (id: string) => categories.find((c) => c.id === id)?.name ?? '';

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
      <PageHeader
        title="Menu"
        subtitle={`${items.data.length} ${items.data.length === 1 ? 'item' : 'items'} in ${categories.length} ${categories.length === 1 ? 'category' : 'categories'}`}
        actions={
          <>
            {preview.data && (
              <a className="btn" href={preview.data} target="_blank" rel="noreferrer">
                <Icon name="eye" size={18} />
                Preview
              </a>
            )}
            {canEdit && selected && (
              <Link className="btn btn-primary" to={`/admin/menu/items/new?category=${selected.id}`}>
                <Icon name="plus" size={18} />
                Add item
              </Link>
            )}
          </>
        }
      />
      <ErrorBanner error={act.error} />
      <div className="menu-layout" style={{ marginTop: act.error ? 12 : 0 }}>
        <aside className="card cat-card" aria-label="Categories">
          <div className="card-head">
            <h2>Categories</h2>
          </div>
          {categories.length === 0 && <p className="muted small" style={{ padding: '0 10px 8px' }}>None yet. Start with one, such as Starters.</p>}
          <ul className="list-plain">
            {categories.map((c, idx) => (
              <li key={c.id} className={'cat-item' + (c.id === selectedId && !q ? ' selected' : '')}>
                <button type="button" className="link-button" aria-current={c.id === selectedId && !q} onClick={() => { setQuery(''); setParams({ category: c.id }); }}>
                  <span className="truncate row-grow">{c.name}</span>
                  {!c.is_enabled && <span className="badge">Hidden</span>}
                  <span className="muted small num">{countIn(c)}</span>
                </button>
                {canEdit && (
                  <span className="row-actions">
                    <button type="button" className="btn btn-ghost btn-icon btn-small" aria-label={`Move ${c.name} up`} disabled={idx === 0 || act.busy}
                      onClick={() => patchCategory(c, { position: idx - 1 })}><Icon name="arrowUp" size={16} /></button>
                    <button type="button" className="btn btn-ghost btn-icon btn-small" aria-label={`Move ${c.name} down`} disabled={idx === categories.length - 1 || act.busy}
                      onClick={() => patchCategory(c, { position: idx + 1 })}><Icon name="arrowDown" size={16} /></button>
                  </span>
                )}
              </li>
            ))}
          </ul>
          {canEdit && (
            <form onSubmit={addCategory} className="cat-add">
              <input aria-label="New category name" placeholder="New category" value={newCat} onChange={(e) => setNewCat(e.target.value)} required />
              <button type="submit" className="btn btn-icon" aria-label="Add category" disabled={act.busy}><Icon name="plus" size={18} /></button>
            </form>
          )}
        </aside>

        <div className="card card-flush">
          <div style={{ padding: '16px 20px' }} className="stack-tight">
            {selected && !q && <CategoryHeader key={selected.id} category={selected} canEdit={canEdit} busy={act.busy} onPatch={(b) => patchCategory(selected, b)} />}
            {q && <h2>Results for “{query.trim()}”</h2>}
            <label className="search">
              <Icon name="search" size={18} />
              <input type="search" aria-label="Search all items" placeholder="Search all items" value={query} onChange={(e) => setQuery(e.target.value)} />
            </label>
          </div>
          {!selected ? (
            <EmptyState icon="utensils" title="Start your menu">Create a category on the left, such as Starters or Drinks. Then add items to it.</EmptyState>
          ) : shown.length === 0 ? (
            q ? (
              <EmptyState icon="search" title="No items found">Try another name.</EmptyState>
            ) : (
              <EmptyState
                icon="utensils"
                title={`No items in ${selected.name} yet`}
                action={canEdit && <Link className="btn btn-primary" to={`/admin/menu/items/new?category=${selected.id}`}><Icon name="plus" size={18} />Add item</Link>}
              >
                Guests see a category only when it has items.
              </EmptyState>
            )
          ) : (
            <ul className="items">
              {shown.map((it, idx) => (
                <li key={it.id} className="item-row">
                  {it.image_url ? <img src={it.image_url} alt="" className="thumb" /> : <span className="thumb"><Icon name="image" /></span>}
                  <Link to={`/admin/menu/items/${it.id}`} className="item-main">
                    <strong className="truncate">{it.name}</strong>
                    <span className="muted small truncate">
                      {[q ? catName(it.category_id) : '', ...it.option_groups.map((g) => g.name)].filter(Boolean).join(' · ') || it.description || 'No options'}
                    </span>
                    <span className="row" style={{ gap: 4 }}>
                      {!it.is_enabled && <span className="badge">Hidden</span>}
                      {!it.is_available && <span className="badge badge-warn">Sold out</span>}
                    </span>
                  </Link>
                  <span className="item-price">{formatMoney(it.price_minor, settings.currency)}</span>
                  {canEdit && (
                    <span className="row-actions">
                      <Toggle label={`${it.name} available`} hideLabel checked={it.is_available} disabled={act.busy} onChange={(v) => patchItem(it, { is_available: v })} />
                      {!q && (
                        <>
                          <button type="button" className="btn btn-ghost btn-icon btn-small" aria-label={`Move ${it.name} up`} disabled={idx === 0 || act.busy}
                            onClick={() => patchItem(it, { position: idx - 1 })}><Icon name="arrowUp" size={16} /></button>
                          <button type="button" className="btn btn-ghost btn-icon btn-small" aria-label={`Move ${it.name} down`} disabled={idx === shown.length - 1 || act.busy}
                            onClick={() => patchItem(it, { position: idx + 1 })}><Icon name="arrowDown" size={16} /></button>
                        </>
                      )}
                    </span>
                  )}
                </li>
              ))}
            </ul>
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
      <form className="row-nowrap" onSubmit={(e) => { e.preventDefault(); onPatch({ name }); setEditing(false); }}>
        <input aria-label="Category name" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        <button type="submit" className="btn btn-primary" disabled={busy}>Save</button>
        <button type="button" className="btn" onClick={() => { setName(category.name); setEditing(false); }}>Cancel</button>
      </form>
    );
  }
  return (
    <div className="row">
      <h2 className="row-grow" style={{ fontSize: '1.2rem' }}>{category.name}</h2>
      {canEdit && (
        <>
          <Toggle label="Shown to guests" checked={category.is_enabled} disabled={busy} onChange={(v) => onPatch({ is_enabled: v })} />
          <button type="button" className="btn btn-small" onClick={() => { setName(category.name); setEditing(true); }}>Rename</button>
        </>
      )}
    </div>
  );
}
