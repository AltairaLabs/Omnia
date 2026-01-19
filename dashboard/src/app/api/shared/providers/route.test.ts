/**
 * Tests for shared providers API routes.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock dependencies
vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  listSharedCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
}));

vi.mock("@/lib/audit", () => ({
  logError: vi.fn(),
}));

describe("GET /api/shared/providers", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("allows anonymous users (read-only public data)", async () => {
    const { listSharedCrd } = await import("@/lib/k8s/crd-operations");

    const mockProviders = [
      {
        metadata: { name: "openai", namespace: "omnia-system" },
        spec: { type: "openai" },
      },
    ];
    vi.mocked(listSharedCrd).mockResolvedValue(mockProviders);

    const { GET } = await import("./route");
    const response = await GET();

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].metadata.name).toBe("openai");
  });

  it("returns providers for authenticated users", async () => {
    const { getUser } = await import("@/lib/auth");
    const { listSharedCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue({
      id: "testuser-id",
      provider: "oauth",
      username: "testuser",
      email: "test@example.com",
      groups: ["users"],
      role: "viewer",
    });

    const mockProviders = [
      {
        metadata: { name: "openai", namespace: "omnia-system" },
        spec: { type: "openai" },
      },
      {
        metadata: { name: "anthropic", namespace: "omnia-system" },
        spec: { type: "anthropic" },
      },
    ];
    vi.mocked(listSharedCrd).mockResolvedValue(mockProviders);

    const { GET } = await import("./route");
    const response = await GET();

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(2);
    expect(body[0].metadata.name).toBe("openai");
    expect(listSharedCrd).toHaveBeenCalledWith("providers", "omnia-system");
  });

  it("returns 500 on error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { listSharedCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue({
      id: "testuser-id",
      provider: "oauth",
      username: "testuser",
      email: "test@example.com",
      groups: ["users"],
      role: "viewer",
    });

    vi.mocked(listSharedCrd).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET();

    expect(response.status).toBe(500);
    const body = await response.json();
    expect(body.error).toBe("Internal Server Error");
  });
});
