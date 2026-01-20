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
        "src/lib/prometheus-queries.ts", // PromQL query builder - tested via integration
        "src/lib/provider-utils.ts", // provider display utilities - simple mappings
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
        "src/lib/data/mock-service.ts", // Mock service (demo mode only)
        "src/lib/data/types.ts", // Type definitions and re-exports
        "src/hooks/use-agent-cost.ts", // Prometheus queries (tested via E2E)
        "src/lib/image-processor.ts", // Canvas API image processing (tested via E2E)
        "src/components/console/image-crop-dialog.tsx", // Image crop UI (tested via E2E)
        "src/components/console/video-player.tsx", // Video player UI (tested via E2E)
        "src/components/topology/graph-builder.ts", // ReactFlow graph builder (tested via E2E)
        "src/components/cost/cost-badge.tsx", // Cost badge display (tested via E2E)
        "src/components/cost/cost-breakdown-table.tsx", // Cost breakdown table (tested via E2E)
        "src/components/cost/cost-by-model-chart.tsx", // Cost chart (tested via E2E)
        "src/components/cost/cost-by-provider-chart.tsx", // Cost chart (tested via E2E)
        "src/components/cost/cost-summary.tsx", // Cost summary display (tested via E2E)
        "src/components/cost/cost-unavailable.tsx", // Cost unavailable display (tested via E2E)
        "src/components/cost/cost-usage-chart.tsx", // Cost usage chart (tested via E2E)
        "src/components/providers/**", // Provider display components (tested via E2E)
        "src/components/tools/**", // Tool registry display components (tested via E2E)
        "src/components/agents/agent-metrics-panel.tsx", // Metrics panel (tested via E2E)
        "src/components/agents/agent-table.tsx", // Agent table (tested via E2E)
        "src/components/agents/deploy-wizard.tsx", // Deploy wizard (tested via E2E)
        "src/components/agents/events-panel.tsx", // Events panel (tested via E2E)
        "src/components/topology/node-summary-card.tsx", // Node summary card (tested via E2E)
        "src/lib/k8s/token-fetcher.ts", // K8s in-cluster auth (requires K8s env, tested via integration)
        "src/lib/k8s/workspace-k8s-client-factory.ts", // K8s client factory (requires K8s env, tested via integration)
        "src/lib/data/workspace-api-service.ts", // API service (tested via E2E)
        "src/hooks/use-agents.ts", // Agent hooks (tested via E2E)
        "src/hooks/use-namespaces.ts", // Namespace hooks (tested via E2E)
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
