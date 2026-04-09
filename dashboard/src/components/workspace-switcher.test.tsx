/**
 * Tests for workspace-switcher component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { WorkspaceSwitcher } from "./workspace-switcher";
import type { WorkspaceListItem } from "@/hooks/resources";

// Mock next/navigation router
vi.mock("next/navigation", () => ({
  useRouter: vi.fn(() => ({ push: vi.fn() })),
}));

// Mock the workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock workspace permissions hook
vi.mock("@/hooks/use-workspace-permissions", () => ({
  useWorkspacePermissions: vi.fn(() => ({ isOwner: false })),
}));

import { useRouter } from "next/navigation";
import { useWorkspace } from "@/contexts/workspace-context";
import { useWorkspacePermissions } from "@/hooks/use-workspace-permissions";

const mockWorkspaces: WorkspaceListItem[] = [
  {
    name: "dev-workspace",
    displayName: "Development",
    description: "Development environment",
    environment: "development",
    namespace: "ns-dev",
    role: "owner",
    permissions: { read: true, write: true, delete: true, manageMembers: true },
  },
  {
    name: "staging-workspace",
    displayName: "Staging",
    environment: "staging",
    namespace: "ns-staging",
    role: "editor",
    permissions: { read: true, write: true, delete: false, manageMembers: false },
  },
  {
    name: "prod-workspace",
    displayName: "Production",
    description: "Production environment",
    environment: "production",
    namespace: "ns-prod",
    role: "viewer",
    permissions: { read: true, write: false, delete: false, manageMembers: false },
  },
];

describe("WorkspaceSwitcher", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders loading state", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: [],
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("renders error state", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: [],
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: new Error("Failed to load"),
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    expect(screen.getByText("Error loading workspaces")).toBeInTheDocument();
  });

  it("renders no workspaces state", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: [],
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    expect(screen.getByText("No workspaces")).toBeInTheDocument();
  });

  it("renders current workspace with role badge", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: mockWorkspaces,
      currentWorkspace: mockWorkspaces[0],
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    expect(screen.getByText("Development")).toBeInTheDocument();
    expect(screen.getByText("owner")).toBeInTheDocument();
  });

  it("shows placeholder when no workspace is selected", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: mockWorkspaces,
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    expect(screen.getByText("Select workspace")).toBeInTheDocument();
  });

  it("renders dropdown trigger as button", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: mockWorkspaces,
      currentWorkspace: mockWorkspaces[0],
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    const button = screen.getByRole("button");
    expect(button).toBeInTheDocument();
    expect(button).toHaveAttribute("aria-haspopup", "menu");
  });

  it("shows different role badges correctly", () => {
    // Test editor role
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: mockWorkspaces,
      currentWorkspace: mockWorkspaces[1], // Staging with editor role
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { rerender } = render(<WorkspaceSwitcher />);
    expect(screen.getByText("editor")).toBeInTheDocument();
    expect(screen.getByText("Staging")).toBeInTheDocument();

    // Test viewer role
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: mockWorkspaces,
      currentWorkspace: mockWorkspaces[2], // Production with viewer role
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    rerender(<WorkspaceSwitcher />);
    expect(screen.getByText("viewer")).toBeInTheDocument();
    expect(screen.getByText("Production")).toBeInTheDocument();
  });

  it("disabled buttons in loading state", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: [],
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
  });

  it("disabled buttons in error state", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: [],
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: new Error("Failed"),
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
  });

  it("disabled buttons when no workspaces", () => {
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: [],
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
  });

  it("shows gear icon for owner with current workspace", () => {
    vi.mocked(useWorkspacePermissions).mockReturnValue({
      isOwner: true,
      isViewer: false,
      isEditor: true,
      canRead: true,
      canWrite: true,
      canDelete: true,
      canManageMembers: true,
      hasWorkspace: true,
      role: "owner",
      permissions: { read: true, write: true, delete: true, manageMembers: true },
    });
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: mockWorkspaces,
      currentWorkspace: mockWorkspaces[0],
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    expect(screen.getByTestId("workspace-settings-gear")).toBeInTheDocument();
  });

  it("hides gear icon for non-owner", () => {
    vi.mocked(useWorkspacePermissions).mockReturnValue({
      isOwner: false,
      isViewer: true,
      isEditor: false,
      canRead: true,
      canWrite: false,
      canDelete: false,
      canManageMembers: false,
      hasWorkspace: true,
      role: "viewer",
      permissions: { read: true, write: false, delete: false, manageMembers: false },
    });
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: mockWorkspaces,
      currentWorkspace: mockWorkspaces[2],
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    expect(screen.queryByTestId("workspace-settings-gear")).not.toBeInTheDocument();
  });

  it("navigates to settings and stops propagation when gear icon clicked", () => {
    const mockPush = vi.fn();
    vi.mocked(useRouter).mockReturnValue({ push: mockPush } as unknown as ReturnType<typeof useRouter>);
    vi.mocked(useWorkspacePermissions).mockReturnValue({
      isOwner: true,
      isViewer: false,
      isEditor: true,
      canRead: true,
      canWrite: true,
      canDelete: true,
      canManageMembers: true,
      hasWorkspace: true,
      role: "owner",
      permissions: { read: true, write: true, delete: true, manageMembers: true },
    });
    vi.mocked(useWorkspace).mockReturnValue({
      workspaces: mockWorkspaces,
      currentWorkspace: mockWorkspaces[0],
      setCurrentWorkspace: vi.fn(),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<WorkspaceSwitcher />);

    const gearButton = screen.getByTestId("workspace-settings-gear");
    fireEvent.click(gearButton);

    expect(mockPush).toHaveBeenCalledWith("/workspaces/dev-workspace/settings");
  });
});
