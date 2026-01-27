/**
 * Tests for template render API route.
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

vi.mock("fs/promises", () => ({
  stat: vi.fn(),
  readFile: vi.fn(),
  readdir: vi.fn(),
  mkdir: vi.fn(),
  writeFile: vi.fn(),
}));

vi.mock("crypto", () => ({
  default: { randomUUID: vi.fn(() => "test-uuid-1234") },
  randomUUID: vi.fn(() => "test-uuid-1234"),
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

// Template index data as it would be read from the JSON file
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
 * Creates a mock for fs.readFile that returns different content based on file path.
 * This is needed because the route reads both the template index JSON and template files.
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
    "http://localhost:3000/api/workspaces/test-ws/arena/template-sources/test-source/templates/basic-chatbot/render",
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

describe("POST /api/workspaces/[name]/arena/template-sources/[id]/templates/[templateName]/render", () => {
  beforeEach(() => {
    vi.resetModules();
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
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(403);
  });

  it("returns 400 when project name is missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    // Mock readFile to return template index
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("name: {{ .projectName }}"));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ variables: {} }), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("projectName");
  });

  it("returns 404 when source is not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(null);

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(404);
  });

  it("returns 400 when source is not ready", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
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
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("not ready");
  });

  it("returns 404 when template is not found in source", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue({
      ...mockTemplateSource,
      status: { phase: "Ready", templates: [], artifact: { contentPath: "content" } },
    });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    // Return empty template index so template is not found
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("", []));

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(404);
  });

  it("creates project successfully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    vi.mocked(fs.mkdir).mockResolvedValue(undefined);
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: "config.yaml", isDirectory: () => false },
    ] as unknown as Awaited<ReturnType<typeof fs.readdir>>);
    // Mock readFile to return template index for index path, template content for others
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("name: {{ .projectName }}"));

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.projectId).toBeDefined();
    expect(body.projectName).toBe("my-project");
  });

  it("handles errors when render fails", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(getCrd).mockResolvedValue(mockTemplateSource);
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace);
    // Mock readFile to return template index for index path
    vi.mocked(fs.readFile).mockImplementation(createReadFileMock("name: {{ .projectName }}"));
    vi.mocked(fs.mkdir).mockRejectedValue(new Error("Disk full"));

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ variables: {}, projectName: "my-project" }),
      createMockContext()
    );

    expect(response.status).toBe(500);
  });
});
