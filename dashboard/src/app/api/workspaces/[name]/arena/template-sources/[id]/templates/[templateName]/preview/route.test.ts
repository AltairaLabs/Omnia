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

vi.mock("node:fs/promises", () => ({
  stat: vi.fn(),
  readFile: vi.fn(),
  readdir: vi.fn(),
}));

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
 * Helper to create a mock for fs.readFile that returns index JSON for index paths
 * and template content for other paths.
 */
function createReadFileMock(templateContent: string, indexData: unknown[] = mockTemplateIndex) {
  return vi.fn().mockImplementation(async (filePath: string) => {
    if (typeof filePath === "string" && filePath.includes("template-indexes")) {
      return JSON.stringify(indexData);
    }
    return templateContent;
  });
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

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ variables: {} }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns rendered files on success", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: "config.yaml", isDirectory: () => false },
    ] as unknown as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("name: {{ .projectName }}"));

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files).toBeDefined();
  });

  it("handles errors when preview fails", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("content"));

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
    const fs = await import("node:fs/promises");

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
    vi.mocked(fs.stat).mockResolvedValue({ isDirectory: () => false } as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("name: {{ .projectName }}", [templateWithFiles]));

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
    const fs = await import("node:fs/promises");

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
    vi.mocked(fs.stat).mockResolvedValue({ isDirectory: () => true } as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: "main.yaml", isDirectory: () => false },
    ] as unknown as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("prompt: test", [templateWithDirSpec]));

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
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);

    // First call returns a directory, second returns files
    let readdirCallCount = 0;
    vi.mocked(fs.readdir).mockImplementation(async () => {
      readdirCallCount++;
      if (readdirCallCount === 1) {
        return [
          { name: "subdir", isDirectory: () => true },
          { name: "root.yaml", isDirectory: () => false },
        ] as unknown as Awaited<ReturnType<typeof fs.readdir>>;
      }
      return [
        { name: "nested.yaml", isDirectory: () => false },
      ] as unknown as Awaited<ReturnType<typeof fs.readdir>>;
    });
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("content"));

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

  it("skips missing files in file specs", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("node:fs/promises");

    const templateWithMissingFile = {
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

    let statCallCount = 0;
    vi.mocked(fs.stat).mockImplementation(async () => {
      statCallCount++;
      if (statCallCount === 1) {
        return { isDirectory: () => false } as Awaited<ReturnType<typeof fs.stat>>;
      }
      throw new Error("ENOENT");
    });
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("content", [templateWithMissingFile]));

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
    const fs = await import("node:fs/promises");

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
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: "config.yaml", isDirectory: () => false },
    ] as unknown as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("content", [templateWithVariables]));

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
