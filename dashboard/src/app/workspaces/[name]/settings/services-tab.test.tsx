import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within, fireEvent } from "@testing-library/react";
import { ServicesTab } from "./services-tab";
import type { Workspace } from "@/types/workspace";
import type { WorkspaceServicesHealth } from "@/lib/k8s/service-health";

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

// Session healthy, memory crashlooping — SAME group. Proves the per-service
// badge replaces the old group-level ready flag applied to both members.
const mixedHealth: WorkspaceServicesHealth = {
  workspaceServices: [
    { service: "privacy-api", state: "ready", ready: true, restarts: 0 },
  ],
  groups: [
    {
      name: "default",
      ready: false,
      members: [
        { service: "session-api", state: "ready", ready: true, restarts: 0 },
        {
          service: "memory-api",
          state: "crashlooping",
          ready: false,
          restarts: 4,
          reason: "CrashLoopBackOff: OOMKilled",
        },
      ],
    },
  ],
  source: "crd",
};

function jsonResponse(body: unknown, ok = true) {
  return Promise.resolve({
    ok,
    status: ok ? 200 : 500,
    json: () => Promise.resolve(body),
  });
}

function mockFetchWith(healthBody: unknown, logsBody: unknown = { logs: [] }) {
  global.fetch = vi.fn((input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input.toString();
    if (url.includes("/logs")) {
      return jsonResponse(logsBody);
    }
    return jsonResponse(healthBody);
  }) as unknown as typeof fetch;
}

describe("ServicesTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders service group name and mode badge", async () => {
    mockFetchWith(mixedHealth);
    render(<ServicesTab workspace={workspace} />);
    await waitFor(() => {
      expect(screen.getByText("session-api")).toBeInTheDocument();
    });
    expect(screen.getByText("default")).toBeInTheDocument();
    expect(screen.getByText("managed")).toBeInTheDocument();
  });

  it("renders session and memory URLs", async () => {
    mockFetchWith(mixedHealth);
    render(<ServicesTab workspace={workspace} />);
    await waitFor(() => {
      expect(
        screen.getByText("https://session-test-ns-default:8080")
      ).toBeInTheDocument();
    });
    expect(
      screen.getByText("https://memory-test-ns-default:8080")
    ).toBeInTheDocument();
  });

  it("shows independent per-service badges within the same group (no group-flag bug)", async () => {
    mockFetchWith(mixedHealth);
    render(<ServicesTab workspace={workspace} />);

    const sessionRow = await screen.findByTestId("service-row-session-api");
    const memoryRow = screen.getByTestId("service-row-memory-api");

    expect(within(sessionRow).getByText("✔ Ready")).toBeInTheDocument();
    expect(within(memoryRow).getByText("✖ Crashlooping")).toBeInTheDocument();
  });

  it("shows the crashlooping reason text", async () => {
    mockFetchWith(mixedHealth);
    render(<ServicesTab workspace={workspace} />);
    await waitFor(() => {
      expect(screen.getByText("CrashLoopBackOff: OOMKilled")).toBeInTheDocument();
    });
  });

  it("shows restart counts per service", async () => {
    mockFetchWith(mixedHealth);
    render(<ServicesTab workspace={workspace} />);
    const memoryRow = await screen.findByTestId("service-row-memory-api");
    expect(within(memoryRow).getByText(/restarts:\s*4/)).toBeInTheDocument();
  });

  it("renders a privacy-api card from workspace-level services", async () => {
    mockFetchWith(mixedHealth);
    render(<ServicesTab workspace={workspace} />);
    await waitFor(() => {
      expect(screen.getByText("Workspace services")).toBeInTheDocument();
    });
    expect(screen.getByTestId("service-row-privacy-api")).toBeInTheDocument();
  });

  it("fetches logs for a service when [logs] is clicked", async () => {
    mockFetchWith(mixedHealth, {
      logs: [{ timestamp: "2026-07-03T00:00:00Z", level: "error", message: "boom" }],
    });
    render(<ServicesTab workspace={workspace} />);

    const memoryRow = await screen.findByTestId("service-row-memory-api");
    fireEvent.click(within(memoryRow).getByRole("button", { name: /logs/i }));

    await waitFor(() => {
      expect(within(memoryRow).getByText("boom")).toBeInTheDocument();
    });
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/workspaces/test-ws/services/default/memory-api/logs?tailLines=100"
    );
  });

  it("shows a loading state while service health is in flight", () => {
    global.fetch = vi.fn(() => new Promise(() => {})) as unknown as typeof fetch;
    render(<ServicesTab workspace={workspace} />);
    expect(screen.getByTestId("services-health-loading")).toBeInTheDocument();
    expect(screen.queryByTestId("service-row-session-api")).not.toBeInTheDocument();
  });

  it("shows an error alert when the health fetch fails", async () => {
    global.fetch = vi.fn(() => jsonResponse({}, false)) as unknown as typeof fetch;
    render(<ServicesTab workspace={workspace} />);
    await waitFor(() => {
      expect(screen.getByText(/could not load service health/i)).toBeInTheDocument();
    });
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
    expect(global.fetch).not.toHaveBeenCalled();
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

  it("renders database secret ref name", async () => {
    mockFetchWith(mixedHealth);
    render(<ServicesTab workspace={workspace} />);
    await waitFor(() => {
      expect(screen.getAllByText("pg-secret").length).toBeGreaterThan(0);
    });
  });

  it("falls back to unknown state for a service missing from the health response", async () => {
    mockFetchWith({ workspaceServices: [], groups: [], source: "unknown" });
    render(<ServicesTab workspace={workspace} />);
    const sessionRow = await screen.findByTestId("service-row-session-api");
    expect(within(sessionRow).getByText("? Unknown")).toBeInTheDocument();
  });
});
