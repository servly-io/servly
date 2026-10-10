import { createBrowserRouter, Outlet, RouterProvider } from 'react-router';
import { AdminLayout } from './admin/AdminLayout';
import { ItemEditor } from './admin/ItemEditor';
import { MenuPage as AdminMenuPage } from './admin/MenuPage';
import { SettingsPage } from './admin/SettingsPage';
import { SetupPage } from './admin/SetupPage';
import { StaffPage } from './admin/StaffPage';
import { TablesPage, TablesPrintPage } from './admin/TablesPage';
import { MenuPage } from './customer/MenuPage';
import { Home } from './shared/Home';
import { LoginPage } from './shared/LoginPage';
import { Placeholder } from './shared/Placeholder';
import { RequireSession } from './shared/RequireSession';
import { SessionProvider } from './shared/session';
import { OrderBoard } from './staff/OrderBoard';

// Route map (docs/docs/architecture.md#frontend):
//   /m/:token  customer menu, public, from a QR code
//   /o/:token  customer order status, public
//   /staff     order board, staff session
//   /admin/*   administration
//   /login     staff sign-in
const router = createBrowserRouter([
  {
    element: (
      <SessionProvider>
        <Outlet />
      </SessionProvider>
    ),
    children: [
      { path: '/', element: <Home /> },
      { path: '/m/:token', element: <MenuPage /> },
      { path: '/o/:token', element: <Placeholder title="Your order" phase="Phase 3, customer ordering" /> },
      { path: '/login', element: <LoginPage /> },
      { path: '/staff', element: <RequireSession><OrderBoard /></RequireSession> },
      {
        path: '/admin',
        element: <RequireSession><AdminLayout /></RequireSession>,
        children: [
          { index: true, element: <SetupPage /> },
          { path: 'menu', element: <AdminMenuPage /> },
          { path: 'menu/items/new', element: <ItemEditor /> },
          { path: 'menu/items/:id', element: <ItemEditor /> },
          { path: 'tables', element: <TablesPage /> },
          { path: 'tables/print', element: <TablesPrintPage /> },
          { path: 'staff', element: <StaffPage /> },
          { path: 'settings', element: <SettingsPage /> },
        ],
      },
      { path: '*', element: <Placeholder title="Not found" phase="this page does not exist" /> },
    ],
  },
]);

export function App() {
  return <RouterProvider router={router} />;
}
