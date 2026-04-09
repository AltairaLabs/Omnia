import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { OverviewTab } from "./overview-tab";
import type { Workspace } from "@/types/workspace";

const baseWorkspace: Workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: {
    name: "test-workspace",
    creationTimestamp: "2024-01-15T10:00:00Z",
  },
  spec: {
    displayName: "Test Workspace",
    description: "A test workspace",
    environment: "development",
    namespace: {
      name: "test-ns",
      create: true,
    },
  },
  status: {
    phase: "Ready",
    observedGeneration: 3,
    namespace: {
      name: "test-ns-status",
      created: true,
    },
    serviceAccounts: {
      owner: "workspace-owner-sa",
      editor: "workspace-editor-sa",
      viewer: "workspace-viewer-sa",
    },
    conditions: [
      {
        type: "Ready",
        status: "True",
        reason: "ReconcileSuccess",
        message: "Workspace is ready",
        lastTransitionTime: "2024-01-15T10:05:00Z",
      },
      {
        type: "NamespaceReady",
        status: "False",
        reason: "NamespaceError",
        message: "Failed to create namespace: permission denied",
        lastTransitionTime: "2024-01-15T10:01:00Z",
      },
    ],
  },
};

describe("OverviewTab", () => {
  it("renders phase badge with correct text", () => {
    render(<OverviewTab workspace={baseWorkspace} />);
    // Phase label appears next to "Phase" label
    const phaseLabel = screen.getByText("Phase");
    expect(phaseLabel.nextElementSibling?.textContent).toBe("Ready");
  });

  it("renders Pending badge when status is absent", () => {
    const workspace: Workspace = {
      ...baseWorkspace,
      status: undefined,
    };
    render(<OverviewTab workspace={workspace} />);
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("renders workspace detail values", () => {
    render(<OverviewTab workspace={baseWorkspace} />);
    expect(screen.getByText("Test Workspace")).toBeInTheDocument();
    expect(screen.getByText("development")).toBeInTheDocument();
    // Namespace comes from status.namespace.name
    expect(screen.getByText("test-ns-status")).toBeInTheDocument();
  });

  it("falls back to spec namespace when status namespace is absent", () => {
    const workspace: Workspace = {
      ...baseWorkspace,
      status: {
        ...baseWorkspace.status,
        namespace: undefined,
      },
    };
    render(<OverviewTab workspace={workspace} />);
    expect(screen.getByText("test-ns")).toBeInTheDocument();
  });

  it("renders service account names", () => {
    render(<OverviewTab workspace={baseWorkspace} />);
    expect(screen.getByText("workspace-owner-sa")).toBeInTheDocument();
    expect(screen.getByText("workspace-editor-sa")).toBeInTheDocument();
    expect(screen.getByText("workspace-viewer-sa")).toBeInTheDocument();
  });

  it("does not render service accounts card when status.serviceAccounts is absent", () => {
    const workspace: Workspace = {
      ...baseWorkspace,
      status: {
        ...baseWorkspace.status,
        serviceAccounts: undefined,
      },
    };
    render(<OverviewTab workspace={workspace} />);
    expect(screen.queryByText("Service Accounts")).not.toBeInTheDocument();
  });

  it("highlights error conditions with reason and message text", () => {
    render(<OverviewTab workspace={baseWorkspace} />);
    expect(screen.getByText("NamespaceError")).toBeInTheDocument();
    expect(
      screen.getByText("Failed to create namespace: permission denied")
    ).toBeInTheDocument();
  });

  it("renders all condition types", () => {
    render(<OverviewTab workspace={baseWorkspace} />);
    // Both condition types appear in the table
    expect(screen.getAllByText("Ready").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("NamespaceReady")).toBeInTheDocument();
  });

  it("shows No conditions reported when conditions list is empty", () => {
    const workspace: Workspace = {
      ...baseWorkspace,
      status: {
        ...baseWorkspace.status,
        conditions: [],
      },
    };
    render(<OverviewTab workspace={workspace} />);
    expect(screen.getByText("No conditions reported")).toBeInTheDocument();
  });

  it("handles missing status gracefully", () => {
    const workspace: Workspace = {
      ...baseWorkspace,
      status: undefined,
    };
    render(<OverviewTab workspace={workspace} />);
    // Phase defaults to Pending
    expect(screen.getByText("Pending")).toBeInTheDocument();
    // Details still render from spec
    expect(screen.getByText("Test Workspace")).toBeInTheDocument();
    // No conditions shown
    expect(screen.getByText("No conditions reported")).toBeInTheDocument();
  });
});
