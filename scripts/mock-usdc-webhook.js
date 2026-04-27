const crypto = require("node:crypto");

const endpoint = process.env.USDC_WEBHOOK_URL || "http://localhost:8080/payments/webhook/usdc";
const token = process.env.USDC_WEBHOOK_BEARER || "your_usdc_webhook_token";
const hmacSecret = process.env.USDC_WEBHOOK_HMAC_SECRET || "";
const orderNo = process.env.ORDER_NO || "ord_replace_me";

const payload = {
  order_no: orderNo,
  chain: process.env.USDC_CHAIN || "ethereum",
  tx_hash: process.env.USDC_TX_HASH || `0x${Date.now().toString(16)}`,
  to_address: process.env.USDC_TO_ADDRESS || "0xreceiver",
  token: "USDC",
  amount: Number(process.env.USDC_AMOUNT || "20"),
  currency: "USD",
  confirmations: Number(process.env.USDC_CONFIRMATIONS || "3"),
  required_confirmations: Number(process.env.USDC_REQUIRED_CONFIRMATIONS || "1")
};

async function main() {
  const body = JSON.stringify(payload);
  const signature = hmacSecret
    ? crypto.createHmac("sha256", hmacSecret).update(body).digest("hex")
    : "";

  const res = await fetch(endpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${token}`,
      ...(signature ? { "X-USDC-Signature": signature } : {})
    },
    body
  });

  const text = await res.text();
  console.log(`status=${res.status}`);
  console.log(text);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
