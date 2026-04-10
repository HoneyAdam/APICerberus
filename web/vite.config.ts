import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { visualizer } from "rollup-plugin-visualizer";
import path from "node:path";

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    visualizer({
      filename: "dist/stats.html",
      open: false,
      gzipSize: true,
      brotliSize: true,
    }),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes("node_modules")) {
            // Split heavy dependencies
            if (id.includes("recharts")) return "recharts";
            if (id.includes("@codemirror")) return "codemirror";
            if (id.includes("@xyflow")) return "react-flow";
            if (id.includes("radix-ui") || id.includes("lucide-react")) return "ui-common";
            if (id.includes("@tanstack")) return "react-query";
            if (id.includes("react-router")) return "react-vendor";
          }
        },
      },
    },
  },
});

