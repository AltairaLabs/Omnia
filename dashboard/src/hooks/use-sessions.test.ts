import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// Mock dependencies
const mockGetSessions = vi.fn();
const mockGetSessionById = vi.fn();
const mockSearchSessions = vi.fn();
const mockGetSessionMessages = vi.fn();

const { mockGetSessionEvalResults, mockGetToolCalls, mockGetProviderCalls } = vi.hoisted(() => {
  return {
    mockGetSessionEvalResults: vi.fn(),
    mockGetToolCalls: vi.fn(),
    mockGetProviderCalls: vi.fn(),
  };
});

vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "MockDataService",
    getSessions: mockGetSessions,
    getSessionById: mockGetSessionById,
    searchSessions: mockSearchSessions,
    getSessionMessages: mockGetSessionMessages,
  }),
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace" },
  }),
}));

vi.mock("@/lib/data/session-api-service", () => ({
  SessionApiService: class MockSessionApiService {
    getSessionEvalResults = mockGetSessionEvalResults;
    getToolCalls = mockGetToolCalls;
    getProviderCalls = mockGetProviderCalls;
  },
}));

// Import after mocks
import { useSessions, useSessionDetail, useSessionSearch, useSessionMessages, useSessionAllMessages, useSessionToolCalls, useSessionProviderCalls, useSessionEvalResults } from "./use-sessions";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("useSessions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetSessions.mockResolvedValue({
      sessions: [
        { id: "s1", agentName: "agent-1", status: "active", startedAt: new Date().toISOString(), messageCount: 5, toolCallCount: 2, totalTokens: 1000 },
      ],
      total: 1,
      hasMore: false,
    });
  });

  it("fetches sessions for the current workspace", async () => {
    const { result } = renderHook(() => useSessions(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetSessions).toHaveBeenCalledWith("test-workspace", {});
    expect(result.current.data?.sessions).toHaveLength(1);
    expect(result.current.data?.total).toBe(1);
  });

  it("passes filter options to the service", async () => {
    const options = { status: "active" as const, agent: "agent-1", limit: 10, offset: 0 };
    const { result } = renderHook(() => useSessions(options), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetSessions).toHaveBeenCalledWith("test-workspace", options);
  });
});

