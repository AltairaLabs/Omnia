/**
 * Tests for template preview API route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

// Mock dependencies before imports
vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    getWorkspace: vi.fn(),
    validateWorkspace: vi.fn(),
  };
});

vi.mock("@/lib/k8s/crd-operations", () => ({
  getCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((err: unknown) => err instanceof Error ? err.message : "Unknown error"),
  isForbiddenError: vi.fn(),
}));

vi.mock("@/lib/data/content-api-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/data/content-api-service")>();
  return { ...actual, getContent: vi.fn() };
});

const mockUser = {
  id: "testuser-id",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: ["users"],
  role: "editor" as const,
};

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
  kind: "Workspace" as const,
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" }, displayName: "Test Workspace", environment: "development" as const },
};

const mockTemplateSource = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "ArenaTemplateSource",
  metadata: { name: "test-source", namespace: "test-ns" },
  spec: { type: "git", git: { url: "https://github.com/test/repo" } },
  status: {
    phase: "Ready",
    templates: [
      {
        name: "basic-chatbot",
        displayName: "Basic Chatbot",
        path: "templates/basic-chatbot",
        variables: [],
        files: [],
      },
    ],
    artifact: { contentPath: "content" },
  },
};

// Template index JSON that would be written by the controller
const mockTemplateIndex = [
  {
    name: "basic-chatbot",
    displayName: "Basic Chatbot",
    path: "templates/basic-chatbot",
    variables: [],
    files: [],
  },
];

/**
 * Build a content-api ContentFile carrying the controller-written template
 * index JSON (mock-to-contract: matches the Go content.FileContent json tags).
 */
function indexContentFile(indexData: unknown[] = mockTemplateIndex) {
  return {
    path: "arena/template-indexes/test-source.json",
    content: JSON.stringify(indexData),
    encoding: "utf-8" as const,
    size: 100,
    modifiedAt: "2025-01-01T00:00:00Z",
  };
}

function createMockRequest(body: unknown): NextRequest {
  return new NextRequest(
    "http://localhost:3000/api/workspaces/test-ws/arena/template-sources/test-source/templates/basic-chatbot/preview",
    {
      method: "POST",
      body: JSON.stringify(body),
      headers: { "Content-Type": "application/json" },
    }
  );
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", id: "test-source", templateName: "basic-chatbot" }),
  };
}

describe("POST /api/workspaces/[name]/arena/template-sources/[id]/templates/[templateName]/preview", () => {
  beforeEach(() => {
    vi.resetModules();
    // Mock global fetch for arena controller API calls
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        files: [{ path: "config.yaml", content: "rendered: content" }],
      }),
    });
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ variables: {} }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 404 when source is not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(null);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ variables: {} }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 400 when source is not ready", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue({
      ...mockTemplateSource,
      status: { phase: "Fetching", templates: [] },
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ variables: {} }), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("not ready");
  });

  it("returns 404 when template is not found in source", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue({
      ...mockTemplateSource,
      status: { phase: "Ready", templates: [] },
    });
    const svc = await import("@/lib/data/content-api-service");
    // Index exists but contains no matching template.
    vi.mocked(svc.getContent).mockResolvedValue(indexContentFile([]));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ variables: {} }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns rendered files on success", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const svc = await import("@/lib/data/content-api-service");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(svc.getContent).mockResolvedValue(indexContentFile());

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files).toBeDefined();
    expect(vi.mocked(svc.getContent)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "arena/template-indexes/test-source.json",
    );
  });

  it("handles errors when preview fails", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const svc = await import("@/lib/data/content-api-service");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(svc.getContent).mockResolvedValue(indexContentFile());

    // Mock fetch to fail for this test
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: () => Promise.resolve("Internal server error"),
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ variables: {} }), createMockContext());

    expect(response.status).toBe(500);
  });

  it("returns 404 when workspace is not found after validation", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(null);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ variables: {} }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("handles template with explicit file specs", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const svc = await import("@/lib/data/content-api-service");

    const templateWithFiles = {
      name: "basic-chatbot",
      displayName: "Basic Chatbot",
      path: "templates/basic-chatbot",
      variables: [],
      files: [
        { path: "config.yaml", render: true },
        { path: "static.txt", render: false },
      ],
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(svc.getContent).mockResolvedValue(indexContentFile([templateWithFiles]));

    // Mock fetch to return 2 files for this test
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        files: [
          { path: "config.yaml", content: "name: my-project" },
          { path: "static.txt", content: "static content" },
        ],
      }),
    });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files).toHaveLength(2);
  });

  it("handles directory in file specs", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const svc = await import("@/lib/data/content-api-service");

    const templateWithDirSpec = {
      name: "basic-chatbot",
      displayName: "Basic Chatbot",
      path: "templates/basic-chatbot",
      variables: [],
      files: [{ path: "prompts/", render: true }],
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(svc.getContent).mockResolvedValue(indexContentFile([templateWithDirSpec]));

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {} }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files).toBeDefined();
  });

  it("handles nested directories in preview", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const svc = await import("@/lib/data/content-api-service");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(svc.getContent).mockResolvedValue(indexContentFile());

    // Mock fetch to return multiple files for nested directories
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        files: [
          { path: "root.yaml", content: "content" },
          { path: "subdir/nested.yaml", content: "content" },
        ],
      }),
    });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {} }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files.length).toBeGreaterThan(1);
  });

  it("returns the files the controller renders", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const svc = await import("@/lib/data/content-api-service");

    const templateWithFiles = {
      name: "basic-chatbot",
      displayName: "Basic Chatbot",
      path: "templates/basic-chatbot",
      variables: [],
      files: [
        { path: "exists.yaml", render: true },
        { path: "missing.yaml", render: true },
      ],
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(svc.getContent).mockResolvedValue(indexContentFile([templateWithFiles]));

    // The controller only renders the file that exists.
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        files: [{ path: "exists.yaml", content: "content" }],
      }),
    });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {} }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files).toHaveLength(1); // Only the existing file
  });

  it("includes validation errors in preview response", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const svc = await import("@/lib/data/content-api-service");

    const templateWithVariables = {
      name: "basic-chatbot",
      displayName: "Basic Chatbot",
      path: "templates/basic-chatbot",
      variables: [{ name: "requiredVar", type: "string", required: true }],
      files: [],
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(svc.getContent).mockResolvedValue(indexContentFile([templateWithVariables]));

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {} }), // Missing required variable
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.errors).toBeDefined();
  });
});
