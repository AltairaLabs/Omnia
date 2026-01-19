/**
 * Tests for workspace-route-helpers.ts
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextResponse } from "next/server";
import {
  validateWorkspace,
  getWorkspaceResource,
  notFoundResponse,
  serverErrorResponse,
  forbiddenResponse,
  unauthorizedResponse,
  requireAuth,
  handleK8sError,
  buildCrdResource,
  CRD_API_VERSION,
  WORKSPACE_LABEL,
  SYSTEM_NAMESPACE,
  CRD_AGENTS,
  CRD_PROMPTPACKS,
} from "./workspace-route-helpers";
import { isForbiddenError, getCrd } from "./crd-operations";

// Mock dependencies
vi.mock("./workspace-client", () => ({
  getWorkspace: vi.fn(),
}));

vi.mock("./crd-operations", () => ({
  extractK8sErrorMessage: vi.fn((error: unknown) => {
    if (error instanceof Error) return error.message;
    return "Unknown error";
  }),
  isForbiddenError: vi.fn(),
  getCrd: vi.fn(),
}));

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

import { getWorkspace } from "./workspace-client";
import { getUser } from "@/lib/auth";

describe("workspace-route-helpers", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("validateWorkspace", () => {
    it("should return workspace and clientOptions when workspace exists", async () => {
      const mockWorkspace = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "Workspace",
        metadata: { name: "test-workspace" },
        spec: { namespace: { name: "test-ns" } },
      };
      vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

      const result = await validateWorkspace("test-workspace", "viewer");

      expect(result.ok).toBe(true);
      if (result.ok) {
        expect(result.workspace).toBe(mockWorkspace);
        expect(result.clientOptions).toEqual({
          workspace: "test-workspace",
          namespace: "test-ns",
          role: "viewer",
        });
      }
    });

    it("should return 404 response when workspace does not exist", async () => {
      vi.mocked(getWorkspace).mockResolvedValue(null);

      const result = await validateWorkspace("nonexistent", "viewer");

      expect(result.ok).toBe(false);
      if (!result.ok) {
        const json = await result.response.json();
        expect(json.error).toBe("Not Found");
        expect(json.message).toContain("nonexistent");
      }
    });

    it("should use provided role in clientOptions", async () => {
      const mockWorkspace = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "Workspace",
        metadata: { name: "test-workspace" },
        spec: { namespace: { name: "test-ns" } },
      };
      vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

      const result = await validateWorkspace("test-workspace", "editor");

      expect(result.ok).toBe(true);
      if (result.ok) {
        expect(result.clientOptions.role).toBe("editor");
      }
    });
  });

  describe("notFoundResponse", () => {
    it("should return a 404 response with the provided message", async () => {
      const response = notFoundResponse("Resource not found");

      expect(response).toBeInstanceOf(NextResponse);
      expect(response.status).toBe(404);

      const json = await response.json();
      expect(json.error).toBe("Not Found");
      expect(json.message).toBe("Resource not found");
    });
  });

  describe("serverErrorResponse", () => {
    it("should return a 500 response with extracted error message", async () => {
      const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      const error = new Error("Something went wrong");

      const response = serverErrorResponse(error, "Operation failed");

      expect(response).toBeInstanceOf(NextResponse);
      expect(response.status).toBe(500);

      const json = await response.json();
      expect(json.error).toBe("Internal Server Error");
      expect(json.message).toBe("Something went wrong");

      // Verify structured logging was called
      expect(consoleSpy).toHaveBeenCalledTimes(1);
      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.level).toBe("error");
      expect(loggedJson.message).toBe("Operation failed");
      expect(loggedJson.error).toBe("Something went wrong");
      expect(loggedJson.context).toBe("workspace-route");
      consoleSpy.mockRestore();
    });
  });

  describe("forbiddenResponse", () => {
    it("should return a 403 response with the provided message", async () => {
      const response = forbiddenResponse("Access denied");

      expect(response).toBeInstanceOf(NextResponse);
      expect(response.status).toBe(403);

      const json = await response.json();
      expect(json.error).toBe("Forbidden");
      expect(json.message).toBe("Access denied");
    });
  });

  describe("handleK8sError", () => {
    it("should return 403 for forbidden errors", async () => {
      vi.mocked(isForbiddenError).mockReturnValue(true);
      const error = new Error("Forbidden");

      const response = handleK8sError(error, "access resource");

      expect(response.status).toBe(403);
      const json = await response.json();
      expect(json.error).toBe("Forbidden");
      expect(json.message).toContain("access resource");
    });

    it("should return 500 for non-forbidden errors", async () => {
      const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      vi.mocked(isForbiddenError).mockReturnValue(false);
      const error = new Error("Internal error");

      const response = handleK8sError(error, "access resource");

      expect(response.status).toBe(500);
      const json = await response.json();
      expect(json.error).toBe("Internal Server Error");
      consoleSpy.mockRestore();
    });
  });

  describe("buildCrdResource", () => {
    it("should build a CRD resource with standard metadata", () => {
      const result = buildCrdResource(
        "AgentRuntime",
        "my-workspace",
        "my-namespace",
        "my-agent",
        { replicas: 1 }
      );

      expect(result).toEqual({
        apiVersion: CRD_API_VERSION,
        kind: "AgentRuntime",
        metadata: {
          name: "my-agent",
          namespace: "my-namespace",
          labels: {
            [WORKSPACE_LABEL]: "my-workspace",
          },
          annotations: undefined,
        },
        spec: { replicas: 1 },
      });
    });

    it("should merge provided labels with workspace label", () => {
      const result = buildCrdResource(
        "PromptPack",
        "my-workspace",
        "my-namespace",
        "my-pack",
        { description: "Test pack" },
        { "app.kubernetes.io/name": "my-pack", tier: "frontend" }
      );

      expect(result.metadata.labels).toEqual({
        "app.kubernetes.io/name": "my-pack",
        tier: "frontend",
        [WORKSPACE_LABEL]: "my-workspace",
      });
    });

    it("should include provided annotations", () => {
      const result = buildCrdResource(
        "AgentRuntime",
        "my-workspace",
        "my-namespace",
        "my-agent",
        { replicas: 1 },
        undefined,
        { "description": "My agent", "created-by": "test" }
      );

      expect(result.metadata.annotations).toEqual({
        description: "My agent",
        "created-by": "test",
      });
    });

    it("should handle both labels and annotations", () => {
      const result = buildCrdResource(
        "AgentRuntime",
        "my-workspace",
        "my-namespace",
        "my-agent",
        { replicas: 1 },
        { env: "test" },
        { note: "testing" }
      );

      expect(result.metadata.labels).toEqual({
        env: "test",
        [WORKSPACE_LABEL]: "my-workspace",
      });
      expect(result.metadata.annotations).toEqual({
        note: "testing",
      });
    });
  });

  describe("unauthorizedResponse", () => {
    it("should return a 401 response with standard message", async () => {
      const response = unauthorizedResponse();

      expect(response).toBeInstanceOf(NextResponse);
      expect(response.status).toBe(401);

      const json = await response.json();
      expect(json.error).toBe("Unauthorized");
      expect(json.message).toBe("Authentication required");
    });
  });

  describe("requireAuth", () => {
    it("should return user when authenticated", async () => {
      const mockUser = { id: "user-1", provider: "oauth" as const, email: "test@example.com", username: "testuser", groups: [], role: "viewer" as const };
      vi.mocked(getUser).mockResolvedValue(mockUser);

      const result = await requireAuth();

      expect(result.ok).toBe(true);
      if (result.ok) {
        expect(result.user).toBe(mockUser);
      }
    });

    it("should return 401 response when anonymous", async () => {
      const anonymousUser = { id: "anon", provider: "anonymous" as const, email: "", username: "", groups: [], role: "viewer" as const };
      vi.mocked(getUser).mockResolvedValue(anonymousUser);

      const result = await requireAuth();

      expect(result.ok).toBe(false);
      if (!result.ok) {
        expect(result.response.status).toBe(401);
        const json = await result.response.json();
        expect(json.error).toBe("Unauthorized");
      }
    });
  });

  describe("getWorkspaceResource", () => {
    it("should return resource when workspace and resource exist", async () => {
      const mockWorkspace = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "Workspace",
        metadata: { name: "test-workspace" },
        spec: { namespace: { name: "test-ns" } },
      };
      const mockResource = { metadata: { name: "test-agent" }, spec: {} };
      vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
      vi.mocked(getCrd).mockResolvedValue(mockResource);

      const result = await getWorkspaceResource("test-workspace", "viewer", "agentruntimes", "test-agent", "Agent");

      expect(result.ok).toBe(true);
      if (result.ok) {
        expect(result.resource).toBe(mockResource);
        expect(result.workspace).toBe(mockWorkspace);
        expect(result.clientOptions.workspace).toBe("test-workspace");
      }
    });

    it("should return 404 when workspace does not exist", async () => {
      vi.mocked(getWorkspace).mockResolvedValue(null);

      const result = await getWorkspaceResource("nonexistent", "viewer", "agentruntimes", "test-agent", "Agent");

      expect(result.ok).toBe(false);
      if (!result.ok) {
        expect(result.response.status).toBe(404);
      }
    });

    it("should return 404 when resource does not exist", async () => {
      const mockWorkspace = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "Workspace",
        metadata: { name: "test-workspace" },
        spec: { namespace: { name: "test-ns" } },
      };
      vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
      vi.mocked(getCrd).mockResolvedValue(null);

      const result = await getWorkspaceResource("test-workspace", "viewer", "agentruntimes", "missing-agent", "Agent");

      expect(result.ok).toBe(false);
      if (!result.ok) {
        expect(result.response.status).toBe(404);
        const json = await result.response.json();
        expect(json.message).toContain("missing-agent");
      }
    });
  });

  describe("constants", () => {
    it("should export correct CRD_API_VERSION", () => {
      expect(CRD_API_VERSION).toBe("omnia.altairalabs.ai/v1alpha1");
    });

    it("should export correct WORKSPACE_LABEL", () => {
      expect(WORKSPACE_LABEL).toBe("omnia.altairalabs.ai/workspace");
    });

    it("should export correct SYSTEM_NAMESPACE", () => {
      expect(SYSTEM_NAMESPACE).toBe("omnia-system");
    });

    it("should export correct CRD_AGENTS", () => {
      expect(CRD_AGENTS).toBe("agentruntimes");
    });

    it("should export correct CRD_PROMPTPACKS", () => {
      expect(CRD_PROMPTPACKS).toBe("promptpacks");
    });
  });
});
