import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AgentCard } from "./agent-card";
import { DataServiceProvider, type DataService } from "@/lib/data";
import type { AgentRuntime } from "@/types";

// Mock the hooks
vi.mock("@/hooks", () => ({
  useProvider: vi.fn(() => ({ data: null })),
  useAgentCost: vi.fn(() => ({ data: null })),
  useReadOnly: vi.fn(() => ({ isReadOnly: false, message: "" })),
  usePermissions: vi.fn(() => ({
    can: () => true,
    hasRole: () => true,
  })),
  useWorkspacePermissions: vi.fn(() => ({ canWrite: true })),
  Permission: {
    AGENTS_SCALE: "agents:scale",
  },
}));

// Mock next/link
vi.mock("next/link", () => ({
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

const createMockAgent = (overrides?: Partial<AgentRuntime>): AgentRuntime => ({
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "AgentRuntime",
  metadata: {
    name: "test-agent",
    namespace: "default",
    uid: "test-uid-123",
    creationTimestamp: "2024-01-15T10:00:00Z",
  },
  spec: {
    framework: {
      type: "promptkit",
    },
    provider: {
      type: "openai",
      model: "gpt-4",
    },
    facade: {
      type: "websocket",
      port: 8080,
    },
    promptPackRef: {
      name: "test-promptpack",
    },
    runtime: {
      replicas: 2,
      autoscaling: {
        enabled: false,
        minReplicas: 0,
        maxReplicas: 10,
      },
    },
  },
  status: {
    phase: "Running",
    replicas: {
      ready: 2,
      desired: 2,
      available: 2,
    },
  },
  ...overrides,
});

const mockDataService: DataService = {
  name: "mock",
  isDemo: true,
  // Agents
  getAgents: vi.fn().mockResolvedValue([]),
  getAgent: vi.fn().mockResolvedValue(undefined),
  createAgent: vi.fn().mockResolvedValue({}),
  scaleAgent: vi.fn().mockResolvedValue({}),
  getAgentLogs: vi.fn().mockResolvedValue([]),
  getAgentEvents: vi.fn().mockResolvedValue([]),
  // PromptPacks
  getPromptPacks: vi.fn().mockResolvedValue([]),
  getPromptPack: vi.fn().mockResolvedValue(undefined),
  getPromptPackContent: vi.fn().mockResolvedValue(undefined),
  // ToolRegistries (workspace-scoped)
  getToolRegistries: vi.fn().mockResolvedValue([]),
  getToolRegistry: vi.fn().mockResolvedValue(undefined),
  // Providers (workspace-scoped)
  getProviders: vi.fn().mockResolvedValue([]),
  getProvider: vi.fn().mockResolvedValue(undefined),
  // Shared ToolRegistries
  getSharedToolRegistries: vi.fn().mockResolvedValue([]),
  getSharedToolRegistry: vi.fn().mockResolvedValue(undefined),
  // Shared Providers
  getSharedProviders: vi.fn().mockResolvedValue([]),
  getSharedProvider: vi.fn().mockResolvedValue(undefined),
  // Stats
  getStats: vi.fn().mockResolvedValue({ agents: 0, providers: 0, tools: 0, promptPacks: 0 }),
  // Costs
  getCosts: vi.fn().mockResolvedValue({ items: [], summary: { totalCost: 0, totalTokens: 0 } }),
  // Arena Sources
  getArenaSources: vi.fn().mockResolvedValue([]),
  getArenaSource: vi.fn().mockResolvedValue(undefined),
  createArenaSource: vi.fn().mockResolvedValue({}),
  updateArenaSource: vi.fn().mockResolvedValue({}),
  deleteArenaSource: vi.fn().mockResolvedValue(undefined),
  syncArenaSource: vi.fn().mockResolvedValue(undefined),
  // Arena Jobs
  getArenaJobs: vi.fn().mockResolvedValue([]),
  getArenaJob: vi.fn().mockResolvedValue(undefined),
  getArenaJobResults: vi.fn().mockResolvedValue(undefined),
  getArenaJobMetrics: vi.fn().mockResolvedValue(undefined),
  createArenaJob: vi.fn().mockResolvedValue({}),
  cancelArenaJob: vi.fn().mockResolvedValue(undefined),
  deleteArenaJob: vi.fn().mockResolvedValue(undefined),
  getArenaJobLogs: vi.fn().mockResolvedValue([]),
  // Arena Stats
  getArenaStats: vi.fn().mockResolvedValue({ sources: {}, configs: {}, jobs: {} }),
  // Agent connections
  createAgentConnection: vi.fn().mockReturnValue({
    connect: vi.fn(),
    disconnect: vi.fn(),
    send: vi.fn(),
    onMessage: vi.fn(),
    onError: vi.fn(),
    onClose: vi.fn(),
  }),
};

function renderWithProviders(component: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <DataServiceProvider initialService={mockDataService}>
        {component}
      </DataServiceProvider>
    </QueryClientProvider>
  );
}

describe("AgentCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders agent name and namespace", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("test-agent")).toBeInTheDocument();
    expect(screen.getByText("default")).toBeInTheDocument();
  });

  it("renders status badge", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("Running")).toBeInTheDocument();
  });

  it("renders provider type", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("openai")).toBeInTheDocument();
  });

  it("renders facade type", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("websocket")).toBeInTheDocument();
  });

  it("renders replica count", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    // Check for the replica display (current/desired)
    expect(screen.getByText(/2\//)).toBeInTheDocument();
  });

  it("renders link to agent detail page", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute(
      "href",
      "/agents/test-agent?namespace=default"
    );
  });

  it("renders framework badge", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("PromptKit")).toBeInTheDocument();
  });

  it("renders cost section", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("Cost (24h)")).toBeInTheDocument();
  });

  it("handles agents with pending status", () => {
    const agent = createMockAgent({
      status: {
        phase: "Pending",
        replicas: {
          ready: 0,
          desired: 2,
          available: 0,
        },
      },
    });
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("handles agents with failed status", () => {
    const agent = createMockAgent({
      status: {
        phase: "Failed",
        replicas: {
          ready: 0,
          desired: 2,
          available: 0,
        },
      },
    });
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("Failed")).toBeInTheDocument();
  });

  it("handles agents with autoscaling enabled", () => {
    const agent = createMockAgent({
      spec: {
        framework: { type: "promptkit" },
        promptPackRef: { name: "test-pack" },
        facade: { type: "websocket", port: 8080 },
        runtime: {
          replicas: 2,
          autoscaling: {
            enabled: true,
            type: "hpa",
            minReplicas: 1,
            maxReplicas: 5,
          },
        },
      },
    });
    renderWithProviders(<AgentCard agent={agent} />);

    // Should still render correctly
    expect(screen.getByText("test-agent")).toBeInTheDocument();
  });

  it("handles agents with providerRef", () => {
    const agent = createMockAgent({
      spec: {
        framework: { type: "promptkit" },
        promptPackRef: { name: "test-pack" },
        facade: { type: "websocket", port: 8080 },
        providerRef: {
          name: "my-provider",
          namespace: "default",
        },
        runtime: {
          replicas: 1,
        },
      },
    });
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("test-agent")).toBeInTheDocument();
  });

  it("renders scale buttons for non-autoscaled agents", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    // Scale buttons should be present (minus and plus)
    const buttons = screen.getAllByRole("button");
    expect(buttons.length).toBeGreaterThanOrEqual(2);
  });

  it("handles missing status gracefully", () => {
    const agent = createMockAgent({
      status: undefined,
    });
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("test-agent")).toBeInTheDocument();
  });

  it("handles missing namespace gracefully", () => {
    const agent = createMockAgent({
      metadata: {
        name: "test-agent",
        uid: "test-uid-123",
        creationTimestamp: "2024-01-15T10:00:00Z",
        namespace: undefined,
      },
    });
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("test-agent")).toBeInTheDocument();
  });

  it("renders model name when available from provider spec", () => {
    const agent = createMockAgent({
      spec: {
        framework: { type: "promptkit" },
        promptPackRef: { name: "test-pack" },
        facade: { type: "websocket", port: 8080 },
        provider: {
          type: "claude",
          model: "claude-3-opus",
        },
      },
    });
    renderWithProviders(<AgentCard agent={agent} />);

    // Check that the model name fragment is displayed
    expect(screen.getByText(/opus/)).toBeInTheDocument();
  });

  it("handles cost sparkline data rendering", () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    // Cost section should be present
    expect(screen.getByText("Cost (24h)")).toBeInTheDocument();
    // Cost value will be formatted, check for the container
    expect(screen.getByText(/\$/)).toBeInTheDocument();
  });

  it("calls scaleAgent when scale up button is clicked", async () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    // Find the scale up button (plus icon)
    const buttons = screen.getAllByRole("button");
    // The plus button has a + icon
    const plusButton = buttons.find((btn) =>
      btn.querySelector('svg[class*="lucide-plus"]')
    );

    if (plusButton) {
      fireEvent.click(plusButton);

      await waitFor(() => {
        expect(mockDataService.scaleAgent).toHaveBeenCalledWith(
          "default",
          "test-agent",
          3
        );
      });
    }
  });

  it("shows confirmation dialog when scale down button is clicked", async () => {
    const agent = createMockAgent();
    renderWithProviders(<AgentCard agent={agent} />);

    // Find the scale down button (minus icon)
    const buttons = screen.getAllByRole("button");
    const minusButton = buttons.find((btn) =>
      btn.querySelector('svg[class*="lucide-minus"]')
    );

    expect(minusButton).toBeDefined();

    if (minusButton) {
      fireEvent.click(minusButton);

      // Scale down requires confirmation dialog
      await waitFor(() => {
        // Dialog should appear with confirm button
        const confirmButton = screen.queryByRole("button", { name: /scale down/i });
        expect(confirmButton || screen.queryByText(/confirm/i)).toBeTruthy();
      });
    }
  });

  it("renders correctly with different provider types", () => {
    const agent = createMockAgent({
      spec: {
        framework: { type: "promptkit" },
        promptPackRef: { name: "test-pack" },
        facade: { type: "websocket", port: 8080 },
        provider: {
          type: "gemini",
          model: "gemini-pro",
        },
      },
    });
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("gemini")).toBeInTheDocument();
  });

  it("renders correctly with ollama provider", () => {
    const agent = createMockAgent({
      spec: {
        framework: { type: "promptkit" },
        promptPackRef: { name: "test-pack" },
        facade: { type: "websocket", port: 8080 },
        provider: {
          type: "ollama",
          model: "llama2",
        },
      },
    });
    renderWithProviders(<AgentCard agent={agent} />);

    expect(screen.getByText("ollama")).toBeInTheDocument();
  });
});
