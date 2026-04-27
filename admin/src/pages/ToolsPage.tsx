import { useState } from "react";
import { Button, Card, Divider, Form, Input, InputNumber, Select, Space, Typography, message } from "antd";
import { api, hmacSha256Hex } from "../api/client";

export default function ToolsPage() {
  const [creating, setCreating] = useState(false);

  const onCreateOrder = async (v: {
    customer_id: string;
    payment_method: string;
    provider: string;
    currency: string;
    amount: number;
  }) => {
    setCreating(true);
    try {
      const { data } = await api.post<{ order_no: string }>("/payments/orders", {
        customer_id: v.customer_id,
        payment_method: v.payment_method,
        provider: v.provider,
        currency: v.currency,
        amount: v.amount,
      });
      message.success(`Created ${data.order_no}`);
    } catch {
      message.error("Create failed");
    } finally {
      setCreating(false);
    }
  };

  return (
    <div>
      <Typography.Title level={4} style={{ marginTop: 0 }}>
        Integration tools
      </Typography.Title>
      <Typography.Paragraph type="secondary">
        Use these forms for local testing. Production Stripe webhooks should come from Stripe with a valid
        signature.
      </Typography.Paragraph>

      <Card title="Create order" style={{ marginBottom: 16 }}>
        <Form
          layout="vertical"
          initialValues={{
            customer_id: "cust_demo",
            payment_method: "fiat",
            provider: "stripe",
            currency: "USD",
            amount: 20,
          }}
          onFinish={onCreateOrder}
        >
          <Form.Item name="customer_id" label="Customer ID" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="payment_method" label="Payment method" rules={[{ required: true }]}>
            <Select options={[{ value: "fiat", label: "fiat" }, { value: "crypto", label: "crypto" }]} />
          </Form.Item>
          <Form.Item name="provider" label="Provider" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="currency" label="Currency" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="amount" label="Amount" rules={[{ required: true }]}>
            <InputNumber min={0.01} step={0.01} style={{ width: "100%" }} />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={creating}>
            Create
          </Button>
        </Form>
      </Card>

      <Card title="Simulate Stripe payment_intent.succeeded" style={{ marginBottom: 16 }}>
        <StripeSimForm />
      </Card>

      <Card title="Simulate USDC webhook">
        <USDCSimForm />
      </Card>

      <Divider />
      <Card title="Batch retry uncredited (server)">
        <Space>
          <Button
            onClick={async () => {
              try {
                const { data } = await api.post<{ retried: number; failed: number }>(
                  "/ops/retry-failed",
                  {},
                  { params: { limit: 50 } }
                );
                message.info(`retried=${data.retried} failed=${data.failed}`);
              } catch {
                message.error("Retry batch failed");
              }
            }}
          >
            POST /ops/retry-failed?limit=50
          </Button>
        </Space>
      </Card>
    </div>
  );
}

function StripeSimForm() {
  const [loading, setLoading] = useState(false);
  return (
    <Form
      layout="vertical"
      initialValues={{
        order_no: "",
        pi_id: "pi_simulated",
        amount_major: 20,
        currency: "usd",
      }}
      onFinish={async (v) => {
        setLoading(true);
        try {
          const cents = Math.round(Number(v.amount_major) * 100);
          const body = {
            id: "evt_sim_" + Date.now(),
            type: "payment_intent.succeeded",
            data: {
              object: {
                id: v.pi_id,
                amount_received: cents,
                currency: v.currency,
                metadata: { order_no: v.order_no },
              },
            },
          };
          await api.post("/payments/webhook/stripe", body);
          message.success("Stripe webhook OK");
        } catch (e: unknown) {
          const msg =
            typeof e === "object" && e !== null && "response" in e
              ? JSON.stringify((e as { response?: { data?: unknown } }).response?.data)
              : "Request failed";
          message.error(String(msg));
        } finally {
          setLoading(false);
        }
      }}
    >
      <Form.Item name="order_no" label="order_no (metadata)" rules={[{ required: true }]}>
        <Input placeholder="ord_xxx" />
      </Form.Item>
      <Form.Item name="pi_id" label="payment_intent id">
        <Input />
      </Form.Item>
      <Form.Item name="amount_major" label="Amount (major units)" rules={[{ required: true }]}>
        <InputNumber min={0.01} step={0.01} style={{ width: "100%" }} />
      </Form.Item>
      <Form.Item name="currency" label="Currency (Stripe lower)">
        <Input />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        Send webhook
      </Button>
    </Form>
  );
}

function USDCSimForm() {
  const [loading, setLoading] = useState(false);
  return (
    <Form
      layout="vertical"
      initialValues={{
        order_no: "",
        chain: "ethereum",
        tx_hash: "",
        to_address: "0xreceiver",
        amount: 20,
        currency: "USD",
        confirmations: 3,
        required_confirmations: 1,
        bearer: "",
        hmac_secret: "",
      }}
      onFinish={async (v) => {
        setLoading(true);
        try {
          const tx = v.tx_hash || `0x${Date.now().toString(16)}`;
          const payload = {
            order_no: v.order_no,
            chain: v.chain,
            tx_hash: tx,
            to_address: v.to_address,
            token: "USDC",
            amount: Number(v.amount),
            currency: v.currency,
            confirmations: Number(v.confirmations),
            required_confirmations: Number(v.required_confirmations),
          };
          const raw = JSON.stringify(payload);
          const headers: Record<string, string> = { "Content-Type": "application/json" };
          if (v.bearer) headers.Authorization = `Bearer ${v.bearer}`;
          if (v.hmac_secret) {
            headers["X-USDC-Signature"] = await hmacSha256Hex(v.hmac_secret, raw);
          }
          await api.post("/payments/webhook/usdc", payload, { headers });
          message.success("USDC webhook OK");
        } catch (e: unknown) {
          const msg =
            typeof e === "object" && e !== null && "response" in e
              ? JSON.stringify((e as { response?: { data?: unknown } }).response?.data)
              : "Request failed";
          message.error(String(msg));
        } finally {
          setLoading(false);
        }
      }}
    >
      <Form.Item name="order_no" label="order_no" rules={[{ required: true }]}>
        <Input />
      </Form.Item>
      <Form.Item name="chain" label="chain">
        <Input />
      </Form.Item>
      <Form.Item name="tx_hash" label="tx_hash (empty = auto)">
        <Input />
      </Form.Item>
      <Form.Item name="to_address" label="to_address">
        <Input />
      </Form.Item>
      <Form.Item name="amount" label="amount">
        <InputNumber min={0.000001} step={0.01} style={{ width: "100%" }} />
      </Form.Item>
      <Form.Item name="currency" label="currency">
        <Input />
      </Form.Item>
      <Form.Item name="confirmations" label="confirmations">
        <InputNumber min={0} style={{ width: "100%" }} />
      </Form.Item>
      <Form.Item name="required_confirmations" label="required_confirmations">
        <InputNumber min={0} style={{ width: "100%" }} />
      </Form.Item>
      <Form.Item name="bearer" label="Authorization bearer (if server requires)">
        <Input.Password placeholder="optional" />
      </Form.Item>
      <Form.Item name="hmac_secret" label="HMAC secret (if server requires X-USDC-Signature)">
        <Input.Password placeholder="optional" />
      </Form.Item>
      <Button type="primary" htmlType="submit" loading={loading}>
        Send webhook
      </Button>
    </Form>
  );
}
