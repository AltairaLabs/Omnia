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
        // Standard exclusions
        "node_modules/",
        "src/test/", // Test setup files
        "e2e/", // E2E test files (covered by Playwright)
        "**/*.d.ts", // Type definitions
        "**/index.ts", // Barrel re-exports
        "**/index.tsx",

        // Generated code
        "src/types/generated/", // Generated CRD types
        "src/lib/api/schema.d.ts", // Generated API schema
        "src/lib/proto/**", // Generated protobuf

        // Vendor code
        "src/components/ui/**", // shadcn components

        // Visual components (tested via E2E, not unit tests)
        "src/components/agents/**",
        "src/components/arena/index.ts", // Arena barrel re-export
        "src/components/arena/project-editor.tsx", // Project editor layout
        "src/components/arena/file-tree.tsx", // File tree browser
        "src/components/arena/editor-tabs.tsx", // Editor tab bar
        "src/components/arena/yaml-editor.tsx", // Monaco YAML editor
        "src/components/arena/lsp-yaml-editor.tsx", // Monaco LSP YAML editor
        "src/components/arena/validation-results-dialog.tsx", // Validation results dialog
        "src/components/console/**",
        "src/components/cost/**",
        "src/components/credentials/**",
        "src/components/dashboard/**", // Dashboard widgets (visual components)
        "src/components/layout/**",
        "src/components/logs/**",
        "src/components/topology/**",

        // Next.js framework files (tested via E2E)
        "src/app/**/layout.tsx",
        "src/app/page.tsx", // Root page
        "src/app/agents/**/page.tsx", // Agent pages (require full context)
        "src/app/arena/configs/[name]/page.tsx", // Arena detail pages (complex UI)
        "src/app/arena/jobs/[name]/page.tsx", // Arena detail pages (complex UI)
        "src/app/arena/sources/[name]/page.tsx", // Arena detail pages (complex UI)
        "src/app/arena/projects/page.tsx", // Arena project editor page (visual)
        "src/app/providers/**/page.tsx", // Provider pages (require full context)
        "src/app/settings/**/page.tsx", // Settings pages (require full context)
        "src/app/toolregistries/**/page.tsx", // Tool registry pages (require full context)
        "src/app/workspaces/**/page.tsx", // Workspace pages (require full context)
        "src/middleware.ts",

        // Auth API routes - require NextAuth server infrastructure
        "src/app/api/auth/**",

        // Prometheus API routes - require Prometheus server
        "src/app/api/prometheus/**",

        // License API routes - require license server
        "src/app/api/license/**",

        // Other API routes that require K8s infrastructure
        "src/app/api/workspaces/[name]/agents/**",
        "src/app/api/workspaces/[name]/costs/**",
        "src/app/api/workspaces/[name]/promptpacks/**",
        "src/app/api/workspaces/[name]/providers/**",
        "src/app/api/workspaces/[name]/toolregistries/**",
        "src/app/api/workspaces/[name]/stats/**",
        "src/app/api/workspaces/[name]/route.ts",
        "src/app/api/workspaces/route.ts",
        "src/app/api/providers/**",
        "src/app/api/config/**",
        "src/app/api/health/**",
        "src/app/api/settings/**",

        // Requires external infrastructure (K8s, Prometheus, etc.)
        "src/lib/data/live-service.ts", // K8s API
        "src/lib/data/operator-service.ts", // Operator API
        "src/lib/data/prometheus-service.ts", // Prometheus
        "src/lib/data/workspace-api-service.ts", // Backend API
        "src/lib/k8s/token-fetcher.ts", // K8s ServiceAccount
        "src/lib/k8s/workspace-k8s-client-factory.ts", // K8s cluster
        "src/lib/image-processor.ts", // Browser Canvas API

        // Type definitions only (no executable code)
        "src/lib/data/types.ts",
        "src/lib/auth/types.ts",
        "src/types/**",

        // Mock/demo data (not production code)
        "src/lib/mock-data.ts",
        "src/lib/data/mock-service.ts",

        // React context wrapper (minimal logic)
        "src/lib/data/provider.tsx",

        // Auth - requires NextAuth server infrastructure
        "src/lib/auth/actions.ts", // Next.js server actions
        "src/lib/auth/api-guard.ts", // API middleware with NextAuth
        "src/lib/auth/config.ts", // NextAuth configuration
        "src/lib/auth/proxy.ts", // HTTP proxy utilities
        "src/lib/auth/session.ts", // NextAuth session handling
        "src/lib/auth/api-keys/**", // File-based API key stores
        "src/lib/auth/oauth/**", // OAuth client (requires providers)
        "src/lib/auth/providers/**", // OAuth provider configs
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
