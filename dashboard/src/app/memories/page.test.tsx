import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";

const {
  mockUseAuth,
  mockUseMemories,
  mockDeleteMutate,
  mockDeleteAllMutate,
  mockExportMutate,
} = vi.hoisted(() => ({
  mockUseAuth: vi.fn(),
  mockUseMemories: vi.fn(),
  mockDeleteMutate: vi.fn(),
  mockDeleteAllMutate: vi.fn(),
  mockExportMutate: vi.fn(),
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

vi.mock("@/hooks/use-auth", () => ({
  useAuth: mockUseAuth,
}));
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    workspaces: [],
    currentWorkspace: { name: "test-ws" },
    setCurrentWorkspace: vi.fn(),
    isLoading: false,
    error: null,
  }),
}));
vi.mock("@/hooks/use-memories", () => ({
  useMemories: mockUseMemories,
}));
vi.mock("@/hooks/use-memory-mutations", () => ({
  useDeleteMemory: () => ({ mutate: mockDeleteMutate }),
  useDeleteAllMemories: () => ({ mutate: mockDeleteAllMutate }),
  useExportMemories: () => ({ mutate: mockExportMutate, isPending: false }),
}));
vi.mock("@/components/memories/memory-graph", () => ({
  MemoryGraph: ({
    memories,
    onNodeClick,
  }: {
    memories: Array<{ id: string; content: string }>;
    onNodeClick: (m: { id: string; content: string }) => void;
  }) => (
    <div data-testid="memory-graph">
      {memories.map((m) => (
        <button
          key={m.id}
          type="button"
          data-testid={`memory-${m.id}`}
          onClick={() => onNodeClick(m)}
        >
          {m.content}
        </button>
      ))}
    </div>
  ),
}));
vi.mock("@/components/memories/memory-detail-panel", () => ({
  MemoryDetailPanel: ({
    memory,
    onDelete,
  }: {
    memory: { id: string } | null;
    onDelete: (id: string) => void;
  }) =>
    memory ? (
      <div data-testid="detail-panel">
        <button
          type="button"
          data-testid="detail-delete"
          onClick={() => onDelete(memory.id)}
        >
          delete
        </button>
      </div>
    ) : null,
}));
vi.mock("@/hooks/use-consent", () => ({
  useConsent: () => ({
    data: { grants: [], defaults: [], denied: [] },
    isLoading: false,
  }),
  useUpdateConsent: () => ({ mutate: vi.fn(), isPending: false }),
}));

import MemoriesPage from "./page";

function makeMemory(
  id: string,
  content: string,
  category = "memory:identity"
) {
  return {
    id,
    type: "fact",
    content,
    confidence: 0.9,
    scope: { workspace: "ws-1" },
    createdAt: new Date().toISOString(),
    metadata: { consent_category: category },
  };
}

describe("MemoriesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      user: { id: "test-user" },
      isAuthenticated: true,
      hasMemoryIdentity: true,
      memoryUserId: "test-user",
    });
    mockUseMemories.mockReturnValue({
      data: { memories: [], total: 0 },
      isLoading: false,
      error: null,
    });
  });

  it("renders empty state when no memories", () => {
    render(<MemoriesPage />);
    expect(screen.getByTestId("empty-state")).toBeTruthy();
  });

  it("renders toolbar", () => {
    render(<MemoriesPage />);
    expect(screen.getByTestId("memories-toolbar")).toBeTruthy();
  });

  it("renders consent banner", () => {
    render(<MemoriesPage />);
    expect(screen.getByTestId("consent-banner")).toBeTruthy();
  });

  it("renders the graph when memories exist", () => {
    const memories = [
      makeMemory("m1", "likes dark mode", "memory:preferences"),
      makeMemory("m2", "based in Denver", "memory:location"),
    ];
    mockUseMemories.mockReturnValue({
      data: { memories, total: 2 },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    expect(screen.getByTestId("memory-graph")).toBeTruthy();
    expect(screen.getByText("Showing 2 of 2 memories")).toBeTruthy();
  });

  it("renders a skeleton while loading", () => {
    mockUseMemories.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });
    const { container } = render(<MemoriesPage />);
    expect(container.querySelector(".rounded-lg")).toBeTruthy();
  });

  it("shows an error alert when the query fails", () => {
    mockUseMemories.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("boom"),
    });
    render(<MemoriesPage />);
    expect(screen.getByTestId("memory-error")).toBeTruthy();
    expect(screen.getByText(/boom/)).toBeTruthy();
  });

  it("filters memories by search query", () => {
    mockUseMemories.mockReturnValue({
      data: {
        memories: [
          makeMemory("m1", "likes dark mode"),
          makeMemory("m2", "based in Denver"),
        ],
        total: 2,
      },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    fireEvent.change(screen.getByTestId("memory-search"), {
      target: { value: "denver" },
    });
    expect(screen.queryByText("likes dark mode")).toBeNull();
    expect(screen.getByText("based in Denver")).toBeTruthy();
  });

  it("invokes the export mutation when Export is clicked", async () => {
    const user = userEvent.setup();
    render(<MemoriesPage />);
    await user.click(screen.getByRole("button", { name: /export/i }));
    expect(mockExportMutate).toHaveBeenCalled();
  });

  it("invokes deleteAll when Forget Everything is confirmed", async () => {
    const user = userEvent.setup();
    render(<MemoriesPage />);
    await user.click(screen.getByTestId("forget-all-button"));
    await user.click(screen.getByTestId("confirm-forget-all"));
    expect(mockDeleteAllMutate).toHaveBeenCalled();
  });

  it("invokes deleteMemory when a memory is deleted from the detail panel", async () => {
    const user = userEvent.setup();
    mockUseMemories.mockReturnValue({
      data: { memories: [makeMemory("m1", "content")], total: 1 },
      isLoading: false,
      error: null,
    });
    render(<MemoriesPage />);
    await user.click(screen.getByTestId("memory-m1"));
    await user.click(screen.getByTestId("detail-delete"));
    expect(mockDeleteMutate).toHaveBeenCalledWith("m1");
  });

  describe("anonymous user without device ID", () => {
    beforeEach(() => {
      mockUseAuth.mockReturnValue({
        user: { id: "anon", provider: "anonymous" },
        isAuthenticated: false,
        hasMemoryIdentity: false,
        memoryUserId: undefined,
      });
    });

    it("shows the anonymous notice instead of the empty state", () => {
      render(<MemoriesPage />);
      expect(screen.getByTestId("memory-anonymous-notice")).toBeTruthy();
      expect(screen.queryByTestId("empty-state")).toBeNull();
    });

    it("hides the toolbar", () => {
      render(<MemoriesPage />);
      expect(screen.queryByTestId("memories-toolbar")).toBeNull();
    });

    it("mentions signing in", () => {
      render(<MemoriesPage />);
      expect(screen.getByText(/sign in/i)).toBeTruthy();
    });
  });
});
