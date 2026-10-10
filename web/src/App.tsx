import { Route, Routes } from 'react-router';
import { AdminHome } from './admin/AdminHome';
import { MenuPage } from './customer/MenuPage';
import { Home } from './shared/Home';
import { Placeholder } from './shared/Placeholder';
import { OrderBoard } from './staff/OrderBoard';

// Route map (docs/docs/architecture.md#frontend):
//   /m/:token  customer menu, public, from a QR code
//   /o/:token  customer order status, public
//   /staff     order board, staff session
//   /admin/*   administration, manager or owner
//   /login     staff sign-in
export function App() {
  return (
    <Routes>
      <Route path="/" element={<Home />} />
      <Route path="/m/:token" element={<MenuPage />} />
      <Route path="/o/:token" element={<Placeholder title="Your order" phase="Phase 3, customer ordering" />} />
      <Route path="/staff" element={<OrderBoard />} />
      <Route path="/admin/*" element={<AdminHome />} />
      <Route path="/login" element={<Placeholder title="Sign in" phase="Phase 2, tenant, access and menu" />} />
      <Route path="*" element={<Placeholder title="Not found" phase="this page does not exist" />} />
    </Routes>
  );
}
