/**
 * Tests for VersionSelector component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { VersionSelector } from "./version-selector";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Mock the hooks
vi.mock("@/hooks/use-arena-source-versions", () => ({
  useArenaSourceVersions: vi.fn(),
  useArenaSourceVersionMutations: vi.fn(),
}));

// Mock source-utils
vi.mock("./source-utils", () => ({
  formatBytes: vi.fn((bytes: number) => `${bytes} bytes`),
  formatDate: vi.fn((date: string) => new Date(date).toLocaleDateString()),
}));

// Create a wrapper with QueryClientProvider
function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}
TestWrapper.displayName = "TestWrapper";

const createWrapper = () => TestWrapper;

describe("VersionSelector", () => {
  const mockRefetch = vi.fn();
  const mockSwitchVersion = vi.fn();

  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("renders loading skeleton when loading", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [],
      headVersion: null,
      loading: true,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion,
      switching: false,
      error: null,
    });

    const { container } = render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    // Should show skeleton during loading (Skeleton component uses data-slot="skeleton")
    const skeleton = container.querySelector('[data-slot="skeleton"]');
    expect(skeleton).toBeDefined();
    expect(skeleton).not.toBeNull();
  });

  it("renders error state when error occurs", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [],
      headVersion: null,
      loading: false,
      error: new Error("Failed to load"),
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion,
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    expect(screen.getByText("Failed to load versions")).toBeInTheDocument();
  });

  it("renders empty state when no versions", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [],
      headVersion: null,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion,
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    expect(screen.getByText("No versions available")).toBeInTheDocument();
  });

  it("renders version selector with versions", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    const mockVersions = [
      { hash: "abc123def456", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      { hash: "xyz789abc012", createdAt: "2026-01-19T10:00:00Z", size: 2048, fileCount: 10, isLatest: false },
    ];

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: mockVersions,
      headVersion: "abc123def456",
      loading: false,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion,
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    // Should show truncated hash in selector
    expect(screen.getByText("abc123def456")).toBeInTheDocument();
    // Should show latest badge
    expect(screen.getByText("latest")).toBeInTheDocument();
  });

  it("shows read-only badge when disabled", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [
        { hash: "abc123def456", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      ],
      headVersion: "abc123def456",
      loading: false,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion,
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" disabled />, { wrapper: createWrapper() });

    expect(screen.getByText("Read-only")).toBeInTheDocument();
  });

  it("shows switching state during version switch", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [
        { hash: "abc123def456", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      ],
      headVersion: "abc123def456",
      loading: false,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion,
      switching: true,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    expect(screen.getByText("Switching...")).toBeInTheDocument();
  });

  it("handles version change", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    const mockVersions = [
      { hash: "abc123def456", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      { hash: "xyz789abc012", createdAt: "2026-01-19T10:00:00Z", size: 2048, fileCount: 10, isLatest: false },
    ];

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: mockVersions,
      headVersion: "abc123def456",
      loading: false,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion.mockResolvedValue(undefined),
      switching: false,
      error: null,
    });

    const onVersionChange = vi.fn();
    render(
      <VersionSelector sourceName="test-source" onVersionChange={onVersionChange} />,
      { wrapper: createWrapper() }
    );

    // Click the select trigger to open dropdown
    const trigger = screen.getByRole("combobox");
    fireEvent.click(trigger);

    // Wait for dropdown to open and click the second version
    await waitFor(() => {
      const option = screen.getByRole("option", { name: /xyz789abc012/i });
      fireEvent.click(option);
    });

    await waitFor(() => {
      expect(mockSwitchVersion).toHaveBeenCalledWith("xyz789abc012");
    });
  });

  it("does not switch version when selecting same version", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [
        { hash: "abc123def456", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      ],
      headVersion: "abc123def456",
      loading: false,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion,
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    // Click the select trigger
    const trigger = screen.getByRole("combobox");
    fireEvent.click(trigger);

    // Select the same version that's already selected
    await waitFor(() => {
      const option = screen.getByRole("option", { name: /abc123def456/i });
      fireEvent.click(option);
    });

    // Should not call switchVersion for same version
    expect(mockSwitchVersion).not.toHaveBeenCalled();
  });

  it("shows switch error when switch fails", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    const mockVersions = [
      { hash: "abc123def456", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      { hash: "xyz789abc012", createdAt: "2026-01-19T10:00:00Z", size: 2048, fileCount: 10, isLatest: false },
    ];

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: mockVersions,
      headVersion: "abc123def456",
      loading: false,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion.mockRejectedValue(new Error("Switch failed")),
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    // Click the select trigger
    const trigger = screen.getByRole("combobox");
    fireEvent.click(trigger);

    // Select a different version
    await waitFor(() => {
      const option = screen.getByRole("option", { name: /xyz789abc012/i });
      fireEvent.click(option);
    });

    // Wait for error to appear
    await waitFor(() => {
      expect(screen.getByText("Switch failed")).toBeInTheDocument();
    });
  });

  it("renders without source name", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [],
      headVersion: null,
      loading: false,
      error: null,
      refetch: mockRefetch,
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: mockSwitchVersion,
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName={undefined} />, { wrapper: createWrapper() });

    expect(screen.getByText("No versions available")).toBeInTheDocument();
  });
});

describe("truncateHash", () => {
  it("truncates long hashes", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [
        { hash: "abc123def456ghi789jkl012mno345", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      ],
      headVersion: "abc123def456ghi789jkl012mno345",
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: vi.fn(),
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    // Should show truncated hash (12 chars)
    expect(screen.getByText("abc123def456")).toBeInTheDocument();
  });

  it("does not truncate short hashes", async () => {
    const { useArenaSourceVersions, useArenaSourceVersionMutations } = await import(
      "@/hooks/use-arena-source-versions"
    );

    vi.mocked(useArenaSourceVersions).mockReturnValue({
      versions: [
        { hash: "abc123", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      ],
      headVersion: "abc123",
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    vi.mocked(useArenaSourceVersionMutations).mockReturnValue({
      switchVersion: vi.fn(),
      switching: false,
      error: null,
    });

    render(<VersionSelector sourceName="test-source" />, { wrapper: createWrapper() });

    // Should show full hash
    expect(screen.getByText("abc123")).toBeInTheDocument();
  });
});
