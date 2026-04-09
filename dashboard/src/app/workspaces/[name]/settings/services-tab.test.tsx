import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ServicesTab } from "./services-tab";
import type { Workspace } from "@/types/workspace";

const workspace: Workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "test-ws" },
  spec: {
    displayName: "Test",
    environment: "development",
    namespace: { name: "test-ns" },
    services: [
      {
        name: "default",
        mode: "managed",
        session: { database: { secretRef: { name: "pg-secret" } } },
        memory: { database: { secretRef: { name: "pg-secret" } } },
      },
    ],
  },
  status: {
    phase: "Ready",
    services: [
      {
        name: "default",
        sessionURL: "https://session-test-ns-default:8080",
        memoryURL: "https://memory-test-ns-default:8080",
        ready: true,
      },
    ],
  },
};

describe("ServicesTab", () => {
  it("renders service group name and mode badge", () => {
    render(<ServicesTab workspace={workspace} />);
    expect(screen.getByText("default")).toBeInTheDocument();
    expect(screen.getByText("managed")).toBeInTheDocument();
  });

  it("renders session and memory URLs", () => {
    render(<ServicesTab workspace={workspace} />);
    expect(
      screen.getByText("https://session-test-ns-default:8080")
    ).toBeInTheDocument();
    expect(
      screen.getByText("https://memory-test-ns-default:8080")
    ).toBeInTheDocument();
  });

  it("shows ready indicator when service is ready", () => {
    render(<ServicesTab workspace={workspace} />);
    expect(screen.getAllByTestId("status-ready").length).toBeGreaterThan(0);
  });

  it("shows not-ready indicator when service is not ready", () => {
    const notReadyWorkspace: Workspace = {
      ...workspace,
      status: {
        phase: "Ready",
        services: [
          {
            name: "default",
            sessionURL: "https://session-test-ns-default:8080",
            memoryURL: "https://memory-test-ns-default:8080",
            ready: false,
          },
        ],
      },
    };
    render(<ServicesTab workspace={notReadyWorkspace} />);
    expect(screen.getAllByTestId("status-not-ready").length).toBeGreaterThan(0);
  });

  it("shows provisioning message when status.services is empty", () => {
    const provisioningWorkspace: Workspace = {
      ...workspace,
      status: {
        phase: "Pending",
        services: [],
      },
    };
    render(<ServicesTab workspace={provisioningWorkspace} />);
    expect(screen.getByText("Services being provisioned")).toBeInTheDocument();
  });

  it("shows no service groups notice when spec.services is undefined", () => {
    const noServicesWorkspace: Workspace = {
      ...workspace,
      spec: {
        ...workspace.spec,
        services: undefined,
      },
    };
    render(<ServicesTab workspace={noServicesWorkspace} />);
    expect(
      screen.getByText("No service groups configured")
    ).toBeInTheDocument();
  });

  it("shows no service groups notice when spec.services is empty", () => {
    const noServicesWorkspace: Workspace = {
      ...workspace,
      spec: {
        ...workspace.spec,
        services: [],
      },
    };
    render(<ServicesTab workspace={noServicesWorkspace} />);
    expect(
      screen.getByText("No service groups configured")
    ).toBeInTheDocument();
  });

  it("renders database secret ref name", () => {
    render(<ServicesTab workspace={workspace} />);
    expect(screen.getAllByText("pg-secret").length).toBeGreaterThan(0);
  });
});
