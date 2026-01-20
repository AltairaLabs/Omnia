/**
 * Tests for Arena Source detail page.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import ArenaSourceDetailPage from "./page";

// Mock hooks
vi.mock("@/hooks", () => ({
  useArenaSource: vi.fn(),
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

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useParams: vi.fn(() => ({ name: "test-source" })),
  useRouter: vi.fn(() => ({ push: vi.fn() })),
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
  ArenaBreadcrumb: ({ items }: { items: { label: string; href?: string }[] }) => (
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

describe("ArenaSourceDetailPage", () => {
  const mockRefetch = vi.fn();
  const mockSyncSource = vi.fn();
  const mockDeleteSource = vi.fn();

  beforeEach(() => {
    vi.resetAllMocks();
    // Reset to default permissions
    mockWorkspacePermissions = { write: true, read: true, delete: true, manageMembers: false };
  });

  it("renders loading skeleton when loading", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: null,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    expect(screen.getByText("Source Details")).toBeInTheDocument();
    expect(screen.getByText("Loading source information...")).toBeInTheDocument();
  });

  it("renders error state when error occurs", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: null,
      linkedConfigs: [],
      loading: false,
      error: new Error("Failed to fetch source"),
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

    render(<ArenaSourceDetailPage />);

    // Find the alert which contains the error title
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Error loading source");
    expect(alert).toHaveTextContent("Failed to fetch source");
  });

  it("renders not found state when source is null", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: null,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    // Find the alert which contains the not found message
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Source not found");
    expect(alert).toHaveTextContent("could not be found");
    expect(screen.getByText("Back to Sources")).toBeInTheDocument();
  });

  it("renders source details with overview tab", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source", creationTimestamp: "2026-01-20T10:00:00Z" },
        spec: {
          type: "git",
          interval: "5m",
          git: { url: "https://github.com/org/repo.git", ref: { branch: "main" }, path: "prompts/" },
        },
        status: {
          phase: "Ready",
          artifact: {
            revision: "main@sha1:abc123",
            checksum: "sha256:abc123def456",
            url: "/internal/url",
            size: 1024,
            lastUpdateTime: "2026-01-20T10:00:00Z",
          },
        },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    // Check breadcrumb contains source name
    const breadcrumb = screen.getByTestId("breadcrumb");
    expect(breadcrumb).toHaveTextContent("test-source");

    // Check status badge shows Ready (might appear in multiple places)
    const readyBadges = screen.getAllByText("Ready");
    expect(readyBadges.length).toBeGreaterThan(0);

    // Check interval is displayed
    expect(screen.getByText("5 mins")).toBeInTheDocument();

    // Check git configuration - URL should be shown
    expect(screen.getByText("https://github.com/org/repo.git")).toBeInTheDocument();

    // Check artifact info
    expect(screen.getByText("main@sha1:abc123")).toBeInTheDocument();
    expect(screen.getByText("1.0 KB")).toBeInTheDocument();
  });

  it("renders action buttons when user has edit permission", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    expect(screen.getByText("Sync Now")).toBeInTheDocument();
    expect(screen.getByText("Edit")).toBeInTheDocument();
    expect(screen.getByText("Delete")).toBeInTheDocument();
  });

  it("hides Edit and Delete buttons when user lacks edit permission", async () => {
    mockWorkspacePermissions = { write: false, read: true, delete: false, manageMembers: false };

    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    expect(screen.getByText("Sync Now")).toBeInTheDocument(); // Sync is still visible but disabled
    expect(screen.queryByText("Edit")).not.toBeInTheDocument();
    expect(screen.queryByText("Delete")).not.toBeInTheDocument();
  });

  it("renders tabs correctly", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [
        { metadata: { name: "config-1" }, spec: { sourceRef: { name: "test-source" } }, status: { phase: "Ready" } },
        { metadata: { name: "config-2" }, spec: { sourceRef: { name: "test-source" } }, status: { phase: "Ready" } },
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

    render(<ArenaSourceDetailPage />);

    expect(screen.getByText("Overview")).toBeInTheDocument();
    expect(screen.getByText("Sync History")).toBeInTheDocument();
    expect(screen.getByText("Linked Configs (2)")).toBeInTheDocument();
  });

  it("renders S3 source configuration", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "s3-source" },
        spec: {
          type: "s3",
          interval: "1h",
          s3: { bucket: "my-bucket", prefix: "prompts/", region: "us-east-1", endpoint: "https://s3.example.com" },
        },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    expect(screen.getByText("my-bucket")).toBeInTheDocument();
    expect(screen.getByText("prompts/")).toBeInTheDocument();
    expect(screen.getByText("us-east-1")).toBeInTheDocument();
    expect(screen.getByText("https://s3.example.com")).toBeInTheDocument();
  });

  it("renders OCI source configuration", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "oci-source" },
        spec: {
          type: "oci",
          interval: "30m",
          oci: { url: "oci://ghcr.io/org/prompts", ref: { tag: "v1.0.0", semver: ">=1.0.0" } },
        },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    expect(screen.getByText("oci://ghcr.io/org/prompts")).toBeInTheDocument();
    expect(screen.getByText("v1.0.0")).toBeInTheDocument();
    expect(screen.getByText(">=1.0.0")).toBeInTheDocument();
  });

  it("renders conditions data for sync history", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: {
          phase: "Ready",
          conditions: [
            { type: "Ready", status: "True", reason: "Succeeded", message: "Artifact ready", lastTransitionTime: "2026-01-20T10:00:00Z" },
          ],
        },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    // Check that sync history tab exists
    const syncHistoryTab = screen.getByRole("tab", { name: /sync history/i });
    expect(syncHistoryTab).toBeInTheDocument();
  });

  it("shows linked configs count in tab", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [
        { metadata: { name: "config-1" }, spec: { sourceRef: { name: "test-source" } }, status: { phase: "Ready", scenarioCount: 10 } },
        { metadata: { name: "config-2" }, spec: { sourceRef: { name: "test-source" } }, status: { phase: "Failed", scenarioCount: 5 } },
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

    render(<ArenaSourceDetailPage />);

    // Check that linked configs tab shows the count
    expect(screen.getByText(/Linked Configs \(2\)/)).toBeInTheDocument();
  });

  it("shows zero linked configs count in tab", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    // Check that linked configs tab shows zero count
    expect(screen.getByText(/Linked Configs \(0\)/)).toBeInTheDocument();
  });

  it("opens edit dialog when Edit button is clicked", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    fireEvent.click(screen.getByText("Edit"));

    expect(screen.getByTestId("source-dialog")).toBeInTheDocument();
  });

  it("renders breadcrumb with correct items", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    const breadcrumb = screen.getByTestId("breadcrumb");
    expect(breadcrumb).toHaveTextContent("Sources");
    expect(breadcrumb).toHaveTextContent("test-source");
  });

  it("displays secret reference when present", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: {
          type: "git",
          git: { url: "https://github.com/org/repo.git" },
          secretRef: { name: "my-git-credentials" },
        },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    expect(screen.getByText("Credentials Secret")).toBeInTheDocument();
    expect(screen.getByText("my-git-credentials")).toBeInTheDocument();
  });

  it("calls syncSource when Sync Now is clicked", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    const syncButton = screen.getByText("Sync Now");
    fireEvent.click(syncButton);

    await vi.waitFor(() => {
      expect(mockSyncSource).toHaveBeenCalledWith("test-source");
    });
  });

  it("calls refetch after successful sync", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    mockSyncSource.mockResolvedValueOnce(undefined);
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourceDetailPage />);

    const syncButton = screen.getByText("Sync Now");
    fireEvent.click(syncButton);

    await vi.waitFor(() => {
      expect(mockRefetch).toHaveBeenCalled();
    });
  });

  it("handles sync error gracefully", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    mockSyncSource.mockRejectedValueOnce(new Error("Sync failed"));
    vi.mocked(useArenaSourceMutations).mockReturnValue({
      createSource: vi.fn(),
      updateSource: vi.fn(),
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });

    render(<ArenaSourceDetailPage />);

    const syncButton = screen.getByText("Sync Now");
    fireEvent.click(syncButton);

    // Should not throw, error is handled gracefully
    await vi.waitFor(() => {
      expect(mockSyncSource).toHaveBeenCalled();
    });
  });

  it("renders configmap source configuration", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "cm-source" },
        spec: {
          type: "configmap",
          interval: "10m",
          configMapRef: { name: "my-config", key: "prompts.yaml" },
        },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    expect(screen.getByText("my-config")).toBeInTheDocument();
  });

  it("renders sync history tab", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: {
          phase: "Ready",
          conditions: [
            { type: "Ready", status: "True", reason: "Succeeded", message: "Fetched", lastTransitionTime: "2026-01-20T10:00:00Z" },
          ],
        },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    // Check sync history tab exists
    const historyTab = screen.getByRole("tab", { name: /sync history/i });
    expect(historyTab).toBeInTheDocument();
  });

  it("renders linked configs tab with count", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [
        { metadata: { name: "config-1" }, spec: { sourceRef: { name: "test-source" } }, status: { phase: "Ready", scenarioCount: 10 } },
        { metadata: { name: "config-2" }, spec: { sourceRef: { name: "test-source" } }, status: { phase: "Failed", scenarioCount: 5 } },
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

    render(<ArenaSourceDetailPage />);

    // Check linked configs tab exists with count
    const configsTab = screen.getByRole("tab", { name: /linked configs \(2\)/i });
    expect(configsTab).toBeInTheDocument();
  });

  it("renders linked configs tab with zero count", async () => {
    const { useArenaSource, useArenaSourceMutations } = await import("@/hooks");
    vi.mocked(useArenaSource).mockReturnValue({
      source: {
        metadata: { name: "test-source" },
        spec: { type: "configmap", configMapRef: { name: "cm" } },
        status: { phase: "Ready" },
      } as any,
      linkedConfigs: [],
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

    render(<ArenaSourceDetailPage />);

    // Check linked configs tab exists with zero count
    const configsTab = screen.getByRole("tab", { name: /linked configs \(0\)/i });
    expect(configsTab).toBeInTheDocument();
  });
});
