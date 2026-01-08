import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import sonarjs from "eslint-plugin-sonarjs";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // SonarJS recommended rules for SonarCloud compatibility
  sonarjs.configs.recommended,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    // Coverage output
    "coverage/**",
    // Generated types
    "src/types/generated/**",
    // Generated API schema (openapi-typescript output)
    "src/lib/api/schema.d.ts",
    // shadcn UI components - vendor code
    "src/components/ui/**",
  ]),
  // Custom stricter rules
  {
    rules: {
      // TypeScript strict rules
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      "@typescript-eslint/no-explicit-any": "warn",

      // React hooks rules
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "warn",

      // Code quality rules
      "no-console": ["error", { allow: ["warn", "error"] }],
      "no-debugger": "error",
      "no-duplicate-imports": "error",
      "no-unused-expressions": "error",
      "prefer-const": "error",
      eqeqeq: ["error", "always", { null: "ignore" }],

      // Import organization (if plugin available)
      "import/no-duplicates": "off", // Handled by no-duplicate-imports

      // SonarJS rules - match SonarCloud defaults
      "sonarjs/cognitive-complexity": ["error", 15],
      "sonarjs/no-duplicate-string": ["error", { threshold: 3 }],
      "sonarjs/no-identical-functions": "error",
      "sonarjs/no-collapsible-if": "error",
      "sonarjs/no-redundant-jump": "error",
      "sonarjs/no-small-switch": "error",
      "sonarjs/prefer-single-boolean-return": "error",
      "sonarjs/no-nested-template-literals": "error",
      "sonarjs/no-nested-conditional": "error",
    },
  },
  // Test files - relaxed rules
  {
    files: ["**/*.test.ts", "**/*.test.tsx", "**/test/**"],
    rules: {
      "@typescript-eslint/no-explicit-any": "off",
      "no-console": "off",
      "sonarjs/no-duplicate-string": "off",
      "sonarjs/cognitive-complexity": "off",
      "sonarjs/no-nested-functions": "off",
      "sonarjs/no-redundant-boolean": "off",
      "sonarjs/pseudo-random": "off",
      "sonarjs/no-identical-functions": "off",
      "sonarjs/use-type-alias": "off",
    },
  },
  // Mock data files - test fixtures
  {
    files: ["**/mock-data.ts", "**/mock-service.ts"],
    rules: {
      "sonarjs/no-duplicate-string": "off",
      "sonarjs/pseudo-random": "off",
      "sonarjs/no-clear-text-protocols": "off",
    },
  },
  // Scripts
  {
    files: ["scripts/**", "server.js"],
    rules: {
      "no-console": "off",
    },
  },
  // API route handlers - allow console for debugging
  {
    files: ["**/app/api/**/*.ts"],
    rules: {
      "no-console": "off", // Server-side logging is acceptable
      "sonarjs/todo-tag": "off", // TODOs are tracked in project management
    },
  },
  // Hooks - relaxed pseudo-random for demo/mock data generation
  {
    files: ["**/hooks/use-agent-*.ts"],
    rules: {
      "sonarjs/pseudo-random": "off", // Demo data generation
    },
  },
  // React components - relaxed rules for JSX patterns
  {
    files: ["**/components/**/*.tsx", "**/app/**/*.tsx"],
    rules: {
      // Nested ternaries are common in JSX for conditional rendering
      "sonarjs/no-nested-conditional": "off",
      // Duplicate strings are common in UI (class names, labels)
      "sonarjs/no-duplicate-string": "off",
    },
  },
  // shadcn UI components - vendor code with specific patterns
  {
    files: ["**/components/ui/**/*.tsx"],
    rules: {
      // Table component is a wrapper that expects users to add TableHeader
      "sonarjs/table-header": "off",
    },
  },
  // Data services - relaxed cognitive complexity for query builders
  {
    files: ["**/lib/data/**/*.ts", "**/lib/auth/**/*.ts"],
    rules: {
      "sonarjs/cognitive-complexity": ["warn", 25], // Higher limit for complex query logic
      "sonarjs/no-identical-functions": "off", // Store implementations follow same interface
      "no-console": "off", // Server-side logging
      // Parameterized SQL queries are safe (using $1, $2 placeholders)
      "sonarjs/sql-queries": "off",
    },
  },
]);

export default eslintConfig;
