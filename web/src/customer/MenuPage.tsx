import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router';
import { publicApi } from '../api/public';
import type { PublicMenu, PublicMenuItem } from '../api/types';
import { Icon } from '../shared/icons';
import { formatMoney } from '../shared/money';
import { EmptyState, ErrorBanner, Spinner, useLoad } from '../shared/ui';
import { useCart } from './cart';

/** Public menu opened from a table QR code: /m/:token. */
export function MenuPage() {
  const { token = '' } = useParams();
  const menu = useLoad(() => publicApi.menu(token), [token]);
  const cart = useCart(token);
  const [open, setOpen] = useState<PublicMenuItem | null>(null);
  const [query, setQuery] = useState('');
  const [catId, setCatId] = useState<string | null>(null);
  const head = useRef<HTMLElement>(null);

  if (!menu.data) {
    if (menu.error) {
      return (
        <main className="auth">
          <div className="auth-card">
            <EmptyState icon="qr" title="Menu not available">This QR code does not open a menu right now. Please ask the staff.</EmptyState>
            <ErrorBanner error={menu.error} onRetry={menu.reload} />
          </div>
        </main>
      );
    }
    return <main className="guest"><Spinner label="Loading the menu…" /></main>;
  }
  const m = menu.data;
  const cur = m.restaurant.currency;
  const pickup = m.dining_location.kind === 'pickup';
  const inCart = (id: string) => cart.lines.filter((l) => l.itemId === id).reduce((n, l) => n + l.quantity, 0);
  const q = query.trim().toLowerCase();
  const results = q
    ? m.categories.flatMap((c) => c.items).filter((it) => it.name.toLowerCase().includes(q) || it.description.toLowerCase().includes(q))
    : [];
  const cat = m.categories.find((c) => c.id === catId) ?? m.categories[0];
  const pick = (id: string, tab: HTMLElement) => {
    setCatId(id);
    tab.scrollIntoView({ inline: 'center', block: 'nearest', behavior: 'smooth' });
    // A guest deep in a long list starts the new category at its top, under the sticky bar.
    const top = head.current ? head.current.offsetTop + head.current.offsetHeight : 0;
    if (window.scrollY > top) window.scrollTo({ top });
  };
  const dish = (it: PublicMenuItem) => <Dish key={it.id} item={it} currency={cur} count={inCart(it.id)} onOpen={() => setOpen(it)} />;

  return (
    <div className="guest">
      <div className="store-hero" aria-hidden="true" />
      <header className="guest-wrap store-head" ref={head}>
        <span className="store-avatar" aria-hidden="true">{m.restaurant.name.charAt(0)}</span>
        <h1>{m.restaurant.name}</h1>
        <div className="store-meta">
          <span><Icon name={pickup ? 'bag' : 'pin'} size={16} /><strong>{m.dining_location.name}</strong>{pickup ? ' · Pickup' : ' · Dine-in'}</span>
          <span><Icon name="wallet" size={16} />{payLine(m.payment, pickup)}</span>
          <span><Icon name="user" size={16} />No account needed</span>
        </div>
      </header>

      <div className="cat-bar">
        <div className="cat-bar-inner">
          <label className="search">
            <Icon name="search" size={18} />
            <input type="search" placeholder={`Search ${m.restaurant.name}`} aria-label="Search the menu" value={query} onChange={(e) => setQuery(e.target.value)} />
          </label>
          {!q && m.categories.length > 0 && (
            <div className="cat-tabs" role="tablist" aria-label="Categories">
              {m.categories.map((c) => (
                <button key={c.id} type="button" role="tab" className="cat-tab" aria-selected={c.id === cat?.id} aria-controls="menu-panel"
                  onClick={(e) => pick(c.id, e.currentTarget)}>
                  {c.name}
                  <span className="cat-count">{c.items.length}</span>
                </button>
              ))}
            </div>
          )}
          {q && <div style={{ height: 8 }} />}
        </div>
      </div>

      <main className="guest-wrap">
        {m.categories.length === 0 && <EmptyState icon="utensils" title="The menu is empty right now">Please ask the staff.</EmptyState>}
        {q ? (
          results.length === 0 ? (
            <div className="no-results"><EmptyState icon="search" title={`Nothing found for “${query.trim()}”`}>Try another word, or browse the categories.</EmptyState></div>
          ) : (
            <section className="menu-section">
              <h2>{results.length} {results.length === 1 ? 'result' : 'results'}</h2>
              <ul className="dishes">{results.map(dish)}</ul>
            </section>
          )
        ) : (
          cat && (
            <section id="menu-panel" role="tabpanel" aria-label={cat.name} className="menu-section">
              {cat.description && <p className="muted">{cat.description}</p>}
              <ul className="dishes">{cat.items.map(dish)}</ul>
            </section>
          )
        )}
      </main>

      {cart.count > 0 && (
        <Link to={`/m/${token}/cart`} className="cart-fab">
          <span className="cart-fab-count">{cart.count}</span>
          <span className="cart-fab-label">View order</span>
          <span className="num">{formatMoney(cart.provisionalTotal, cur)}</span>
        </Link>
      )}
      {open && <ItemSheet item={open} menu={m} onClose={() => setOpen(null)} onAdd={(line) => { cart.add(line); setOpen(null); }} />}
    </div>
  );
}

