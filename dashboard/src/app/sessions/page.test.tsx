/**
 * Tests for Sessions list page.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import SessionsPage from "./page";

// Mock hooks
vi.mock("@/hooks", () => ({
  useSessions: vi.fn(),
  useSessionSearch: vi.fn(),
  useAgents: vi.fn(),
  useDebounce: vi.fn((v: string) => v),
}));

// Mock layout components that require providers
vi.mock("@/components/layout", () => ({
  Header: function MockHeader({ title, description }: { title: string; description: string }) {
    return (
      <div data-testid="header">
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
    );
  },
}));

// Mock next/link
vi.mock("next/link", () => ({
  default: function MockLink({ children, href }: { children: React.ReactNode; href: string }) {
    return <a href={href}>{children}</a>;
  },
}));

const mockSessions = [
  {
    id: "sess-1",
    agentName: "support-agent",
    agentNamespace: "default",
    status: "active" as const,
    startedAt: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
    messageCount: 10,
    toolCallCount: 3,
    totalTokens: 5000,
    lastMessage: "How can I help you?",
  },
  {
    id: "sess-2",
    agentName: "code-agent",
    agentNamespace: "default",
    status: "completed" as const,
    startedAt: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
    messageCount: 20,
    toolCallCount: 8,
    totalTokens: 12000,
    lastMessage: "Task completed successfully",
  },
];

describe("SessionsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders loading skeletons when loading", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: undefined } as any);

    render(<SessionsPage />);

    expect(screen.getByText("Sessions")).toBeInTheDocument();
    expect(screen.getByText("Browse and replay agent conversations")).toBeInTheDocument();
  });

  it("renders sessions data in table", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: { sessions: mockSessions, total: 2, hasMore: false },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({
      data: [{ metadata: { name: "support-agent" } }, { metadata: { name: "code-agent" } }],
    } as any);

    render(<SessionsPage />);

    // Check session IDs
    expect(screen.getByText("sess-1")).toBeInTheDocument();
    expect(screen.getByText("sess-2")).toBeInTheDocument();

    // Check agent names
    expect(screen.getByText("support-agent")).toBeInTheDocument();
    expect(screen.getByText("code-agent")).toBeInTheDocument();

    // Check status badges
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("Completed")).toBeInTheDocument();

    // Check stats
    expect(screen.getByText("Total Sessions")).toBeInTheDocument();
    expect(screen.getByText("Active Now")).toBeInTheDocument();
    expect(screen.getByText("Total Tokens")).toBeInTheDocument();
    expect(screen.getByText("Tool Calls")).toBeInTheDocument();
  });

  it("renders error state", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch sessions"),
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: undefined } as any);

    render(<SessionsPage />);

    expect(screen.getByText("Error loading sessions")).toBeInTheDocument();
    expect(screen.getByText("Failed to fetch sessions")).toBeInTheDocument();
  });

  it("renders empty state when no sessions", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: { sessions: [], total: 0, hasMore: false },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: [] } as any);

    render(<SessionsPage />);

    expect(screen.getByText("No sessions found")).toBeInTheDocument();
  });

  it("renders pagination controls when there are more pages", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: { sessions: mockSessions, total: 50, hasMore: true },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: [] } as any);

    render(<SessionsPage />);

    expect(screen.getByText("Previous")).toBeInTheDocument();
    expect(screen.getByText("Next")).toBeInTheDocument();
    expect(screen.getByText(/Showing 1–20 of 50 sessions/)).toBeInTheDocument();
  });

  it("renders filter dropdowns", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: { sessions: [], total: 0, hasMore: false },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: [] } as any);

    render(<SessionsPage />);

    expect(screen.getByPlaceholderText("Search sessions...")).toBeInTheDocument();
  });

  it("renders stats cards with correct values", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: { sessions: mockSessions, total: 2, hasMore: false },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: [] } as any);

    render(<SessionsPage />);

    // Total sessions from API
    expect(screen.getByText("2")).toBeInTheDocument();
    // Active count (only sess-1 is active)
    expect(screen.getByText("1")).toBeInTheDocument();
    // Total tokens (5000 + 12000 = 17000)
    expect(screen.getByText("17,000")).toBeInTheDocument();
    // Tool calls (3 + 8 = 11)
    expect(screen.getByText("11")).toBeInTheDocument();
  });

  it("renders status badges for all session statuses", async () => {
    const allStatusSessions = [
      { ...mockSessions[0], id: "s1", status: "active" as const },
      { ...mockSessions[0], id: "s2", status: "completed" as const },
      { ...mockSessions[0], id: "s3", status: "error" as const },
      { ...mockSessions[0], id: "s4", status: "expired" as const },
    ];

    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: { sessions: allStatusSessions, total: 4, hasMore: false },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: [] } as any);

    render(<SessionsPage />);

    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("Completed")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.getByText("Expired")).toBeInTheDocument();
  });

  it("renders table column headers", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: { sessions: [], total: 0, hasMore: false },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: [] } as any);

    render(<SessionsPage />);

    expect(screen.getByText("Session ID")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("Started")).toBeInTheDocument();
    expect(screen.getByText("Messages")).toBeInTheDocument();
    expect(screen.getByText("Tools")).toBeInTheDocument();
    expect(screen.getByText("Tokens")).toBeInTheDocument();
    expect(screen.getByText("Last Message")).toBeInTheDocument();
  });

  it("renders non-Error error as generic message", async () => {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: "string error",
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useAgents).mockReturnValue({ data: undefined } as any);

    render(<SessionsPage />);

    expect(screen.getByText("An unexpected error occurred")).toBeInTheDocument();
  });
});
