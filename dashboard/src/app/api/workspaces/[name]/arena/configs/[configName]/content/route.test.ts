/**
 * Tests for Arena config content API routes.
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

// Mock fs for filesystem content reading
vi.mock("fs", () => ({
  existsSync: vi.fn(),
  readdirSync: vi.fn(),
  readFileSync: vi.fn(),
}));

vi.mock("fs", () => ({
  existsSync: vi.fn(),
  readdirSync: vi.fn(),
  readFileSync: vi.fn(),
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

function createMockRequest(): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/arena/configs/eval-config/content";
  return new NextRequest(url, { method: "GET" });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", configName: "eval-config" }),
  };
}

describe("GET /api/workspaces/[name]/arena/configs/[configName]/content", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns empty content when config has no source reference", async () => {
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
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("No source");
    expect(body.files).toEqual([]);
  });

  it("returns empty content when source is not found", async () => {
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
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("Source not found");
  });

  it("returns content from ConfigMap when available", async () => {
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
      "config.arena.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test-config
spec:
  defaults:
    temperature: 0.7`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files.length).toBeGreaterThan(0);
    expect(body.files[0].path).toBe("config.arena.yaml");
    expect(body.files[0].type).toBe("arena");
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(403);
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
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns empty content when no content is available", async () => {
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
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("No content available");
    expect(body.files).toEqual([]);
  });

  it("parses prompt configs correctly", async () => {
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
      "prompts/greeting.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: greeting
spec:
  id: greeting
  description: A greeting prompt
  task_type: conversation`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.promptConfigs.length).toBe(1);
    expect(body.promptConfigs[0].id).toBe("greeting");
    expect(body.promptConfigs[0].description).toBe("A greeting prompt");
  });

  it("parses provider configs correctly", async () => {
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
      "providers/openai.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-gpt4
spec:
  id: openai-gpt4
  type: openai
  model: gpt-4`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.providers.length).toBe(1);
    expect(body.providers[0].type).toBe("openai");
    expect(body.providers[0].model).toBe("gpt-4");
  });

  it("parses scenario files correctly", async () => {
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
      "scenarios/test.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: test-scenario
spec:
  id: test-scenario
  description: A test scenario
  turns:
    - role: user
      content: Hello
    - role: assistant
      content: Hi there`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.scenarios.length).toBe(1);
    expect(body.scenarios[0].turnCount).toBe(2);
  });

  it("parses tool files correctly", async () => {
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
      "tools/calculator.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: calculator
spec:
  description: A calculator tool
  config:
    mode: mock
    mock_result: 42`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tools.length).toBe(1);
    expect(body.tools[0].name).toBe("calculator");
    expect(body.tools[0].hasMockData).toBe(true);
  });

  it("builds file tree correctly", async () => {
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
      "config.arena.yaml": "kind: Arena",
      "prompts/greeting.yaml": "kind: PromptConfig",
      "scenarios/test.yaml": "kind: Scenario",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.fileTree.length).toBeGreaterThan(0);

    // Should have prompts and scenarios directories
    const directories = body.fileTree.filter((n: any) => n.isDirectory);
    expect(directories.length).toBe(2);
  });

  it("handles K8s errors gracefully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });

  it("parses arena config with MCP servers", async () => {
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
      "config.arena.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test-arena
spec:
  mcp_servers:
    math:
      command: python
      args: ["-m", "mcp_math"]
  judges:
    default:
      provider: openai-gpt4
  judge_defaults:
    prompt: "Rate the response"
  self_play:
    enabled: true
    persona: assistant
  defaults:
    temperature: 0.7
    max_tokens: 1000`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.mcpServers).toHaveProperty("math");
    expect(body.judges).toHaveProperty("default");
    expect(body.judgeDefaults).toBeDefined();
    expect(body.selfPlay).toBeDefined();
    expect(body.defaults).toBeDefined();
    expect(body.defaults.temperature).toBe(0.7);
  });

  it("handles invalid YAML content gracefully", async () => {
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
      "invalid.yaml": "this is: [not valid yaml",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    // Should still return 200 with file info, but type as "other"
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files.length).toBe(1);
    expect(body.files[0].type).toBe("other");
  });

  it("handles persona file type", async () => {
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
      "personas/user.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Persona
metadata:
  name: helpful-user`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files[0].type).toBe("persona");
  });

  it("detects entry point correctly", async () => {
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
      "config.arena.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test-arena`,
      "prompts/test.yaml": "kind: PromptConfig",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.entryPoint).toBe("config.arena.yaml");
  });

  it("parses prompt with variables and validators", async () => {
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
      "prompts/advanced.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: advanced
spec:
  id: advanced
  version: "1.0"
  system_template: "You are a helpful assistant"
  variables:
    - name: topic
      type: string
      required: true
      description: The topic to discuss
  allowed_tools:
    - calculator
  validators:
    - type: length
      config:
        max: 1000`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.promptConfigs[0].variables.length).toBe(1);
    expect(body.promptConfigs[0].allowedTools).toContain("calculator");
    expect(body.promptConfigs[0].validators.length).toBe(1);
    expect(body.promptConfigs[0].systemTemplate).toBeDefined();
    expect(body.promptConfigs[0].version).toBe("1.0");
  });

  it("parses provider with pricing and defaults", async () => {
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
      "providers/openai.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: openai-gpt4
spec:
  id: openai-gpt4
  type: openai
  model: gpt-4
  pricing:
    input_per_1k_tokens: 0.03
    output_per_1k_tokens: 0.06
  defaults:
    temperature: 0.7
    max_tokens: 1000
    top_p: 0.9`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.providers[0].pricing).toBeDefined();
    expect(body.providers[0].pricing.inputPer1kTokens).toBe(0.03);
    expect(body.providers[0].defaults).toBeDefined();
    expect(body.providers[0].defaults.temperature).toBe(0.7);
  });

  it("parses scenario with tags", async () => {
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
      "scenarios/tagged.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Scenario
metadata:
  name: tagged
spec:
  id: tagged
  task_type: conversation
  tags:
    - regression
    - important`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.scenarios[0].tags).toContain("regression");
    expect(body.scenarios[0].taskType).toBe("conversation");
  });

  it("parses tool with input/output schemas", async () => {
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
      "tools/calculator.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Tool
metadata:
  name: calculator
spec:
  description: A calculator tool
  input_schema:
    type: object
    properties:
      expression: { type: string }
  output_schema:
    type: number
  config:
    mode: api
    timeout: 30`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tools[0].inputSchema).toBeDefined();
    expect(body.tools[0].outputSchema).toBeDefined();
    expect(body.tools[0].mode).toBe("api");
    expect(body.tools[0].timeout).toBe(30);
  });

  it("parses arena defaults with state config", async () => {
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
      "config.arena.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test-arena
spec:
  defaults:
    temperature: 0.7
    top_p: 0.9
    max_tokens: 1000
    seed: 42
    concurrency: 4
    timeout: "30s"
    max_retries: 3
    output:
      dir: /output
      formats: [json, csv]
    session:
      enabled: true
      dir: /sessions
    fail_on: [error, timeout]
    state:
      enabled: true
      max_history_turns: 10
      persistence: redis
      redis_url: redis://localhost:6379`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.defaults).toBeDefined();
    expect(body.defaults.temperature).toBe(0.7);
    expect(body.defaults.topP).toBe(0.9);
    expect(body.defaults.maxTokens).toBe(1000);
    expect(body.defaults.seed).toBe(42);
    expect(body.defaults.concurrency).toBe(4);
    expect(body.defaults.timeout).toBe("30s");
    expect(body.defaults.maxRetries).toBe(3);
    expect(body.defaults.output).toBeDefined();
    expect(body.defaults.session).toBeDefined();
    expect(body.defaults.failOn).toContain("error");
    expect(body.defaults.state).toBeDefined();
    expect(body.defaults.state.persistence).toBe("redis");
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
      "config.arena.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test-arena`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files.length).toBe(1);
    expect(body.files[0].type).toBe("arena");
  });

  it("handles fetch error gracefully", async () => {
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
    // Mock fetch to throw
    mockFetch.mockRejectedValue(new Error("Network error"));
    // No ConfigMap either
    vi.mocked(getConfigMapContent).mockResolvedValue(null);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("No content available");
  });

  it("truncates long system template in prompt", async () => {
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
    const longTemplate = "a".repeat(1000);
    vi.mocked(getConfigMapContent).mockResolvedValue({
      "prompts/long.yaml": `apiVersion: omnia.altairalabs.ai/v1alpha1
kind: PromptConfig
metadata:
  name: long-prompt
spec:
  id: long-prompt
  system_template: "${longTemplate}"`,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    // systemTemplate should be truncated to 500 chars + "..."
    expect(body.promptConfigs[0].systemTemplate.length).toBeLessThanOrEqual(503);
    expect(body.promptConfigs[0].systemTemplate).toContain("...");
  });

  it("reads content from filesystem when contentPath is available", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("fs");

    const sourceWithContentPath = {
      ...mockSource,
      status: {
        phase: "Ready",
        artifact: {
          contentPath: "arena/test-source/.arena/versions/abc123",
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
        resource: sourceWithContentPath,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });

    // Mock filesystem reads
    vi.mocked(fs.existsSync).mockReturnValue(true);
    vi.mocked(fs.readdirSync).mockImplementation((dir: any) => {
      if (dir.toString().endsWith("abc123")) {
        return [
          { name: "config.arena.yaml", isDirectory: () => false, isFile: () => true },
        ] as any;
      }
      return [];
    });
    vi.mocked(fs.readFileSync).mockReturnValue(`apiVersion: omnia.altairalabs.ai/v1alpha1
kind: Arena
metadata:
  name: test-arena`);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files.length).toBe(1);
    expect(body.files[0].type).toBe("arena");
  });

  it("returns empty content when filesystem path does not exist", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("fs");
    const { getConfigMapContent } = await import("@/lib/k8s/crd-operations");

    const sourceWithContentPath = {
      ...mockSource,
      spec: {},  // No configMap
      status: {
        phase: "Ready",
        artifact: {
          contentPath: "arena/test-source/.arena/versions/abc123",
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
        resource: sourceWithContentPath,
        workspace: mockWorkspace as any,
        clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      });

    // Mock filesystem to not exist
    vi.mocked(fs.existsSync).mockReturnValue(false);
    vi.mocked(getConfigMapContent).mockResolvedValue(null);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("No content available");
  });
});
