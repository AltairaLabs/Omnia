import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";
import { GET, POST } from "./route";

// Mock auth
const mockGetUser = vi.fn();
const mockUserHasPermission = vi.fn();

vi.mock("@/lib/auth", () => ({
  getUser: () => mockGetUser(),
}));

vi.mock("@/lib/auth/permissions", () => ({
  Permission: {
    CREDENTIALS_VIEW: "credentials:view",
    CREDENTIALS_CREATE: "credentials:create",
    CREDENTIALS_EDIT: "credentials:edit",
  },
  userHasPermission: (user: unknown, permission: string) =>
    mockUserHasPermission(user, permission),
}));

// Mock K8s secrets
const mockListSecrets = vi.fn();
const mockCreateOrUpdateSecret = vi.fn();

vi.mock("@/lib/k8s/secrets", () => ({
  listSecrets: () => mockListSecrets(),
  createOrUpdateSecret: (req: unknown) => mockCreateOrUpdateSecret(req),
}));

// Helper to create NextRequest
function createRequest(
  url: string,
  options?: { method?: string; body?: unknown }
): NextRequest {
  const fullUrl = `http://localhost${url}`;
  const init: { method: string; body?: string; headers?: Record<string, string> } = {
    method: options?.method || "GET",
  };
  if (options?.body) {
    init.body = JSON.stringify(options.body);
    init.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(fullUrl, init);
}

describe("GET /api/secrets", () => {
  const mockUser = { id: "user-1", role: "editor" };

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetUser.mockResolvedValue(mockUser);
    mockUserHasPermission.mockReturnValue(true);
    mockListSecrets.mockResolvedValue([
      {
        namespace: "default",
        name: "test-secret",
        keys: ["API_KEY"],
        referencedBy: [],
        createdAt: "2024-01-15T10:00:00Z",
        modifiedAt: "2024-01-15T10:00:00Z",
      },
    ]);
  });

  it("should return secrets list", async () => {
    const request = createRequest("/api/secrets");
    const response = await GET(request);
    const data = await response.json();

    expect(response.status).toBe(200);
    expect(data.secrets).toHaveLength(1);
    expect(data.secrets[0].name).toBe("test-secret");
  });

  it("should filter by namespace query param", async () => {
    const request = createRequest("/api/secrets?namespace=production");
    await GET(request);

    expect(mockListSecrets).toHaveBeenCalled();
  });

  it("should return 403 without permission", async () => {
    mockUserHasPermission.mockReturnValue(false);

    const request = createRequest("/api/secrets");
    const response = await GET(request);

    expect(response.status).toBe(403);
  });

  it("should return 500 on error", async () => {
    mockListSecrets.mockRejectedValue(new Error("K8s error"));

    const request = createRequest("/api/secrets");
    const response = await GET(request);

    expect(response.status).toBe(500);
  });
});

describe("POST /api/secrets", () => {
  const mockUser = { id: "user-1", role: "editor" };

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetUser.mockResolvedValue(mockUser);
    mockUserHasPermission.mockReturnValue(true);
    mockCreateOrUpdateSecret.mockResolvedValue({
      namespace: "default",
      name: "new-secret",
      keys: ["API_KEY"],
      referencedBy: [],
      createdAt: "2024-01-15T10:00:00Z",
      modifiedAt: "2024-01-15T10:00:00Z",
    });
  });

  it("should create a secret", async () => {
    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        namespace: "default",
        name: "new-secret",
        data: { API_KEY: "test-value" },
      },
    });

    const response = await POST(request);
    const data = await response.json();

    expect(response.status).toBe(201);
    expect(data.secret.name).toBe("new-secret");
  });

  it("should validate namespace is required", async () => {
    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        name: "new-secret",
        data: { API_KEY: "test-value" },
      },
    });

    const response = await POST(request);
    const data = await response.json();

    expect(response.status).toBe(400);
    expect(data.error).toBe("namespace is required");
  });

  it("should validate name is required", async () => {
    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        namespace: "default",
        data: { API_KEY: "test-value" },
      },
    });

    const response = await POST(request);
    const data = await response.json();

    expect(response.status).toBe(400);
    expect(data.error).toBe("name is required");
  });

  it("should validate data is required", async () => {
    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        namespace: "default",
        name: "new-secret",
      },
    });

    const response = await POST(request);
    const data = await response.json();

    expect(response.status).toBe(400);
    expect(data.error).toBe("data is required");
  });

  it("should validate secret name format", async () => {
    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        namespace: "default",
        name: "INVALID_NAME",
        data: { API_KEY: "test-value" },
      },
    });

    const response = await POST(request);
    const data = await response.json();

    expect(response.status).toBe(400);
    expect(data.error).toContain("Invalid secret name");
  });

  it("should validate key names", async () => {
    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        namespace: "default",
        name: "valid-name",
        data: { "invalid key!": "test-value" },
      },
    });

    const response = await POST(request);
    const data = await response.json();

    expect(response.status).toBe(400);
    expect(data.error).toContain("Invalid key name");
  });

  it("should require at least one key-value pair", async () => {
    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        namespace: "default",
        name: "valid-name",
        data: {},
      },
    });

    const response = await POST(request);
    const data = await response.json();

    expect(response.status).toBe(400);
    expect(data.error).toBe("At least one key-value pair is required");
  });

  it("should return 403 without permission", async () => {
    mockUserHasPermission.mockReturnValue(false);

    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        namespace: "default",
        name: "new-secret",
        data: { API_KEY: "test-value" },
      },
    });

    const response = await POST(request);

    expect(response.status).toBe(403);
  });

  it("should handle conflict errors", async () => {
    mockCreateOrUpdateSecret.mockRejectedValue(
      new Error("Secret default/existing is not a managed credential secret")
    );

    const request = createRequest("/api/secrets", {
      method: "POST",
      body: {
        namespace: "default",
        name: "existing",
        data: { API_KEY: "test-value" },
      },
    });

    const response = await POST(request);

    expect(response.status).toBe(409);
  });

  it("should handle invalid JSON body", async () => {
    const request = new NextRequest("http://localhost/api/secrets", {
      method: "POST",
      body: "invalid json",
      headers: { "Content-Type": "application/json" },
    });

    const response = await POST(request);

    expect(response.status).toBe(400);
  });
});
