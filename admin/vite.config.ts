import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// When built and served from Go at /admin/, assets load under /admin/assets/
const proxyTarget = process.env.VITE_PROXY_TARGET || "http://127.0.0.1:18080";

export default defineConfig({
  plugins: [react()],
  base: "/admin/",
  server: {
    port: 5173,
    proxy: {
      "/payments": { target: proxyTarget, changeOrigin: true },
      "/ops": { target: proxyTarget, changeOrigin: true },
      "/metrics": { target: proxyTarget, changeOrigin: true },
    },
  },
});
