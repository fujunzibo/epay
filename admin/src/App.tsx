import { Routes, Route, Navigate } from "react-router-dom";
import { App as AntdApp } from "antd";
import AdminLayout from "./layouts/AdminLayout";
import DashboardPage from "./pages/DashboardPage";
import OrdersPage from "./pages/OrdersPage";
import OrderDetailPage from "./pages/OrderDetailPage";
import WalletsPage from "./pages/WalletsPage";
import ReconcilePage from "./pages/ReconcilePage";
import ToolsPage from "./pages/ToolsPage";
import CheckoutPage from "./pages/CheckoutPage";
import CheckoutSuccessPage from "./pages/CheckoutSuccessPage";

export default function App() {
  return (
    <AntdApp>
      <Routes>
        <Route path="/checkout" element={<CheckoutPage />} />
        <Route path="/checkout/success" element={<CheckoutSuccessPage />} />
        <Route path="/" element={<AdminLayout />}>
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route path="dashboard" element={<DashboardPage />} />
          <Route path="orders" element={<OrdersPage />} />
          <Route path="orders/:orderNo" element={<OrderDetailPage />} />
          <Route path="wallets" element={<WalletsPage />} />
          <Route path="reconcile" element={<ReconcilePage />} />
          <Route path="tools" element={<ToolsPage />} />
        </Route>
      </Routes>
    </AntdApp>
  );
}
