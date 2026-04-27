import { useState } from "react";
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Tabs,
  Typography,
  QRCode,
  Divider,
  message,
} from "antd";
import { CreditCardOutlined, WalletOutlined } from "@ant-design/icons";
import { api } from "../api/client";

type StripeCheckoutResp = {
  order_no: string;
  checkout_url: string;
  status: string;
};

type CryptoCheckoutResp = {
  order_no: string;
  status: string;
  amount: number;
  currency: string;
  token: string;
  chain: string;
  to_address: string;
  instructions?: string;
};

export default function CheckoutPage() {
  const [fiatLoading, setFiatLoading] = useState(false);
  const [cryptoLoading, setCryptoLoading] = useState(false);
  const [cryptoResult, setCryptoResult] = useState<CryptoCheckoutResp | null>(null);

  const onFiat = async (v: { customer_id: string; amount: number; currency: string }) => {
    setFiatLoading(true);
    try {
      const { data } = await api.post<StripeCheckoutResp>("/payments/checkout/stripe", {
        customer_id: v.customer_id,
        amount: v.amount,
        currency: v.currency || "USD",
      });
      message.success("正在跳转 Stripe 安全收银台…");
      window.location.href = data.checkout_url;
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      message.error(err.response?.data?.error || "创建支付失败");
    } finally {
      setFiatLoading(false);
    }
  };

  const onCrypto = async (v: { customer_id: string; amount: number; currency: string }) => {
    setCryptoLoading(true);
    setCryptoResult(null);
    try {
      const { data } = await api.post<CryptoCheckoutResp>("/payments/checkout/crypto", {
        customer_id: v.customer_id,
        amount: v.amount,
        currency: v.currency || "USD",
      });
      setCryptoResult(data);
      message.success("订单已创建，请按下方信息转账");
    } catch (e: unknown) {
      const err = e as { response?: { data?: { error?: string } } };
      message.error(err.response?.data?.error || "创建订单失败");
    } finally {
      setCryptoLoading(false);
    }
  };

  const qrValue = cryptoResult
    ? `USDC | ${cryptoResult.chain}\nTo: ${cryptoResult.to_address}\nAmount: ${cryptoResult.amount} ${cryptoResult.currency}\nOrder: ${cryptoResult.order_no}`
    : "";

  return (
    <div
      style={{
        minHeight: "100vh",
        background: "linear-gradient(160deg, #0f172a 0%, #1e3a5f 45%, #0f172a 100%)",
        padding: "32px 16px",
      }}
    >
      <div style={{ maxWidth: 520, margin: "0 auto" }}>
        <Typography.Title level={2} style={{ color: "#fff", textAlign: "center", marginBottom: 8 }}>
          收银台
        </Typography.Title>
        <Typography.Paragraph style={{ color: "rgba(255,255,255,0.75)", textAlign: "center", marginBottom: 28 }}>
          使用银行卡（Stripe）或链上 USDC 完成充值。请先填写您的客户编号与金额。
        </Typography.Paragraph>

        <Card style={{ borderRadius: 12, boxShadow: "0 12px 40px rgba(0,0,0,0.25)" }}>
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message="客户编号说明"
            description="请使用您在平台侧的用户标识（与后台绑定 OneAPI 时使用的 customer_id 一致），以便到账后正确入账。"
          />

          <Tabs
            defaultActiveKey="fiat"
            items={[
              {
                key: "fiat",
                label: (
                  <span>
                    <CreditCardOutlined /> 法定货币（银行卡）
                  </span>
                ),
                children: (
                  <Form layout="vertical" onFinish={onFiat} initialValues={{ currency: "USD", amount: 20 }}>
                    <Form.Item name="customer_id" label="客户编号" rules={[{ required: true, message: "请输入客户编号" }]}>
                      <Input placeholder="例如 cust_001" autoComplete="username" />
                    </Form.Item>
                    <Form.Item name="amount" label="金额（主币种单位，如 USD）" rules={[{ required: true }]}>
                      <InputNumber min={0.5} step={1} style={{ width: "100%" }} />
                    </Form.Item>
                    <Form.Item name="currency" label="币种">
                      <Select
                        options={[
                          { value: "USD", label: "USD" },
                          { value: "EUR", label: "EUR" },
                          { value: "GBP", label: "GBP" },
                        ]}
                      />
                    </Form.Item>
                    <Button type="primary" htmlType="submit" block size="large" loading={fiatLoading}>
                      前往 Stripe 支付
                    </Button>
                  </Form>
                ),
              },
              {
                key: "crypto",
                label: (
                  <span>
                    <WalletOutlined /> 加密货币（USDC）
                  </span>
                ),
                children: (
                  <>
                    <Form layout="vertical" onFinish={onCrypto} initialValues={{ currency: "USD", amount: 20 }}>
                      <Form.Item name="customer_id" label="客户编号" rules={[{ required: true, message: "请输入客户编号" }]}>
                        <Input placeholder="例如 cust_001" />
                      </Form.Item>
                      <Form.Item name="amount" label="应付金额（与订单一致）" rules={[{ required: true }]}>
                        <InputNumber min={0.01} step={1} style={{ width: "100%" }} />
                      </Form.Item>
                      <Form.Item name="currency" label="标价币种（记账用）">
                        <Select options={[{ value: "USD", label: "USD" }]} />
                      </Form.Item>
                      <Button type="primary" htmlType="submit" block size="large" loading={cryptoLoading}>
                        生成收款信息
                      </Button>
                    </Form>

                    {cryptoResult && (
                      <>
                        <Divider>收款信息</Divider>
                        <Space direction="vertical" size="middle" style={{ width: "100%" }}>
                          <div style={{ textAlign: "center" }}>
                            <QRCode value={qrValue} size={200} />
                          </div>
                          <DescriptionsBlock
                            orderNo={cryptoResult.order_no}
                            chain={cryptoResult.chain}
                            token={cryptoResult.token}
                            amount={cryptoResult.amount}
                            currency={cryptoResult.currency}
                            address={cryptoResult.to_address}
                          />
                          {cryptoResult.instructions && (
                            <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                              {cryptoResult.instructions}
                            </Typography.Paragraph>
                          )}
                        </Space>
                      </>
                    )}
                  </>
                ),
              },
            ]}
          />
        </Card>

        <Typography.Paragraph style={{ color: "rgba(255,255,255,0.55)", textAlign: "center", marginTop: 24, fontSize: 12 }}>
          支付遇到问题请联系客服，并提供订单号。
        </Typography.Paragraph>
      </div>
    </div>
  );
}

function DescriptionsBlock(props: {
  orderNo: string;
  chain: string;
  token: string;
  amount: number;
  currency: string;
  address: string;
}) {
  const { orderNo, chain, token, amount, currency, address } = props;
  const row = (label: string, value: string, mono?: boolean) => (
    <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
      <Typography.Text type="secondary">{label}</Typography.Text>
      <Typography.Text copyable style={mono ? { fontFamily: "monospace", wordBreak: "break-all" } : undefined}>
        {value}
      </Typography.Text>
    </div>
  );
  return (
    <Space direction="vertical" size="small" style={{ width: "100%" }}>
      {row("订单号", orderNo, true)}
      {row("网络", chain)}
      {row("代币", token)}
      {row("金额", `${amount} ${currency}`)}
      {row("收款地址", address, true)}
    </Space>
  );
}
