/**
 * Tests for Arena Configs list page.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import ArenaConfigsPage from "./page";

// Mock hooks
vi.mock("@/hooks/use-arena-configs", () => ({
  useArenaConfigs: vi.fn(),
  useArenaConfigMutations: vi.fn(),
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
    ArenaBreadcrumb: ({ items }: { items: { label: string }[] }) => (
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

describe("ArenaConfigsPage", () => {
  const mockRefetch = vi.fn();
  const mockDeleteConfig = vi.fn();

  beforeEach(() => {
    vi.resetAllMocks();
    // Reset to default permissions
    mockWorkspacePermissions = { write: true, read: true, delete: true, manageMembers: false };
  });

  it("renders loading skeleton when loading", async () => {
    const { useArenaConfigs, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
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

    render(<ArenaConfigsPage />);

    expect(screen.getByText("Configs")).toBeInTheDocument();
    expect(screen.getByText("Manage Arena evaluation configurations")).toBeInTheDocument();
  });

  it("renders error state when error occurs", async () => {
    const { useArenaConfigs, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
      loading: false,
      error: new Error("Failed to fetch configs"),
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

    render(<ArenaConfigsPage />);

    expect(screen.getByText("Error loading configs")).toBeInTheDocument();
    expect(screen.getByText("Failed to fetch configs")).toBeInTheDocument();
  });

  it("renders empty state when no configs", async () => {
    const { useArenaConfigs, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
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

    render(<ArenaConfigsPage />);

    expect(screen.getByText("No configs found")).toBeInTheDocument();
    expect(screen.getByText("Create your first config to get started with Arena.")).toBeInTheDocument();
  });

  it("renders configs in grid view by default", async () => {
    const { useArenaConfigs, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [
        {
          apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
          kind: "ArenaConfig" as const,
          metadata: { name: "test-config" },
          spec: { sourceRef: { name: "test-source" } },
          status: { phase: "Ready", scenarioCount: 5 },
        },
      ],
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
      sources: [
        {
          apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
          kind: "ArenaSource" as const,
          metadata: { name: "test-source" },
          spec: { type: "git" },
          status: { phase: "Ready" },
        },
      ],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaConfigsPage />);

    expect(screen.getByText("test-config")).toBeInTheDocument();
    expect(screen.getByText("test-source")).toBeInTheDocument();
    // Check for scenario count badge
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("opens create dialog when Create Config button is clicked", async () => {
    const { useArenaConfigs, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
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

    render(<ArenaConfigsPage />);

    const createButton = screen.getByText("Create Config");
    fireEvent.click(createButton);

    expect(screen.getByTestId("config-dialog")).toBeInTheDocument();
  });

  it("hides Create Config button when user lacks write permission", async () => {
    mockWorkspacePermissions = { write: false, read: true, delete: false, manageMembers: false };

    const { useArenaConfigs, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
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

    render(<ArenaConfigsPage />);

    expect(screen.queryByText("Create Config")).not.toBeInTheDocument();
  });

  it("has view mode toggle tabs", async () => {
    const { useArenaConfigs, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [
        {
          apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
          kind: "ArenaConfig" as const,
          metadata: { name: "test-config" },
          spec: { sourceRef: { name: "test-source" } },
          status: { phase: "Ready", scenarioCount: 5 },
        },
      ],
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

    render(<ArenaConfigsPage />);

    // Verify view mode toggle tabs exist
    const tabsList = screen.getAllByRole("tab");
    expect(tabsList).toHaveLength(2);

    // In grid view (default), the config card should be visible
    expect(screen.getByText("test-config")).toBeInTheDocument();
  });

  it("renders config with providers and tool registries count", async () => {
    const { useArenaConfigs, useArenaConfigMutations } = await import("@/hooks/use-arena-configs");
    const { useArenaSources } = await import("@/hooks");

    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [
        {
          apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
          kind: "ArenaConfig" as const,
          metadata: { name: "full-config" },
          spec: {
            sourceRef: { name: "test-source" },
            providers: [{ name: "openai" }, { name: "anthropic" }],
            toolRegistries: [{ name: "tools-1" }],
          },
          status: { phase: "Ready", scenarioCount: 10 },
        },
      ],
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

    render(<ArenaConfigsPage />);

    // Check provider count badge shows 2
    expect(screen.getByText("2")).toBeInTheDocument();
    // Check tool registry count badge shows 1
    expect(screen.getByText("1")).toBeInTheDocument();
    // Check scenario count shows 10
    expect(screen.getByText("10")).toBeInTheDocument();
  });
});
