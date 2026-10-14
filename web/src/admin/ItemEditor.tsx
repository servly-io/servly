import { useEffect, useMemo, useState } from 'react';
import { useBlocker, useNavigate, useParams, useSearchParams } from 'react-router';
import { menuApi } from '../api/admin';
import type { MenuItem, MenuItemInput } from '../api/types';
import { formatMoney, minorToInput, parseMoney } from '../shared/money';
import { Icon } from '../shared/icons';
import { fitImage, formatBytes } from '../shared/image';
import { CountInput, MoneyInput } from '../shared/inputs';
import { ErrorBanner, Field, PageHeader, Spinner, Toggle, useAction, useLoad } from '../shared/ui';
import { useAdmin } from './AdminLayout';

interface OptionDraft { id?: string; name: string; price: string; is_available: boolean }
interface GroupDraft { id?: string; name: string; min_select: number; max_select: number; options: OptionDraft[] }
interface Draft {
  category_id: string;
  name: string;
  description: string;
  price: string;
  is_enabled: boolean;
  is_available: boolean;
  image_asset_id: string | null;
  image_preview: string | null;
  groups: GroupDraft[];
}

function toDraft(it: MenuItem, currency: string): Draft {
  return {
    category_id: it.category_id, name: it.name, description: it.description, price: minorToInput(it.price_minor, currency),
    is_enabled: it.is_enabled, is_available: it.is_available, image_asset_id: it.image_asset_id,
    image_preview: it.image_asset_id ? `/api/admin/assets/${it.image_asset_id}` : null,
    groups: it.option_groups.map((g) => ({
      id: g.id, name: g.name, min_select: g.min_select, max_select: g.max_select,
      options: g.options.map((o) => ({ id: o.id, name: o.name, price: minorToInput(o.price_delta_minor, currency), is_available: o.is_available })),
    })),
  };
}

/** Builds the API body; returns an error message for unparseable prices. */
function toInput(d: Draft, currency: string): MenuItemInput | string {
  const price = parseMoney(d.price, currency);
  if (price === null || price < 0) return 'Enter the price as a number, such as 9.50.';
  const groups: NonNullable<MenuItemInput['option_groups']> = [];
  for (const g of d.groups) {
    const label = g.name.trim() || 'An option group';
    if (g.options.length === 0) return `${label} has no options. Add one, or remove the group.`;
    if (g.max_select < 1) return `${label}: guests must be able to choose at least 1 option (Max).`;
    if (g.min_select > g.max_select) return `${label}: Min (${g.min_select}) cannot be more than Max (${g.max_select}).`;
    if (g.min_select > g.options.length) return `${label}: guests must choose ${g.min_select}, but there are only ${g.options.length} options.`;
    const options = [];
    for (const o of g.options) {
      const delta = parseMoney(o.price || '0', currency);
      if (delta === null) return `Option "${o.name}": enter the price change as a number, such as 1.50 or -0.50.`;
      options.push({ id: o.id, name: o.name, price_delta_minor: delta, is_available: o.is_available });
    }
    groups.push({ id: g.id, name: g.name, min_select: g.min_select, max_select: g.max_select, options });
  }
  return {
    category_id: d.category_id, name: d.name, description: d.description, price_minor: price,
    is_enabled: d.is_enabled, is_available: d.is_available, image_asset_id: d.image_asset_id, option_groups: groups,
  };
}

