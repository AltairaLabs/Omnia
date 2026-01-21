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
  auditSuccess,
  auditDenied,
  auditError,
  createAuditContext,
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

// Mock audit logger
const mockLogCrdSuccess = vi.fn();
const mockLogCrdDenied = vi.fn();
const mockLogCrdError = vi.fn();
const mockLogError = vi.fn();
vi.mock("@/lib/audit/logger", () => ({
  logCrdSuccess: (args: unknown) => mockLogCrdSuccess(args),
  logCrdDenied: (args: unknown) => mockLogCrdDenied(args),
  logCrdError: (args: unknown) => mockLogCrdError(args),
  logError: (...args: unknown[]) => mockLogError(...args),
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
      const error = new Error("Something went wrong");

      const response = serverErrorResponse(error, "Operation failed");

      expect(response).toBeInstanceOf(NextResponse);
      expect(response.status).toBe(500);

      const json = await response.json();
      expect(json.error).toBe("Internal Server Error");
      expect(json.message).toBe("Something went wrong");

      // Verify structured logging was called
      expect(mockLogError).toHaveBeenCalledWith("Operation failed", error, "workspace-route");
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
      vi.mocked(isForbiddenError).mockReturnValue(false);
      const error = new Error("Internal error");

      const response = handleK8sError(error, "access resource");

      expect(response.status).toBe(500);
      const json = await response.json();
      expect(json.error).toBe("Internal Server Error");
      // Verify logging was called
      expect(mockLogError).toHaveBeenCalled();
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

  describe("audit functions", () => {
    const mockUser = {
      id: "user-1",
      provider: "oauth" as const,
      email: "test@example.com",
      username: "testuser",
      groups: [],
      role: "viewer" as const,
    };

    const mockUserNoEmail = {
      id: "user-2",
      provider: "oauth" as const,
      email: "",
      username: "usernameonly",
      groups: [],
      role: "viewer" as const,
    };

    const mockUserNoIdentifier = {
      id: "user-3",
      provider: "oauth" as const,
      email: "",
      username: "",
      groups: [],
      role: "viewer" as const,
    };

    describe("createAuditContext", () => {
      it("should create an audit context", () => {
        const ctx = createAuditContext("my-workspace", "my-namespace", mockUser, "editor", "agentruntimes");

        expect(ctx.workspace).toBe("my-workspace");
        expect(ctx.namespace).toBe("my-namespace");
        expect(ctx.user).toBe(mockUser);
        expect(ctx.role).toBe("editor");
        expect(ctx.resourceType).toBe("agentruntimes");
      });
    });

    describe("auditSuccess", () => {
      it("should log success with user email", () => {
        const ctx = createAuditContext("ws", "ns", mockUser, "editor", "agents");

        auditSuccess(ctx, "create", "my-agent", { key: "value" });

        expect(mockLogCrdSuccess).toHaveBeenCalledWith({
          action: "create",
          resourceType: "agents",
          resourceName: "my-agent",
          workspace: "ws",
          namespace: "ns",
          user: "test@example.com",
          role: "editor",
          metadata: { key: "value" },
        });
      });

      it("should use username when email is empty", () => {
        const ctx = createAuditContext("ws", "ns", mockUserNoEmail, "viewer", "agents");

        auditSuccess(ctx, "get");

        expect(mockLogCrdSuccess).toHaveBeenCalledWith(
          expect.objectContaining({
            user: "usernameonly",
          })
        );
      });

      it("should use 'unknown' when no identifier", () => {
        const ctx = createAuditContext("ws", "ns", mockUserNoIdentifier, "viewer", "agents");

        auditSuccess(ctx, "list");

        expect(mockLogCrdSuccess).toHaveBeenCalledWith(
          expect.objectContaining({
            user: "unknown",
          })
        );
      });
    });

    describe("auditDenied", () => {
      it("should log denied action", () => {
        const ctx = createAuditContext("ws", "ns", mockUser, "viewer", "agents");

        auditDenied(ctx, "delete", "my-agent", "Permission denied");

        expect(mockLogCrdDenied).toHaveBeenCalledWith({
          action: "delete",
          resourceType: "agents",
          resourceName: "my-agent",
          workspace: "ws",
          namespace: "ns",
          user: "test@example.com",
          role: "viewer",
          errorMessage: "Permission denied",
        });
      });
    });

    describe("auditError", () => {
      it("should log error with Error object", () => {
        const ctx = createAuditContext("ws", "ns", mockUser, "editor", "agents");
        const error = new Error("Something went wrong");

        auditError(ctx, "update", "my-agent", error, 500);

        expect(mockLogCrdError).toHaveBeenCalledWith({
          action: "update",
          resourceType: "agents",
          resourceName: "my-agent",
          workspace: "ws",
          namespace: "ns",
          user: "test@example.com",
          role: "editor",
          errorMessage: "Something went wrong",
          statusCode: 500,
        });
      });

      it("should log error with string error", () => {
        const ctx = createAuditContext("ws", "ns", mockUser, "editor", "agents");

        auditError(ctx, "create", "my-agent", "String error message");

        expect(mockLogCrdError).toHaveBeenCalledWith(
          expect.objectContaining({
            errorMessage: "String error message",
          })
        );
      });
    });
  });
});