function Dish({ item: it, currency, count, onOpen }: { item: PublicMenuItem; currency: string; count: number; onOpen: () => void }) {
  return (
    <li>
      <button type="button" className="dish" disabled={!it.available} onClick={onOpen}
        aria-label={`${it.name}, ${it.available ? formatMoney(it.price_minor, currency) : 'sold out'}${count ? `, ${count} in your order` : ''}`}>
        <span className="dish-text">
          <span className="dish-name">{it.name}</span>
          {it.description && <span className="dish-desc clamp-2">{it.description}</span>}
          <span className="dish-price">
            {it.available ? formatMoney(it.price_minor, currency) : <span className="badge sold-out">Sold out</span>}
          </span>
        </span>
        <span className={`dish-media${it.image_url ? '' : ' no-img'}`}>
          {it.image_url && <img src={it.image_url} alt="" className="dish-img" loading="lazy" />}
          {it.available && (
            <span className={`dish-add${count ? ' dish-qty' : ''}`} aria-hidden="true">
              {count ? count : <Icon name="plus" size={18} />}
            </span>
          )}
        </span>
      </button>
    </li>
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
      <form method="dialog" className="sheet-form" onSubmit={(e) => {
        e.preventDefault();
        if (unmet.length) return;
        onAdd({
          itemId: item.id, name: item.name, quantity: qty, optionIds, notes: notes.trim(), unitPrice: unit,
          optionLabels: optionIds.map((id) => allOptions.find((o) => o.id === id)?.name ?? ''),
        });
      }}>
        <div className="sheet-body">
          <div className="sheet-hero">
            <button type="button" className="sheet-close" aria-label="Close" onClick={onClose}><Icon name="x" /></button>
            {item.image_url && <img src={item.image_url} alt="" />}
          </div>
          <div className={`sheet-intro${item.image_url ? '' : ' no-img'}`}>
            <h2 id="sheet-title">{item.name}</h2>
            <span className="sheet-price num">{formatMoney(item.price_minor, cur)}</span>
            {item.description && <p className="muted">{item.description}</p>}
          </div>
          {item.option_groups.map((g) => {
            const single = g.max_select === 1;
            const n = chosen[g.id]?.length ?? 0;
            const met = n >= g.min_select && n <= g.max_select;
            const hint = g.min_select === g.max_select ? `Choose ${g.min_select}` : g.min_select > 0 ? `Choose ${g.min_select} to ${g.max_select}` : `Choose up to ${g.max_select}`;
            return (
              <fieldset key={g.id} className="opt-group">
                <div className="opt-head">
                  <div>
                    <legend>{g.name}</legend>
                    <span className="muted small">{hint}</span>
                  </div>
                  {g.min_select > 0 && (
                    met ? <span className="badge badge-ok"><Icon name="check" size={12} />Done</span> : <span className="badge badge-ink">Required</span>
                  )}
                  {g.min_select === 0 && <span className="badge">Optional</span>}
                </div>
                {g.options.map((o) => {
                  const checked = chosen[g.id]?.includes(o.id) ?? false;
                  const full = !single && !checked && n >= g.max_select;
                  return (
                    <label key={o.id} className="opt">
                      <input type={single ? 'radio' : 'checkbox'} name={g.id} checked={checked} disabled={!o.available || full}
                        onChange={() => toggle(g.id, o.id, single)} />
                      <span className="row-grow">{o.name}{!o.available && <span className="muted small"> · sold out</span>}</span>
                      {o.price_delta_minor !== 0 && <span className="muted num">{o.price_delta_minor > 0 ? '+' : ''}{formatMoney(o.price_delta_minor, cur)}</span>}
                    </label>
                  );
                })}
              </fieldset>
            );
          })}
          {menu.restaurant.order_notes_enabled && (
            <label className="field sheet-note">
              <span className="field-label">Note for the kitchen</span>
              <input value={notes} maxLength={500} placeholder="e.g. no onions" onChange={(e) => setNotes(e.target.value)} />
            </label>
          )}
        </div>
        <div className="sheet-foot">
          <span className="stepper" role="group" aria-label="Quantity">
            <button type="button" aria-label="One less" disabled={qty <= 1} onClick={() => setQty(qty - 1)}><Icon name="minus" size={18} /></button>
            <output aria-live="polite">{qty}</output>
            <button type="button" aria-label="One more" disabled={qty >= 99} onClick={() => setQty(qty + 1)}><Icon name="plus" size={18} /></button>
          </span>
          <button type="submit" className="btn btn-primary btn-large" disabled={unmet.length > 0}>
            {unmet.length > 0 ? (
              <span>Choose {unmet[0]!.name}</span>
            ) : (
              <>
                <span>Add {qty} to order</span>
                <span className="num">{formatMoney(unit * qty, cur)}</span>
              </>
            )}
          </button>
        </div>
      </form>
    </dialog>
  );
}

function payLine(p: PublicMenu['payment'], pickup: boolean): string {
  switch (p) {
    case 'before':
      return 'Pay online when ordering';
    case 'either':
      return 'Pay online or at the counter';
    case 'after':
      return pickup ? 'Pay online or when you collect' : 'Pay on your phone or at the table';
    default:
      return pickup ? 'Pay when you collect' : 'Pay at the table';
  }
}
