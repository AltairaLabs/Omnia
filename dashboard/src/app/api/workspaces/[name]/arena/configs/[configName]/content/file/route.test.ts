/**
 * Tests for Arena config file content API routes.
 */

/* eslint-disable sonarjs/no-clear-text-protocols -- test URLs */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";
import { gzipSync } from "zlib";
import * as tar from "tar-stream";

// Helper to create a valid tar.gz buffer for testing using tar-stream
async function createTarGzBuffer(files: Record<string, string>): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const pack = tar.pack();
    const chunks: Buffer[] = [];

    pack.on("data", (chunk: Buffer) => chunks.push(chunk));
    pack.on("end", () => {
      const tarBuffer = Buffer.concat(chunks);
      const gzipped = gzipSync(tarBuffer);
      resolve(gzipped);
    });
    pack.on("error", reject);

    for (const [name, content] of Object.entries(files)) {
      pack.entry({ name }, content);
    }
    pack.finalize();
  });
}

// Mock dependencies before imports
vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  getCrd: vi.fn(),
  getConfigMapContent: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(() => false),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    getWorkspaceResource: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "viewer",
      resourceType: "ArenaConfig",
    })),
    auditSuccess: vi.fn(),
    auditError: vi.fn(),
  };
});

vi.mock("@/lib/audit", () => ({
  logError: vi.fn(),
  logCrdSuccess: vi.fn(),
  logCrdDenied: vi.fn(),
  logCrdError: vi.fn(),
}));

// Mock global fetch for artifact URL tests
const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockUser = {
  id: "testuser-id",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: ["users"],
  role: "viewer" as const,
};

const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockConfig = {
  metadata: { name: "eval-config", namespace: "test-ns" },
  spec: { sourceRef: { name: "test-source" } },
  status: { phase: "Ready" },
};

const mockSource = {
  metadata: { name: "test-source", namespace: "test-ns" },
  spec: { configMap: { name: "test-configmap" } },
  status: {
    phase: "Ready",
    artifact: {
      checksum: "abc123",
      lastUpdateTime: "2025-01-01T00:00:00Z",
    },
  },
};

function createMockRequest(filePath?: string): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/configs/eval-config/content/file");
  if (filePath) {
    url.searchParams.set("path", filePath);
  }
  return new NextRequest(url.toString(), { method: "GET" });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", configName: "eval-config" }),
  };
}

