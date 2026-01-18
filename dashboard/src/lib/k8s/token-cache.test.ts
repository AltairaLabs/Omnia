import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import {
  getCachedToken,
  setCachedToken,
  invalidateWorkspaceTokens,
  invalidateToken,
  clearTokenCache,
  getTokenCacheStats,
  pruneExpiredTokens,
} from "./token-cache";

describe("token-cache", () => {
  const mockToken = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.test-token";

  beforeEach(() => {
    clearTokenCache();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("getCachedToken", () => {
    it("should return null for uncached entries", () => {
      const result = getCachedToken("my-workspace", "editor");
      expect(result).toBeNull();
    });

    it("should return cached token after set", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000; // 1 hour
      setCachedToken("my-workspace", "editor", mockToken, expiresAt);

      const result = getCachedToken("my-workspace", "editor");
      expect(result).toBe(mockToken);
    });

    it("should return null for expired entries", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000; // 1 hour
      setCachedToken("my-workspace", "editor", mockToken, expiresAt);

      // Advance time past expiry
      vi.advanceTimersByTime(60 * 60 * 1000 + 1000);

      const result = getCachedToken("my-workspace", "editor");
      expect(result).toBeNull();
    });

    it("should return null for entries expiring within safety margin", () => {
      const expiresAt = Date.now() + 4 * 60 * 1000; // 4 minutes (within 5 min margin)
      setCachedToken("my-workspace", "editor", mockToken, expiresAt);

      const result = getCachedToken("my-workspace", "editor");
      expect(result).toBeNull();
    });

    it("should return valid entry just outside safety margin", () => {
      const expiresAt = Date.now() + 6 * 60 * 1000; // 6 minutes (outside 5 min margin)
      setCachedToken("my-workspace", "editor", mockToken, expiresAt);

      const result = getCachedToken("my-workspace", "editor");
      expect(result).toBe(mockToken);
    });

    it("should use default TTL when expiresAt not provided", () => {
      setCachedToken("my-workspace", "editor", mockToken);

      // Should be valid immediately
      expect(getCachedToken("my-workspace", "editor")).toBe(mockToken);

      // Advance time to 40 minutes (should still be valid with 50 min TTL - 5 min margin = 45 min effective)
      vi.advanceTimersByTime(40 * 60 * 1000);
      expect(getCachedToken("my-workspace", "editor")).toBe(mockToken);

      // Advance another 6 minutes (now at 46 min, past 45 min effective TTL)
      vi.advanceTimersByTime(6 * 60 * 1000);
      expect(getCachedToken("my-workspace", "editor")).toBeNull();
    });
  });

  describe("setCachedToken", () => {
    it("should store tokens by workspace and role", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000;

      setCachedToken("workspace-a", "owner", "token-a-owner", expiresAt);
      setCachedToken("workspace-a", "editor", "token-a-editor", expiresAt);
      setCachedToken("workspace-b", "viewer", "token-b-viewer", expiresAt);

      expect(getCachedToken("workspace-a", "owner")).toBe("token-a-owner");
      expect(getCachedToken("workspace-a", "editor")).toBe("token-a-editor");
      expect(getCachedToken("workspace-b", "viewer")).toBe("token-b-viewer");
    });

    it("should update existing entry", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000;

      setCachedToken("my-workspace", "editor", "old-token", expiresAt);
      setCachedToken("my-workspace", "editor", "new-token", expiresAt);

      const result = getCachedToken("my-workspace", "editor");
      expect(result).toBe("new-token");
    });

    it("should evict oldest entries when at capacity", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000;

      // Fill cache to capacity (100 entries)
      for (let i = 0; i < 100; i++) {
        setCachedToken(`workspace-${i}`, "editor", `token-${i}`, expiresAt);
      }

      // Verify first entry exists
      expect(getCachedToken("workspace-0", "editor")).toBe("token-0");

      // Add one more entry
      setCachedToken("workspace-new", "editor", "new-token", expiresAt);

      // First entry should be evicted (but we accessed it above so it moved to end)
      // The second entry (workspace-1) should be evicted instead
      expect(getCachedToken("workspace-1", "editor")).toBeNull();

      // New entry should exist
      expect(getCachedToken("workspace-new", "editor")).toBe("new-token");
    });
  });

  describe("invalidateWorkspaceTokens", () => {
    it("should remove all tokens for a workspace", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000;

      setCachedToken("my-workspace", "owner", "token-owner", expiresAt);
      setCachedToken("my-workspace", "editor", "token-editor", expiresAt);
      setCachedToken("my-workspace", "viewer", "token-viewer", expiresAt);
      setCachedToken("other-workspace", "editor", "other-token", expiresAt);

      invalidateWorkspaceTokens("my-workspace");

      expect(getCachedToken("my-workspace", "owner")).toBeNull();
      expect(getCachedToken("my-workspace", "editor")).toBeNull();
      expect(getCachedToken("my-workspace", "viewer")).toBeNull();
      expect(getCachedToken("other-workspace", "editor")).toBe("other-token");
    });
  });

  describe("invalidateToken", () => {
    it("should remove specific token", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000;

      setCachedToken("my-workspace", "owner", "token-owner", expiresAt);
      setCachedToken("my-workspace", "editor", "token-editor", expiresAt);

      invalidateToken("my-workspace", "editor");

      expect(getCachedToken("my-workspace", "owner")).toBe("token-owner");
      expect(getCachedToken("my-workspace", "editor")).toBeNull();
    });
  });

  describe("clearTokenCache", () => {
    it("should remove all entries", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000;

      setCachedToken("workspace-a", "editor", "token-a", expiresAt);
      setCachedToken("workspace-b", "editor", "token-b", expiresAt);

      clearTokenCache();

      expect(getCachedToken("workspace-a", "editor")).toBeNull();
      expect(getCachedToken("workspace-b", "editor")).toBeNull();
      expect(getTokenCacheStats().size).toBe(0);
    });
  });

  describe("getTokenCacheStats", () => {
    it("should return correct stats", () => {
      const expiresAt = Date.now() + 60 * 60 * 1000;

      setCachedToken("workspace-a", "editor", "token-a", expiresAt);
      setCachedToken("workspace-b", "editor", "token-b", expiresAt);

      const stats = getTokenCacheStats();

      expect(stats.size).toBe(2);
      expect(stats.maxSize).toBe(100);
      expect(stats.defaultTtlMs).toBe(50 * 60 * 1000);
    });
  });

  describe("pruneExpiredTokens", () => {
    it("should remove expired entries", () => {
      const shortExpiry = Date.now() + 10 * 60 * 1000; // 10 minutes
      const longExpiry = Date.now() + 60 * 60 * 1000; // 1 hour

      setCachedToken("workspace-short", "editor", "short-token", shortExpiry);
      setCachedToken("workspace-long", "editor", "long-token", longExpiry);

      // Advance time past short expiry but not long expiry
      vi.advanceTimersByTime(15 * 60 * 1000);

      const removed = pruneExpiredTokens();

      expect(removed).toBe(1);
      expect(getCachedToken("workspace-short", "editor")).toBeNull();
      expect(getCachedToken("workspace-long", "editor")).toBe("long-token");
    });
  });
});
