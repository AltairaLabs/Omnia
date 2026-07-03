import React, { useEffect } from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import type { WorkspaceServicesHealth } from "@/lib/k8s/service-health";

const { mockUseWorkspace, mockUseMemoryProjection, mockDeleteMutate, mockGetItem, mockSetItem, mockUseEnterpriseConfig } =
  vi.hoisted(() => ({
    mockUseWorkspace: vi.fn(),
    mockUseMemoryProjection: vi.fn(),
    mockDeleteMutate: vi.fn(),
    mockGetItem: vi.fn((_key: string) => null as string | null),
    mockSetItem: vi.fn(),
    mockUseEnterpriseConfig: vi.fn(),
  }));

// Mock layout components that pull in complex infrastructure (WorkspaceSwitcher, UserMenu)
vi.mock("@/components/layout", () => ({
  Header: ({
    title,
    description,
  }: {
    title: string;
    description?: string;
  }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </div>
  ),
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => mockUseWorkspace(),
}));
vi.mock("@/hooks/use-memory-projection", () => ({
  useMemoryProjection: mockUseMemoryProjection,
}));
vi.mock("@/hooks/use-memory-mutations", () => ({
  useDeleteMemory: () => ({ mutate: mockDeleteMutate }),
}));

// usePersistedViewMode reads/writes localStorage; stub it out
vi.mock("@/hooks/use-persisted-view-mode", () => ({
  usePersistedViewMode: (key: string, defaultValue: string) => {
    const value = mockGetItem(key) ?? defaultValue;
    return [value, mockSetItem];
  },
}));

// next/dynamic with ssr:false skips rendering in vitest (jsdom has no browser renderer).
// Override it to render the MemoryGalaxy stub synchronously.
vi.mock("next/dynamic", () => ({
  default: (_importFn: unknown, _opts: unknown) => {
    // Inline stub: identical shape to the vi.mock below for memory-galaxy
    const Stub = ({
      points,
      onDelete,
    }: {
      points: Array<{ id: string; title?: string; preview?: string }>;
      onDelete: (id: string) => void;
    }) => (
      <div data-testid="memory-galaxy">
        {points.map((p) => (
          <button
            key={p.id}
            type="button"
            data-testid={`galaxy-point-${p.id}`}
            onClick={() => onDelete(p.id)}
          >
            {p.title ?? p.preview}
          </button>
        ))}
      </div>
    );
    Stub.displayName = "DynamicMemoryGalaxy";
    return Stub;
  },
}));

// MemoryGalaxy is a canvas component — render a stub
vi.mock("@/components/memories/memory-galaxy", () => ({
  MemoryGalaxy: ({
    points,
    onDelete,
  }: {
    points: Array<{ id: string; title?: string; preview?: string }>;
    onDelete: (id: string) => void;
  }) => (
    <div data-testid="memory-galaxy">
      {points.map((p) => (
        <button
          key={p.id}
          type="button"
          data-testid={`galaxy-point-${p.id}`}
          onClick={() => onDelete(p.id)}
        >
          {p.title ?? p.preview}
        </button>
      ))}
    </div>
  ),
}));

// EnterpriseGate uses useEnterpriseConfig — mock @/hooks/core so we can
// control enterpriseEnabled per-test (default: enabled so existing tests pass)
vi.mock("@/hooks/core", () => ({
  useEnterpriseConfig: () => mockUseEnterpriseConfig(),
}));

// FacetRail stub
vi.mock("@/components/memories/facet-rail", () => ({
  FacetRail: ({ onToggle }: { onToggle: (key: string) => void }) => (
    <div data-testid="facet-rail">
      <button type="button" data-testid="facet-toggle-user" onClick={() => onToggle("user")}>
        user
      </button>
    </div>
  ),
}));

// Default stub: simulates the banner resolving to "no culprit found" so
// tests that don't care about the banner/culprit composition (the bulk of
// this file) keep seeing the pre-existing error/loading states. The
// dedicated "proactive service banner" describe block further down
// unmocks this and exercises the real ServiceUnreadyBanner against a
// mocked `/services` fetch.
vi.mock("@/components/sessions/service-unready-banner", () => ({
  ServiceUnreadyBanner: ({ onResult }: { onResult?: (hasCulprit: boolean) => void }) => {
    useEffect(() => {
      onResult?.(false);
    }, [onResult]);
    return <div data-testid="service-unready-banner" />;
  },
}));

import MemoriesPage from "./page";

function makePoint(
  id: string,
  title = "test point",
  category = "memory:identity",
): {
  id: string;
  x: number;
  y: number;
  tier: "user";
  category: string;
  confidence: number;
  title: string;
  preview: string;
  observedAt: string;
} {
  return {
    id,
    x: 0,
    y: 0,
    tier: "user",
    category,
    confidence: 0.9,
    title,
    preview: `${title} preview`,
    observedAt: new Date().toISOString(),
  };
}

