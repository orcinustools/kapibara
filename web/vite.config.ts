import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build output is embedded into the Go binary (pkg/webui/dist).
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../pkg/webui/dist",
    emptyOutDir: true,
  },
  server: {
    // Dev proxy so the SPA can call the Go API during `npm run dev`.
    proxy: {
      "/api": "http://localhost:9000",
      "/healthz": "http://localhost:9000",
    },
  },
});
