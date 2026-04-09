import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import WorkspaceSettingsPage from "./page";

const mockUseWorkspaceDetail = vi.fn();
const mockUseWorkspacePatch = vi.fn();

vi.mock("@/hooks/use-workspace-detail", () => ({
  useWorkspaceDetail: () => mockUseWorkspaceDetail(),
  useWorkspacePatch: () => mockUseWorkspacePatch(),
}));

vi.mock("next/navigation", () => ({
  useParams: () => ({ name: "test-ws" }),
}));

vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: string; description?: string }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </div>
  ),
}));

const workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "test-ws" },
  spec: {
    displayName: "Test Workspace",
    environment: "development",
    namespace: { name: "test-ns" },
  },
  status: { phase: "Ready" },
};

describe("WorkspaceSettingsPage", () => {
  beforeEach(() => {
    mockUseWorkspacePatch.mockReturnValue({ mutate: vi.fn() });
  });

  it("renders header with 'Workspace Settings'", () => {
    mockUseWorkspaceDetail.mockReturnValue({
      data: workspace,
      isLoading: false,
      error: null,
    });
    render(<WorkspaceSettingsPage />);
    expect(screen.getByTestId("header")).toBeInTheDocument();
    expect(screen.getByText("Workspace Settings")).toBeInTheDocument();
  });

  it("renders three tab triggers (Overview, Services, Access)", () => {
    mockUseWorkspaceDetail.mockReturnValue({
      data: workspace,
      isLoading: false,
      error: null,
    });
    render(<WorkspaceSettingsPage />);
    expect(screen.getByRole("tab", { name: "Overview" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Services" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Access" })).toBeInTheDocument();
  });

  it("shows loading skeleton when isLoading", () => {
    mockUseWorkspaceDetail.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });
    render(<WorkspaceSettingsPage />);
    expect(screen.getByTestId("settings-loading")).toBeInTheDocument();
  });

  it("shows error alert on fetch failure", () => {
    mockUseWorkspaceDetail.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch workspace"),
    });
    render(<WorkspaceSettingsPage />);
    expect(screen.getByText("Failed to fetch workspace")).toBeInTheDocument();
  });

  it("switches to Services tab and shows no service groups message", async () => {
    mockUseWorkspaceDetail.mockReturnValue({
      data: workspace,
      isLoading: false,
      error: null,
    });
    const user = userEvent.setup();
    render(<WorkspaceSettingsPage />);
    await user.click(screen.getByRole("tab", { name: "Services" }));
    expect(screen.getByText("No service groups configured")).toBeInTheDocument();
  });
});
