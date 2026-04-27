import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Alert, Button, Card, Spin, Typography } from "antd";
import { CheckCircleOutlined, LoadingOutlined } from "@ant-design/icons";
import { api } from "../api/client";

type Order = {
  order_no: string;
  status: string;
  credited: boolean;
  amount: number;
  currency: string;
};

export default function CheckoutSuccessPage() {
  const [params] = useSearchParams();
  const orderNo = params.get("order_no") || "";
  const nav = useNavigate();
  const [order, setOrder] = useState<Order | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!orderNo) {
      setErr("缺少订单号参数");
      return;
    }
    let cancelled = false;
    let n = 0;
    const tick = async () => {
      try {
        const { data } = await api.get<Order>(`/payments/orders/${encodeURIComponent(orderNo)}`);
        if (cancelled) return;
        setOrder(data);
        setErr(null);
        if (data.credited || data.status === "credited") {
          return;
        }
      } catch {
        if (!cancelled) setErr("无法查询订单状态");
      }
      if (cancelled) return;
      n++;
      if (n < 45) {
        setTimeout(tick, 2000);
      }
    };
    tick();
    return () => {
      cancelled = true;
    };
  }, [orderNo]);

  const done = order?.credited || order?.status === "credited";

  return (
    <div
      style={{
        minHeight: "100vh",
        background: "linear-gradient(160deg, #0f172a 0%, #14532d 40%, #0f172a 100%)",
        padding: "48px 16px",
      }}
    >
      <Card style={{ maxWidth: 480, margin: "0 auto", borderRadius: 12 }}>
        <Typography.Title level={3} style={{ textAlign: "center" }}>
          支付结果
        </Typography.Title>

        {!orderNo && <Alert type="error" message={err || "无效链接"} showIcon />}

        {orderNo && !order && !err && (
          <div style={{ textAlign: "center", padding: "32px 0" }}>
            <Spin indicator={<LoadingOutlined style={{ fontSize: 32 }} spin />} />
            <Typography.Paragraph style={{ marginTop: 16 }}>正在确认支付结果…</Typography.Paragraph>
            <Typography.Text type="secondary">订单号：{orderNo}</Typography.Text>
          </div>
        )}

        {err && orderNo && <Alert type="warning" message={err} showIcon style={{ marginBottom: 16 }} />}

        {order && (
          <>
            {done ? (
              <Alert
                type="success"
                showIcon
                icon={<CheckCircleOutlined />}
                message="支付已确认并入账"
                description={`订单 ${order.order_no}，金额 ${order.amount} ${order.currency}。`}
                style={{ marginBottom: 16 }}
              />
            ) : (
              <Alert
                type="info"
                showIcon
                message="Stripe 支付已提交"
                description="入账可能有几秒延迟，本页会自动刷新状态。您也可以稍后在订单中查看。"
                style={{ marginBottom: 16 }}
              />
            )}
            <Typography.Paragraph>
              <strong>状态：</strong>
              {order.status}
              {order.credited ? "（已入账）" : ""}
            </Typography.Paragraph>
          </>
        )}

        <Button type="primary" block onClick={() => nav("/checkout")} style={{ marginTop: 8 }}>
          返回收银台
        </Button>
      </Card>
    </div>
  );
}
