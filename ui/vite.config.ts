import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  build: { 
    outDir: "dist",
    assetsDir: "assets",
    rollupOptions: {
      output: {
        manualChunks: undefined
      }
    }
  },
  // Important: Set base to relative paths
  base: "./",
  server: { port: 5173 },
  proxy: {
    '/api': { target: 'http://localhost:9090', changeOrigin: true }
  }
});