describe("MemoriesPage (galaxy)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseEnterpriseConfig.mockReturnValue({ enterpriseEnabled: true, hideEnterprise: false, loading: false });
    mockUseWorkspace.mockReturnValue({ currentWorkspace: { name: "test-ws" } });
    mockUseMemoryProjection.mockReturnValue({
      data: { points: [], total: 0, capped: false, model: "tsne", embeddingModel: "text-embedding-3-small", embeddingDim: 1536, computedAt: new Date().toISOString() },
      isLoading: false,
      error: null,
    });
  });

  it("renders empty state when no points", () => {
    render(<MemoriesPage />);
    expect(screen.getByTestId("empty-state")).toBeTruthy();
  });

  it("renders the building-galaxy state while the projection is pending", () => {
    mockUseMemoryProjection.mockReturnValue({
      data: {
        points: [],
        total: 1240,
        capped: false,
        model: "tsne",
        embeddingModel: "text-embedding-3-small",
        embeddingDim: 1536,
        computedAt: new Date().toISOString(),
        status: "pending",
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    // Pending shows progress (with the live count), NOT the misleading "empty" state.
    expect(screen.getByTestId("galaxy-pending")).toBeTruthy();
    expect(screen.getByText(/Building galaxy/i)).toBeTruthy();
    expect(screen.getByText(/1,240 memories/)).toBeTruthy();
    expect(screen.queryByTestId("empty-state")).toBeNull();
    expect(screen.queryByTestId("memory-galaxy")).toBeNull();
  });

  it("renders toolbar when authenticated", () => {
    render(<MemoriesPage />);
    expect(screen.getByTestId("memories-toolbar")).toBeTruthy();
  });

  it("renders the galaxy and tier rail when points exist", () => {
    mockUseMemoryProjection.mockReturnValue({
      data: {
        points: [makePoint("p1", "dark mode"), makePoint("p2", "Denver", "memory:location")],
        total: 2,
        capped: false,
        model: "tsne",
        embeddingModel: "text-embedding-3-small",
        embeddingDim: 1536,
        computedAt: new Date().toISOString(),
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    expect(screen.getByTestId("memory-galaxy")).toBeTruthy();
    expect(screen.getByTestId("facet-rail")).toBeTruthy();
    expect(screen.getByText(/2 memories/)).toBeTruthy();
  });

  it("shows a loading skeleton while fetching", () => {
    mockUseMemoryProjection.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });
    render(<MemoriesPage />);
    expect(screen.getByTestId("galaxy-loading")).toBeTruthy();
  });

  it("shows an error alert when the query fails", () => {
    mockUseMemoryProjection.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("projection failed"),
    });
    render(<MemoriesPage />);
    expect(screen.getByTestId("memory-error")).toBeTruthy();
    expect(screen.getByText(/projection failed/)).toBeTruthy();
  });

  it("invokes deleteMemory when the galaxy requests a delete", async () => {
    const user = userEvent.setup();
    mockUseMemoryProjection.mockReturnValue({
      data: {
        points: [makePoint("p1", "remembers stuff")],
        total: 1,
        capped: false,
        model: "tsne",
        embeddingModel: "text-embedding-3-small",
        embeddingDim: 1536,
        computedAt: new Date().toISOString(),
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    await user.click(screen.getByTestId("galaxy-point-p1"));
    expect(mockDeleteMutate).toHaveBeenCalledWith("p1");
  });

  describe("without a workspace", () => {
    beforeEach(() => {
      mockUseWorkspace.mockReturnValue({ currentWorkspace: null });
    });

    it("shows the no-workspace notice instead of the empty state", () => {
      render(<MemoriesPage />);
      expect(screen.getByTestId("no-workspace-notice")).toBeTruthy();
      expect(screen.queryByTestId("empty-state")).toBeNull();
    });

    it("hides the toolbar and tier rail", () => {
      render(<MemoriesPage />);
      expect(screen.queryByTestId("memories-toolbar")).toBeNull();
      expect(screen.queryByTestId("facet-rail")).toBeNull();
    });
  });

  it("renders the galaxy for an anonymous session that has a workspace", () => {
    // No auth identity is involved at all — only a workspace is required.
    mockUseMemoryProjection.mockReturnValue({
      data: {
        points: [makePoint("p1", "anon-visible")],
        total: 1,
        capped: false,
        model: "tsne",
        embeddingModel: "text-embedding-3-small",
        embeddingDim: 1536,
        computedAt: new Date().toISOString(),
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    expect(screen.getByTestId("memory-galaxy")).toBeTruthy();
    expect(screen.queryByTestId("no-workspace-notice")).toBeNull();
  });

  it("shows semantic clustering hint by default", () => {
    mockUseMemoryProjection.mockReturnValue({
      data: {
        points: [makePoint("p1")],
        total: 1,
        capped: false,
        model: "tsne",
        projectionInput: "embedding",
        embeddingModel: "text-embedding-3-small",
        embeddingDim: 1536,
        computedAt: new Date().toISOString(),
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    expect(screen.getByText(/semantic clustering/)).toBeTruthy();
  });

  it("shows lexical clustering hint when projectionInput is tfidf", () => {
    mockUseMemoryProjection.mockReturnValue({
      data: {
        points: [makePoint("p1")],
        total: 1,
        capped: false,
        model: "tsne",
        projectionInput: "tfidf",
        embeddingModel: "bm25",
        embeddingDim: 0,
        computedAt: new Date().toISOString(),
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    expect(screen.getByText(/lexical clustering/)).toBeTruthy();
  });

  it("shows capped notice when response is capped", () => {
    mockUseMemoryProjection.mockReturnValue({
      data: {
        points: [makePoint("p1")],
        total: 1000,
        capped: true,
        model: "tsne",
        embeddingModel: "text-embedding-3-small",
        embeddingDim: 1536,
        computedAt: new Date().toISOString(),
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    expect(screen.getByText(/showing a capped subset/)).toBeTruthy();
  });

  it("filters the search input is rendered and interactive", () => {
    render(<MemoriesPage />);
    const input = screen.getByTestId("memory-search");
    fireEvent.change(input, { target: { value: "test" } });
    expect((input as HTMLInputElement).value).toBe("test");
  });

  it("shows a match count in the footer when a search is active", () => {
    mockUseMemoryProjection.mockReturnValue({
      data: {
        points: [makePoint("p1", "billing dispute"), makePoint("p2", "rate limit")],
        total: 2,
        capped: false,
        model: "tsne",
        embeddingModel: "text-embedding-3-small",
        embeddingDim: 1536,
        computedAt: new Date().toISOString(),
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    fireEvent.change(screen.getByTestId("memory-search"), { target: { value: "billing" } });
    expect(screen.getByText(/1 match\b/)).toBeInTheDocument();
  });

  it("renders the upgrade gate when enterprise is disabled", () => {
    mockUseEnterpriseConfig.mockReturnValue({ enterpriseEnabled: false, hideEnterprise: false, loading: false });
    render(<MemoriesPage />);
    expect(screen.getByText("Enterprise Feature")).toBeInTheDocument();
    expect(screen.queryByText("Memory Galaxy")).not.toBeInTheDocument();
  });

  it("renders the galaxy when enterprise is enabled", () => {
    mockUseEnterpriseConfig.mockReturnValue({ enterpriseEnabled: true, hideEnterprise: false, loading: false });
    render(<MemoriesPage />);
    expect(screen.getByText("Memory Galaxy")).toBeInTheDocument();
  });

  // These tests render the REAL ServiceUnreadyBanner (not the stub above)
  // to verify the proactive-render composition: while the projection is
  // still loading, a hung memory-api never surfaces an error on its own,
  // so the banner must be checked eagerly — and once it names a culprit,
  // the loading skeleton (which would otherwise spin forever) must be
  // suppressed in favor of the banner. vi.resetModules + vi.doUnmock
  // forces a fresh module graph so the real banner component is used for
  // just this block.
  describe("proactive service-unready banner", () => {
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

    beforeEach(() => {
      vi.resetModules();
      vi.doUnmock("@/components/sessions/service-unready-banner");
    });

    afterEach(() => {
      vi.doMock("@/components/sessions/service-unready-banner", () => ({
        ServiceUnreadyBanner: ({ onResult }: { onResult?: (hasCulprit: boolean) => void }) => {
          useEffect(() => {
            onResult?.(false);
          }, [onResult]);
          return <div data-testid="service-unready-banner" />;
        },
      }));
    });

    it("shows the culprit banner and suppresses the endless loading skeleton", async () => {
      global.fetch = vi.fn(() =>
        Promise.resolve({ ok: true, json: () => Promise.resolve(crashloopingHealth) })
      ) as unknown as typeof fetch;

      mockUseMemoryProjection.mockReturnValue({
        data: undefined,
        isLoading: true,
        error: null,
      });

      const { default: MemoriesPageReal } = await import("./page");
      render(<MemoriesPageReal />);

      // No endless spinner while /services is still pending, right after
      // the initial synchronous render.
      expect(screen.getByTestId("galaxy-loading")).toBeTruthy();

      await waitFor(() => {
        expect(screen.getByText(/memory-api unhealthy/i)).toBeInTheDocument();
      });

      expect(screen.queryByTestId("galaxy-loading")).toBeNull();
    });
  });
});
