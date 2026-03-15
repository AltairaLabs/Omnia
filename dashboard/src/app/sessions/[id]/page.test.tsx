/**
 * Tests for Session detail page.
 */

import { Suspense } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import SessionDetailPage from "./page";

// Mock hooks
vi.mock("@/hooks", () => ({
  useSessionDetail: vi.fn(),
  useSessionEvalResults: vi.fn(),
  useSessionToolCalls: vi.fn(() => ({ data: [] })),
  useSessionProviderCalls: vi.fn(() => ({ data: [] })),
  useSessionAllMessages: vi.fn(() => ({
    messages: [],
    totalLoaded: 0,
    hasMore: false,
    isLoading: false,
    isFetchingMore: false,
    fetchMore: vi.fn(),
    error: null,
  })),
}));

// Mock next/link
vi.mock("next/link", () => ({
  default: function MockLink({ children, href }: { children: React.ReactNode; href: string }) {
    return <a href={href}>{children}</a>;
  },
}));

// Mock next/navigation
const mockPush = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mockPush,
  }),
  useSearchParams: () => new URLSearchParams(),
}));

// Mock layout components
vi.mock("@/components/layout", () => ({
  Header: function MockHeader({ title, description, children }: { title: React.ReactNode; description?: React.ReactNode; children?: React.ReactNode }) {
    return (
      <div data-testid="header">
        <div>{title}</div>
        {description && <div>{description}</div>}
        {children}
      </div>
    );
  },
}));

const mockSession = {
  id: "sess-123",
  agentName: "support-agent",
  agentNamespace: "default",
  status: "completed" as const,
  startedAt: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
  endedAt: new Date().toISOString(),
  messages: [
    {
      id: "m1",
      role: "user" as const,
      content: "Hello, I need help",
      timestamp: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
      tokens: { input: 10 },
    },
    {
      id: "m2",
      role: "assistant" as const,
      content: '{"name":"search_docs","arguments":{"query":"help"}}',
      timestamp: new Date(Date.now() - 59 * 60 * 1000).toISOString(),
      metadata: { type: "tool_call", duration_ms: "250", status: "success" },
      toolCallId: "tc1",
    },
    {
      id: "m3",
      role: "assistant" as const,
      content: "How can I help you?",
      timestamp: new Date(Date.now() - 58 * 60 * 1000).toISOString(),
      tokens: { output: 20 },
    },
  ],
  metrics: {
    messageCount: 3,
    toolCallCount: 1,
    totalTokens: 30,
    inputTokens: 10,
    outputTokens: 20,
    estimatedCost: 0.0005,
    avgResponseTime: 1200,
  },
  metadata: {
    tags: ["support", "urgent"],
    userAgent: "Mozilla/5.0",
    clientIp: "client-ip-test",
  },
};

async function renderPage(id = "sess-123") {
  await act(async () => {
    render(
      <Suspense fallback={<div>Loading...</div>}>
        <SessionDetailPage params={Promise.resolve({ id })} />
      </Suspense>
    );
  });
}

