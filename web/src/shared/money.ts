import type { Currency, MinorUnits } from '../api/types';

/**
 * Format minor units for display.
 * Display only: totals that matter come from the server.
 */
export function formatMoney(minor: MinorUnits, currency: Currency, locale?: string): string {
  const fmt = new Intl.NumberFormat(locale, { style: 'currency', currency });
  const digits = fmt.resolvedOptions().maximumFractionDigits ?? 2;
  return fmt.format(minor / 10 ** digits);
}
