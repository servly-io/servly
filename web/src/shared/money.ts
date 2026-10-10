import type { Currency, MinorUnits } from '../api/types';

/** Number of decimal places the currency uses (2 for EUR, 0 for JPY). */
export function currencyDigits(currency: Currency): number {
  return new Intl.NumberFormat('en', { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2;
}

/**
 * Format minor units for display. Display only: totals that matter come from
 * the server, never from this function.
 */
export function formatMoney(minor: MinorUnits, currency: Currency, locale?: string): string {
  const fmt = new Intl.NumberFormat(locale, { style: 'currency', currency });
  return fmt.format(minor / 10 ** currencyDigits(currency));
}

/**
 * Parse a price typed by an administrator ("9,50", "9.5", "-2") into minor
 * units, without floating point. Returns null when the text is not a price.
 */
export function parseMoney(text: string, currency: Currency): MinorUnits | null {
  const digits = currencyDigits(currency);
  const m = /^\s*([-+]?)(\d{1,9})(?:[.,](\d*))?\s*$/.exec(text);
  if (!m) return null;
  const [, sign, whole = '0', frac = ''] = m;
  if (frac.length > digits) return null;
  const minor = Number(whole) * 10 ** digits + Number(frac.padEnd(digits, '0') || '0');
  return sign === '-' ? -minor : minor;
}

/** The inverse of parseMoney, for prefilling inputs: 950 EUR -> "9.50". */
export function minorToInput(minor: MinorUnits, currency: Currency): string {
  const digits = currencyDigits(currency);
  const sign = minor < 0 ? '-' : '';
  const abs = Math.abs(minor);
  if (digits === 0) return sign + String(abs);
  const s = String(abs).padStart(digits + 1, '0');
  return `${sign}${s.slice(0, -digits)}.${s.slice(-digits)}`;
}
