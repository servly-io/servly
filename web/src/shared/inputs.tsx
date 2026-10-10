// Inputs that only accept what the server accepts: timezones and currencies
// from a list, money and counts as digits. Typing anything else is ignored.
import { currencyDigits, parseMoney } from './money';

const supported = (key: 'timeZone' | 'currency'): string[] =>
  typeof Intl.supportedValuesOf === 'function' ? Intl.supportedValuesOf(key) : [];

// "UTC" is valid on the server but missing from some browsers' lists.
const zones = [...new Set(['UTC', ...supported('timeZone')])];

function offsetOf(zone: string): string {
  try {
    const part = new Intl.DateTimeFormat('en', { timeZone: zone, timeZoneName: 'shortOffset' })
      .formatToParts(new Date())
      .find((p) => p.type === 'timeZoneName');
    return part?.value ?? '';
  } catch {
    return '';
  }
}

// Region -> zones, built once: "Europe" -> [{ id: "Europe/Berlin", label: "Berlin (GMT+2)" }].
const zoneGroups = (() => {
  const groups = new Map<string, { id: string; label: string }[]>();
  for (const id of zones) {
    const slash = id.indexOf('/');
    const region = slash < 0 ? 'Other' : id.slice(0, slash);
    const city = (slash < 0 ? id : id.slice(slash + 1)).replaceAll('_', ' ').replaceAll('/', ' / ');
    const off = offsetOf(id);
    groups.set(region, [...(groups.get(region) ?? []), { id, label: off ? `${city} (${off})` : city }]);
  }
  return [...groups.entries()];
})();

/** A timezone picker grouped by region, with each zone's current UTC offset. */
export function TimezoneSelect({ value, onChange, id }: { value: string; onChange: (v: string) => void; id?: string }) {
  if (zoneGroups.length <= 1) {
    // Very old browser without the list: free text, validated by the server.
    return <input id={id} required value={value} placeholder="Europe/Berlin" onChange={(e) => onChange(e.target.value)} />;
  }
  return (
    <select id={id} required value={value} onChange={(e) => onChange(e.target.value)}>
      {!value && <option value="">Choose a timezone</option>}
      {value && !zones.includes(value) && <option value={value}>{value}</option>}
      {zoneGroups.map(([region, list]) => (
        <optgroup key={region} label={region}>
          {list.map((z) => (
            <option key={z.id} value={z.id}>
              {z.label}
            </option>
          ))}
        </optgroup>
      ))}
    </select>
  );
}

const common = ['EUR', 'USD', 'GBP', 'CHF', 'PLN', 'CZK', 'SEK', 'NOK', 'DKK', 'TRY', 'JPY', 'AUD', 'CAD'];
const currencyNames = (() => {
  try {
    return new Intl.DisplayNames(['en'], { type: 'currency' });
  } catch {
    return null;
  }
})();
const currencyLabel = (c: string) => {
  const name = currencyNames?.of(c);
  return name && name !== c ? `${c} · ${name}` : c;
};
const allCurrencies = supported('currency');

/** A currency picker: the common ones first, then every ISO 4217 code. */
export function CurrencySelect({ value, onChange, id, disabled }: { value: string; onChange: (v: string) => void; id?: string; disabled?: boolean }) {
  if (allCurrencies.length === 0) {
    return (
      <input id={id} required disabled={disabled} maxLength={3} pattern="[A-Za-z]{3}" value={value}
        onChange={(e) => onChange(e.target.value.replace(/[^A-Za-z]/g, '').toUpperCase())} />
    );
  }
  const rest = allCurrencies.filter((c) => !common.includes(c));
  return (
    <select id={id} required disabled={disabled} value={value} onChange={(e) => onChange(e.target.value)}>
      {value && !allCurrencies.includes(value) && <option value={value}>{value}</option>}
      <optgroup label="Common">
        {common.filter((c) => allCurrencies.includes(c)).map((c) => <option key={c} value={c}>{currencyLabel(c)}</option>)}
      </optgroup>
      <optgroup label="All currencies">
        {rest.map((c) => <option key={c} value={c}>{currencyLabel(c)}</option>)}
      </optgroup>
    </select>
  );
}

/**
 * A money amount as typed: digits with at most the currency's decimals, "."
 * or "," as the separator, and a sign only where negative amounts make sense.
 * Keystrokes that would make anything else are dropped.
 */
export function MoneyInput({
  value,
  onChange,
  currency,
  signed,
  ...rest
}: { value: string; onChange: (v: string) => void; currency: string; signed?: boolean } & Omit<React.InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange'>) {
  const digits = currencyDigits(currency);
  const shape = new RegExp(`^${signed ? '[-+]?' : ''}\\d{0,9}${digits > 0 ? `(?:[.,]\\d{0,${digits}})?` : ''}$`);
  const invalid = value.trim() !== '' && parseMoney(value, currency) === null;
  return (
    <input
      {...rest}
      inputMode="decimal"
      autoComplete="off"
      value={value}
      aria-invalid={invalid || undefined}
      onChange={(e) => {
        const v = e.target.value.trim();
        if (shape.test(v)) onChange(v);
      }}
    />
  );
}

/** A whole number between min and max, typed as digits only. */
export function CountInput({
  value,
  onChange,
  min = 0,
  max = 99,
  ...rest
}: { value: number; onChange: (v: number) => void; min?: number; max?: number } & Omit<React.InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange' | 'min' | 'max'>) {
  return (
    <input
      {...rest}
      inputMode="numeric"
      autoComplete="off"
      value={String(value)}
      onChange={(e) => {
        const v = e.target.value.replace(/\D/g, '').slice(0, 3);
        onChange(Math.min(max, Math.max(min, v === '' ? min : Number(v))));
      }}
      onFocus={(e) => e.target.select()}
    />
  );
}
