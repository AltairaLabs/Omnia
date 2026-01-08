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
  ]),
  // Custom stricter rules
  {
    rules: {
      // TypeScript strict rules - using warn to allow gradual adoption
      "@typescript-eslint/no-unused-vars": [
        "warn",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      "@typescript-eslint/no-explicit-any": "warn",

      // React hooks rules
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "warn",

      // Code quality rules - using warn to allow gradual adoption
      "no-console": ["warn", { allow: ["warn", "error"] }],
      "no-debugger": "error",
      "no-duplicate-imports": "warn",
      "no-unused-expressions": "warn",
      "prefer-const": "warn",
      eqeqeq: ["warn", "always", { null: "ignore" }],

      // Import organization (if plugin available)
      "import/no-duplicates": "off", // Handled by no-duplicate-imports

      // SonarJS rules - align with SonarCloud
      "sonarjs/cognitive-complexity": ["warn", 15],
      "sonarjs/no-duplicate-string": ["warn", { threshold: 3 }],
      "sonarjs/no-identical-functions": "warn",
      "sonarjs/no-collapsible-if": "warn",
      "sonarjs/no-redundant-jump": "warn",
      "sonarjs/no-small-switch": "warn",
      "sonarjs/prefer-single-boolean-return": "warn",
      "sonarjs/no-nested-template-literals": "warn",
    },
  },
  // Test file overrides - more relaxed rules
  {
    files: ["**/*.test.ts", "**/*.test.tsx", "**/test/**"],
    rules: {
      "@typescript-eslint/no-explicit-any": "off",
      "no-console": "off",
      "sonarjs/no-duplicate-string": "off",
      "sonarjs/cognitive-complexity": "off",
    },
  },
  // Scripts and server files - allow console
  {
    files: ["scripts/**", "server.js"],
    rules: {
      "no-console": "off",
    },
  },
  // Mock data files - relaxed rules for test fixtures
  {
    files: ["**/mock-data.ts", "**/mock-service.ts"],
    rules: {
      "sonarjs/no-duplicate-string": "off",
      "sonarjs/pseudo-random": "off", // Mock data uses Math.random for demo values
      "sonarjs/no-clear-text-protocols": "off", // Mock URLs don't need HTTPS
    },
  },
  // Test files - additional relaxed rules
  {
    files: ["**/*.test.ts", "**/*.test.tsx"],
    rules: {
      "sonarjs/no-nested-functions": "off", // Jest describe/it nesting is fine
      "sonarjs/no-redundant-boolean": "off", // Test assertions may use explicit booleans
      "sonarjs/pseudo-random": "off", // Test data can use Math.random
      "sonarjs/no-identical-functions": "off", // Test helpers may be similar
      "sonarjs/use-type-alias": "off", // Inline types are fine in tests
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
