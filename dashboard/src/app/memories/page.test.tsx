import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockUseWorkspace, mockUseMemoryProjection, mockDeleteMutate, mockGetItem, mockSetItem } =
  vi.hoisted(() => ({
    mockUseWorkspace: vi.fn(),
    mockUseMemoryProjection: vi.fn(),
    mockDeleteMutate: vi.fn(),
    mockGetItem: vi.fn((_key: string) => null as string | null),
    mockSetItem: vi.fn(),
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
});
