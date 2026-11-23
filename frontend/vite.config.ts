import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In dev, /graphql is proxied to the BFF so the app can use a relative URL.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      "/graphql": process.env.BFF_URL ?? "http://localhost:8080",
    },
  },
});
