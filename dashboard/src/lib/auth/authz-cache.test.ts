import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import {
  getCachedAccess,
  setCachedAccess,
  invalidateWorkspaceCache,
  invalidateUserCache,
  clearAuthzCache,
  getCacheStats,
  pruneExpiredEntries,
} from "./authz-cache";
import type { WorkspaceAccess } from "@/types/workspace";

describe("authz-cache", () => {
  const mockAccess: WorkspaceAccess = {
    granted: true,
    role: "editor",
    permissions: {
      read: true,
      write: true,
      delete: true,
      manageMembers: false,
    },
  };

  beforeEach(() => {
    clearAuthzCache();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("getCachedAccess", () => {
    it("should return null for uncached entries", () => {
      const result = getCachedAccess("user@example.com", "my-workspace");
      expect(result).toBeNull();
    });

    it("should return cached access after set", () => {
      setCachedAccess("user@example.com", "my-workspace", mockAccess);
      const result = getCachedAccess("user@example.com", "my-workspace");
      expect(result).toEqual(mockAccess);
    });

    it("should return null for expired entries", () => {
      setCachedAccess("user@example.com", "my-workspace", mockAccess);

      // Advance time past TTL (5 minutes + 1 second)
      vi.advanceTimersByTime(5 * 60 * 1000 + 1000);

      const result = getCachedAccess("user@example.com", "my-workspace");
      expect(result).toBeNull();
    });

    it("should return valid entry just before expiry", () => {
      setCachedAccess("user@example.com", "my-workspace", mockAccess);

      // Advance time to just before TTL
      vi.advanceTimersByTime(5 * 60 * 1000 - 1000);

      const result = getCachedAccess("user@example.com", "my-workspace");
      expect(result).toEqual(mockAccess);
    });
  });

  describe("setCachedAccess", () => {
    it("should store access by email and workspace", () => {
      setCachedAccess("user1@example.com", "workspace-a", mockAccess);
      setCachedAccess("user2@example.com", "workspace-a", {
        ...mockAccess,
        role: "viewer",
      });
      setCachedAccess("user1@example.com", "workspace-b", {
        ...mockAccess,
        role: "owner",
      });

      expect(getCachedAccess("user1@example.com", "workspace-a")).toEqual(
        mockAccess
      );
      expect(getCachedAccess("user2@example.com", "workspace-a")?.role).toBe(
        "viewer"
      );
      expect(getCachedAccess("user1@example.com", "workspace-b")?.role).toBe(
        "owner"
      );
    });

    it("should update existing entry", () => {
      setCachedAccess("user@example.com", "my-workspace", mockAccess);
      setCachedAccess("user@example.com", "my-workspace", {
        ...mockAccess,
        role: "owner",
      });

      const result = getCachedAccess("user@example.com", "my-workspace");
      expect(result?.role).toBe("owner");
    });

    it("should evict oldest entries when at capacity", () => {
      // Fill cache to capacity (1000 entries)
      for (let i = 0; i < 1000; i++) {
        setCachedAccess(`user${i}@example.com`, "workspace", mockAccess);
      }

      // Verify cache is at capacity
      expect(getCacheStats().size).toBe(1000);

      // Add one more entry - should evict oldest (user0)
      setCachedAccess("user1000@example.com", "workspace", mockAccess);

      // Cache size should still be 1000 (not 1001)
      expect(getCacheStats().size).toBe(1000);

      // First entry should be evicted (oldest)
      expect(getCachedAccess("user0@example.com", "workspace")).toBeNull();
      // New entry should be present
      expect(
        getCachedAccess("user1000@example.com", "workspace")
      ).not.toBeNull();
      // Entry added just after user0 should still be present
      expect(
        getCachedAccess("user1@example.com", "workspace")
      ).not.toBeNull();
    });
  });

  describe("invalidateWorkspaceCache", () => {
    it("should remove all entries for a workspace", () => {
      setCachedAccess("user1@example.com", "workspace-a", mockAccess);
      setCachedAccess("user2@example.com", "workspace-a", mockAccess);
      setCachedAccess("user1@example.com", "workspace-b", mockAccess);

      invalidateWorkspaceCache("workspace-a");

      expect(getCachedAccess("user1@example.com", "workspace-a")).toBeNull();
      expect(getCachedAccess("user2@example.com", "workspace-a")).toBeNull();
      // Other workspace should not be affected
      expect(
        getCachedAccess("user1@example.com", "workspace-b")
      ).not.toBeNull();
    });

    it("should handle workspace not in cache", () => {
      setCachedAccess("user@example.com", "workspace-a", mockAccess);

      // Should not throw
      invalidateWorkspaceCache("workspace-nonexistent");

      // Existing entries should remain
      expect(
        getCachedAccess("user@example.com", "workspace-a")
      ).not.toBeNull();
    });
  });

  describe("invalidateUserCache", () => {
    it("should remove all entries for a user", () => {
      setCachedAccess("user1@example.com", "workspace-a", mockAccess);
      setCachedAccess("user1@example.com", "workspace-b", mockAccess);
      setCachedAccess("user2@example.com", "workspace-a", mockAccess);

      invalidateUserCache("user1@example.com");

      expect(getCachedAccess("user1@example.com", "workspace-a")).toBeNull();
      expect(getCachedAccess("user1@example.com", "workspace-b")).toBeNull();
      // Other user should not be affected
      expect(
        getCachedAccess("user2@example.com", "workspace-a")
      ).not.toBeNull();
    });
  });

  describe("clearAuthzCache", () => {
    it("should remove all entries", () => {
      setCachedAccess("user1@example.com", "workspace-a", mockAccess);
      setCachedAccess("user2@example.com", "workspace-b", mockAccess);

      clearAuthzCache();

      expect(getCachedAccess("user1@example.com", "workspace-a")).toBeNull();
      expect(getCachedAccess("user2@example.com", "workspace-b")).toBeNull();
    });
  });

  describe("getCacheStats", () => {
    it("should return current cache statistics", () => {
      const stats = getCacheStats();

      expect(stats).toHaveProperty("size");
      expect(stats).toHaveProperty("maxSize", 1000);
      expect(stats).toHaveProperty("ttlMs", 5 * 60 * 1000);
    });

    it("should reflect current cache size", () => {
      expect(getCacheStats().size).toBe(0);

      setCachedAccess("user1@example.com", "workspace-a", mockAccess);
      expect(getCacheStats().size).toBe(1);

      setCachedAccess("user2@example.com", "workspace-b", mockAccess);
      expect(getCacheStats().size).toBe(2);

      clearAuthzCache();
      expect(getCacheStats().size).toBe(0);
    });
  });

  describe("pruneExpiredEntries", () => {
    it("should remove only expired entries", () => {
      setCachedAccess("user1@example.com", "workspace-a", mockAccess);

      // Advance time by 3 minutes
      vi.advanceTimersByTime(3 * 60 * 1000);

      // Add another entry
      setCachedAccess("user2@example.com", "workspace-b", mockAccess);

      // Advance time by 3 more minutes (first entry now expired, second not)
      vi.advanceTimersByTime(3 * 60 * 1000);

      const removed = pruneExpiredEntries();

      expect(removed).toBe(1);
      expect(getCachedAccess("user1@example.com", "workspace-a")).toBeNull();
      expect(
        getCachedAccess("user2@example.com", "workspace-b")
      ).not.toBeNull();
    });

    it("should return 0 when no entries are expired", () => {
      setCachedAccess("user@example.com", "workspace", mockAccess);

      const removed = pruneExpiredEntries();

      expect(removed).toBe(0);
    });
  });

  describe("LRU behavior", () => {
    it("should update entry position on get", () => {
      // Add entries
      setCachedAccess("user1@example.com", "workspace", mockAccess);
      setCachedAccess("user2@example.com", "workspace", mockAccess);
      setCachedAccess("user3@example.com", "workspace", mockAccess);

      // Access user1 to move it to the end (most recently used)
      getCachedAccess("user1@example.com", "workspace");

      // Fill remaining capacity to trigger eviction
      for (let i = 4; i <= 1000; i++) {
        setCachedAccess(`user${i}@example.com`, "workspace", mockAccess);
      }

      // Add one more to trigger eviction
      setCachedAccess("user1001@example.com", "workspace", mockAccess);

      // user2 should be evicted (was oldest after user1 was accessed)
      expect(getCachedAccess("user2@example.com", "workspace")).toBeNull();
      // user1 should still be present (was accessed, moved to end)
      expect(getCachedAccess("user1@example.com", "workspace")).not.toBeNull();
    });
  });
});
