import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    coverage: {
      provider: "v8",
      reporter: ["text", "json", "html", "lcov"],
      reportsDirectory: "./coverage",
      exclude: [
        "node_modules/",
        "src/test/",
        "src/types/generated/",
        "**/*.d.ts",
        "**/index.ts",
        "**/index.tsx",
        "src/components/ui/**", // shadcn components - vendor code
        "src/components/console/markdown.tsx", // visual markdown renderer - tested via E2E
        "src/lib/api/schema.d.ts", // generated API schema
        "src/lib/mock-data.ts", // static mock data for demos
        "src/app/**/layout.tsx", // Next.js layouts
        "src/app/**/page.tsx", // Next.js pages (tested via E2E)
        "src/app/api/**", // API routes (tested via integration)
        "src/lib/auth/actions.ts", // Server actions (tested via integration)
        "src/lib/auth/api-guard.ts", // API middleware (tested via integration)
        "src/lib/auth/config.ts", // Auth config (tested via integration)
        "src/lib/auth/proxy.ts", // Proxy utilities (tested via integration)
        "src/lib/auth/session.ts", // Session handling (tested via integration)
        "src/lib/auth/types.ts", // Type utilities
        "src/lib/auth/api-keys/**", // API key stores (tested via integration)
        "src/lib/auth/oauth/**", // OAuth client (tested via integration)
        "src/lib/auth/providers/**", // OAuth providers (config only)
        "src/lib/data/live-service.ts", // Kubernetes API client (tested via E2E)
        "src/lib/data/operator-service.ts", // Kubernetes operator client (tested via E2E)
        "src/lib/data/prometheus-service.ts", // Prometheus client (tested via E2E)
      ],
      thresholds: {
        statements: 80,
        branches: 80,
        functions: 80,
        lines: 80,
      },
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
});
