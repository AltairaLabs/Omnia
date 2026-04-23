import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { InstitutionalMemoryService } from "./institutional-memory-service";

const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("InstitutionalMemoryService", () => {
  let service: InstitutionalMemoryService;

  beforeEach(() => {
    service = new InstitutionalMemoryService();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("list", () => {
    it("fetches institutional memories for a workspace", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () =>
          Promise.resolve({
            memories: [
              {
                id: "inst-1",
                type: "policy",
                content: "snake_case rule",
                confidence: 1.0,
                scope: {},
                createdAt: "2026-04-22T00:00:00Z",
              },
            ],
            total: 1,
          }),
      });

      const result = await service.list({ workspace: "my-ws" });

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/my-ws/institutional-memory");
      expect(result.memories).toHaveLength(1);
      expect(result.total).toBe(1);
    });

    it("passes limit and offset as query params", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ memories: [], total: 0 }),
      });

      await service.list({ workspace: "ws", limit: 10, offset: 5 });

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("limit=10");
      expect(url).toContain("offset=5");
    });

    it("returns empty on 401/403/404", async () => {
      for (const status of [401, 403, 404]) {
        mockFetch.mockResolvedValueOnce({
          ok: false,
          status,
          statusText: "denied",
        });
        const result = await service.list({ workspace: "ws" });
        expect(result).toEqual({ memories: [], total: 0 });
      }
    });

    it("throws on other errors", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "server" });
      await expect(service.list({ workspace: "ws" })).rejects.toThrow();
    });

    it("handles missing memories field", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve({}) });
      const result = await service.list({ workspace: "ws" });
      expect(result.memories).toEqual([]);
      expect(result.total).toBe(0);
    });
  });

  describe("create", () => {
    it("posts a new memory and returns the created entity", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () =>
          Promise.resolve({
            memory: {
              id: "new-1",
              type: "policy",
              content: "API returns ISO timestamps",
              confidence: 1.0,
              scope: {},
              createdAt: "2026-04-22T00:00:00Z",
            },
          }),
      });

      const result = await service.create({
        workspace: "ws",
        type: "policy",
        content: "API returns ISO timestamps",
      });

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/ws/institutional-memory",
        expect.objectContaining({
          method: "POST",
          headers: { "Content-Type": "application/json" },
        })
      );
      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body).toMatchObject({
        type: "policy",
        content: "API returns ISO timestamps",
        confidence: 1.0,
      });
      expect(result.id).toBe("new-1");
    });

    it("defaults confidence to 1.0 and metadata to empty", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ memory: { id: "x", type: "t", content: "c", confidence: 1, scope: {}, createdAt: "" } }),
      });

      await service.create({ workspace: "ws", type: "t", content: "c" });

      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.confidence).toBe(1.0);
      expect(body.metadata).toEqual({});
    });

    it("forwards metadata and explicit confidence", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ memory: { id: "x", type: "t", content: "c", confidence: 0.8, scope: {}, createdAt: "" } }),
      });

      await service.create({
        workspace: "ws",
        type: "t",
        content: "c",
        confidence: 0.8,
        metadata: { source: "doc" },
      });

      const body = JSON.parse(mockFetch.mock.calls[0][1].body);
      expect(body.confidence).toBe(0.8);
      expect(body.metadata).toEqual({ source: "doc" });
    });

    it("throws on non-ok responses with the server error message", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "bad",
        json: () => Promise.resolve({ error: "validation failed" }),
      });
      await expect(service.create({ workspace: "ws", type: "t", content: "c" })).rejects.toThrow(/validation failed/);
    });

    it("falls back to statusText when error body is not JSON", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "server",
        json: () => Promise.reject(new Error("not json")),
      });
      await expect(service.create({ workspace: "ws", type: "t", content: "c" })).rejects.toThrow(/server/);
    });
  });

  describe("delete", () => {
    it("deletes a memory by ID", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.delete("ws", "inst-1");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/ws/institutional-memory/inst-1",
        { method: "DELETE" }
      );
    });

    it("encodes URL-unsafe IDs", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });
      await service.delete("ws", "a/b c");
      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("a%2Fb%20c");
    });

    it("throws on non-ok with server message", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "bad",
        json: () => Promise.resolve({ error: "not institutional" }),
      });
      await expect(service.delete("ws", "m-1")).rejects.toThrow(/not institutional/);
    });

    it("falls back to statusText", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "server",
        json: () => Promise.reject(new Error("nope")),
      });
      await expect(service.delete("ws", "m-1")).rejects.toThrow(/server/);
    });
  });
});
