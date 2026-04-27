import { useCallback, useEffect, useState } from "react";
import { Button, Table, Typography } from "antd";
import type { ColumnsType } from "antd/es/table";
import { ReloadOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import { api } from "../api/client";

type WalletRow = {
  customer_id: string;
  currency: string;
  balance: number;
  updated_at: string;
};

export default function WalletsPage() {
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<WalletRow[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [limit] = useState(20);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { data } = await api.get<{ items: WalletRow[]; total: number }>("/ops/wallets", {
        params: { page, limit },
      });
      setItems(data.items ?? []);
      setTotal(data.total ?? 0);
    } finally {
      setLoading(false);
    }
  }, [page, limit]);

  useEffect(() => {
    void load();
  }, [load]);

  const columns: ColumnsType<WalletRow> = [
    { title: "Customer", dataIndex: "customer_id" },
    { title: "Currency", dataIndex: "currency", width: 100 },
    { title: "Balance", dataIndex: "balance", width: 140 },
    {
      title: "Updated",
      dataIndex: "updated_at",
      width: 200,
      render: (t: string) => dayjs(t).format("YYYY-MM-DD HH:mm:ss"),
    },
  ];

  return (
    <div>
      <Typography.Title level={4} style={{ marginTop: 0 }}>
        API wallet balances
      </Typography.Title>
      <Button icon={<ReloadOutlined />} onClick={() => void load()} style={{ marginBottom: 16 }}>
        Refresh
      </Button>
      <Table<WalletRow>
        rowKey="customer_id"
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
