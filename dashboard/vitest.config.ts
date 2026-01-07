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
        "src/lib/api/schema.d.ts", // generated API schema
        "src/lib/mock-data.ts", // static mock data for demos
        "src/app/**/layout.tsx", // Next.js layouts
        "src/app/**/page.tsx", // Next.js pages (tested via E2E)
        "src/app/api/**", // API routes (tested via integration)
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
