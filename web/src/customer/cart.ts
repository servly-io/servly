import { useCallback, useEffect, useState } from 'react';
import { newIdempotencyKey } from '../api/client';
import type { MinorUnits, OrderRequest, PublicOrder } from '../api/types';

export interface CartLine {
  key: string;
  itemId: string;
  name: string;
  quantity: number;
  optionIds: string[];
  optionLabels: string[];
  notes: string;
  /** Provisional unit price for display; the server prices the order. */
  unitPrice: MinorUnits;
}

interface CartState {
  lines: CartLine[];
  notes: string;
  /** One key per checkout, reused on retries so a retry cannot double-order. */
  checkoutKey: string;
}

const storageKey = (token: string) => `servly.cart.${token}`;
const empty = (): CartState => ({ lines: [], notes: '', checkoutKey: newIdempotencyKey() });

function read(token: string): CartState {
  try {
    const raw = sessionStorage.getItem(storageKey(token));
    if (raw) return { ...empty(), ...(JSON.parse(raw) as Partial<CartState>) };
  } catch {
    /* storage unavailable: an in-memory cart still works */
  }
  return empty();
}

/** The guest's cart for one table, kept across navigation and reloads. */
export function useCart(token: string) {
  const [cart, setCart] = useState<CartState>(() => read(token));

  useEffect(() => {
    try {
      sessionStorage.setItem(storageKey(token), JSON.stringify(cart));
    } catch {
      /* ignore */
    }
  }, [token, cart]);

  // Any change to the content starts a new checkout key: the old key belongs
  // to the old content.
  const change = useCallback((f: (c: CartState) => Omit<CartState, 'checkoutKey'>) => {
    setCart((c) => ({ ...f(c), checkoutKey: newIdempotencyKey() }));
  }, []);

  return {
    lines: cart.lines,
    notes: cart.notes,
    checkoutKey: cart.checkoutKey,
    count: cart.lines.reduce((n, l) => n + l.quantity, 0),
    provisionalTotal: cart.lines.reduce((s, l) => s + l.unitPrice * l.quantity, 0),
    add: (line: Omit<CartLine, 'key'>) => change((c) => ({ ...c, lines: [...c.lines, { ...line, key: newIdempotencyKey() }] })),
    setQuantity: (key: string, quantity: number) =>
      change((c) => ({ ...c, lines: c.lines.map((l) => (l.key === key ? { ...l, quantity } : l)).filter((l) => l.quantity > 0) })),
    setNotes: (notes: string) => change((c) => ({ ...c, notes })),
    clear: () => setCart(empty()),
    request: (): OrderRequest => ({
      dining_location_token: token,
      notes: cart.notes,
      items: cart.lines.map((l) => ({ menu_item_id: l.itemId, quantity: l.quantity, option_ids: l.optionIds, notes: l.notes })),
    }),
  };
}

/** Remember placed orders, to show the confirmation even when the status page is off. */
export function rememberOrder(o: PublicOrder, menuToken: string) {
  try {
    sessionStorage.setItem(`servly.order.${o.order_token}`, JSON.stringify(o));
    sessionStorage.setItem(`servly.order.${o.order_token}.menu`, menuToken);
  } catch {
    /* ignore */
  }
}

export function recallOrder(token: string): PublicOrder | null {
  try {
    const raw = sessionStorage.getItem(`servly.order.${token}`);
    return raw ? (JSON.parse(raw) as PublicOrder) : null;
  } catch {
    return null;
  }
}

/** The menu an order was placed from, for "Order more". */
export function recallMenuToken(orderToken: string): string | null {
  try {
    return sessionStorage.getItem(`servly.order.${orderToken}.menu`);
  } catch {
    return null;
  }
}
