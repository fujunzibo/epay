import { useState } from "react";
import { Layout, Menu, theme } from "antd";
import {
  DashboardOutlined,
  OrderedListOutlined,
  WalletOutlined,
  AuditOutlined,
  ToolOutlined,
} from "@ant-design/icons";
import { Outlet, useLocation, useNavigate } from "react-router-dom";

const { Header, Sider, Content } = Layout;

const menuItems = [
  { key: "/dashboard", icon: <DashboardOutlined />, label: "Dashboard" },
  { key: "/orders", icon: <OrderedListOutlined />, label: "Orders" },
  { key: "/wallets", icon: <WalletOutlined />, label: "Wallets" },
  { key: "/reconcile", icon: <AuditOutlined />, label: "Reconcile" },
  { key: "/tools", icon: <ToolOutlined />, label: "Tools" },
];

export default function AdminLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const nav = useNavigate();
  const loc = useLocation();
  const {
    token: { colorBgContainer },
  } = theme.useToken();

  const selected =
    menuItems.find((m) => {
      if (m.key === "/dashboard") return loc.pathname === "/dashboard";
      return loc.pathname === m.key || loc.pathname.startsWith(`${m.key}/`);
    })?.key ?? "/dashboard";

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Sider collapsible collapsed={collapsed} onCollapse={setCollapsed} theme="dark" width={220}>
        <div
          style={{
            height: 56,
            margin: 16,
            borderRadius: 8,
            background: "rgba(255,255,255,0.08)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: "#fff",
            fontWeight: 600,
            letterSpacing: 0.5,
          }}
        >
          ePay Admin
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selected]}
          items={menuItems}
          onClick={({ key }) => nav(key)}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            padding: "0 24px",
            background: colorBgContainer,
            display: "flex",
            alignItems: "center",
            borderBottom: "1px solid rgba(0,0,0,0.06)",
          }}
        >
          <span style={{ fontSize: 16, fontWeight: 600 }}>Payment &amp; top-up console</span>
        </Header>
        <Content style={{ margin: 24 }}>
          <div
            style={{
              padding: 24,
              minHeight: 360,
              background: colorBgContainer,
              borderRadius: 12,
            }}
          >
            <Outlet />
          </div>
        </Content>
      </Layout>
    </Layout>
  );
}
