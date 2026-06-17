import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockUseEnterpriseConfig, mockUseInstitutionalMemories, mockUseCreateInstitutionalMemory, mockUseDeleteInstitutionalMemory } =
  vi.hoisted(() => ({
    mockUseEnterpriseConfig: vi.fn(),
    mockUseInstitutionalMemories: vi.fn(),
    mockUseCreateInstitutionalMemory: vi.fn(),
    mockUseDeleteInstitutionalMemory: vi.fn(),
  }));

vi.mock("@/hooks/core", () => ({
  useEnterpriseConfig: () => mockUseEnterpriseConfig(),
}));

vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: string; description?: string }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </div>
  ),
}));

vi.mock("@/hooks/use-institutional-memories", () => ({
  useInstitutionalMemories: () => mockUseInstitutionalMemories(),
  useCreateInstitutionalMemory: () => mockUseCreateInstitutionalMemory(),
  useDeleteInstitutionalMemory: () => mockUseDeleteInstitutionalMemory(),
}));

import WorkspaceKnowledgePage from "./page";

describe("WorkspaceKnowledgePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default: enterprise enabled so existing/non-gate tests pass
    mockUseEnterpriseConfig.mockReturnValue({ enterpriseEnabled: true, hideEnterprise: false, loading: false });
    mockUseInstitutionalMemories.mockReturnValue({ data: { memories: [], total: 0 }, isLoading: false, error: null });
    mockUseCreateInstitutionalMemory.mockReturnValue({ mutateAsync: vi.fn(), isPending: false, isError: false });
    mockUseDeleteInstitutionalMemory.mockReturnValue({ mutate: vi.fn() });
  });

  it("renders the knowledge page header when enterprise is enabled", () => {
    render(<WorkspaceKnowledgePage />);
    expect(screen.getByTestId("header")).toBeInTheDocument();
    expect(screen.getByText("Workspace knowledge")).toBeInTheDocument();
  });

  it("gates the institutional knowledge view when enterprise is disabled", () => {
    mockUseEnterpriseConfig.mockReturnValue({ enterpriseEnabled: false, hideEnterprise: false, loading: false });
    render(<WorkspaceKnowledgePage />);
    expect(screen.getByText("Enterprise Feature")).toBeInTheDocument();
  });

  it("shows institutional knowledge panel when enterprise is enabled", () => {
    render(<WorkspaceKnowledgePage />);
    expect(screen.getByTestId("create-form")).toBeInTheDocument();
  });

  it("renders nothing when hideEnterprise is true", () => {
    mockUseEnterpriseConfig.mockReturnValue({ enterpriseEnabled: false, hideEnterprise: true, loading: false });
    render(<WorkspaceKnowledgePage />);
    expect(screen.queryByText("Enterprise Feature")).not.toBeInTheDocument();
    expect(screen.queryByTestId("create-form")).not.toBeInTheDocument();
  });
});
