/**
 * Tests for workspace-context.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { WorkspaceProvider, useWorkspace } from "./workspace-context";

// Mock the useWorkspaces hook
vi.mock("@/hooks/use-workspaces", () => ({
  useWorkspaces: vi.fn(),
}));

import { useWorkspaces, type WorkspaceListItem } from "@/hooks/use-workspaces";

const mockWorkspaces: WorkspaceListItem[] = [
  {
    name: "workspace-1",
    displayName: "Workspace One",
    environment: "development",
    namespace: "ns-1",
    role: "owner",
    permissions: { read: true, write: true, delete: true, manageMembers: true },
  },
  {
    name: "workspace-2",
    displayName: "Workspace Two",
    environment: "production",
    namespace: "ns-2",
    role: "viewer",
    permissions: { read: true, write: false, delete: false, manageMembers: false },
  },
];

function TestConsumer() {
  const { workspaces, currentWorkspace, setCurrentWorkspace, isLoading, error } = useWorkspace();

  return (
    <div>
      <div data-testid="loading">{isLoading ? "loading" : "loaded"}</div>
      <div data-testid="error">{error ? error.message : "no-error"}</div>
      <div data-testid="workspaces-count">{workspaces.length}</div>
      <div data-testid="current-workspace">{currentWorkspace?.name || "none"}</div>
      <button onClick={() => setCurrentWorkspace("workspace-2")}>Switch</button>
      <button onClick={() => setCurrentWorkspace(null)}>Clear</button>
    </div>
  );
}

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    );
  };
}

describe("WorkspaceContext", () => {
  const mockLocalStorage: Record<string, string> = {};

  beforeEach(() => {
    vi.clearAllMocks();
    // Mock localStorage
    Object.defineProperty(window, "localStorage", {
      value: {
        getItem: vi.fn((key: string) => mockLocalStorage[key] || null),
        setItem: vi.fn((key: string, value: string) => {
          mockLocalStorage[key] = value;
        }),
        removeItem: vi.fn((key: string) => {
          delete mockLocalStorage[key];
        }),
      },
      writable: true,
    });
  });

  afterEach(() => {
    for (const key in mockLocalStorage) {
      delete mockLocalStorage[key];
    }
  });

  it("provides workspaces data from useWorkspaces hook", async () => {
    vi.mocked(useWorkspaces).mockReturnValue({
      data: mockWorkspaces,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useWorkspaces>);

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <WorkspaceProvider>
          <TestConsumer />
        </WorkspaceProvider>
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId("workspaces-count").textContent).toBe("2");
    });
  });

  it("shows loading state", () => {
    vi.mocked(useWorkspaces).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useWorkspaces>);

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <WorkspaceProvider>
          <TestConsumer />
        </WorkspaceProvider>
      </Wrapper>
    );

    expect(screen.getByTestId("loading").textContent).toBe("loading");
  });

  it("shows error state", () => {
    const mockError = new Error("Failed to fetch");
    vi.mocked(useWorkspaces).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: mockError,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useWorkspaces>);

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <WorkspaceProvider>
          <TestConsumer />
        </WorkspaceProvider>
      </Wrapper>
    );

    expect(screen.getByTestId("error").textContent).toBe("Failed to fetch");
  });

  it("auto-selects first workspace when none selected", async () => {
    vi.mocked(useWorkspaces).mockReturnValue({
      data: mockWorkspaces,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useWorkspaces>);

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <WorkspaceProvider>
          <TestConsumer />
        </WorkspaceProvider>
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId("current-workspace").textContent).toBe("workspace-1");
    });
  });

  it("allows switching workspaces", async () => {
    vi.mocked(useWorkspaces).mockReturnValue({
      data: mockWorkspaces,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useWorkspaces>);

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <WorkspaceProvider>
          <TestConsumer />
        </WorkspaceProvider>
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId("current-workspace").textContent).toBe("workspace-1");
    });

    act(() => {
      screen.getByText("Switch").click();
    });

    await waitFor(() => {
      expect(screen.getByTestId("current-workspace").textContent).toBe("workspace-2");
    });
  });

  it("persists workspace selection to localStorage", async () => {
    vi.mocked(useWorkspaces).mockReturnValue({
      data: mockWorkspaces,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useWorkspaces>);

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <WorkspaceProvider>
          <TestConsumer />
        </WorkspaceProvider>
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId("current-workspace").textContent).toBe("workspace-1");
    });

    act(() => {
      screen.getByText("Switch").click();
    });

    await waitFor(() => {
      expect(window.localStorage.setItem).toHaveBeenCalledWith(
        "omnia-selected-workspace",
        "workspace-2"
      );
    });
  });

  it("clears workspace selection from localStorage", async () => {
    vi.mocked(useWorkspaces).mockReturnValue({
      data: mockWorkspaces,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useWorkspaces>);

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <WorkspaceProvider>
          <TestConsumer />
        </WorkspaceProvider>
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId("current-workspace").textContent).toBe("workspace-1");
    });

    act(() => {
      screen.getByText("Clear").click();
    });

    await waitFor(() => {
      expect(window.localStorage.removeItem).toHaveBeenCalledWith(
        "omnia-selected-workspace"
      );
    });
  });

  it("throws error when useWorkspace is used outside provider", () => {
    // Suppress console.error for this test
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    expect(() => {
      render(<TestConsumer />);
    }).toThrow("useWorkspace must be used within a WorkspaceProvider");

    consoleSpy.mockRestore();
  });

  it("returns null for currentWorkspace when no workspaces exist", async () => {
    vi.mocked(useWorkspaces).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    } as unknown as ReturnType<typeof useWorkspaces>);

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <WorkspaceProvider>
          <TestConsumer />
        </WorkspaceProvider>
      </Wrapper>
    );

    await waitFor(() => {
      expect(screen.getByTestId("current-workspace").textContent).toBe("none");
    });
  });
});
