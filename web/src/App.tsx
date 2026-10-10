import { createBrowserRouter, Outlet, RouterProvider } from 'react-router';
import { AdminLayout } from './admin/AdminLayout';
import { ItemEditor } from './admin/ItemEditor';
import { MenuPage as AdminMenuPage } from './admin/MenuPage';
import { SettingsPage } from './admin/SettingsPage';
import { SetupWizard } from './admin/SetupWizard';
import { SetupPage } from './admin/SetupPage';
import { StaffPage } from './admin/StaffPage';
import { TablesPage, TablesPrintPage } from './admin/TablesPage';
import { CartPage } from './customer/CartPage';
import { MenuPage } from './customer/MenuPage';
import { OrderStatusPage } from './customer/OrderStatusPage';
import { Home } from './shared/Home';
import { LoginPage } from './shared/LoginPage';
import { Placeholder } from './shared/Placeholder';
import { RequireSession } from './shared/RequireSession';
import { SessionProvider } from './shared/session';
import { OrderBoard } from './staff/OrderBoard';
import { PickupDisplay } from './staff/PickupDisplay';

// Route map (docs/docs/architecture.md#frontend):
//   /m/:token  customer menu, public, from a QR code
//   /o/:token  customer order status, public
//   /staff     order board, staff session
//   /staff/display  pickup screen for guests (order numbers), staff session
//   /admin/*   administration
//   /login     staff sign-in
//   /setup     first-run setup of a fresh installation
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
      { path: '/m/:token/cart', element: <CartPage /> },
      { path: '/o/:token', element: <OrderStatusPage /> },
      { path: '/login', element: <LoginPage /> },
      { path: '/setup', element: <SetupWizard /> },
      { path: '/staff', element: <RequireSession><OrderBoard /></RequireSession> },
      { path: '/staff/display', element: <RequireSession><PickupDisplay /></RequireSession> },
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
