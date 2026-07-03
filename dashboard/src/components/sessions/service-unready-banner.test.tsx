/**
 * Tests for ServiceUnreadyBanner.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { ServiceUnreadyBanner } from "./service-unready-banner";
import type { WorkspaceServicesHealth } from "@/lib/k8s/service-health";

const crashloopingResponse: WorkspaceServicesHealth = {
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
      name: "default",
      ready: false,
      members: [
        {
          service: "memory-api",
          state: "crashlooping",
          ready: false,
          restarts: 5,
          reason: "CrashLoopBackOff: OOMKilled",
        },
        {
          service: "session-api",
          state: "ready",
          ready: true,
          restarts: 0,
        },
      ],
    },
  ],
  source: "crd",
};

const healthyResponse: WorkspaceServicesHealth = {
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
      name: "default",
      ready: true,
      members: [
        {
          service: "memory-api",
          state: "ready",
          ready: true,
          restarts: 0,
        },
        {
          service: "session-api",
          state: "ready",
          ready: true,
          restarts: 0,
        },
      ],
    },
  ],
  source: "crd",
};

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    json: () => Promise.resolve(body),
  });
}

function mockFetchWith(body: unknown) {
  global.fetch = vi.fn(() => jsonResponse(body)) as unknown as typeof fetch;
}

describe("ServiceUnreadyBanner", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("names the crashlooping service and links to the affected group", async () => {
    mockFetchWith(crashloopingResponse);

    render(<ServiceUnreadyBanner workspaceName="demo" />);

    await waitFor(() => {
      expect(screen.getByText(/memory-api/)).toBeInTheDocument();
    });

    expect(screen.getByText(/service group 'default' not ready/i)).toBeInTheDocument();
    expect(screen.getByText(/memory-api unhealthy/i)).toBeInTheDocument();

    const link = screen.getByRole("link", { name: /open services/i });
    expect(link).toHaveAttribute("href", "/services?group=default");

    expect(global.fetch).toHaveBeenCalledWith("/api/workspaces/demo/services");
  });

  it("falls back to the first group when 'default' is absent", async () => {
    mockFetchWith({
      ...crashloopingResponse,
      groups: [{ ...crashloopingResponse.groups[0], name: "grp-1" }],
    });

    render(<ServiceUnreadyBanner workspaceName="demo" />);

    await waitFor(() => {
      expect(screen.getByText(/memory-api/)).toBeInTheDocument();
    });

    expect(screen.getByText(/service group 'grp-1' not ready/i)).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /open services/i });
    expect(link).toHaveAttribute("href", "/services?group=grp-1");
  });

  it("renders nothing when all group members are ready", async () => {
    mockFetchWith(healthyResponse);

    render(<ServiceUnreadyBanner workspaceName="demo" />);

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalled();
    });

    expect(screen.queryByText(/not ready/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /open services/i })).not.toBeInTheDocument();
  });

  it("renders nothing when the services fetch fails", async () => {
    global.fetch = vi.fn(() =>
      Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({}) })
    ) as unknown as typeof fetch;

    render(<ServiceUnreadyBanner workspaceName="demo" />);

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalled();
    });

    expect(screen.queryByText(/not ready/i)).not.toBeInTheDocument();
  });

  it("renders nothing when the services fetch throws (network error)", async () => {
    global.fetch = vi.fn(() => Promise.reject(new Error("network down"))) as unknown as typeof fetch;

    render(<ServiceUnreadyBanner workspaceName="demo" />);

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalled();
    });

    expect(screen.queryByText(/not ready/i)).not.toBeInTheDocument();
  });

  it("renders nothing when there are no groups reported", async () => {
    mockFetchWith({ workspaceServices: [], groups: [], source: "unknown" });

    render(<ServiceUnreadyBanner workspaceName="demo" />);

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalled();
    });

    expect(screen.queryByText(/not ready/i)).not.toBeInTheDocument();
  });
});
