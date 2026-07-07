/**
 * Tests for the shared service-health UI bits (badge, row, inline log view)
 * used by the workspace settings Services tab.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { ServiceRow, StatusBadge } from "./service-health-bits";
import type { ServiceHealth } from "@/lib/k8s/service-health";

function jsonResponse(body: unknown, ok = true) {
  return Promise.resolve({
    ok,
    status: ok ? 200 : 500,
    json: () => Promise.resolve(body),
  });
}

const readyService: ServiceHealth = {
  service: "session-api",
  state: "ready",
  ready: true,
  restarts: 0,
};

const crashloopingService: ServiceHealth = {
  service: "memory-api",
  state: "crashlooping",
  ready: false,
  restarts: 4,
  reason: "CrashLoopBackOff: OOMKilled",
};

describe("StatusBadge", () => {
  it.each([
    ["ready", "✔ Ready"],
    ["crashlooping", "✖ Crashlooping"],
    ["pending", "⏳ Pending"],
    ["notDeployed", "○ Not deployed"],
    ["unknown", "? Unknown"],
  ] as const)("renders the %s badge label", (state, label) => {
    render(<StatusBadge state={state} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });
});

describe("ServiceRow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the badge, name, and restart count", () => {
    render(
      <ServiceRow workspaceName="demo" groupName="default" service={readyService} />
    );
    expect(screen.getByText("session-api")).toBeInTheDocument();
    expect(screen.getByText(/restarts:\s*0/)).toBeInTheDocument();
    expect(screen.getByText("✔ Ready")).toBeInTheDocument();
  });

  it("renders the reason text when present", () => {
    render(
      <ServiceRow workspaceName="demo" groupName="default" service={crashloopingService} />
    );
    expect(screen.getByText("CrashLoopBackOff: OOMKilled")).toBeInTheDocument();
  });

  it("fetches and renders log entries when [logs] is clicked", async () => {
    global.fetch = vi.fn(() =>
      jsonResponse({ logs: [{ timestamp: "2026-07-03T00:00:00Z", level: "error", message: "boom" }] })
    ) as unknown as typeof fetch;

    render(
      <ServiceRow workspaceName="demo" groupName="default" service={readyService} />
    );
    fireEvent.click(screen.getByRole("button", { name: /logs/i }));

    await waitFor(() => {
      expect(screen.getByText("boom")).toBeInTheDocument();
    });
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/workspaces/demo/services/default/session-api/logs?tailLines=100"
    );
  });

  it("shows a loading state while logs are in flight", async () => {
    global.fetch = vi.fn(() => new Promise(() => {})) as unknown as typeof fetch;

    render(
      <ServiceRow workspaceName="demo" groupName="default" service={readyService} />
    );
    fireEvent.click(screen.getByRole("button", { name: /logs/i }));

    expect(screen.getByText(/loading logs/i)).toBeInTheDocument();
  });

  it("shows an empty state when there are no log entries", async () => {
    global.fetch = vi.fn(() => jsonResponse({ logs: [] })) as unknown as typeof fetch;

    render(
      <ServiceRow workspaceName="demo" groupName="default" service={readyService} />
    );
    fireEvent.click(screen.getByRole("button", { name: /logs/i }));

    await waitFor(() => {
      expect(screen.getByText(/no logs yet/i)).toBeInTheDocument();
    });
  });

  it("shows an error state when the logs fetch fails", async () => {
    global.fetch = vi.fn(() => jsonResponse({}, false)) as unknown as typeof fetch;

    render(
      <ServiceRow workspaceName="demo" groupName="default" service={readyService} />
    );
    fireEvent.click(screen.getByRole("button", { name: /logs/i }));

    await waitFor(() => {
      expect(screen.getByText(/failed to load logs/i)).toBeInTheDocument();
    });
  });

  it("shows an error state when the logs fetch throws", async () => {
    global.fetch = vi.fn(() => Promise.reject(new Error("network down"))) as unknown as typeof fetch;

    render(
      <ServiceRow workspaceName="demo" groupName="default" service={readyService} />
    );
    fireEvent.click(screen.getByRole("button", { name: /logs/i }));

    await waitFor(() => {
      expect(screen.getByText(/failed to load logs/i)).toBeInTheDocument();
    });
  });

  it("toggles the log view closed when [logs] is clicked again", async () => {
    global.fetch = vi.fn(() => jsonResponse({ logs: [] })) as unknown as typeof fetch;

    render(
      <ServiceRow workspaceName="demo" groupName="default" service={readyService} />
    );
    const row = screen.getByTestId("service-row-session-api");
    const logsButton = within(row).getByRole("button", { name: /logs/i });

    fireEvent.click(logsButton);
    await waitFor(() => {
      expect(screen.getByText(/no logs yet/i)).toBeInTheDocument();
    });

    fireEvent.click(logsButton);
    expect(screen.queryByText(/no logs yet/i)).not.toBeInTheDocument();
  });
});
