/**
 * Tests for individual shared tool registry API route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

// Mock dependencies
vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  getSharedCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
}));

vi.mock("@/lib/audit", () => ({
  logError: vi.fn(),
}));

describe("GET /api/shared/toolregistries/:registryName", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns 401 for anonymous users", async () => {
    const { getUser } = await import("@/lib/auth");
    vi.mocked(getUser).mockResolvedValue({
      id: "anonymous",
      provider: "anonymous",
      username: "anonymous",
      groups: [],
      role: "viewer",
    });

    const { GET } = await import("./route");
    const request = new NextRequest("http://localhost/api/shared/toolregistries/default-tools");
    const context = { params: Promise.resolve({ registryName: "default-tools" }) };

    const response = await GET(request, context);

    expect(response.status).toBe(401);
  });

  it("returns tool registry for authenticated users", async () => {
    const { getUser } = await import("@/lib/auth");
    const { getSharedCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue({
      id: "testuser-id",
      provider: "oauth",
      username: "testuser",
      email: "test@example.com",
      groups: ["users"],
      role: "viewer",
    });

    const mockToolRegistry = {
      metadata: { name: "default-tools", namespace: "omnia-system" },
      spec: { tools: [{ name: "web-search" }] },
    };
    vi.mocked(getSharedCrd).mockResolvedValue(mockToolRegistry);

    const { GET } = await import("./route");
    const request = new NextRequest("http://localhost/api/shared/toolregistries/default-tools");
    const context = { params: Promise.resolve({ registryName: "default-tools" }) };

    const response = await GET(request, context);

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("default-tools");
    expect(getSharedCrd).toHaveBeenCalledWith("toolregistries", "omnia-system", "default-tools");
  });

  it("returns 404 when tool registry not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { getSharedCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue({
      id: "testuser-id",
      provider: "oauth",
      username: "testuser",
      email: "test@example.com",
      groups: ["users"],
      role: "viewer",
    });

    vi.mocked(getSharedCrd).mockResolvedValue(null);

    const { GET } = await import("./route");
    const request = new NextRequest("http://localhost/api/shared/toolregistries/nonexistent");
    const context = { params: Promise.resolve({ registryName: "nonexistent" }) };

    const response = await GET(request, context);

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toBe("Not Found");
  });

  it("returns 500 on error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { getSharedCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue({
      id: "testuser-id",
      provider: "oauth",
      username: "testuser",
      email: "test@example.com",
      groups: ["users"],
      role: "viewer",
    });

    vi.mocked(getSharedCrd).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const request = new NextRequest("http://localhost/api/shared/toolregistries/default-tools");
    const context = { params: Promise.resolve({ registryName: "default-tools" }) };

    const response = await GET(request, context);

    expect(response.status).toBe(500);
  });
});
