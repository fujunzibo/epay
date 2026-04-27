import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button, Descriptions, Space, Spin, Tag, Typography, message } from "antd";
import { ArrowLeftOutlined, RedoOutlined } from "@ant-design/icons";
import dayjs from "dayjs";
import { api } from "../api/client";

type Order = {
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

export default function OrderDetailPage() {
  const { orderNo } = useParams<{ orderNo: string }>();
  const nav = useNavigate();
  const [loading, setLoading] = useState(true);
  const [order, setOrder] = useState<Order | null>(null);

  const load = async () => {
    if (!orderNo) return;
    setLoading(true);
    try {
      const { data } = await api.get<Order>(`/payments/orders/${encodeURIComponent(orderNo)}`);
      setOrder(data);
    } catch {
      message.error("Order not found");
      setOrder(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reload when route id changes
  }, [orderNo]);

  const retry = async () => {
    if (!orderNo) return;
    try {
      await api.post(`/payments/orders/${encodeURIComponent(orderNo)}/retry-credit`);
      message.success("Retry triggered");
      await load();
    } catch (e: unknown) {
      const msg =
        typeof e === "object" && e !== null && "response" in e
          ? String((e as { response?: { data?: { error?: string } } }).response?.data?.error)
          : "Retry failed";
      message.error(msg);
    }
  };

  if (loading) {
    return (
      <div style={{ textAlign: "center", padding: 48 }}>
        <Spin />
      </div>
    );
  }

  if (!order) {
    return <Typography.Text type="danger">No data</Typography.Text>;
  }

  return (
    <div>
      <Space style={{ marginBottom: 16 }}>
        <Button icon={<ArrowLeftOutlined />} onClick={() => nav("/orders")}>
          Back
        </Button>
        <Button type="primary" icon={<RedoOutlined />} onClick={() => void retry()}>
          Retry credit
        </Button>
      </Space>
      <Descriptions title={order.order_no} bordered column={1} size="small">
        <Descriptions.Item label="Customer">{order.customer_id}</Descriptions.Item>
        <Descriptions.Item label="Payment method">{order.payment_method}</Descriptions.Item>
        <Descriptions.Item label="Provider">{order.provider}</Descriptions.Item>
        <Descriptions.Item label="Amount">
          {order.amount} {order.currency}
        </Descriptions.Item>
        <Descriptions.Item label="Status">
          <Tag>{order.status}</Tag>
        </Descriptions.Item>
        <Descriptions.Item label="Credited">{order.credited ? "yes" : "no"}</Descriptions.Item>
        <Descriptions.Item label="Created">{dayjs(order.created_at).format("YYYY-MM-DD HH:mm:ss")}</Descriptions.Item>
        <Descriptions.Item label="Updated">{dayjs(order.updated_at).format("YYYY-MM-DD HH:mm:ss")}</Descriptions.Item>
      </Descriptions>
    </div>
  );
}
