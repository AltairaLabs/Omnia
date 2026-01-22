/**
 * Tests for Arena Config detail page.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import ArenaConfigDetailPage from "./page";

// Mock hooks
vi.mock("@/hooks/use-arena-configs", () => ({
  useArenaConfig: vi.fn(),
  useArenaConfigMutations: vi.fn(),
  useArenaConfigContent: vi.fn(() => ({
    content: null,
    loading: false,
    error: null,
    refetch: vi.fn(),
  })),
  useArenaConfigFile: vi.fn(() => ({
    content: null,
    loading: false,
    error: null,
  })),
}));

vi.mock("@/hooks", () => ({
  useArenaSources: vi.fn(),
}));

// Workspace mock with configurable permissions
let mockWorkspacePermissions = { write: true, read: true, delete: true, manageMembers: false };
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({
    currentWorkspace: {
      name: "default",
      permissions: mockWorkspacePermissions,
    },
  })),
}));

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useParams: vi.fn(() => ({ name: "test-config" })),
  useRouter: vi.fn(() => ({
    push: vi.fn(),
    back: vi.fn(),
  })),
}));

// Mock layout components
vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: string; description: string }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      <p>{description}</p>
    </div>
  ),
}));

// Mock arena components
vi.mock("@/components/arena", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/arena")>();
  return {
    ...actual,
    ArenaBreadcrumb: ({ items }: { items: { label: string; href?: string }[] }) => (
      <nav data-testid="breadcrumb">
        {items.map((item) => (
          <span key={item.label}>{item.label}</span>
        ))}
      </nav>
    ),
    ConfigDialog: ({ open }: { open: boolean }) => (
      open ? <div data-testid="config-dialog">Dialog</div> : null
    ),
  };
});

// Mock next/link
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

const mockConfig = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
  kind: "ArenaConfig" as const,
  metadata: { name: "test-config", creationTimestamp: "2026-01-15T10:00:00Z" },
  spec: {
    sourceRef: { name: "test-source" },
    scenarios: {
      include: ["scenarios/**/*.yaml"],
      exclude: ["scenarios/*-wip.yaml"],
    },
    defaults: {
      temperature: 0.7,
      concurrency: 5,
      timeout: "30s",
    },
  },
  status: {
    phase: "Ready" as const,
    scenarioCount: 10,
    sourceRevision: "main@sha1:abc123",
    providers: [
      { name: "openai", status: "Ready" },
      { name: "anthropic", status: "Ready" },
    ],
    toolRegistries: [
      { name: "tools-1", status: "Ready", toolCount: 5 },
    ],
    conditions: [
      {
        type: "Ready",
        status: "True" as const,
        lastTransitionTime: "2026-01-15T10:00:00Z",
        reason: "ReconcileSucceeded",
        message: "Config is ready",
      },
    ],
  },
};

const mockScenarios = [
  {
    name: "test-scenario",
    displayName: "Test Scenario",
    description: "A test scenario for evaluation",
    tags: ["test", "basic"],
    path: "scenarios/test.yaml",
  },
  {
    name: "advanced-scenario",
    displayName: "Advanced Scenario",
    description: "An advanced test scenario",
    tags: ["test", "advanced", "multi-turn", "complex"],
    path: "scenarios/advanced.yaml",
  },
];

const mockJobs = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
    kind: "ArenaJob" as const,
    metadata: { name: "job-1" },
    spec: { configRef: { name: "test-config" }, type: "evaluation" as const },
    status: { phase: "Completed" as const, completedTasks: 10, totalTasks: 10, startTime: "2026-01-15T09:00:00Z" },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
    kind: "ArenaJob" as const,
    metadata: { name: "job-2" },
    spec: { configRef: { name: "test-config" }, type: "loadtest" as const },
    status: { phase: "Running" as const, completedTasks: 5, totalTasks: 10, startTime: "2026-01-15T10:30:00Z" },
  },
];

