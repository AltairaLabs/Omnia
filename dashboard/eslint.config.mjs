import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    // Coverage output
    "coverage/**",
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
    },
  },
  // Test file overrides - more relaxed rules
  {
    files: ["**/*.test.ts", "**/*.test.tsx", "**/test/**"],
    rules: {
      "@typescript-eslint/no-explicit-any": "off",
      "no-console": "off",
    },
  },
  // Scripts and server files - allow console
  {
    files: ["scripts/**", "server.js"],
    rules: {
      "no-console": "off",
    },
  },
]);

export default eslintConfig;
