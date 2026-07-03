import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";

const { mockUseWorkspace, mockUseSearchParams } = vi.hoisted(() => ({
  mockUseWorkspace: vi.fn(),
  mockUseSearchParams: vi.fn(),
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => mockUseWorkspace(),
}));

// Header pulls in query-client/theme infrastructure not relevant here.
vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: string; description?: string }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </div>
  ),
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => mockUseSearchParams(),
}));

vi.mock("@/components/services/service-health-panel", () => ({
  ServiceHealthPanel: ({
    workspaceName,
    initialExpandedGroup,
  }: {
    workspaceName: string;
    initialExpandedGroup?: string;
  }) => (
    <div data-testid="service-health-panel">
      {workspaceName}:{initialExpandedGroup ?? "none"}
    </div>
  ),
}));

import ServicesPage from "./page";

describe("ServicesPage", () => {
  it("renders a loading skeleton while the workspace is loading", () => {
    mockUseWorkspace.mockReturnValue({ currentWorkspace: null, isLoading: true });
    mockUseSearchParams.mockReturnValue(new URLSearchParams());

    render(<ServicesPage />);

    expect(screen.getByText("Services")).toBeInTheDocument();
    expect(screen.queryByTestId("service-health-panel")).not.toBeInTheDocument();
  });

  it("renders a no-workspace notice when there is no current workspace", () => {
    mockUseWorkspace.mockReturnValue({ currentWorkspace: null, isLoading: false });
    mockUseSearchParams.mockReturnValue(new URLSearchParams());

    render(<ServicesPage />);

    expect(screen.getByTestId("no-workspace-notice")).toBeInTheDocument();
  });

  it("renders the ServiceHealthPanel with the workspace name and group deep-link", () => {
    mockUseWorkspace.mockReturnValue({
      currentWorkspace: { name: "demo" },
      isLoading: false,
    });
    mockUseSearchParams.mockReturnValue(new URLSearchParams("?group=grp-1"));

    render(<ServicesPage />);

    expect(screen.getByTestId("service-health-panel")).toHaveTextContent("demo:grp-1");
  });
});
