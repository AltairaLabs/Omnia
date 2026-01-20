import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useAgents, useAgent } from "./use-agents";

// Mock workspace context
const mockCurrentWorkspace = {
  name: "test-workspace",
  namespace: "test-namespace",
  role: "editor",
};

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: mockCurrentWorkspace,
    workspaces: [mockCurrentWorkspace],
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

// Mock agent data
const mockAgents = [
  {
    metadata: {
      name: "agent-1",
      namespace: "test-namespace",
      uid: "uid-1",
    },
    spec: {
      model: "gpt-4",
    },
    status: {
      phase: "Running",
    },
  },
  {
    metadata: {
      name: "agent-2",
      namespace: "test-namespace",
      uid: "uid-2",
    },
    spec: {
      model: "claude-3",
    },
    status: {
      phase: "Pending",
    },
  },
  {
    metadata: {
      name: "agent-3",
      namespace: "test-namespace",
      uid: "uid-3",
    },
    spec: {
      model: "llama2",
    },
    status: {
      phase: "Running",
    },
  },
];

// Mock useDataService
const mockGetAgents = vi.fn();
const mockGetAgent = vi.fn();
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getAgents: mockGetAgents,
    getAgent: mockGetAgent,
  }),
}));

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
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useAgents", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetAgents.mockResolvedValue(mockAgents);
  });

  it("fetches agents for the current workspace", async () => {
    const { result } = renderHook(() => useAgents(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetAgents).toHaveBeenCalledWith("test-workspace");
    expect(result.current.data).toHaveLength(3);
  });

  it("filters agents by phase when specified", async () => {
    const { result } = renderHook(() => useAgents({ phase: "Running" }), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toHaveLength(2);
    expect(result.current.data?.every((a) => a.status?.phase === "Running")).toBe(
      true
    );
  });

  it("filters to empty array when no agents match phase", async () => {
    mockGetAgents.mockResolvedValue(mockAgents);

    const { result } = renderHook(() => useAgents({ phase: "Failed" }), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // No agents have "Failed" phase
    expect(result.current.data).toHaveLength(0);
  });

  it("handles API errors gracefully", async () => {
    mockGetAgents.mockRejectedValue(new Error("API Error"));

    const { result } = renderHook(() => useAgents(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error?.message).toBe("API Error");
  });
});

describe("useAgent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetAgent.mockResolvedValue(mockAgents[0]);
  });

  it("fetches a single agent by name", async () => {
    const { result } = renderHook(() => useAgent("agent-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetAgent).toHaveBeenCalledWith("test-workspace", "agent-1");
    expect(result.current.data?.metadata.name).toBe("agent-1");
  });

  it("returns null when agent not found", async () => {
    mockGetAgent.mockResolvedValue(undefined);

    const { result } = renderHook(() => useAgent("nonexistent"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBeNull();
  });

  it("does not fetch when name is empty", async () => {
    const { result } = renderHook(() => useAgent(""), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGetAgent).not.toHaveBeenCalled();
  });

  it("handles deprecated namespace parameter", async () => {
    const { result } = renderHook(() => useAgent("agent-1", "old-namespace"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // Should use current workspace, not the deprecated namespace parameter
    expect(mockGetAgent).toHaveBeenCalledWith("test-workspace", "agent-1");
  });

  it("handles API errors gracefully", async () => {
    mockGetAgent.mockRejectedValue(new Error("Not found"));

    const { result } = renderHook(() => useAgent("agent-1"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error?.message).toBe("Not found");
  });
});
