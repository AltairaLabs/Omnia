import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { MemoryApiService } from "./memory-api-service";

const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("MemoryApiService", () => {
  let service: MemoryApiService;

  beforeEach(() => {
    service = new MemoryApiService();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("getMemories", () => {
    it("fetches memories with no optional params", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({
          memories: [
            {
              id: "m1",
              type: "fact",
              content: "User prefers dark mode",
              confidence: 0.9,
              scope: { userId: "u1" },
              createdAt: "2025-01-01T00:00:00Z",
            },
          ],
          total: 1,
        }),
      });

      const result = await service.getMemories({ workspace: "my-ws" });

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/my-ws/memory");
      expect(result.memories).toHaveLength(1);
      expect(result.memories[0].id).toBe("m1");
      expect(result.total).toBe(1);
    });

    it("passes filter options as query params", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ memories: [], total: 0 }),
      });

      await service.getMemories({
        workspace: "ws",
        userId: "u1",
        type: "fact",
        purpose: "personalization",
        limit: 10,
        offset: 5,
      });

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("userId=u1");
      expect(url).toContain("type=fact");
      expect(url).toContain("purpose=personalization");
      expect(url).toContain("limit=10");
      expect(url).toContain("offset=5");
    });

    it("returns empty result on 403", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 403, statusText: "Forbidden" });

      const result = await service.getMemories({ workspace: "ws" });
      expect(result).toEqual({ memories: [], total: 0 });
    });

    it("returns empty result on 401", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 401, statusText: "Unauthorized" });

      const result = await service.getMemories({ workspace: "ws" });
      expect(result).toEqual({ memories: [], total: 0 });
    });

    it("returns empty result on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404, statusText: "Not Found" });

      const result = await service.getMemories({ workspace: "ws" });
      expect(result).toEqual({ memories: [], total: 0 });
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Internal Server Error" });

      await expect(service.getMemories({ workspace: "ws" })).rejects.toThrow("Failed to fetch memories");
    });

    it("handles response with no memories field", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ total: 0 }),
      });

      const result = await service.getMemories({ workspace: "ws" });
      expect(result.memories).toEqual([]);
    });
  });

  describe("searchMemories", () => {
    it("includes query param in request", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ memories: [], total: 0 }),
      });

      await service.searchMemories({ workspace: "ws", query: "dark mode" });

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("/api/workspaces/ws/memory/search");
      expect(url).toContain("query=dark+mode");
    });

    it("passes all search options as query params", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ memories: [], total: 0 }),
      });

      await service.searchMemories({
        workspace: "ws",
        query: "test",
        userId: "u1",
        type: "preference",
        purpose: "search",
        limit: 20,
        offset: 0,
        minConfidence: 0.8,
      });

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("query=test");
      expect(url).toContain("userId=u1");
      expect(url).toContain("type=preference");
      expect(url).toContain("purpose=search");
      expect(url).toContain("limit=20");
      expect(url).toContain("offset=0");
      expect(url).toContain("minConfidence=0.8");
    });

    it("returns empty result on 403", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 403, statusText: "Forbidden" });

      const result = await service.searchMemories({ workspace: "ws", query: "test" });
      expect(result).toEqual({ memories: [], total: 0 });
    });

    it("returns empty result on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404, statusText: "Not Found" });

      const result = await service.searchMemories({ workspace: "ws", query: "test" });
      expect(result).toEqual({ memories: [], total: 0 });
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Server Error" });

      await expect(service.searchMemories({ workspace: "ws", query: "test" })).rejects.toThrow("Failed to search memories");
    });
  });

  describe("exportMemories", () => {
    it("includes workspace and userId params", async () => {
      const memories = [
        { id: "m1", type: "fact", content: "data", confidence: 1.0, scope: {}, createdAt: "2025-01-01T00:00:00Z" },
      ];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ memories }),
      });

      const result = await service.exportMemories("my-ws", "user-123");

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("/api/workspaces/my-ws/memory/export");
      expect(url).toContain("userId=user-123");
      expect(result).toHaveLength(1);
      expect(result[0].id).toBe("m1");
    });

    it("handles direct array response", async () => {
      const memories = [
        { id: "m1", type: "fact", content: "data", confidence: 1.0, scope: {}, createdAt: "2025-01-01T00:00:00Z" },
      ];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(memories),
      });

      const result = await service.exportMemories("ws", "u1");
      expect(result).toHaveLength(1);
    });

    it("returns empty array on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404, statusText: "Not Found" });

      const result = await service.exportMemories("ws", "u1");
      expect(result).toEqual([]);
    });

    it("returns empty array on 403", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 403, statusText: "Forbidden" });

      const result = await service.exportMemories("ws", "u1");
      expect(result).toEqual([]);
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Server Error" });

      await expect(service.exportMemories("ws", "u1")).rejects.toThrow("Failed to export memories");
    });
  });

  describe("deleteMemory", () => {
    it("sends DELETE request to correct URL", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.deleteMemory("my-ws", "memory-abc");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/my-ws/memory/memory-abc",
        { method: "DELETE" }
      );
    });

    it("encodes workspace and memoryId in URL", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.deleteMemory("my workspace", "memory/with/slashes");

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("my%20workspace");
      expect(url).toContain("memory%2Fwith%2Fslashes");
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Server Error" });

      await expect(service.deleteMemory("ws", "m1")).rejects.toThrow("Failed to delete memory");
    });

    it("throws on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404, statusText: "Not Found" });

      await expect(service.deleteMemory("ws", "m1")).rejects.toThrow("Failed to delete memory");
    });
  });

  describe("deleteAllMemories", () => {
    it("sends DELETE request with userId param", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.deleteAllMemories("my-ws", "user-123");

      const [url, opts] = mockFetch.mock.calls[0] as [string, RequestInit];
      expect(url).toContain("/api/workspaces/my-ws/memory");
      expect(url).toContain("userId=user-123");
      expect(opts.method).toBe("DELETE");
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Server Error" });

      await expect(service.deleteAllMemories("ws", "u1")).rejects.toThrow("Failed to delete all memories");
    });

    it("throws on 403", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 403, statusText: "Forbidden" });

      await expect(service.deleteAllMemories("ws", "u1")).rejects.toThrow("Failed to delete all memories");
    });
  });
});