describe("GET /api/workspaces/[name]/arena/configs/[configName]/content/file", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns 400 when path parameter is missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("path");
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 404 when config has no source reference", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    const configWithoutSource = {
      ...mockConfig,
      spec: {},
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: configWithoutSource,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toContain("No source");
  });

  it("returns 404 when source is not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: false,
        response: notFoundResponse("Arena source not found"),
      });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns file content from ConfigMap", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { getConfigMapContent } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: mockSource,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });
    vi.mocked(getConfigMapContent).mockResolvedValue({
      "config.yaml": "apiVersion: v1\nkind: Arena",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("config.yaml");
    expect(body.content).toBe("apiVersion: v1\nkind: Arena");
  });

  it("returns 404 when file is not found in content", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { getConfigMapContent } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: mockSource,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });
    vi.mocked(getConfigMapContent).mockResolvedValue({
      "other.yaml": "content",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toContain("File not found");
  });

  it("returns 404 when no content is available", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { getConfigMapContent } = await import("@/lib/k8s/crd-operations");

    const sourceWithoutConfigMap = {
      ...mockSource,
      spec: {},
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: sourceWithoutConfigMap,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });
    vi.mocked(getConfigMapContent).mockResolvedValue(null);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toContain("No content");
  });

  it("handles K8s errors gracefully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(500);
  });

  it("returns file with correct size", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { getConfigMapContent } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: mockSource,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });
    const content = "This is a test file with some content";
    vi.mocked(getConfigMapContent).mockResolvedValue({
      "test-file.txt": content,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("test-file.txt"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("test-file.txt");
    expect(body.content).toBe(content);
    expect(body.size).toBe(content.length);
  });

  it("returns 404 when config is not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: notFoundResponse("Arena config not found"),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(404);
  });

  it("falls back to ConfigMap when artifact URL fetch fails", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { getConfigMapContent } = await import("@/lib/k8s/crd-operations");

    const sourceWithArtifact = {
      ...mockSource,
      status: {
        phase: "Ready",
        artifact: {
          url: "http://localhost:8082/artifact.tar.gz",
          checksum: "abc123",
          lastUpdateTime: "2025-01-01T00:00:00Z",
        },
      },
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: sourceWithArtifact,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });
    // Mock fetch to fail
    mockFetch.mockResolvedValue({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    });
    // Fall back to ConfigMap
    vi.mocked(getConfigMapContent).mockResolvedValue({
      "config.yaml": "apiVersion: v1\nkind: Arena",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.content).toBe("apiVersion: v1\nkind: Arena");
  });

  it("returns 404 when artifact fetch fails and ConfigMap is empty", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { getConfigMapContent } = await import("@/lib/k8s/crd-operations");

    const sourceWithArtifact = {
      ...mockSource,
      status: {
        phase: "Ready",
        artifact: {
          url: "http://localhost:8082/artifact.tar.gz",
          checksum: "abc123",
          lastUpdateTime: "2025-01-01T00:00:00Z",
        },
      },
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: sourceWithArtifact,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });
    // Mock fetch to fail
    mockFetch.mockRejectedValue(new Error("Network error"));
    // No ConfigMap either
    vi.mocked(getConfigMapContent).mockResolvedValue(null);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toContain("No content");
  });

  it("extracts file content from tar.gz artifact successfully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    const sourceWithArtifact = {
      ...mockSource,
      spec: {}, // No ConfigMap
      status: {
        phase: "Ready",
        artifact: {
          url: "http://some-server:8080/artifact.tar.gz",
          checksum: "abc123",
          lastUpdateTime: "2025-01-01T00:00:00Z",
        },
      },
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: sourceWithArtifact,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });

    // Create a valid tar.gz with test content
    const tarGzBuffer = await createTarGzBuffer({
      "config.yaml": "apiVersion: v1\nkind: Arena",
      "prompts/test.yaml": "apiVersion: v1\nkind: PromptConfig",
    });

    mockFetch.mockResolvedValue({
      ok: true,
      arrayBuffer: () => Promise.resolve(tarGzBuffer.buffer.slice(tarGzBuffer.byteOffset, tarGzBuffer.byteOffset + tarGzBuffer.byteLength)),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("config.yaml");
    expect(body.content).toBe("apiVersion: v1\nkind: Arena");
  });

  it("rewrites localhost:8082 URL to K8S service URL", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    const sourceWithLocalhostArtifact = {
      ...mockSource,
      spec: {}, // No ConfigMap
      status: {
        phase: "Ready",
        artifact: {
          url: "http://localhost:8082/arena/test.tar.gz",
          checksum: "abc123",
          lastUpdateTime: "2025-01-01T00:00:00Z",
        },
      },
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: sourceWithLocalhostArtifact,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });

    // Create a valid tar.gz
    const tarGzBuffer = await createTarGzBuffer({
      "config.yaml": "apiVersion: v1\nkind: Arena",
    });

    mockFetch.mockResolvedValue({
      ok: true,
      arrayBuffer: () => Promise.resolve(tarGzBuffer.buffer.slice(tarGzBuffer.byteOffset, tarGzBuffer.byteOffset + tarGzBuffer.byteLength)),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    expect(response.status).toBe(200);
    // Verify the URL was rewritten
    expect(mockFetch).toHaveBeenCalledWith("http://omnia-controller-manager.omnia-system:8082/arena/test.tar.gz");
  });

  it("handles malformed tar.gz content gracefully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { getConfigMapContent } = await import("@/lib/k8s/crd-operations");

    const sourceWithArtifact = {
      ...mockSource,
      status: {
        phase: "Ready",
        artifact: {
          url: "http://some-server:8080/artifact.tar.gz",
          checksum: "abc123",
          lastUpdateTime: "2025-01-01T00:00:00Z",
        },
      },
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: sourceWithArtifact,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });

    // Return invalid gzip content
    mockFetch.mockResolvedValue({
      ok: true,
      arrayBuffer: () => Promise.resolve(new ArrayBuffer(10)), // Invalid gzip
    });

    // Fall back to ConfigMap
    vi.mocked(getConfigMapContent).mockResolvedValue({
      "config.yaml": "fallback content",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("config.yaml"), createMockContext());

    // Should fall back to ConfigMap
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.content).toBe("fallback content");
  });

  it("returns file from nested path in tar.gz", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    const sourceWithArtifact = {
      ...mockSource,
      spec: {}, // No ConfigMap
      status: {
        phase: "Ready",
        artifact: {
          url: "http://some-server:8080/artifact.tar.gz",
          checksum: "abc123",
          lastUpdateTime: "2025-01-01T00:00:00Z",
        },
      },
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource)
      .mockResolvedValueOnce({
        ok: true,
        resource: mockConfig,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      })
      .mockResolvedValueOnce({
        ok: true,
        resource: sourceWithArtifact,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });

    // Create a valid tar.gz with nested content
    const tarGzBuffer = await createTarGzBuffer({
      "prompts/greeting.yaml": "kind: PromptConfig\nspec:\n  id: greeting",
      "./other/file.txt": "other content",
    });

    mockFetch.mockResolvedValue({
      ok: true,
      arrayBuffer: () => Promise.resolve(tarGzBuffer.buffer.slice(tarGzBuffer.byteOffset, tarGzBuffer.byteOffset + tarGzBuffer.byteLength)),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("prompts/greeting.yaml"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("prompts/greeting.yaml");
    expect(body.content).toContain("greeting");
  });
});