describe("useSessionDetail", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetSessionById.mockResolvedValue({
      id: "s1",
      agentName: "agent-1",
      agentNamespace: "default",
      status: "completed",
      startedAt: new Date().toISOString(),
      messages: [],
      metrics: { messageCount: 0, toolCallCount: 0, totalTokens: 0, inputTokens: 0, outputTokens: 0 },
    });
  });

  it("fetches a single session by ID", async () => {
    const { result } = renderHook(() => useSessionDetail("s1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetSessionById).toHaveBeenCalledWith("test-workspace", "s1");
    expect(result.current.data?.id).toBe("s1");
  });

  it("is disabled when sessionId is empty", () => {
    const { result } = renderHook(() => useSessionDetail(""), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("useSessionSearch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSearchSessions.mockResolvedValue({
      sessions: [{ id: "s1", agentName: "agent-1", status: "active" }],
      total: 1,
      hasMore: false,
    });
  });

  it("searches sessions with a query", async () => {
    const options = { q: "hello" };
    const { result } = renderHook(() => useSessionSearch(options), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockSearchSessions).toHaveBeenCalledWith("test-workspace", options);
  });

  it("is disabled when q is empty", () => {
    const { result } = renderHook(() => useSessionSearch({ q: "" }), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("useSessionMessages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetSessionMessages.mockResolvedValue({
      messages: [{ id: "m1", role: "user", content: "hello", timestamp: new Date().toISOString() }],
      hasMore: false,
    });
  });

  it("fetches messages for a session", async () => {
    const { result } = renderHook(() => useSessionMessages("s1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetSessionMessages).toHaveBeenCalledWith("test-workspace", "s1", {});
    expect(result.current.data?.messages).toHaveLength(1);
  });

  it("passes pagination options", async () => {
    const opts = { limit: 10, after: 5 };
    const { result } = renderHook(() => useSessionMessages("s1", opts), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetSessionMessages).toHaveBeenCalledWith("test-workspace", "s1", opts);
  });
});

describe("useSessionAllMessages", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches first page of messages", async () => {
    mockGetSessionMessages.mockResolvedValue({
      messages: [
        { id: "m1", role: "user", content: "hello", timestamp: new Date().toISOString(), sequenceNum: 1 },
        { id: "m2", role: "assistant", content: "hi", timestamp: new Date().toISOString(), sequenceNum: 2 },
      ],
      hasMore: false,
    });

    const { result } = renderHook(() => useSessionAllMessages("s1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(mockGetSessionMessages).toHaveBeenCalledWith("test-workspace", "s1", { limit: 100 });
    expect(result.current.messages).toHaveLength(2);
    expect(result.current.totalLoaded).toBe(2);
    expect(result.current.hasMore).toBe(false);
  });

  it("is disabled when sessionId is empty", () => {
    const { result } = renderHook(() => useSessionAllMessages(""), { wrapper: createWrapper() });
    expect(result.current.isLoading).toBe(false);
    expect(result.current.messages).toHaveLength(0);
  });

  it("is disabled when enabled=false", () => {
    const { result } = renderHook(() => useSessionAllMessages("s1", false), { wrapper: createWrapper() });
    expect(result.current.isLoading).toBe(false);
    expect(result.current.messages).toHaveLength(0);
  });

  it("deduplicates messages across pages", async () => {
    mockGetSessionMessages.mockResolvedValue({
      messages: [
        { id: "m1", role: "user", content: "hello", timestamp: new Date().toISOString(), sequenceNum: 1 },
        { id: "m1", role: "user", content: "hello", timestamp: new Date().toISOString(), sequenceNum: 1 },
      ],
      hasMore: false,
    });

    const { result } = renderHook(() => useSessionAllMessages("s1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.messages).toHaveLength(1);
  });

  it("reports hasMore when more pages available", async () => {
    mockGetSessionMessages.mockResolvedValue({
      messages: Array.from({ length: 100 }, (_, i) => ({
        id: `m${i}`,
        role: "user",
        content: `msg ${i}`,
        timestamp: new Date().toISOString(),
        sequenceNum: i + 1,
      })),
      hasMore: true,
    });

    const { result } = renderHook(() => useSessionAllMessages("s1"), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.messages).toHaveLength(100);
    expect(result.current.hasMore).toBe(true);
  });
});

describe("useSessionEvalResults", () => {
  beforeEach(() => {
    mockGetSessionEvalResults.mockReset();
    mockGetSessionEvalResults.mockResolvedValue([
      { id: "e1", sessionId: "s1", evalId: "tone", evalType: "llm_judge", passed: true, source: "in_proc", createdAt: new Date().toISOString() },
    ]);
  });

  it("fetches eval results for a session", async () => {
    const { result } = renderHook(() => useSessionEvalResults("s1"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isPending).toBe(false);
    });

    expect(result.current.isSuccess).toBe(true);
    expect(mockGetSessionEvalResults).toHaveBeenCalledWith("test-workspace", "s1");
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data![0].evalId).toBe("tone");
  });

  it("is disabled when sessionId is empty", () => {
    const { result } = renderHook(() => useSessionEvalResults(""), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("useSessionToolCalls", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetToolCalls.mockResolvedValue([
      { id: "tc1", callId: "call-1", sessionId: "s1", name: "search", status: "success", createdAt: "2024-01-01T00:00:00Z" },
    ]);
  });

  it("fetches tool calls for a session", async () => {
    const { result } = renderHook(() => useSessionToolCalls("s1"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isPending).toBe(false);
    });

    expect(result.current.isSuccess).toBe(true);
    expect(mockGetToolCalls).toHaveBeenCalledWith("test-workspace", "s1");
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data![0].name).toBe("search");
  });

  it("is disabled when sessionId is empty", () => {
    const { result } = renderHook(() => useSessionToolCalls(""), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});

describe("useSessionProviderCalls", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetProviderCalls.mockResolvedValue([
      { id: "pc1", sessionId: "s1", provider: "claude", model: "sonnet", status: "completed", createdAt: "2024-01-01T00:00:00Z" },
    ]);
  });

  it("fetches provider calls for a session", async () => {
    const { result } = renderHook(() => useSessionProviderCalls("s1"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isPending).toBe(false);
    });

    expect(result.current.isSuccess).toBe(true);
    expect(mockGetProviderCalls).toHaveBeenCalledWith("test-workspace", "s1");
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data![0].provider).toBe("claude");
  });

  it("is disabled when sessionId is empty", () => {
    const { result } = renderHook(() => useSessionProviderCalls(""), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe("idle");
  });
});
