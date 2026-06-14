/**
 * Tests for the server-side ServiceAccount token reader used to authenticate
 * the dashboard's outbound proxy calls to session-api / memory-api.
 *
 * Covers: file present (trimmed + cached within TTL), re-read after TTL,
 * file missing (returns "" without throwing), env-var path override, and the
 * serviceApiHeaders helper attaching / omitting the Authorization header.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock node:fs so the reader is exercised without a real projected token file.
vi.mock("node:fs", () => {
  const readFileSync = vi.fn();
  return { readFileSync, default: { readFileSync } };
});
import { readFileSync } from "node:fs";

import {
  getSessionApiToken,
  serviceApiHeaders,
  resetSessionApiTokenCache,
} from "./session-api-token";

const DEFAULT_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token";

describe("getSessionApiToken", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.unstubAllEnvs();
    resetSessionApiTokenCache();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    resetSessionApiTokenCache();
  });

  it("returns the trimmed token contents from the default path", () => {
    vi.mocked(readFileSync).mockReturnValue("  sa-jwt-token\n");

    const token = getSessionApiToken();

    expect(token).toBe("sa-jwt-token");
    expect(readFileSync).toHaveBeenCalledWith(DEFAULT_PATH, "utf-8");
  });

  it("reads from SESSION_API_TOKEN_PATH when set", () => {
    vi.stubEnv("SESSION_API_TOKEN_PATH", "/custom/token");
    vi.mocked(readFileSync).mockReturnValue("custom-token");

    expect(getSessionApiToken()).toBe("custom-token");
    expect(readFileSync).toHaveBeenCalledWith("/custom/token", "utf-8");
  });

  it("caches the token within the TTL (does not re-read on the second call)", () => {
    vi.mocked(readFileSync).mockReturnValue("tok-v1");

    const t0 = 1_000_000;
    // First call reads from disk.
    expect(getSessionApiToken(t0)).toBe("tok-v1");
    // Second call 30s later (within the 60s TTL) returns the cached value.
    vi.mocked(readFileSync).mockReturnValue("tok-v2");
    expect(getSessionApiToken(t0 + 30_000)).toBe("tok-v1");

    expect(readFileSync).toHaveBeenCalledTimes(1);
  });

  it("re-reads from disk after the TTL expires (kubelet rotation)", () => {
    vi.mocked(readFileSync).mockReturnValue("tok-v1");

    const t0 = 2_000_000;
    expect(getSessionApiToken(t0)).toBe("tok-v1");

    vi.mocked(readFileSync).mockReturnValue("tok-v2");
    // 61s later — past the 60s TTL — re-reads and returns the rotated token.
    expect(getSessionApiToken(t0 + 61_000)).toBe("tok-v2");

    expect(readFileSync).toHaveBeenCalledTimes(2);
  });

  it("returns '' without throwing when the token file is missing", () => {
    vi.mocked(readFileSync).mockImplementation(() => {
      throw new Error("ENOENT: no such file or directory");
    });

    expect(() => getSessionApiToken()).not.toThrow();
    expect(getSessionApiToken()).toBe("");
  });
});

describe("serviceApiHeaders", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.unstubAllEnvs();
    resetSessionApiTokenCache();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    resetSessionApiTokenCache();
  });

  it("attaches Authorization: Bearer when a token is present", () => {
    vi.mocked(readFileSync).mockReturnValue("sa-jwt");

    const headers = serviceApiHeaders({ Accept: "application/json" });

    expect(headers).toEqual({
      Accept: "application/json",
      Authorization: "Bearer sa-jwt",
    });
  });

  it("omits the Authorization header when no token is present (local dev no-op)", () => {
    vi.mocked(readFileSync).mockImplementation(() => {
      throw new Error("ENOENT");
    });

    const headers = serviceApiHeaders({ Accept: "application/json" });

    expect(headers).toEqual({ Accept: "application/json" });
    expect(headers.Authorization).toBeUndefined();
  });

  it("returns an empty object when no extra headers and no token", () => {
    vi.mocked(readFileSync).mockImplementation(() => {
      throw new Error("ENOENT");
    });

    expect(serviceApiHeaders()).toEqual({});
  });
});
