/**
 * Tests for workspace eval-results discover proxy route.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/service-url-resolver", () => ({
  resolveServiceURLs: vi.fn(),
}));

const mockUser = {
  id: "testuser-id",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: ["users"],
  role: "viewer" as const,
};

const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };

const mockFetch = vi.fn();
global.fetch = mockFetch;

function makeRequest(): NextRequest {
  return new NextRequest(
    "http://localhost:3000/api/workspaces/test-ws/eval-results/discover",
    { method: "GET" }
  );
}

function makeContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

describe("GET /api/workspaces/[name]/eval-results/discover", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  async function setupAuth() {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({
      sessionURL: "https://session-api:8080",
      memoryURL: "https://memory-api:8080", namespace: "omnia-test", privacyURL: ""
    });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
  }

  it("returns the list of distinct evals for the workspace", async () => {
    await setupAuth();

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve({
          evals: [
            { evalId: "acc", evalType: "llm_judge" },
            { evalId: "lat", evalType: "assertion" },
          ],
        }),
    });

    const { GET } = await import("./route");
    const response = await GET(makeRequest(), makeContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.evals).toHaveLength(2);
    expect(body.evals[0].evalId).toBe("acc");

    const calledURL = mockFetch.mock.calls[0][0] as string;
    expect(calledURL).toBe(
      "https://session-api:8080/api/v1/eval-results/discover?namespace=omnia-test"
    );
  });

  it("returns 503 when session-api URL cannot be resolved", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(null);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { GET } = await import("./route");
    const response = await GET(makeRequest(), makeContext());

    expect(response.status).toBe(503);
    const body = await response.json();
    expect(body.evals).toEqual([]);
  });

  it("returns 502 when session-api fetch throws", async () => {
    await setupAuth();
    mockFetch.mockRejectedValueOnce(new Error("network down"));

    const { GET } = await import("./route");
    const response = await GET(makeRequest(), makeContext());

    expect(response.status).toBe(502);
    const body = await response.json();
    expect(body.evals).toEqual([]);
  });
});
