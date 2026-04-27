import axios from "axios";

/** In dev, Vite proxy forwards /payments, /ops, /metrics to backend — use "" */
function defaultBase(): string {
  if (import.meta.env.DEV) {
    return "";
  }
  return import.meta.env.VITE_API_BASE_URL || "";
}

export const api = axios.create({
  baseURL: defaultBase(),
  headers: { "Content-Type": "application/json" },
  timeout: 60_000,
});

export async function hmacSha256Hex(secret: string, message: string): Promise<string> {
  const enc = new TextEncoder();
  const key = await crypto.subtle.importKey(
    "raw",
    enc.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"]
  );
  const sig = await crypto.subtle.sign("HMAC", key, enc.encode(message));
  return Array.from(new Uint8Array(sig))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
