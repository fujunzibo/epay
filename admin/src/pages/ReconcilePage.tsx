import { useEffect, useState } from "react";
import { Button, Card, Select, Space, Table, Tabs, Typography, message } from "antd";
import type { ColumnsType } from "antd/es/table";
import dayjs from "dayjs";
import { api } from "../api/client";

type Summary = Record<string, unknown>;

type DetailItem = {
  order_no: string;
  customer_id: string;
  status: string;
  credited: boolean;
  provider: string;
  amount: number;
  currency: string;
  updated_at: string;
};

export default function ReconcilePage() {
  const [sinceHours, setSinceHours] = useState(24);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [details, setDetails] = useState<DetailItem[]>([]);
  const [mode, setMode] = useState<"uncredited_paid" | "orders_no_event">("uncredited_paid");
  const [loading, setLoading] = useState(false);

  const loadSummary = async () => {
    try {
      const { data } = await api.get<Summary>("/ops/reconcile/summary", { params: { since_hours: sinceHours } });
      setSummary(data);
    } catch {
      message.error("Failed to load summary");
    }
  };

  const loadDetails = async () => {
    setLoading(true);
    try {
      const { data } = await api.get<{ items: DetailItem[] }>("/ops/reconcile/details", {
        params: { mode, limit: 200 },
      });
      setDetails(data.items ?? []);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadSummary();
  }, [sinceHours]);

  useEffect(() => {
    void loadDetails();
  }, [mode]);

  const columns: ColumnsType<DetailItem> = [
    { title: "Order", dataIndex: "order_no" },
    { title: "Customer", dataIndex: "customer_id" },
    { title: "Status", dataIndex: "status" },
    { title: "Credited", dataIndex: "credited", render: (c: boolean) => (c ? "yes" : "no") },
    {
      title: "Amount",
      key: "a",
      render: (_, r) => `${r.amount} ${r.currency}`,
    },
    {
      title: "Updated",
      dataIndex: "updated_at",
      render: (t: string) => dayjs(t).format("YYYY-MM-DD HH:mm:ss"),
    },
  ];

  return (
    <div>
      <Typography.Title level={4} style={{ marginTop: 0 }}>
        Reconcile
      </Typography.Title>
      <Tabs
        items={[
          {
            key: "summary",
            label: "Summary",
            children: (
              <Space direction="vertical" style={{ width: "100%" }} size="middle">
                <Space wrap>
                  <span>Since hours</span>
                  <Select
                    value={sinceHours}
                    style={{ width: 120 }}
                    onChange={(v) => setSinceHours(v)}
                    options={[6, 12, 24, 48, 72].map((h) => ({ value: h, label: String(h) }))}
                  />
                  <Button onClick={() => void loadSummary()}>Refresh</Button>
                </Space>
                <Card>
                  <pre style={{ margin: 0 }}>{JSON.stringify(summary, null, 2)}</pre>
                </Card>
              </Space>
            ),
          },
          {
            key: "details",
            label: "Details",
            children: (
              <Space direction="vertical" style={{ width: "100%" }} size="middle">
                <Space wrap>
                  <span>Mode</span>
                  <Select
                    value={mode}
                    style={{ width: 220 }}
                    onChange={(v) => setMode(v)}
                    options={[
                      { value: "uncredited_paid", label: "uncredited_paid" },
                      { value: "orders_no_event", label: "orders_no_event" },
                    ]}
                  />
                  <Button onClick={() => void loadDetails()}>Refresh</Button>
                </Space>
                <Table<DetailItem>
                  rowKey="order_no"
                  loading={loading}
                  columns={columns}
                  dataSource={details}
                  pagination={false}
                />
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}
