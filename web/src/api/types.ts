// Wire types of the API contract (docs/docs/api.md). Keep in step with the
// contract; the server is authoritative for every value here.

/** Integer minor units (cents). */
export type MinorUnits = number;

/** ISO 4217 code, such as "EUR". */
export type Currency = string;

export type OrderStatus = 'new' | 'accepted' | 'preparing' | 'ready' | 'completed' | 'cancelled';

export type PaymentStatus = 'unpaid' | 'pending' | 'paid' | 'failed' | 'partially_refunded' | 'refunded';

export type PaymentMethod = 'cash' | 'external_terminal' | 'other';

export type OrderMode = 'dine_in' | 'pickup';

export type Role = 'owner' | 'manager' | 'staff';

/** The body of every error response. */
export interface ErrorBody {
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
    request_id?: string;
  };
}

/** A page of a collection; pass next_cursor back as ?cursor= for the next one. */
export interface Page<T> {
  items: T[];
  next_cursor: string | null;
}

export type Permission =
  | 'orders.view'
  | 'orders.update_status'
  | 'orders.cancel_started'
  | 'payments.record'
  | 'payments.refund'
  | 'menu.manage'
  | 'dining_locations.manage'
  | 'staff.manage'
  | 'settings.manage';

export interface Session {
  user: { id: string; email: string; display_name: string; is_platform_admin: boolean };
  csrf_token: string;
  active_tenant: { id: string; name: string; role: Role; permissions: Permission[] } | null;
  memberships: { tenant_id: string; tenant_name: string; role: Role }[];
  expires_at: string;
}

export interface Items<T> {
  items: T[];
}

export interface Category {
  id: string;
  name: string;
  description: string;
  position: number;
  is_enabled: boolean;
}

export interface MenuOption {
  id: string;
  name: string;
  price_delta_minor: MinorUnits;
  is_available: boolean;
}

export interface OptionGroup {
  id: string;
  name: string;
  min_select: number;
  max_select: number;
  options: MenuOption[];
}

export interface MenuItem {
  id: string;
  category_id: string;
  name: string;
  description: string;
  price_minor: MinorUnits;
  image_asset_id: string | null;
  image_url: string | null;
  is_enabled: boolean;
  is_available: boolean;
  position: number;
  option_groups: OptionGroup[];
}

/** Item create/patch body; groups and options without id are new. */
export interface MenuItemInput {
  category_id?: string;
  name?: string;
  description?: string;
  price_minor?: MinorUnits;
  image_asset_id?: string | null;
  is_enabled?: boolean;
  is_available?: boolean;
  position?: number;
  option_groups?: {
    id?: string;
    name: string;
    min_select: number;
    max_select: number;
    options: { id?: string; name: string; price_delta_minor: MinorUnits; is_available: boolean }[];
  }[];
}

export interface Asset {
  id: string;
  url: string;
  admin_url: string;
  content_type: string;
  size_bytes: number;
}

export type DiningKind = 'table' | 'pickup';

export interface DiningLocation {
  id: string;
  name: string;
  kind: DiningKind;
  public_token: string;
  menu_url: string;
  is_enabled: boolean;
}

export interface Settings {
  name: string;
  currency: Currency;
  timezone: string;
  order_notes_enabled: boolean;
  public_status_enabled: boolean;
}

export interface StaffMember {
  user_id: string;
  email: string;
  display_name: string;
  role: Role;
  status: 'active' | 'disabled';
}

export interface PublicMenu {
  restaurant: { name: string; currency: Currency; order_notes_enabled: boolean };
  dining_location: { name: string; kind: DiningKind };
  categories: {
    id: string;
    name: string;
    description: string;
    items: {
      id: string;
      name: string;
      description: string;
      price_minor: MinorUnits;
      image_url: string | null;
      available: boolean;
      option_groups: {
        id: string;
        name: string;
        min_select: number;
        max_select: number;
        options: { id: string; name: string; price_delta_minor: MinorUnits; available: boolean }[];
      }[];
    }[];
  }[];
}

export type PublicMenuItem = PublicMenu['categories'][number]['items'][number];

export interface OrderLine {
  name: string;
  quantity: number;
  unit_price_minor: MinorUnits;
  options_total_minor: MinorUnits;
  line_total_minor: MinorUnits;
  notes: string;
  options: { group_name: string; option_name: string; price_delta_minor: MinorUnits }[];
}

export interface Quote {
  mode: OrderMode;
  dining_location: string;
  currency: Currency;
  subtotal_minor: MinorUnits;
  total_minor: MinorUnits;
  items: OrderLine[];
}

export interface PublicOrder extends Quote {
  order_token: string;
  number: number;
  status: OrderStatus;
  payment_status: PaymentStatus;
  notes: string;
  created_at: string;
}

export interface OrderRequest {
  dining_location_token: string;
  notes: string;
  items: { menu_item_id: string; quantity: number; option_ids: string[]; notes: string }[];
}

export interface StaffOrder {
  id: string;
  number: number;
  business_date: string;
  mode: OrderMode;
  dining_location: { id: string; name: string } | null;
  status: OrderStatus;
  payment_status: PaymentStatus;
  version: number;
  currency: Currency;
  subtotal_minor: MinorUnits;
  total_minor: MinorUnits;
  notes: string;
  items: OrderLine[];
  created_at: string;
  updated_at: string;
}

export interface Payment {
  id: string;
  method: PaymentMethod;
  status: PaymentStatus;
  amount_minor: MinorUnits;
  refunded_minor: MinorUnits;
  currency: Currency;
  reference: string;
  recorded_by: { id: string; name: string } | null;
  created_at: string;
}

export interface OrderDetail extends StaffOrder {
  outstanding_minor: MinorUnits;
  events: { from: OrderStatus | null; to: OrderStatus; actor: { id: string; name: string } | null; reason: string; created_at: string }[];
  payments: Payment[];
}
