/**
 * Tests for ServiceHealthPanel.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { ServiceHealthPanel } from "./service-health-panel";
import type { WorkspaceServicesHealth } from "@/lib/k8s/service-health";

const healthyGroupsResponse: WorkspaceServicesHealth = {
  workspaceServices: [
    {
      service: "privacy-api",
      state: "ready",
      ready: true,
      restarts: 0,
    },
  ],
  groups: [
    {
      name: "grp-1",
      ready: false,
      members: [
        {
          service: "session-api",
          state: "ready",
          ready: true,
          restarts: 0,
        },
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

const emptyResponse: WorkspaceServicesHealth = {
  workspaceServices: [],
  groups: [],
  source: "unknown",
};

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
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

describe("ServiceHealthPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders badges, restart counts, reason text, and blocked-by for groups", async () => {
    mockFetchWith(healthyGroupsResponse);

    render(<ServiceHealthPanel workspaceName="demo" />);

    await waitFor(() => {
      expect(screen.getByText("session-api")).toBeInTheDocument();
    });

    // Group is auto-expanded when it's not ready, so members are visible.
    expect(screen.getByText("memory-api")).toBeInTheDocument();
    expect(screen.getByText(/blocked by: memory-api/i)).toBeInTheDocument();

    // Badges for each state.
    expect(screen.getAllByText(/ready/i).length).toBeGreaterThan(0);
    expect(screen.getByText(/crashlooping/i)).toBeInTheDocument();

    // Restart count and reason text.
    expect(screen.getByText(/restarts:\s*4/i)).toBeInTheDocument();
    expect(screen.getByText("CrashLoopBackOff: OOMKilled")).toBeInTheDocument();

    // Workspace-level privacy-api.
    expect(screen.getByText("privacy-api")).toBeInTheDocument();
  });

  it("fetches logs for a service when [logs] is clicked", async () => {
    mockFetchWith(healthyGroupsResponse);

    render(<ServiceHealthPanel workspaceName="demo" />);

    await waitFor(() => {
      expect(screen.getByText("memory-api")).toBeInTheDocument();
    });

    const memoryRow = screen.getByTestId("service-row-memory-api");
    const logsButton = within(memoryRow).getByRole("button", { name: /logs/i });
    fireEvent.click(logsButton);

    await waitFor(() => {
      const calls = (global.fetch as ReturnType<typeof vi.fn>).mock.calls;
      const logsCall = calls.find(([input]) => {
        const url = typeof input === "string" ? input : String(input);
        return url.includes("/logs");
      });
      expect(logsCall).toBeDefined();
      expect(String(logsCall?.[0])).toBe(
        "/api/workspaces/demo/services/grp-1/memory-api/logs?tailLines=100"
      );
    });
  });

  it("renders an empty state when there are no services", async () => {
    mockFetchWith(emptyResponse);

    render(<ServiceHealthPanel workspaceName="demo" />);

    await waitFor(() => {
      expect(
        screen.getByText(/no services reported for this workspace/i)
      ).toBeInTheDocument();
    });
  });

  it("renders an error alert when the health fetch fails", async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({}) })
    ) as unknown as typeof fetch;

    render(<ServiceHealthPanel workspaceName="demo" />);

    await waitFor(() => {
      expect(
        screen.getByText(/could not load service health/i)
      ).toBeInTheDocument();
    });
  });

  it("re-fetches health when Refresh is clicked", async () => {
    mockFetchWith(healthyGroupsResponse);

    render(<ServiceHealthPanel workspaceName="demo" />);

    await waitFor(() => {
      expect(screen.getByText("session-api")).toBeInTheDocument();
    });

    const callsBefore = (global.fetch as ReturnType<typeof vi.fn>).mock.calls.length;
    fireEvent.click(screen.getByRole("button", { name: /refresh/i }));

    await waitFor(() => {
      expect((global.fetch as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(
        callsBefore
      );
    });
  });

  it("renders an error message when the logs fetch fails", async () => {
    global.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : input.toString();
      if (url.includes("/logs")) {
        return Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({}) });
      }
      return jsonResponse(healthyGroupsResponse);
    }) as unknown as typeof fetch;

    render(<ServiceHealthPanel workspaceName="demo" />);

    await waitFor(() => {
      expect(screen.getByText("memory-api")).toBeInTheDocument();
    });

    const memoryRow = screen.getByTestId("service-row-memory-api");
    fireEvent.click(within(memoryRow).getByRole("button", { name: /logs/i }));

    await waitFor(() => {
      expect(screen.getByText(/failed to load logs/i)).toBeInTheDocument();
    });
  });

  it("renders log entries once the logs fetch resolves", async () => {
    mockFetchWith(healthyGroupsResponse, {
      logs: [
        { timestamp: "2026-07-03T00:00:00Z", level: "error", message: "boom" },
      ],
    });

    render(<ServiceHealthPanel workspaceName="demo" />);

    await waitFor(() => {
      expect(screen.getByText("memory-api")).toBeInTheDocument();
    });

    const memoryRow = screen.getByTestId("service-row-memory-api");
    fireEvent.click(within(memoryRow).getByRole("button", { name: /logs/i }));

    await waitFor(() => {
      expect(screen.getByText("boom")).toBeInTheDocument();
    });
  });
});
