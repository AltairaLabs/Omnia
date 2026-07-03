/**
 * Tests for Sessions list page.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useEffect } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import SessionsPage from "./page";
import type { WorkspaceServicesHealth } from "@/lib/k8s/service-health";

// Mock hooks
vi.mock("@/hooks/agents", () => ({
  useAgents: vi.fn(),
}));
vi.mock("@/hooks/sessions", () => ({
  useSessions: vi.fn(),
  useSessionSearch: vi.fn(),
}));
vi.mock("@/hooks/core", () => ({
  useDebounce: vi.fn((v: string) => v),
}));

// Owner-only purge — mutable flag + a stub dialog (the real one is tested
// separately in purge-sessions-dialog.test.tsx).
const mockPermissions = { isOwner: false, isEditor: false };
vi.mock("@/hooks/use-workspace-permissions", () => ({
  useWorkspacePermissions: () => mockPermissions,
}));
vi.mock("@/components/sessions/purge-sessions-dialog", () => ({
  PurgeSessionsDialog: () => <button data-testid="purge-sessions-open">Purge</button>,
}));
// Default stub: simulates the banner resolving to "no culprit found" so the
// pre-existing generic-error tests below (which don't care about the
// banner/alert composition) keep seeing the generic alert. The dedicated
// composition describe block further down unmocks this and exercises the
// real ServiceUnreadyBanner against a mocked `/services` fetch.
vi.mock("@/components/sessions/service-unready-banner", () => ({
  ServiceUnreadyBanner: ({ onResult }: { onResult?: (hasCulprit: boolean) => void }) => {
    useEffect(() => {
      onResult?.(false);
    }, [onResult]);
    return <div data-testid="service-unready-banner" />;
  },
}));
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({ currentWorkspace: { name: "demo-workspace" } }),
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

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
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
    mockPermissions.isOwner = false;
    mockPermissions.isEditor = false;
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

    // The stubbed banner resolves onResult(false) asynchronously (mirrors
    // the real banner's async /services fetch), so the generic alert
    // appears once that resolves.
    await waitFor(() => {
      expect(screen.getByText("Error loading sessions")).toBeInTheDocument();
    });
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

    await waitFor(() => {
      expect(screen.getByText("An unexpected error occurred")).toBeInTheDocument();
    });
  });

  async function setupListData() {
    const { useSessions, useSessionSearch, useAgents } = await import("@/hooks");
    vi.mocked(useSessions).mockReturnValue({
      data: { sessions: mockSessions, total: 2, hasMore: false },
      isLoading: false,
      error: null,
    } as any);
    vi.mocked(useSessionSearch).mockReturnValue({ data: undefined, isLoading: false, error: null } as any);
    vi.mocked(useAgents).mockReturnValue({ data: [] } as any);
  }

  it("hides the Purge action for non-owners", async () => {
    mockPermissions.isOwner = false;
    await setupListData();
    render(<SessionsPage />);
    expect(screen.queryByTestId("purge-sessions-open")).not.toBeInTheDocument();
  });

  it("shows the Purge action for owners", async () => {
    mockPermissions.isOwner = true;
    await setupListData();
    render(<SessionsPage />);
    expect(screen.getByTestId("purge-sessions-open")).toBeInTheDocument();
  });

  // These tests render the REAL ServiceUnreadyBanner (not the stub above) to
  // verify the page-level composition: the culprit banner must REPLACE the
  // generic error alert rather than stack on top of it (see #1690 review
  // finding). vi.resetModules + vi.doUnmock forces a fresh module graph so
  // the real banner component is used for just this block.
  describe("error banner composition (real ServiceUnreadyBanner)", () => {
    const crashloopingHealth: WorkspaceServicesHealth = {
      workspaceServices: [],
      groups: [
        {
          name: "default",
          ready: false,
          members: [
            { service: "memory-api", state: "crashlooping", ready: false, restarts: 5 },
            { service: "session-api", state: "ready", ready: true, restarts: 0 },
          ],
        },
      ],
      source: "crd",
    };

    const healthyHealth: WorkspaceServicesHealth = {
      workspaceServices: [],
      groups: [
        {
          name: "default",
          ready: true,
          members: [
            { service: "memory-api", state: "ready", ready: true, restarts: 0 },
            { service: "session-api", state: "ready", ready: true, restarts: 0 },
          ],
        },
      ],
      source: "crd",
    };

    beforeEach(() => {
      vi.resetModules();
      vi.doUnmock("@/components/sessions/service-unready-banner");
    });

    afterEach(() => {
      // Restore the default stub for every other test in this file.
      vi.doMock("@/components/sessions/service-unready-banner", () => ({
        ServiceUnreadyBanner: ({ onResult }: { onResult?: (hasCulprit: boolean) => void }) => {
          useEffect(() => {
            onResult?.(false);
          }, [onResult]);
          return <div data-testid="service-unready-banner" />;
        },
      }));
    });

    async function renderWithError(health: WorkspaceServicesHealth) {
      global.fetch = vi.fn(() =>
        Promise.resolve({ ok: true, json: () => Promise.resolve(health) })
      ) as unknown as typeof fetch;

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

      const { default: SessionsPageReal } = await import("./page");
      render(<SessionsPageReal />);
    }

    it("shows only the culprit banner — the generic alert is suppressed", async () => {
      await renderWithError(crashloopingHealth);

      // No flash of the generic (misleading) alert while /services is
      // still pending, right after the initial synchronous render.
      expect(screen.queryByText("Error loading sessions")).not.toBeInTheDocument();

      await waitFor(() => {
        expect(screen.getByText(/memory-api unhealthy/i)).toBeInTheDocument();
      });

      expect(screen.queryByText("Error loading sessions")).not.toBeInTheDocument();
      expect(screen.getAllByRole("alert")).toHaveLength(1);
    });

    it("shows only the generic alert when all services are healthy", async () => {
      await renderWithError(healthyHealth);

      await waitFor(() => {
        expect(screen.getByText("Error loading sessions")).toBeInTheDocument();
      });

      expect(screen.queryByText(/unhealthy/i)).not.toBeInTheDocument();
      expect(screen.getAllByRole("alert")).toHaveLength(1);
    });
  });
});
