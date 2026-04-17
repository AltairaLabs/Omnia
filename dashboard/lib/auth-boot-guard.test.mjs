/**
 * Tests for auth-boot-guard — refuse-to-start logic when the dashboard
 * is configured to serve anonymously in a production-looking environment
 * without an explicit opt-in.
 */

import { describe, it, expect } from "vitest";
import { createRequire } from "node:module";

// The guard itself is CommonJS (server.js is plain CJS and requires() it
// synchronously at boot), so pull it in via createRequire rather than
// trying ESM named-import interop.
const require = createRequire(import.meta.url);
const { checkAnonymousAuthGuard } = require("./auth-boot-guard.js");

describe("checkAnonymousAuthGuard", () => {
  it("allows boot when OMNIA_AUTH_MODE is not set and NODE_ENV is empty (dev/test default)", () => {
    expect(checkAnonymousAuthGuard({}).ok).toBe(true);
  });

  it.each(["oauth", "builtin", "proxy", "OAuth", "  BUILTIN  "])(
    "allows boot regardless of NODE_ENV when mode=%j",
    (mode) => {
      const result = checkAnonymousAuthGuard({
        OMNIA_AUTH_MODE: mode,
        NODE_ENV: "production",
      });
      expect(result.ok).toBe(true);
    },
  );

  it("allows boot in dev when mode=anonymous", () => {
    const result = checkAnonymousAuthGuard({
      OMNIA_AUTH_MODE: "anonymous",
      NODE_ENV: "development",
    });
    expect(result.ok).toBe(true);
  });

  it("allows boot in test when mode=anonymous", () => {
    const result = checkAnonymousAuthGuard({
      OMNIA_AUTH_MODE: "anonymous",
      NODE_ENV: "test",
    });
    expect(result.ok).toBe(true);
  });

  it("allows boot when NODE_ENV is unset even if mode=anonymous", () => {
    const result = checkAnonymousAuthGuard({ OMNIA_AUTH_MODE: "anonymous" });
    expect(result.ok).toBe(true);
  });

  it("REFUSES to boot when mode=anonymous + NODE_ENV=production + no opt-in", () => {
    const result = checkAnonymousAuthGuard({
      OMNIA_AUTH_MODE: "anonymous",
      NODE_ENV: "production",
    });
    expect(result.ok).toBe(false);
    expect(result.message).toMatch(/REFUSING TO START/);
    expect(result.message).toMatch(/OMNIA_ALLOW_ANONYMOUS=true/);
  });

  it("allows boot when mode=anonymous + NODE_ENV=production + OMNIA_ALLOW_ANONYMOUS=true", () => {
    const result = checkAnonymousAuthGuard({
      OMNIA_AUTH_MODE: "anonymous",
      NODE_ENV: "production",
      OMNIA_ALLOW_ANONYMOUS: "true",
    });
    expect(result.ok).toBe(true);
  });

  it.each(["TRUE", "True", "  true  "])(
    "accepts OMNIA_ALLOW_ANONYMOUS=%j (case / whitespace insensitive)",
    (val) => {
      const result = checkAnonymousAuthGuard({
        OMNIA_AUTH_MODE: "anonymous",
        NODE_ENV: "production",
        OMNIA_ALLOW_ANONYMOUS: val,
      });
      expect(result.ok).toBe(true);
    },
  );

  it.each(["false", "1", "yes", ""])(
    "rejects OMNIA_ALLOW_ANONYMOUS=%j as not a positive opt-in",
    (val) => {
      const result = checkAnonymousAuthGuard({
        OMNIA_AUTH_MODE: "anonymous",
        NODE_ENV: "production",
        OMNIA_ALLOW_ANONYMOUS: val,
      });
      expect(result.ok).toBe(false);
    },
  );

  it("treats unknown NODE_ENV values as non-production (permissive)", () => {
    const result = checkAnonymousAuthGuard({
      OMNIA_AUTH_MODE: "anonymous",
      NODE_ENV: "staging",
    });
    // Intentionally permissive: NODE_ENV=production is the specific prod
    // signal we look for. Anything else is allowed through so we don't
    // block weird deployment patterns.
    expect(result.ok).toBe(true);
  });

  it("treats case variations of NODE_ENV=production as production", () => {
    const result = checkAnonymousAuthGuard({
      OMNIA_AUTH_MODE: "anonymous",
      NODE_ENV: "PRODUCTION",
    });
    expect(result.ok).toBe(false);
  });
});
