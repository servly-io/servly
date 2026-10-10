import { useEffect, useMemo, useState } from 'react';
import { useBlocker, useNavigate, useParams, useSearchParams } from 'react-router';
import { menuApi } from '../api/admin';
import type { MenuItem, MenuItemInput } from '../api/types';
import { formatMoney, minorToInput, parseMoney } from '../shared/money';
import { ErrorBanner, Field, Spinner, Toggle, useAction, useLoad } from '../shared/ui';
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
    const as = await act.run(() => menuApi.upload(file));
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

  return (
    <form onSubmit={save} className="stack editor">
      <h1>{isNew ? 'New item' : `Edit ${item.data?.name ?? ''}`}</h1>
      <div className="grid-2">
        <Field label="Name">
          <input value={draft.name} maxLength={120} required onChange={(e) => set({ name: e.target.value })} />
        </Field>
        <Field label={`Price (${cur})`} hint={unit !== null && unit >= 0 ? formatMoney(unit, cur) : 'for example 9.50'}>
          <input inputMode="decimal" value={draft.price} required onChange={(e) => set({ price: e.target.value })} />
        </Field>
      </div>
      <Field label="Description">
        <textarea rows={3} maxLength={1000} value={draft.description} onChange={(e) => set({ description: e.target.value })} />
      </Field>
      <div className="grid-2">
        <Field label="Category">
          <select value={draft.category_id} onChange={(e) => set({ category_id: e.target.value })}>
            {cats.data.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        </Field>
        <div className="stack-tight">
          <Toggle label="Shown on the menu" checked={draft.is_enabled} onChange={(v) => set({ is_enabled: v })} />
          <Toggle label="Available (untick when sold out)" checked={draft.is_available} onChange={(v) => set({ is_available: v })} />
        </div>
      </div>

      <fieldset className="card">
        <legend>Photo</legend>
        {draft.image_preview && <img src={draft.image_preview} alt="" className="preview" />}
        <div className="row">
          <input type="file" accept="image/jpeg,image/png,image/webp" aria-label="Upload photo" onChange={(e) => void upload(e.target.files?.[0])} />
          {draft.image_asset_id && <button type="button" className="btn" onClick={() => set({ image_asset_id: null, image_preview: null })}>Remove photo</button>}
        </div>
      </fieldset>

      <fieldset className="card stack">
        <legend>Options and extras</legend>
        {draft.groups.length === 0 && <p className="muted small">Add a group such as "Size" (choose exactly 1) or "Extras" (choose up to 3).</p>}
        {draft.groups.map((g, gi) => (
          <div key={g.id ?? `new-${gi}`} className="group stack-tight">
            <div className="row">
              <Field label="Group"><input value={g.name} required onChange={(e) => setGroup(gi, { name: e.target.value })} /></Field>
              <Field label="Choose at least"><input type="number" min={0} value={g.min_select} onChange={(e) => setGroup(gi, { min_select: Number(e.target.value) })} /></Field>
              <Field label="at most"><input type="number" min={1} value={g.max_select} onChange={(e) => setGroup(gi, { max_select: Number(e.target.value) })} /></Field>
              <button type="button" className="btn" onClick={() => set({ groups: draft.groups.filter((_, i) => i !== gi) })}>Remove group</button>
            </div>
            {g.options.map((o, oi) => (
              <div key={o.id ?? `new-${oi}`} className="row option-row">
                <input aria-label="Option name" placeholder="Option" value={o.name} required onChange={(e) => setOption(gi, oi, { name: e.target.value })} />
                <input aria-label={`Price change (${cur})`} placeholder="+0.00" inputMode="decimal" className="input-short" value={o.price}
                  onChange={(e) => setOption(gi, oi, { price: e.target.value })} />
                <Toggle label="Available" checked={o.is_available} onChange={(v) => setOption(gi, oi, { is_available: v })} />
                <button type="button" className="btn btn-icon" aria-label={`Remove option ${o.name}`}
                  onClick={() => setGroup(gi, { options: g.options.filter((_, i) => i !== oi) })}>×</button>
              </div>
            ))}
            <button type="button" className="btn btn-small" onClick={() => setGroup(gi, { options: [...g.options, { name: '', price: '', is_available: true }] })}>
              Add option
            </button>
          </div>
        ))}
        <button type="button" className="btn" onClick={() => set({ groups: [...draft.groups, { name: '', min_select: 0, max_select: 1, options: [{ name: '', price: '', is_available: true }] }] })}>
          Add option group
        </button>
      </fieldset>

      <ErrorBanner error={act.error} />
      <div className="row sticky-actions">
        <button type="submit" className="btn btn-primary btn-large" disabled={act.busy}>{act.busy ? 'Saving…' : 'Save'}</button>
        <button type="button" className="btn btn-large" onClick={() => navigate(`/admin/menu?category=${draft.category_id}`)}>Cancel</button>
        {!isNew && (
          <>
            <button type="button" className="btn" disabled={act.busy || dirty} title={dirty ? 'Save first' : undefined} onClick={() => void duplicate()}>Duplicate</button>
            <button type="button" className="btn btn-danger" disabled={act.busy} onClick={() => void remove()}>Delete</button>
          </>
        )}
        {dirty && <span className="muted small">Unsaved changes</span>}
      </div>
    </form>
  );
}