export function ItemEditor() {
  const { id } = useParams();
  const isNew = id === undefined;
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { settings } = useAdmin();
  const cur = settings.currency;
  const cats = useLoad(() => menuApi.categories().then((r) => r.items));
  const item = useLoad(() => (isNew ? Promise.resolve(undefined) : menuApi.item(id)), [id]);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [saved, setSaved] = useState<string>('');
  const act = useAction();

  useEffect(() => {
    if (draft) return;
    if (isNew && cats.data) {
      const d: Draft = { category_id: params.get('category') ?? cats.data[0]?.id ?? '', name: '', description: '', price: '',
        is_enabled: true, is_available: true, image_asset_id: null, image_preview: null, groups: [] };
      setDraft(d);
      setSaved(JSON.stringify(d));
    } else if (item.data) {
      const d = toDraft(item.data, cur);
      setDraft(d);
      setSaved(JSON.stringify(d));
    }
  }, [draft, isNew, cats.data, item.data, params, cur]);

  const dirty = useMemo(() => draft !== null && JSON.stringify(draft) !== saved, [draft, saved]);
  const blocker = useBlocker(({ currentLocation, nextLocation }) => dirty && currentLocation.pathname !== nextLocation.pathname);
  useEffect(() => {
    if (blocker.state === 'blocked') {
      if (window.confirm('Discard your unsaved changes?')) blocker.proceed();
      else blocker.reset();
    }
  }, [blocker]);
  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);

  if (item.error) return <ErrorBanner error={item.error} />;
  if (!draft || !cats.data) return <Spinner />;
  const set = (patch: Partial<Draft>) => setDraft({ ...draft, ...patch });
  const setGroup = (gi: number, patch: Partial<GroupDraft>) =>
    set({ groups: draft.groups.map((g, i) => (i === gi ? { ...g, ...patch } : g)) });
  const setOption = (gi: number, oi: number, patch: Partial<OptionDraft>) =>
    setGroup(gi, { options: draft.groups[gi]!.options.map((o, i) => (i === oi ? { ...o, ...patch } : o)) });

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    const body = toInput(draft, cur);
    if (typeof body === 'string') {
      act.setError(new Error(body));
      return;
    }
    const res = await act.run(() => (isNew ? menuApi.createItem(body) : menuApi.updateItem(id, body)));
    if (res) {
      setSaved(JSON.stringify(draft));
      setDraft(null);
      setTimeout(() => navigate(`/admin/menu?category=${res.category_id}`), 0);
    }
  };

  const upload = async (file: File | undefined) => {
    if (!file) return;
    const as = await act.run(async () => menuApi.upload(await fitImage(file, settings.max_image_bytes)));
    if (as) set({ image_asset_id: as.id, image_preview: as.admin_url });
  };

  const remove = async () => {
    if (isNew || !window.confirm(`Delete "${draft.name}"? Items that past orders contain are archived instead.`)) return;
    const ok = await act.run(async () => {
      await menuApi.deleteItem(id);
      return true;
    });
    if (ok) {
      setSaved(JSON.stringify(draft));
      navigate(`/admin/menu?category=${draft.category_id}`);
    }
  };

  const duplicate = async () => {
    if (isNew) return;
    const copy = await act.run(() => menuApi.duplicateItem(id));
    if (copy) navigate(`/admin/menu/items/${copy.id}`);
  };

  const unit = parseMoney(draft.price, cur);

  const back = `/admin/menu?category=${draft.category_id}`;
  const rule = (g: GroupDraft) =>
    g.min_select === g.max_select ? `Guests choose exactly ${g.min_select}` : g.min_select > 0 ? `Guests choose ${g.min_select} to ${g.max_select}` : `Guests choose up to ${g.max_select}`;

  return (
    <form onSubmit={save} className="stack">
      <PageHeader
        back={{ to: back, label: 'Menu' }}
        title={isNew ? 'New item' : item.data?.name ?? ''}
        subtitle={!isNew && (
          <span className="row" style={{ gap: 6 }}>
            <span className={`badge ${draft.is_available ? 'badge-ok' : 'badge-warn'}`}>{draft.is_available ? 'Available' : 'Sold out'}</span>
            {!draft.is_enabled && <span className="badge">Hidden</span>}
          </span>
        )}
        actions={!isNew && (
          <>
            <button type="button" className="btn" disabled={act.busy || dirty} title={dirty ? 'Save first' : undefined} onClick={() => void duplicate()}>
              <Icon name="copy" size={18} />Duplicate
            </button>
            <button type="button" className="btn btn-danger" disabled={act.busy} onClick={() => void remove()}>
              <Icon name="trash" size={18} />Delete
            </button>
          </>
        )}
      />

      <div className="editor-grid">
        <div className="stack">
          <section className="card stack">
            <h2>Details</h2>
            <Field label="Name">
              <input value={draft.name} maxLength={120} required placeholder="Margherita" onChange={(e) => set({ name: e.target.value })} />
            </Field>
            <Field label="Description" hint="Guests read this on the menu. Mention what makes it special.">
              <textarea rows={3} maxLength={1000} value={draft.description} placeholder="Tomato, mozzarella, fresh basil" onChange={(e) => set({ description: e.target.value })} />
            </Field>
            <div className="grid-2">
              <Field label="Price" hint={unit !== null ? `Guests see ${formatMoney(unit, cur)}` : 'For example 9.50'}
                error={draft.price.trim() !== '' && unit === null ? 'Enter a price such as 9.50' : undefined}>
                <span className="input-affix">
                  <span>{cur}</span>
                  <MoneyInput currency={cur} value={draft.price} required placeholder="0.00" onChange={(v) => set({ price: v })} />
                </span>
              </Field>
              <Field label="Category">
                <select value={draft.category_id} onChange={(e) => set({ category_id: e.target.value })}>
                  {cats.data.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
                </select>
              </Field>
            </div>
          </section>

          <section className="card stack">
            <div>
              <h2>Options and extras</h2>
              <p className="muted small card-sub">Sizes, sides or toppings. Each group sets how many options guests may pick.</p>
            </div>
            {draft.groups.map((g, gi) => (
              <div key={g.id ?? `new-${gi}`} className="group">
                <div className="group-head">
                  <Field label="Group name"><input value={g.name} required maxLength={120} placeholder="Size" onChange={(e) => setGroup(gi, { name: e.target.value })} /></Field>
                  <Field label="Min"><CountInput min={0} max={100} value={g.min_select} onChange={(v) => setGroup(gi, { min_select: v })} /></Field>
                  <Field label="Max"><CountInput min={1} max={100} value={g.max_select} onChange={(v) => setGroup(gi, { max_select: v })} /></Field>
                  <button type="button" className="btn btn-ghost btn-icon" aria-label={`Remove group ${g.name}`} onClick={() => set({ groups: draft.groups.filter((_, i) => i !== gi) })}>
                    <Icon name="trash" size={18} />
                  </button>
                </div>
                {g.min_select > g.max_select ? (
                  <p className="field-error">Min cannot be more than Max.</p>
                ) : g.min_select > g.options.length ? (
                  <p className="field-error">Guests must choose {g.min_select}, but there {g.options.length === 1 ? 'is only 1 option' : `are only ${g.options.length} options`}.</p>
                ) : (
                  <p className="muted small">{rule(g)}{g.min_select > 0 ? ' (required)' : ' (optional)'}.</p>
                )}
                {g.options.map((o, oi) => (
                  <div key={o.id ?? `new-${oi}`} className="option-row">
                    <input aria-label="Option name" placeholder="Option, such as Large" value={o.name} required maxLength={120} onChange={(e) => setOption(gi, oi, { name: e.target.value })} />
                    <span className="input-affix input-affix-wide">
                      <span>+{cur}</span>
                      <MoneyInput signed currency={cur} aria-label={`Price change (${cur}), negative for a discount`} placeholder="0.00" value={o.price}
                        onChange={(v) => setOption(gi, oi, { price: v })} />
                    </span>
                    <Toggle label={`${o.name || 'Option'} available`} hideLabel checked={o.is_available} onChange={(v) => setOption(gi, oi, { is_available: v })} />
                    <button type="button" className="btn btn-ghost btn-icon btn-small" aria-label={`Remove option ${o.name}`}
                      onClick={() => setGroup(gi, { options: g.options.filter((_, i) => i !== oi) })}><Icon name="x" size={18} /></button>
                  </div>
                ))}
                <div>
                  <button type="button" className="btn btn-small btn-ghost" onClick={() => setGroup(gi, { options: [...g.options, { name: '', price: '', is_available: true }] })}>
                    <Icon name="plus" size={16} />Add option
                  </button>
                </div>
              </div>
            ))}
            <div>
              <button type="button" className="btn" onClick={() => set({ groups: [...draft.groups, { name: '', min_select: 0, max_select: 1, options: [{ name: '', price: '', is_available: true }] }] })}>
                <Icon name="plus" size={18} />Add option group
              </button>
            </div>
          </section>
        </div>

        <div className="stack">
          <section className="card stack-tight">
            <h2>Status</h2>
            <Toggle label="Shown on the menu" description="Hidden items stay here but guests do not see them." checked={draft.is_enabled} onChange={(v) => set({ is_enabled: v })} />
            <Toggle label="Available" description="Turn off when it sells out. Guests see it as sold out." checked={draft.is_available} onChange={(v) => set({ is_available: v })} />
          </section>
          <section className="card stack-tight">
            <h2>Photo</h2>
            {draft.image_preview ? (
              <>
                <img src={draft.image_preview} alt="" className="photo-preview" />
                <div className="row">
                  <label className="btn btn-small">
                    Replace
                    <input type="file" className="visually-hidden" accept="image/jpeg,image/png,image/webp" onChange={(e) => void upload(e.target.files?.[0])} />
                  </label>
                  <button type="button" className="btn btn-small btn-ghost" onClick={() => set({ image_asset_id: null, image_preview: null })}>Remove</button>
                </div>
              </>
            ) : (
              <label className="photo-drop">
                <Icon name="image" size={28} />
                <span className="strong" style={{ color: 'var(--fg)' }}>Add a photo</span>
                <span className="small">JPEG, PNG or WebP. Large photos are made smaller to fit {formatBytes(settings.max_image_bytes)}.</span>
                <input type="file" accept="image/jpeg,image/png,image/webp" aria-label="Upload photo" onChange={(e) => void upload(e.target.files?.[0])} />
              </label>
            )}
          </section>
        </div>
      </div>

      <ErrorBanner error={act.error} />
      {(dirty || isNew) && (
        <div className="savebar" role="region" aria-label="Save changes">
          <span className="savebar-text"><Icon name="alert" size={18} />{isNew ? 'New item, not saved yet' : 'Unsaved changes'}</span>
          <button type="button" className="btn" onClick={() => navigate(back)}>Discard</button>
          <button type="submit" className="btn btn-accent" disabled={act.busy}>{act.busy ? 'Saving…' : 'Save'}</button>
        </div>
      )}
    </form>
  );
}