describe("ArenaConfigDetailPage", () => {
  const mockRefetch = vi.fn();
  const mockDeleteConfig = vi.fn();

  beforeEach(() => {
    vi.resetAllMocks();
    mockWorkspacePermissions = { write: true, read: true, delete: true, manageMembers: false };
  });

  it("renders loading skeleton when loading", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: null,
      scenarios: [],
      linkedJobs: [],
      loading: true,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    expect(screen.getByText("Config Details")).toBeInTheDocument();
    expect(screen.getByText("Loading config information...")).toBeInTheDocument();
  });

  it("renders error state when error occurs", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: null,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: new Error("Failed to fetch config"),
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    // Error text may appear in header and alert
    expect(screen.getAllByText("Error loading config").length).toBeGreaterThan(0);
    expect(screen.getByText("Failed to fetch config")).toBeInTheDocument();
  });

  it("renders not found state when config is null", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: null,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    // Check for config not found alert (may appear multiple times)
    expect(screen.getAllByText("Config not found").length).toBeGreaterThan(0);
    expect(screen.getByText(/could not be found/)).toBeInTheDocument();
    expect(screen.getByText("Back to Configs")).toBeInTheDocument();
  });

  it("renders config overview with all sections", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: mockScenarios,
      linkedJobs: mockJobs,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    // Check header and breadcrumb contain config name
    const configNameElements = screen.getAllByText("test-config");
    expect(configNameElements.length).toBeGreaterThan(0);

    // Check status section
    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getAllByText("Ready").length).toBeGreaterThan(0);

    // Check source configuration
    expect(screen.getByText("Source Configuration")).toBeInTheDocument();
    expect(screen.getByText("test-source")).toBeInTheDocument();

    // Check scenario filters
    expect(screen.getByText("Scenario Filters")).toBeInTheDocument();

    // Check default values
    expect(screen.getByText("Default Values")).toBeInTheDocument();
    expect(screen.getByText("Temperature")).toBeInTheDocument();
    expect(screen.getByText("0.7")).toBeInTheDocument();
    expect(screen.getByText("Concurrency")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();

    // Check provider status
    expect(screen.getByText("Provider Status")).toBeInTheDocument();
    expect(screen.getByText("openai")).toBeInTheDocument();
    expect(screen.getByText("anthropic")).toBeInTheDocument();

    // Check tool registry status
    expect(screen.getByText("Tool Registry Status")).toBeInTheDocument();
    expect(screen.getByText("tools-1")).toBeInTheDocument();
  });

  it("renders scenarios tab content", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: mockScenarios,
      linkedJobs: mockJobs,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    // Verify scenarios tab shows correct count
    const scenariosTab = screen.getByRole("tab", { name: /Scenarios/ });
    expect(scenariosTab).toBeInTheDocument();
    // Tab should contain scenario count in its name
    expect(scenariosTab.textContent).toContain("2");
  });

  it("renders jobs tab with correct count", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: mockScenarios,
      linkedJobs: mockJobs,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    // Verify jobs tab shows correct count
    const jobsTab = screen.getByRole("tab", { name: /Jobs/ });
    expect(jobsTab).toBeInTheDocument();
    // Tab should contain job count in its name
    expect(jobsTab.textContent).toContain("2");
  });

  it("opens edit dialog when Edit button is clicked", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    const editButton = screen.getByText("Edit");
    fireEvent.click(editButton);

    expect(screen.getByTestId("config-dialog")).toBeInTheDocument();
  });

  it("hides action buttons when user lacks write permission", async () => {
    mockWorkspacePermissions = { write: false, read: true, delete: false, manageMembers: false };

    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    // Edit and Delete buttons should not be visible
    expect(screen.queryByRole("button", { name: "Edit" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
  });

  it("renders config with empty scenarios and jobs", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    // Verify tabs exist with empty data
    expect(screen.getByRole("tab", { name: /Scenarios/ })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /Jobs/ })).toBeInTheDocument();

    // The page should render without crashing
    expect(screen.getByTestId("header")).toBeInTheDocument();
  });

  it("triggers Run Job action and navigates", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    // Mock window.location
    const mockLocation = { href: "" };
    Object.defineProperty(window, "location", {
      value: mockLocation,
      writable: true,
    });

    render(<ArenaConfigDetailPage />);

    const runJobButton = screen.getByRole("button", { name: /Run Job/ });
    fireEvent.click(runJobButton);

    expect(mockLocation.href).toBe("/arena/jobs?configRef=test-config");
  });

  it("triggers delete action and navigates on success", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");
    const { useRouter } = await import("next/navigation");

    const mockPush = vi.fn();
    vi.mocked(useRouter).mockReturnValue({
      push: mockPush,
      back: vi.fn(),
      forward: vi.fn(),
      refresh: vi.fn(),
      replace: vi.fn(),
      prefetch: vi.fn(),
    });

    const mockDelete = vi.fn().mockResolvedValue(undefined);
    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDelete,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    vi.spyOn(window, "confirm").mockReturnValue(true);

    render(<ArenaConfigDetailPage />);

    const deleteButton = screen.getByRole("button", { name: /Delete/ });
    fireEvent.click(deleteButton);

    expect(mockDelete).toHaveBeenCalledWith("test-config");
  });

  it("cancels delete when confirmation is declined", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    const mockDelete = vi.fn();
    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDelete,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    vi.spyOn(window, "confirm").mockReturnValue(false);

    render(<ArenaConfigDetailPage />);

    const deleteButton = screen.getByRole("button", { name: /Delete/ });
    fireEvent.click(deleteButton);

    expect(mockDelete).not.toHaveBeenCalled();
  });

  it("handles delete error gracefully", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    const mockDelete = vi.fn().mockRejectedValue(new Error("Delete failed"));
    vi.mocked(useArenaConfig).mockReturnValue({
      config: mockConfig,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDelete,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    vi.spyOn(window, "confirm").mockReturnValue(true);

    render(<ArenaConfigDetailPage />);

    const deleteButton = screen.getByRole("button", { name: /Delete/ });
    fireEvent.click(deleteButton);

    // Should call deleteConfig even if it fails
    expect(mockDelete).toHaveBeenCalledWith("test-config");
  });

  it("renders config without optional fields", async () => {
    const { useArenaConfig, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    const minimalConfig = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
      kind: "ArenaConfig" as const,
      metadata: { name: "test-config" },
      spec: {
        sourceRef: { name: "test-source" },
      },
      status: {
        phase: "Ready" as const,
      },
    };

    vi.mocked(useArenaConfig).mockReturnValue({
      config: minimalConfig,
      scenarios: [],
      linkedJobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaConfigMutations).mockReturnValue({
      createConfig: vi.fn(),
      updateConfig: vi.fn(),
      deleteConfig: mockDeleteConfig,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigDetailPage />);

    // Should render without crashing even without optional fields
    expect(screen.getAllByText("test-config").length).toBeGreaterThan(0);
  });
});
