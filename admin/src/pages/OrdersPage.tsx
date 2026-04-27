import { useCallback, useEffect, useState } from "react";
import { Button, Select, Space, Table, Tag, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import { Link } from "react-router-dom";
import { ReloadOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import { api } from "../api/client";

type OrderRow = {
  order_no: string;
  customer_id: string;
  payment_method: string;
  provider: string;
  currency: string;
  amount: number;
  status: string;
  credited: boolean;
  created_at: string;
  updated_at: string;
};

export default function OrdersPage() {
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<OrderRow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [limit] = useState(20);
  const [status, setStatus] = useState<string | undefined>(undefined);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.get<{ items: OrderRow[]; total: number; page: number; limit: number }>(
        "/payments/orders",
        { params: { page, limit, status: status || undefined } }
      );
      setItems(data.items ?? []);
      setTotal(data.total ?? 0);
    } finally {
      setLoading(false);
    }
  }, [page, limit, status]);

  useEffect(() => {
    void load();
  }, [load]);

  const columns: ColumnsType<OrderRow> = [
    {
      title: "Order",
      dataIndex: "order_no",
      render: (v: string) => <Link to={`/orders/${encodeURIComponent(v)}`}>{v}</Link>,
    },
    { title: "Customer", dataIndex: "customer_id" },
    { title: "Method", dataIndex: "payment_method", width: 90 },
    { title: "Provider", dataIndex: "provider", width: 100 },
    {
      title: "Amount",
      key: "amt",
      render: (_, r) => `${r.amount} ${r.currency}`,
    },
    {
      title: "Status",
      dataIndex: "status",
      width: 110,
      render: (s: string) => {
        const color =
          s === "credited" ? "green" : s === "paid" ? "blue" : s === "failed" ? "red" : "default";
        return <Tag color={color}>{s}</Tag>;
      },
    },
    {
      title: "Credited",
      dataIndex: "credited",
      width: 90,
      render: (c: boolean) => (c ? <Tag color="success">yes</Tag> : <Tag>no</Tag>),
    },
    {
      title: "Updated",
      dataIndex: "updated_at",
      width: 180,
      render: (t: string) => dayjs(t).format("YYYY-MM-DD HH:mm:ss"),
    },
  ];

  return (
    <div>
      <Space style={{ marginBottom: 16 }} wrap>
        <Typography.Title level={4} style={{ margin: 0 }}>
          Orders
        </Typography.Title>
        <Select
          allowClear
          placeholder="Filter status"
          style={{ width: 160 }}
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={[
            { value: "pending", label: "pending" },
            { value: "paid", label: "paid" },
            { value: "credited", label: "credited" },
            { value: "failed", label: "failed" },
          ]}
        />
        <Button icon={<ReloadOutlined />} onClick={() => void load()}>
          Refresh
        </Button>
      </Space>
      <Table<OrderRow>
        rowKey="order_no"
        loading={loading}
        columns={columns}
        dataSource={items}
        pagination={{
          current: page,
          pageSize: limit,
          total,
          showSizeChanger: false,
          onChange: (p) => setPage(p),
        }}
      />
    </div>
  );
}
