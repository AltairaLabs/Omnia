/**
 * Tests for individual shared provider API route.
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

describe("GET /api/shared/providers/:providerName", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("allows anonymous users (read-only public data)", async () => {
    const { getSharedCrd } = await import("@/lib/k8s/crd-operations");

    const mockProvider = {
      metadata: { name: "openai", namespace: "omnia-system" },
      spec: { type: "openai", models: ["gpt-4"] },
    };
    vi.mocked(getSharedCrd).mockResolvedValue(mockProvider);

    const { GET } = await import("./route");
    const request = new NextRequest("http://localhost/api/shared/providers/openai");
    const context = { params: Promise.resolve({ providerName: "openai" }) };

    const response = await GET(request, context);

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("openai");
  });

  it("returns provider for authenticated users", async () => {
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

    const mockProvider = {
      metadata: { name: "openai", namespace: "omnia-system" },
      spec: { type: "openai", models: ["gpt-4"] },
    };
    vi.mocked(getSharedCrd).mockResolvedValue(mockProvider);

    const { GET } = await import("./route");
    const request = new NextRequest("http://localhost/api/shared/providers/openai");
    const context = { params: Promise.resolve({ providerName: "openai" }) };

    const response = await GET(request, context);

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("openai");
    expect(getSharedCrd).toHaveBeenCalledWith("providers", "omnia-system", "openai");
  });

  it("returns 404 when provider not found", async () => {
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
    const request = new NextRequest("http://localhost/api/shared/providers/nonexistent");
    const context = { params: Promise.resolve({ providerName: "nonexistent" }) };

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
    const request = new NextRequest("http://localhost/api/shared/providers/openai");
    const context = { params: Promise.resolve({ providerName: "openai" }) };

    const response = await GET(request, context);

    expect(response.status).toBe(500);
  });
});
