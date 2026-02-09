import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { SessionApiService } from "./session-api-service";

const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("SessionApiService", () => {
  let service: SessionApiService;

  beforeEach(() => {
    service = new SessionApiService();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("getSessions", () => {
    it("fetches sessions with default params", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({
          sessions: [
            { id: "s1", agentName: "agent-1", namespace: "default", createdAt: "2024-01-01T00:00:00Z", status: "active", messageCount: 5, toolCallCount: 2, totalInputTokens: 100, totalOutputTokens: 200 },
          ],
          total: 1,
          hasMore: false,
        }),
      });

      const result = await service.getSessions("my-workspace");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/my-workspace/sessions");
      expect(result.sessions).toHaveLength(1);
      expect(result.sessions[0].agentNamespace).toBe("default");
      expect(result.sessions[0].startedAt).toBe("2024-01-01T00:00:00Z");
      expect(result.sessions[0].totalTokens).toBe(300);
      expect(result.total).toBe(1);
      expect(result.hasMore).toBe(false);
    });

    it("passes filter options as query params", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ sessions: [], total: 0, hasMore: false }),
      });

      await service.getSessions("ws", { agent: "a1", status: "completed", limit: 10, offset: 20, from: "2024-01-01", to: "2024-12-31" });

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("agent=a1");
      expect(url).toContain("status=completed");
      expect(url).toContain("limit=10");
      expect(url).toContain("offset=20");
      expect(url).toContain("from=2024-01-01");
      expect(url).toContain("to=2024-12-31");
    });

    it("returns empty result on 403", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 403, statusText: "Forbidden" });

      const result = await service.getSessions("ws");
      expect(result).toEqual({ sessions: [], total: 0, hasMore: false });
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Internal Server Error" });

      await expect(service.getSessions("ws")).rejects.toThrow("Failed to fetch sessions");
    });
  });

  describe("getSessionById", () => {
    it("fetches and transforms a single session", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({
          session: {
            id: "s1",
            agentName: "agent-1",
            namespace: "ns",
            createdAt: "2024-01-01T00:00:00Z",
            status: "completed",
            messageCount: 3,
            toolCallCount: 1,
            totalInputTokens: 500,
            totalOutputTokens: 1000,
            estimatedCostUSD: 0.05,
            tags: ["support"],
            lastMessagePreview: "Hello",
          },
          messages: [
            { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:01Z", inputTokens: 10 },
          ],
        }),
      });

      const result = await service.getSessionById("ws", "s1");

      expect(result).toBeDefined();
      expect(result!.id).toBe("s1");
      expect(result!.agentNamespace).toBe("ns");
      expect(result!.startedAt).toBe("2024-01-01T00:00:00Z");
      expect(result!.metrics.inputTokens).toBe(500);
      expect(result!.metrics.outputTokens).toBe(1000);
      expect(result!.metrics.totalTokens).toBe(1500);
      expect(result!.metrics.estimatedCost).toBe(0.05);
      expect(result!.metadata?.tags).toEqual(["support"]);
      expect(result!.messages).toHaveLength(1);
      expect(result!.messages[0].tokens?.input).toBe(10);
    });

    it("returns undefined on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404, statusText: "Not Found" });

      const result = await service.getSessionById("ws", "missing");
      expect(result).toBeUndefined();
    });
  });

  describe("searchSessions", () => {
    it("sends search query to the proxy route", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ sessions: [], total: 0, hasMore: false }),
      });

      await service.searchSessions("ws", { q: "hello world", limit: 5 });

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("q=hello+world");
      expect(url).toContain("limit=5");
    });

    it("returns empty on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404, statusText: "Not Found" });

      const result = await service.searchSessions("ws", { q: "test" });
      expect(result).toEqual({ sessions: [], total: 0, hasMore: false });
    });
  });

  describe("getSessionMessages", () => {
    it("fetches and transforms messages", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({
          messages: [
            { id: "m1", role: "user", content: "Hello", timestamp: "2024-01-01T00:00:00Z", inputTokens: 10, outputTokens: 0 },
            { id: "m2", role: "assistant", content: "Hi!", timestamp: "2024-01-01T00:00:01Z", inputTokens: 0, outputTokens: 50, toolCallId: "tc1" },
          ],
          hasMore: true,
        }),
      });

      const result = await service.getSessionMessages("ws", "s1", { limit: 2 });

      expect(result.messages).toHaveLength(2);
      expect(result.messages[0].tokens?.input).toBe(10);
      expect(result.messages[1].toolCallId).toBe("tc1");
      expect(result.hasMore).toBe(true);

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("limit=2");
    });

    it("passes before/after params", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ messages: [], hasMore: false }),
      });

      await service.getSessionMessages("ws", "s1", { before: 10, after: 5 });

      const url = mockFetch.mock.calls[0][0] as string;
      expect(url).toContain("before=10");
      expect(url).toContain("after=5");
    });

    it("returns empty on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404, statusText: "Not Found" });

      const result = await service.getSessionMessages("ws", "missing");
      expect(result).toEqual({ messages: [], hasMore: false });
    });

    it("throws on server error", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Server Error" });

      await expect(service.getSessionMessages("ws", "s1")).rejects.toThrow("Failed to fetch session messages");
    });
  });
});
