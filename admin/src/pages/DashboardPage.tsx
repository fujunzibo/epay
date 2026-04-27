import { useEffect, useState } from "react";
import { Card, Col, Row, Spin, Statistic, Typography } from "antd";
import { api } from "../api/client";

type Summary = {
  since: string;
  total_orders: number;
  paid_or_credited: number;
  credited_orders: number;
  ledger_topups: number;
  uncredited_paid: number;
  orders_no_event: number;
};

type Metrics = {
  window_hours: number;
  paid_or_credited_orders: number;
  credited_orders: number;
  uncredited_paid_orders: number;
};

export default function DashboardPage() {
  const [loading, setLoading] = useState(true);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setErr(null);
      try {
        const [s, m] = await Promise.all([
          api.get<Summary>("/ops/reconcile/summary", { params: { since_hours: 24 } }),
          api.get<Metrics>("/metrics/payment"),
        ]);
        if (!cancelled) {
          setSummary(s.data);
          setMetrics(m.data);
        }
      } catch (e: unknown) {
        if (!cancelled) {
          setErr(e instanceof Error ? e.message : "Failed to load");
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return (
      <div style={{ textAlign: "center", padding: 48 }}>
        <Spin size="large" />
      </div>
    );
  }

  if (err) {
    return <Typography.Text type="danger">{err}</Typography.Text>;
  }

  return (
    <div>
      <Typography.Title level={4} style={{ marginTop: 0 }}>
        Overview (last 24h)
      </Typography.Title>
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic title="Paid / credited orders" value={metrics?.paid_or_credited_orders ?? 0} />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic title="Credited orders" value={metrics?.credited_orders ?? 0} />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic title="Uncredited (paid)" value={metrics?.uncredited_paid_orders ?? 0} />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic title="Total orders" value={summary?.total_orders ?? 0} />
          </Card>
        </Col>
      </Row>
      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card title="Reconcile summary">
            <pre style={{ margin: 0, fontSize: 13 }}>{JSON.stringify(summary, null, 2)}</pre>
          </Card>
        </Col>
      </Row>
    </div>
  );
}
