import type { Currency, MinorUnits } from '../api/types';

// Minor-unit digits: a copy of internal/money/money.go (a Go test keeps the two
// in step). Not the browser's opinion: browsers with older currency data say
// IDR has 2 decimals, newer ones 0, and a price typed on one device would show
// 100 times off on another.
const zeroDecimal = new Set(['AFN', 'ALL', 'BIF', 'CLP', 'COP', 'DJF', 'GNF', 'HUF', 'IDR', 'IQD', 'IRR', 'ISK', 'JPY', 'KMF', 'KPW', 'KRW', 'LAK', 'LBP', 'MGA', 'MMK', 'PKR', 'PYG', 'RWF', 'SLL', 'SOS', 'SYP', 'UGX', 'VND', 'VUV', 'XAF', 'XOF', 'XPF', 'YER']);
const threeDecimal = new Set(['BHD', 'JOD', 'KWD', 'LYD', 'OMR', 'TND']);

/** Number of decimal places the currency uses (2 for EUR, 0 for JPY and IDR). */
export function currencyDigits(currency: Currency): number {
  return zeroDecimal.has(currency) ? 0 : threeDecimal.has(currency) ? 3 : 2;
}

/**
 * Format minor units for display. Display only: totals that matter come from
 * the server, never from this function.
 */
export function formatMoney(minor: MinorUnits, currency: Currency, locale?: string): string {
  const digits = currencyDigits(currency);
  const fmt = new Intl.NumberFormat(locale, { style: 'currency', currency, minimumFractionDigits: digits, maximumFractionDigits: digits });
  return fmt.format(minor / 10 ** digits);
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
