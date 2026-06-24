import { describe, it, expect, afterEach, vi } from "vitest";
import { cliTokenTtlSeconds, CLI_CODE_TTL_SECONDS } from "./config";

afterEach(() => vi.unstubAllEnvs());

describe("cliTokenTtlSeconds", () => {
  it("defaults to 3600", () => expect(cliTokenTtlSeconds()).toBe(3600));
  it("reads a positive override", () => {
    vi.stubEnv("OMNIA_AUTH_CLI_TOKEN_TTL_SECONDS", "900");
    expect(cliTokenTtlSeconds()).toBe(900);
  });
  it.each(["0", "-5", "abc", ""])("ignores invalid %s", (v) => {
    vi.stubEnv("OMNIA_AUTH_CLI_TOKEN_TTL_SECONDS", v);
    expect(cliTokenTtlSeconds()).toBe(3600);
  });
  it("pins the one-time code TTL", () => expect(CLI_CODE_TTL_SECONDS).toBe(60));
});