describe("SessionDetailPage", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    // Default mocks to avoid errors in tests that don't set them
    const { useSessionEvalResults, useSessionToolCalls, useSessionProviderCalls } = await import("@/hooks");
    vi.mocked(useSessionEvalResults).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionToolCalls).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionProviderCalls).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as any);
  });

  it("renders loading skeleton when loading", async () => {
    const { useSessionDetail, useSessionEvalResults } = await import("@/hooks");
    vi.mocked(useSessionDetail).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as any);
    vi.mocked(useSessionEvalResults).mockReturnValue({
      data: [],
      isLoading: true,
      error: null,
    } as any);

    await renderPage();

    expect(screen.getByRole("link")).toHaveAttribute("href", "/sessions");
  });

  it("renders error state", async () => {
    const { useSessionDetail } = await import("@/hooks");
    vi.mocked(useSessionDetail).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Connection refused"),
    } as any);

    await renderPage();

    expect(screen.getByText("Failed to load session")).toBeInTheDocument();
    expect(screen.getByText("Connection refused")).toBeInTheDocument();
    expect(screen.getByText("Back to Sessions")).toBeInTheDocument();
  });

  it("renders non-Error error as generic message", async () => {
    const { useSessionDetail } = await import("@/hooks");
    vi.mocked(useSessionDetail).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: "string error",
    } as any);

    await renderPage();

    expect(screen.getByText("An unexpected error occurred")).toBeInTheDocument();
  });

  it("renders not found state when session is undefined", async () => {
    const { useSessionDetail } = await import("@/hooks");
    vi.mocked(useSessionDetail).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);

    await renderPage("sess-unknown");

    expect(screen.getByText(/was not found/)).toBeInTheDocument();
    expect(screen.getByText("Back to Sessions")).toBeInTheDocument();
  });

  it("renders session detail with conversation", async () => {
    const { useSessionDetail } = await import("@/hooks");
    vi.mocked(useSessionDetail).mockReturnValue({
      data: mockSession,
      isLoading: false,
      error: null,
    } as any);

    await renderPage();

    expect(screen.getByText(/sess-123/)).toBeInTheDocument();

    // Check agent name
    expect(screen.getByText("support-agent")).toBeInTheDocument();

    // Check tabs
    expect(screen.getByText("Conversation")).toBeInTheDocument();
    expect(screen.getByText("Metrics")).toBeInTheDocument();
    expect(screen.getByText("Metadata")).toBeInTheDocument();

    // Check message content (tool messages are filtered out)
    expect(screen.getByText("Hello, I need help")).toBeInTheDocument();
    expect(screen.getByText("How can I help you?")).toBeInTheDocument();

    // Check export buttons
    expect(screen.getByText("Export MD")).toBeInTheDocument();
    expect(screen.getByText("Export JSON")).toBeInTheDocument();
  });

  it("renders tool call indicator from first-class tool calls", async () => {
    const { useSessionDetail, useSessionToolCalls } = await import("@/hooks");
    vi.mocked(useSessionDetail).mockReturnValue({
      data: mockSession,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionToolCalls).mockReturnValue({
      data: [
        {
          id: "tc1",
          callId: "call-1",
          sessionId: "sess-123",
          name: "search_docs",
          arguments: { query: "help" },
          status: "success" as const,
          durationMs: 250,
          createdAt: new Date(Date.now() - 59 * 60 * 1000).toISOString(),
        },
      ],
      isLoading: false,
      error: null,
    } as any);

    await renderPage();

    expect(screen.getByText("search_docs")).toBeInTheDocument();
    expect(screen.getByText("Success")).toBeInTheDocument();
    expect(screen.getByText("250ms")).toBeInTheDocument();
  });

  it("renders session with active status", async () => {
    const { useSessionDetail } = await import("@/hooks");
    const activeSession = { ...mockSession, status: "active" as const, endedAt: undefined };
    vi.mocked(useSessionDetail).mockReturnValue({
      data: activeSession,
      isLoading: false,
      error: null,
    } as any);

    await renderPage();

    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("renders session without metadata", async () => {
    const { useSessionDetail } = await import("@/hooks");
    const noMetaSession = { ...mockSession, metadata: undefined };
    vi.mocked(useSessionDetail).mockReturnValue({
      data: noMetaSession,
      isLoading: false,
      error: null,
    } as any);

    await renderPage();

    expect(screen.getByText(/sess-123/)).toBeInTheDocument();
  });

  it("filters out duplicate EventBus messages with source=runtime", async () => {
    const { useSessionDetail } = await import("@/hooks");
    const sessionWithDuplicates = {
      ...mockSession,
      messages: [
        { id: "m1", role: "user" as const, content: "Hello", timestamp: new Date().toISOString() },
        { id: "m1-dup", role: "user" as const, content: "Hello", timestamp: new Date().toISOString(), metadata: { source: "runtime", index: "0" } },
        { id: "m2", role: "assistant" as const, content: "Hi there!", timestamp: new Date().toISOString() },
        { id: "m2-dup", role: "assistant" as const, content: "Hi there!", timestamp: new Date().toISOString(), metadata: { source: "runtime", index: "1" } },
      ],
    };
    vi.mocked(useSessionDetail).mockReturnValue({
      data: sessionWithDuplicates,
      isLoading: false,
      error: null,
    } as any);

    await renderPage();

    // Each message should appear once, not twice
    expect(screen.getAllByText("Hello")).toHaveLength(1);
    expect(screen.getAllByText("Hi there!")).toHaveLength(1);
  });

  it("renders eval results badge next to evaluated messages", async () => {
    const { useSessionDetail, useSessionEvalResults } = await import("@/hooks");
    vi.mocked(useSessionDetail).mockReturnValue({
      data: mockSession,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionEvalResults).mockReturnValue({
      data: [
        {
          id: "e1",
          sessionId: "sess-123",
          messageId: "m3",
          agentName: "support-agent",
          namespace: "default",
          promptpackName: "pp-1",
          evalId: "tone-check",
          evalType: "llm_judge",
          trigger: "on_response",
          passed: true,
          score: 0.95,
          source: "in_proc",
          createdAt: new Date().toISOString(),
        },
      ],
      isLoading: false,
      error: null,
    } as any);

    await renderPage();

    expect(screen.getByText("1 eval passed")).toBeInTheDocument();
  });

  it("renders without eval results when data is undefined", async () => {
    const { useSessionDetail, useSessionEvalResults } = await import("@/hooks");
    vi.mocked(useSessionDetail).mockReturnValue({
      data: mockSession,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionEvalResults).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);

    await renderPage();

    // Should still render normally without eval badges
    expect(screen.getByText("Hello, I need help")).toBeInTheDocument();
    expect(screen.queryByTestId("eval-results-badge")).not.toBeInTheDocument();
  });

  describe("windowed message rendering", () => {
    function makeMessages(count: number) {
      return Array.from({ length: count }, (_, i) => ({
        id: `msg-${i}`,
        role: "user" as const,
        content: `Message ${i}`,
        timestamp: new Date(Date.now() - (count - i) * 60000).toISOString(),
      }));
    }

    function makeSessionWithMessages(count: number) {
      return {
        ...mockSession,
        messages: makeMessages(count),
        metrics: { ...mockSession.metrics, messageCount: count },
      };
    }

    it("shows load more button when messages exceed window", async () => {
      const { useSessionDetail } = await import("@/hooks");
      vi.mocked(useSessionDetail).mockReturnValue({
        data: makeSessionWithMessages(60),
        isLoading: false,
        error: null,
      } as any);

      await renderPage();

      // Should show the last 50 messages (Message 10 through Message 59)
      expect(screen.getByText("Message 59")).toBeInTheDocument();
      expect(screen.getByText("Message 10")).toBeInTheDocument();
      expect(screen.queryByText("Message 9")).not.toBeInTheDocument();

      // Should show the load more button with correct count
      expect(screen.getByText("Show earlier messages (10 remaining)")).toBeInTheDocument();
    });

    it("shows all messages when count is within window", async () => {
      const { useSessionDetail } = await import("@/hooks");
      vi.mocked(useSessionDetail).mockReturnValue({
        data: makeSessionWithMessages(30),
        isLoading: false,
        error: null,
      } as any);

      await renderPage();

      // All 30 messages should be visible
      expect(screen.getByText("Message 0")).toBeInTheDocument();
      expect(screen.getByText("Message 29")).toBeInTheDocument();

      // No load more button
      expect(screen.queryByText(/Show earlier messages/)).not.toBeInTheDocument();
    });

    it("loads more messages on button click", async () => {
      const { useSessionDetail } = await import("@/hooks");
      vi.mocked(useSessionDetail).mockReturnValue({
        data: makeSessionWithMessages(70),
        isLoading: false,
        error: null,
      } as any);

      await renderPage();

      // Initially 20 messages are hidden
      expect(screen.queryByText("Message 0")).not.toBeInTheDocument();
      expect(screen.getByText("Show earlier messages (20 remaining)")).toBeInTheDocument();

      // Click the load more button
      const user = userEvent.setup();
      await user.click(screen.getByText("Show earlier messages (20 remaining)"));

      // Now all messages should be visible (50 + 50 = 100 window, only 70 messages)
      expect(screen.getByText("Message 0")).toBeInTheDocument();
      expect(screen.getByText("Message 69")).toBeInTheDocument();

      // No more button needed
      expect(screen.queryByText(/Show earlier messages/)).not.toBeInTheDocument();
    });
  });
});
