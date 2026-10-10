// Typed calls of the staff API (docs/docs/api.md). One function per endpoint.
import { api } from './client';
import type {
  Asset,
  Category,
  DiningKind,
  DiningLocation,
  Items,
  MenuItem,
  MenuItemInput,
  Role,
  Session,
  Settings,
  StaffMember,
} from './types';

export const auth = {
  session: () => api<Session>('GET', '/api/auth/session'),
  login: (email: string, password: string) => api<Session>('POST', '/api/auth/login', { body: { email, password } }),
  logout: () => api<void>('POST', '/api/auth/logout'),
  switchTenant: (tenant_id: string) => api<Session>('PUT', '/api/auth/session/tenant', { body: { tenant_id } }),
};

export const menuApi = {
  categories: () => api<Items<Category>>('GET', '/api/admin/menu/categories'),
  createCategory: (body: Partial<Pick<Category, 'name' | 'description' | 'is_enabled' | 'position'>>) =>
    api<Category>('POST', '/api/admin/menu/categories', { body }),
  updateCategory: (id: string, body: Partial<Pick<Category, 'name' | 'description' | 'is_enabled' | 'position'>>) =>
    api<Category>('PATCH', `/api/admin/menu/categories/${id}`, { body }),
  items: (categoryId?: string) =>
    api<Items<MenuItem>>('GET', '/api/admin/menu/items' + (categoryId ? `?category_id=${categoryId}` : '')),
  item: (id: string) => api<MenuItem>('GET', `/api/admin/menu/items/${id}`),
  createItem: (body: MenuItemInput) => api<MenuItem>('POST', '/api/admin/menu/items', { body }),
  updateItem: (id: string, body: MenuItemInput) => api<MenuItem>('PATCH', `/api/admin/menu/items/${id}`, { body }),
  duplicateItem: (id: string) => api<MenuItem>('POST', `/api/admin/menu/items/${id}/duplicate`),
  deleteItem: (id: string) => api<{ archived: boolean } | undefined>('DELETE', `/api/admin/menu/items/${id}`),
  upload: (file: File) => {
    const form = new FormData();
    form.append('file', file);
    return api<Asset>('POST', '/api/admin/assets', { form });
  },
};

export const diningApi = {
  list: () => api<Items<DiningLocation>>('GET', '/api/admin/dining-locations'),
  create: (name: string, kind: DiningKind) => api<DiningLocation>('POST', '/api/admin/dining-locations', { body: { name, kind } }),
  update: (id: string, body: { name?: string; is_enabled?: boolean }) =>
    api<DiningLocation>('PATCH', `/api/admin/dining-locations/${id}`, { body }),
  rotate: (id: string) => api<DiningLocation>('POST', `/api/admin/dining-locations/${id}/rotate-token`),
  qrURL: (id: string, format: 'svg' | 'png' = 'svg') =>
    `/api/admin/dining-locations/${id}/qr` + (format === 'png' ? '?format=png' : ''),
};

export const settingsApi = {
  get: () => api<Settings>('GET', '/api/admin/settings'),
  update: (body: Partial<Settings>) => api<Settings>('PATCH', '/api/admin/settings', { body }),
};

export const staffApi = {
  list: () => api<Items<StaffMember>>('GET', '/api/admin/staff'),
  add: (body: { email: string; display_name?: string; role: Role; password?: string }) =>
    api<StaffMember>('POST', '/api/admin/staff', { body }),
  update: (userId: string, body: { role?: Role; password?: string }) =>
    api<StaffMember>('PATCH', `/api/admin/staff/${userId}`, { body }),
  remove: (userId: string) => api<void>('DELETE', `/api/admin/staff/${userId}`),
};
