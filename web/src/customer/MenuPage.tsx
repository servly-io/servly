import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router';
import { publicApi } from '../api/public';
import type { PublicMenu, PublicMenuItem } from '../api/types';
import { formatMoney } from '../shared/money';
import { ErrorBanner, Spinner, useLoad } from '../shared/ui';
import { useCart } from './cart';

/** Public menu opened from a table QR code: /m/:token. */
export function MenuPage() {
  const { token = '' } = useParams();
  const menu = useLoad(() => publicApi.menu(token), [token]);
  const cart = useCart(token);
  const [open, setOpen] = useState<PublicMenuItem | null>(null);

  if (!menu.data) {
    if (menu.error) {
      return (
        <main className="page page-narrow">
          <h1>Menu not available</h1>
          <p>This QR code does not open a menu right now. Please ask the staff.</p>
          <ErrorBanner error={menu.error} onRetry={menu.reload} />
        </main>
      );
    }
    return <main className="page"><Spinner label="Loading the menu…" /></main>;
  }
  const m = menu.data;
  const cur = m.restaurant.currency;

  return (
    <div className="guest">
      <header className="guest-header">
        <h1>{m.restaurant.name}</h1>
        <p className="muted">{m.dining_location.kind === 'pickup' ? `Pickup: ${m.dining_location.name}` : m.dining_location.name}</p>
        <nav className="chips" aria-label="Categories">
          {m.categories.map((c) => <a key={c.id} href={`#cat-${c.id}`} className="chip">{c.name}</a>)}
        </nav>
      </header>
      <main className="page guest-main">
        {m.categories.length === 0 && <p className="muted">The menu is empty right now.</p>}
        {m.categories.map((c) => (
          <section key={c.id} id={`cat-${c.id}`} aria-labelledby={`cat-h-${c.id}`} className="guest-cat">
            <h2 id={`cat-h-${c.id}`}>{c.name}</h2>
            {c.description && <p className="muted">{c.description}</p>}
            <ul className="list-plain stack-tight">
              {c.items.map((it) => (
                <li key={it.id}>
                  <button type="button" className="guest-item" disabled={!it.available} onClick={() => setOpen(it)}>
                    <span className="guest-item-text">
                      <strong>{it.name}</strong>
                      {it.description && <span className="muted small">{it.description}</span>}
                      <span>{it.available ? formatMoney(it.price_minor, cur) : 'Sold out'}</span>
                    </span>
                    {it.image_url && <img src={it.image_url} alt="" className="guest-thumb" loading="lazy" />}
                  </button>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </main>
      {cart.count > 0 && (
        <div className="cart-bar">
          <Link to={`/m/${token}/cart`} className="btn btn-primary btn-large btn-block">
            View order · {cart.count} {cart.count === 1 ? 'item' : 'items'} · {formatMoney(cart.provisionalTotal, cur)}
          </Link>
        </div>
      )}
      {open && <ItemSheet item={open} menu={m} onClose={() => setOpen(null)} onAdd={(line) => { cart.add(line); setOpen(null); }} />}
    </div>
  );
}

function ItemSheet({ item, menu, onClose, onAdd }: {
  item: PublicMenuItem; menu: PublicMenu; onClose: () => void;
  onAdd: (l: Parameters<ReturnType<typeof useCart>['add']>[0]) => void;
}) {
  const cur = menu.restaurant.currency;
  const dialog = useRef<HTMLDialogElement>(null);
  const [chosen, setChosen] = useState<Record<string, string[]>>(() =>
    Object.fromEntries(item.option_groups.map((g) => {
      // Preselect the first available option of a "choose exactly one" group.
      const first = g.min_select === 1 && g.max_select === 1 ? g.options.find((o) => o.available) : undefined;
      return [g.id, first ? [first.id] : []];
    })),
  );
  const [qty, setQty] = useState(1);
  const [notes, setNotes] = useState('');

  useEffect(() => {
    const d = dialog.current;
    d?.showModal();
    return () => d?.close();
  }, []);

  const toggle = (groupId: string, optionId: string, single: boolean) =>
    setChosen((c) => {
      const cur = c[groupId] ?? [];
      if (single) return { ...c, [groupId]: [optionId] };
      return { ...c, [groupId]: cur.includes(optionId) ? cur.filter((x) => x !== optionId) : [...cur, optionId] };
    });

  const unmet = item.option_groups.filter((g) => {
    const n = chosen[g.id]?.length ?? 0;
    return n < g.min_select || n > g.max_select;
  });
  const allOptions = item.option_groups.flatMap((g) => g.options.map((o) => ({ ...o, group: g.name })));
  const optionIds = item.option_groups.flatMap((g) => chosen[g.id] ?? []);
  const unit = item.price_minor + optionIds.reduce((s, id) => s + (allOptions.find((o) => o.id === id)?.price_delta_minor ?? 0), 0);

  return (
    <dialog ref={dialog} className="sheet" onClose={onClose} aria-labelledby="sheet-title">
      <form method="dialog" className="stack" onSubmit={(e) => {
        e.preventDefault();
        if (unmet.length) return;
        onAdd({
          itemId: item.id, name: item.name, quantity: qty, optionIds, notes: notes.trim(), unitPrice: unit,
          optionLabels: optionIds.map((id) => allOptions.find((o) => o.id === id)?.name ?? ''),
        });
      }}>
        {item.image_url && <img src={item.image_url} alt="" className="sheet-image" />}
        <h2 id="sheet-title">{item.name}</h2>
        {item.description && <p className="muted">{item.description}</p>}
        {item.option_groups.map((g) => {
          const single = g.max_select === 1;
          const hint = g.min_select === g.max_select ? `choose ${g.min_select}` : g.min_select > 0 ? `choose ${g.min_select}-${g.max_select}` : `up to ${g.max_select}`;
          return (
            <fieldset key={g.id} className="stack-tight">
              <legend>{g.name} <span className="muted small">({hint})</span></legend>
              {g.options.map((o) => {
                const checked = chosen[g.id]?.includes(o.id) ?? false;
                const full = !single && !checked && (chosen[g.id]?.length ?? 0) >= g.max_select;
                return (
                  <label key={o.id} className="choice">
                    <input type={single ? 'radio' : 'checkbox'} name={g.id} checked={checked} disabled={!o.available || full}
                      onChange={() => toggle(g.id, o.id, single)} />
                    <span className="row-grow">{o.name}{!o.available && ' (sold out)'}</span>
                    {o.price_delta_minor !== 0 && <span className="muted">{o.price_delta_minor > 0 ? '+' : ''}{formatMoney(o.price_delta_minor, cur)}</span>}
                  </label>
                );
              })}
            </fieldset>
          );
        })}
        {menu.restaurant.order_notes_enabled && (
          <label className="field">
            <span className="field-label">Note for the kitchen</span>
            <input value={notes} maxLength={500} placeholder="e.g. no onions" onChange={(e) => setNotes(e.target.value)} />
          </label>
        )}
        <div className="row">
          <span className="stepper" role="group" aria-label="Quantity">
            <button type="button" className="btn btn-icon" aria-label="One less" disabled={qty <= 1} onClick={() => setQty(qty - 1)}>−</button>
            <output aria-live="polite">{qty}</output>
            <button type="button" className="btn btn-icon" aria-label="One more" disabled={qty >= 99} onClick={() => setQty(qty + 1)}>+</button>
          </span>
          <button type="submit" className="btn btn-primary btn-large row-grow" disabled={unmet.length > 0}>
            Add · {formatMoney(unit * qty, cur)}
          </button>
        </div>
        {unmet.length > 0 && <p className="muted small" role="status">Choose {unmet.map((g) => g.name).join(', ')} first.</p>}
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
      </form>
    </dialog>
  );
}
