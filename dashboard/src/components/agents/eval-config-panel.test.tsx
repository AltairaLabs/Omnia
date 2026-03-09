import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { EvalConfigPanel } from "./eval-config-panel";
import { DataServiceProvider, type DataService } from "@/lib/data";

// Mock enterprise config
const mockEnterpriseConfig = vi.fn();
vi.mock("@/hooks/use-runtime-config", () => ({
  useEnterpriseConfig: () => mockEnterpriseConfig(),
  useRuntimeConfig: vi.fn(() => ({
    config: { demoMode: true, enterpriseEnabled: true },
    loading: false,
  })),
}));

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({
    currentWorkspace: { name: "test-ws" },
  })),
}));

function createMockDataService(overrides?: Partial<DataService>): DataService {
  return {
    name: "mock",
    isDemo: true,
    getAgents: vi.fn().mockResolvedValue([]),
    getAgent: vi.fn().mockResolvedValue(undefined),
    createAgent: vi.fn().mockResolvedValue({}),
    scaleAgent: vi.fn().mockResolvedValue({}),
    updateAgentEvals: vi.fn().mockResolvedValue({}),
    getAgentLogs: vi.fn().mockResolvedValue([]),
    getAgentEvents: vi.fn().mockResolvedValue([]),
    getPromptPacks: vi.fn().mockResolvedValue([]),
    getPromptPack: vi.fn().mockResolvedValue(undefined),
    getPromptPackContent: vi.fn().mockResolvedValue(undefined),
    getToolRegistries: vi.fn().mockResolvedValue([]),
    getToolRegistry: vi.fn().mockResolvedValue(undefined),
    getProviders: vi.fn().mockResolvedValue([]),
    getProvider: vi.fn().mockResolvedValue(undefined),
    getSharedToolRegistries: vi.fn().mockResolvedValue([]),
    getWorkspaces: vi.fn().mockResolvedValue([]),
    getWorkspace: vi.fn().mockResolvedValue(undefined),
    getSessions: vi.fn().mockResolvedValue({ sessions: [], total: 0 }),
    getSession: vi.fn().mockResolvedValue(undefined),
    getSessionMessages: vi.fn().mockResolvedValue({ messages: [], total: 0 }),
    getSessionEvalResults: vi.fn().mockResolvedValue({ results: [] }),
    getAgentMetrics: vi.fn().mockResolvedValue(undefined),
    getAgentCost: vi.fn().mockResolvedValue(undefined),
    getAgentActivity: vi.fn().mockResolvedValue(undefined),
    createAgentConnection: vi.fn(),
    ...overrides,
  } as unknown as DataService;
}

function renderPanel(
  props: Partial<React.ComponentProps<typeof EvalConfigPanel>> = {},
  dataService?: DataService
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const service = dataService ?? createMockDataService();

  return render(
    <QueryClientProvider client={queryClient}>
      <DataServiceProvider initialService={service}>
        <EvalConfigPanel
          agentName="test-agent"
          frameworkType="promptkit"
          {...props}
        />
      </DataServiceProvider>
    </QueryClientProvider>
  );
}

describe("EvalConfigPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: true,
      hideEnterprise: false,
      showUpgradePrompts: false,
      loading: false,
    });
  });

  it("renders when enterprise is enabled", () => {
    renderPanel();
    expect(screen.getByText("Inline Eval Execution")).toBeInTheDocument();
  });

  it("renders nothing when enterprise is disabled", () => {
    mockEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: false,
      hideEnterprise: false,
      showUpgradePrompts: true,
      loading: false,
    });
    const { container } = renderPanel();
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing when enterprise is hidden", () => {
    mockEnterpriseConfig.mockReturnValue({
      enterpriseEnabled: false,
      hideEnterprise: true,
      showUpgradePrompts: false,
      loading: false,
    });
    const { container } = renderPanel();
    expect(container.innerHTML).toBe("");
  });

  it("shows info alert about offline evals", () => {
    renderPanel();
    expect(screen.getByText(/automatically run offline by the eval worker/)).toBeInTheDocument();
  });

  it("shows toggle enabled for promptkit agents", () => {
    renderPanel({ frameworkType: "promptkit" });
    const toggle = screen.getByRole("switch", { name: /toggle inline eval/i });
    expect(toggle).not.toBeDisabled();
  });

  it("disables toggle for non-promptkit agents", () => {
    renderPanel({ frameworkType: "custom" });
    const toggle = screen.getByRole("switch", { name: /toggle inline eval/i });
    expect(toggle).toBeDisabled();
    expect(screen.getByText("Only available for PromptKit agents")).toBeInTheDocument();
  });

  it("shows sampling controls when enabled", () => {
    renderPanel({ evalsEnabled: true });
    expect(screen.getByText("Lightweight eval sampling")).toBeInTheDocument();
    expect(screen.getByText("Extended eval sampling")).toBeInTheDocument();
  });

  it("hides sampling controls when disabled", () => {
    renderPanel({ evalsEnabled: false });
    expect(screen.queryByText("Lightweight eval sampling")).not.toBeInTheDocument();
  });

  it("calls updateAgentEvals when toggled", async () => {
    const mockUpdate = vi.fn().mockResolvedValue({});
    const service = createMockDataService({ updateAgentEvals: mockUpdate });

    renderPanel({ evalsEnabled: false }, service);
    const toggle = screen.getByRole("switch", { name: /toggle inline eval/i });
    fireEvent.click(toggle);

    await waitFor(() => {
      expect(mockUpdate).toHaveBeenCalledWith("test-ws", "test-agent", {
        enabled: true,
      });
    });
  });

  it("reverts toggle on error", async () => {
    const mockUpdate = vi.fn().mockRejectedValue(new Error("fail"));
    const service = createMockDataService({ updateAgentEvals: mockUpdate });

    renderPanel({ evalsEnabled: false }, service);
    const toggle = screen.getByRole("switch", { name: /toggle inline eval/i });
    fireEvent.click(toggle);

    await waitFor(() => {
      expect(toggle).toHaveAttribute("data-state", "unchecked");
    });
  });

  it("shows sampling values from props", () => {
    renderPanel({
      evalsEnabled: true,
      sampling: { defaultRate: 50, extendedRate: 25 },
    });
    expect(screen.getByText("50%")).toBeInTheDocument();
    expect(screen.getByText("25%")).toBeInTheDocument();
  });

  it("defaults lightweight to 100% and extended to 10%", () => {
    renderPanel({ evalsEnabled: true });
    expect(screen.getByText("100%")).toBeInTheDocument();
    expect(screen.getByText("10%")).toBeInTheDocument();
  });
});
