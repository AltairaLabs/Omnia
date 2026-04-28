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
    expect(screen.getByText("Realtime Evals")).toBeInTheDocument();
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

  it("shows info alert describing the split routing", () => {
    renderPanel();
    expect(
      screen.getByText(/cheap deterministic evals .* run inline/i),
    ).toBeInTheDocument();
  });

  it("shows toggle enabled for promptkit agents", () => {
    renderPanel({ frameworkType: "promptkit" });
    const toggle = screen.getByRole("switch", { name: /toggle eval execution/i });
    expect(toggle).not.toBeDisabled();
  });

  it("disables toggle for non-promptkit agents", () => {
    renderPanel({ frameworkType: "custom" });
    const toggle = screen.getByRole("switch", { name: /toggle eval execution/i });
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
    const toggle = screen.getByRole("switch", { name: /toggle eval execution/i });
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
    const toggle = screen.getByRole("switch", { name: /toggle eval execution/i });
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

  // #988 — Advanced routing UI
  describe("advanced routing", () => {
    it("hides advanced routing when evals are disabled", () => {
      renderPanel({ evalsEnabled: false });
      expect(screen.queryByText("Advanced routing")).not.toBeInTheDocument();
    });

    it("shows the advanced routing disclosure when evals are enabled", () => {
      renderPanel({ evalsEnabled: true });
      expect(screen.getByText("Advanced routing")).toBeInTheDocument();
    });

    it("offers the four built-in groups when no pack is configured", async () => {
      renderPanel({ evalsEnabled: true });
      fireEvent.click(screen.getByText("Advanced routing"));

      await waitFor(() => {
        // Each group is rendered twice (once per path) — we just need
        // to confirm presence, not count.
        expect(screen.getAllByText("default").length).toBeGreaterThan(0);
        expect(screen.getAllByText("fast-running").length).toBeGreaterThan(0);
        expect(screen.getAllByText("long-running").length).toBeGreaterThan(0);
        expect(screen.getAllByText("external").length).toBeGreaterThan(0);
      });
    });

    it("calls updateAgentEvals with the inline.groups patch when a group is toggled", async () => {
      const mockUpdate = vi.fn().mockResolvedValue({});
      const service = createMockDataService({ updateAgentEvals: mockUpdate });
      renderPanel({ evalsEnabled: true, inlineGroups: ["fast-running"] }, service);

      fireEvent.click(screen.getByText("Advanced routing"));

      // Toggle "default" on the inline path. The id encodes the path
      // prefix so we can target the inline checkbox specifically.
      await waitFor(() => {
        expect(document.getElementById("evals-inline-default")).not.toBeNull();
      });
      const inlineDefault = document.getElementById("evals-inline-default") as HTMLInputElement;
      fireEvent.click(inlineDefault);

      await waitFor(() => {
        expect(mockUpdate).toHaveBeenCalledWith("test-ws", "test-agent", {
          inline: { groups: ["fast-running", "default"] },
        });
      });
    });

    it("renders custom groups already on the agent as removable chips", async () => {
      renderPanel({
        evalsEnabled: true,
        workerGroups: ["long-running", "my-custom"],
      });
      fireEvent.click(screen.getByText("Advanced routing"));

      // The custom group renders both as a removable badge AND in the
      // list when it's selected — assert at least one occurrence.
      await waitFor(() => {
        expect(screen.getAllByText("my-custom").length).toBeGreaterThan(0);
      });
    });

    it("rolls back optimistic group change on error", async () => {
      const mockUpdate = vi.fn().mockRejectedValue(new Error("conflict"));
      const service = createMockDataService({ updateAgentEvals: mockUpdate });
      renderPanel(
        { evalsEnabled: true, workerGroups: ["long-running"] },
        service,
      );

      fireEvent.click(screen.getByText("Advanced routing"));
      await waitFor(() => {
        expect(document.getElementById("evals-worker-external")).not.toBeNull();
      });
      const workerExternal = document.getElementById("evals-worker-external") as HTMLInputElement;
      fireEvent.click(workerExternal);

      // Wait for the patch attempt and the rollback. The "external"
      // checkbox should end up unchecked again because the patch
      // failed and we restored the prior selection.
      await waitFor(() => {
        expect(mockUpdate).toHaveBeenCalled();
      });
      await waitFor(() => {
        const cb = document.getElementById("evals-worker-external") as HTMLInputElement;
        expect(cb.getAttribute("data-state")).toBe("unchecked");
      });
    });
  });
});
