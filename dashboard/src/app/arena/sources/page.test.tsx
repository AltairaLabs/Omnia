/**
 * Tests for Arena Sources list page.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import ArenaSourcesPage from "./page";

// Mock hooks
vi.mock("@/hooks", () => ({
  useArenaSources: vi.fn(),
  useArenaSourceMutations: vi.fn(),
}));

// Mock hooks/use-license
vi.mock("@/hooks/use-license", () => ({
  useLicense: vi.fn(() => ({
    license: { tier: "enterprise" },
    isEnterprise: true,
    canUseSourceType: () => true,
  })),
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
vi.mock("@/components/arena", () => ({
  ArenaBreadcrumb: ({ items }: { items: { label: string }[] }) => (
    <nav data-testid="breadcrumb">
      {items.map((item) => (
        <span key={item.label}>{item.label}</span>
      ))}
    </nav>
  ),
  SourceDialog: ({ open }: { open: boolean }) => (
    open ? <div data-testid="source-dialog">Dialog</div> : null
  ),
}));

// Mock next/link
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

describe("ArenaSourcesPage", () => {
  const mockRefetch = vi.fn();
  const mockSyncSource = vi.fn();
  const mockDeleteSource = vi.fn();

  beforeEach(() => {
    vi.resetAllMocks();
    // Reset to default permissions
    mockWorkspacePermissions = { write: true, read: true, delete: true, manageMembers: false };
  });

  it("renders loading skeleton when loading", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: true,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("Sources")).toBeInTheDocument();
    expect(screen.getByText("Manage PromptKit bundle sources")).toBeInTheDocument();
  });

  it("renders error state when error occurs", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: new Error("Failed to fetch sources"),
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("Error loading sources")).toBeInTheDocument();
    expect(screen.getByText("Failed to fetch sources")).toBeInTheDocument();
  });

  it("renders empty state when no sources", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("No sources found")).toBeInTheDocument();
    expect(screen.getByText("Create your first source to get started with Arena.")).toBeInTheDocument();
  });

  it("renders sources in grid view by default", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        {
          metadata: { name: "git-source-1" },
          spec: { type: "git", interval: "5m", git: { url: "https://github.com/org/repo.git" } },
          status: { phase: "Ready", artifact: { lastUpdateTime: "2026-01-20T10:00:00Z", revision: "main@sha1:abc123", checksum: "sha256:abc", url: "/internal" } },
        },
        {
          metadata: { name: "s3-source-1" },
          spec: { type: "s3", interval: "1h", s3: { bucket: "my-bucket", prefix: "prompts/" } },
          status: { phase: "Failed" },
        },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    // Check source names are rendered
    expect(screen.getByText("git-source-1")).toBeInTheDocument();
    expect(screen.getByText("s3-source-1")).toBeInTheDocument();

    // Check status badges
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("Failed")).toBeInTheDocument();
  });

  it("renders Create Source button when user has edit permission", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("Create Source")).toBeInTheDocument();
  });

  it("hides Create Source button when user lacks edit permission", async () => {
    mockWorkspacePermissions = { write: false, read: true, delete: false, manageMembers: false };

    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.queryByText("Create Source")).not.toBeInTheDocument();
  });

  it("opens dialog when Create Source is clicked", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    // Click Create Source button
    const createButton = screen.getByRole("button", { name: /create source/i });
    fireEvent.click(createButton);

    // Dialog should open - check for dialog role (Radix creates this)
    // The dialog is rendered via portal so we query the document
    await vi.waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
  });

  it("renders source type badges correctly", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "git-src" }, spec: { type: "git", git: { url: "https://github.com/org/repo" } }, status: { phase: "Ready" } },
        { metadata: { name: "oci-src" }, spec: { type: "oci", oci: { url: "oci://ghcr.io/org/pkg" } }, status: { phase: "Ready" } },
        { metadata: { name: "s3-src" }, spec: { type: "s3", s3: { bucket: "bucket" } }, status: { phase: "Ready" } },
        { metadata: { name: "cm-src" }, spec: { type: "configmap", configMapRef: { name: "my-cm" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    // Check that all source types are displayed
    expect(screen.getByText("git")).toBeInTheDocument();
    expect(screen.getByText("oci")).toBeInTheDocument();
    expect(screen.getByText("s3")).toBeInTheDocument();
    expect(screen.getByText("configmap")).toBeInTheDocument();
  });

  it("renders source URLs correctly", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "git-src" }, spec: { type: "git", git: { url: "https://github.com/org/repo.git" } }, status: { phase: "Ready" } },
        { metadata: { name: "s3-src" }, spec: { type: "s3", s3: { bucket: "my-bucket", prefix: "prompts" } }, status: { phase: "Ready" } },
        { metadata: { name: "cm-src" }, spec: { type: "configmap", configMapRef: { name: "my-configmap" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("https://github.com/org/repo.git")).toBeInTheDocument();
    expect(screen.getByText("s3://my-bucket/prompts")).toBeInTheDocument();
    expect(screen.getByText("my-configmap")).toBeInTheDocument();
  });

  it("renders all status badges correctly", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "src-1" }, spec: { type: "configmap", configMapRef: { name: "cm1" } }, status: { phase: "Ready" } },
        { metadata: { name: "src-2" }, spec: { type: "configmap", configMapRef: { name: "cm2" } }, status: { phase: "Failed" } },
        { metadata: { name: "src-3" }, spec: { type: "configmap", configMapRef: { name: "cm3" } }, status: { phase: "Pending" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("formats interval correctly", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "src-1" }, spec: { type: "configmap", interval: "5m", configMapRef: { name: "cm1" } }, status: { phase: "Ready" } },
        { metadata: { name: "src-2" }, spec: { type: "configmap", interval: "1h", configMapRef: { name: "cm2" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("5 mins")).toBeInTheDocument();
    expect(screen.getByText("1 hour")).toBeInTheDocument();
  });

  it("renders breadcrumb with correct items", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    const breadcrumb = screen.getByTestId("breadcrumb");
    expect(breadcrumb).toHaveTextContent("Sources");
  });

  it("renders links to source detail pages", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "my-source" }, spec: { type: "configmap", configMapRef: { name: "cm" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    const link = screen.getByRole("link", { name: "my-source" });
    expect(link).toHaveAttribute("href", "/arena/sources/my-source");
  });

  it("renders view toggle tabs", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "my-source" }, spec: { type: "configmap", configMapRef: { name: "cm" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    // Find tab list which contains view toggle
    const tabList = screen.getByRole("tablist");
    expect(tabList).toBeInTheDocument();
    // Grid view should be active by default
    expect(screen.getAllByRole("tab")).toHaveLength(2);
  });

  it("renders action menu button for each source", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "my-source" }, spec: { type: "configmap", configMapRef: { name: "cm" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    // Find action menu trigger button (ghost button with MoreHorizontal icon)
    const actionButtons = screen.getAllByRole("button");
    // Should have at least the action menu button plus the create button
    expect(actionButtons.length).toBeGreaterThanOrEqual(2);
  });

  it("renders OCI source type correctly", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "oci-source" }, spec: { type: "oci", oci: { url: "oci://ghcr.io/org/pkg" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("oci-source")).toBeInTheDocument();
    expect(screen.getByText("oci://ghcr.io/org/pkg")).toBeInTheDocument();
  });

  it("renders s3 source without prefix correctly", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "s3-source" }, spec: { type: "s3", s3: { bucket: "my-bucket" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("s3://my-bucket")).toBeInTheDocument();
  });

  it("switches to table view when table tab is clicked", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "my-source" }, spec: { type: "configmap", configMapRef: { name: "cm" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    // Get all tabs
    const tabs = screen.getAllByRole("tab");
    // Click on the second tab (table view)
    fireEvent.click(tabs[1]);

    // Check that source name is still visible in table view
    expect(screen.getByText("my-source")).toBeInTheDocument();
  });

  it("displays seconds interval correctly", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "src-1" }, spec: { type: "configmap", interval: "30s", configMapRef: { name: "cm1" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("30 secs")).toBeInTheDocument();
  });

  it("displays hours interval correctly", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        { metadata: { name: "src-1" }, spec: { type: "configmap", interval: "6h", configMapRef: { name: "cm1" } }, status: { phase: "Ready" } },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("6 hours")).toBeInTheDocument();
  });

  it("renders source with artifact info showing last sync time", async () => {
    const { useArenaSources, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSources).mockReturnValue({
      sources: [
        {
          metadata: { name: "synced-source" },
          spec: { type: "configmap", configMapRef: { name: "cm" } },
          status: {
            phase: "Ready",
            artifact: {
              lastUpdateTime: "2026-01-20T10:00:00Z",
              revision: "v1.0.0",
              checksum: "sha256:abc",
              url: "/artifacts/cm"
            }
          }
        },
      ] as any,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourcesPage />);

    expect(screen.getByText("synced-source")).toBeInTheDocument();
  });
});
