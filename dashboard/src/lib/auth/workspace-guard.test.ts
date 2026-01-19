import { describe, it, expect, beforeEach, vi, Mock } from "vitest";
import { NextRequest, NextResponse } from "next/server";
import {
  withWorkspaceAccess,
  withWorkspaceQuery,
  checkWorkspace,
  workspaceAccessDenied,
} from "./workspace-guard";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "./types";

// Mock dependencies
vi.mock("./index", () => ({
  getUser: vi.fn(),
}));

vi.mock("./workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

import { getUser } from "./index";
import { checkWorkspaceAccess } from "./workspace-authz";

describe("workspace-guard", () => {
  const mockUser: User = {
    id: "user-123",
    username: "testuser",
    email: "testuser@example.com",
    displayName: "Test User",
    groups: ["developers"],
    role: "editor",
    provider: "oauth",
  };

  const mockAnonymousUser: User = {
    id: "anonymous",
    username: "anonymous",
    groups: [],
    role: "viewer",
    provider: "anonymous",
  };

  const mockAccessGranted: WorkspaceAccess = {
    granted: true,
    role: "editor",
    permissions: {
      read: true,
      write: true,
      delete: true,
      manageMembers: false,
    },
  };

  const mockAccessDeniedNoRole: WorkspaceAccess = {
    granted: false,
    role: null,
    permissions: {
      read: false,
      write: false,
      delete: false,
      manageMembers: false,
    },
  };

  const mockAccessDeniedInsufficientRole: WorkspaceAccess = {
    granted: false,
    role: "viewer",
    permissions: {
      read: true,
      write: false,
      delete: false,
      manageMembers: false,
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
    (getUser as Mock).mockResolvedValue(mockUser);
    (checkWorkspaceAccess as Mock).mockResolvedValue(mockAccessGranted);
  });

  describe("withWorkspaceAccess", () => {
    const createMockRequest = () => {
      return new NextRequest("http://localhost:3000/api/workspaces/test-ws");
    };

    const createMockContext = (name: string = "test-workspace") => ({
      params: Promise.resolve({ name }),
    });

    it("should allow anonymous users with viewer access from checkWorkspaceAccess", async () => {
      (getUser as Mock).mockResolvedValue(mockAnonymousUser);
      // checkWorkspaceAccess now grants viewer access to anonymous users
      (checkWorkspaceAccess as Mock).mockResolvedValue({
        granted: true,
        role: "viewer",
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      });

      const handler = vi.fn().mockResolvedValue(NextResponse.json({ ok: true }));
      const wrapped = withWorkspaceAccess("viewer", handler);

      const response = await wrapped(createMockRequest(), createMockContext());

      expect(response.status).toBe(200);
      expect(handler).toHaveBeenCalled();
    });

    it("should return 403 when user has no role in workspace", async () => {
      (checkWorkspaceAccess as Mock).mockResolvedValue(mockAccessDeniedNoRole);

      const handler = vi.fn().mockResolvedValue(NextResponse.json({ ok: true }));
      const wrapped = withWorkspaceAccess("viewer", handler);

      const response = await wrapped(createMockRequest(), createMockContext());
      const body = await response.json();

      expect(response.status).toBe(403);
      expect(body.error).toBe("Forbidden");
      expect(body.message).toBe("Access denied to workspace: test-workspace");
      expect(body.workspace).toBe("test-workspace");
      expect(handler).not.toHaveBeenCalled();
    });

    it("should return 403 when user has insufficient role", async () => {
      (checkWorkspaceAccess as Mock).mockResolvedValue(
        mockAccessDeniedInsufficientRole
      );

      const handler = vi.fn().mockResolvedValue(NextResponse.json({ ok: true }));
      const wrapped = withWorkspaceAccess("owner", handler);

      const response = await wrapped(createMockRequest(), createMockContext());
      const body = await response.json();

      expect(response.status).toBe(403);
      expect(body.error).toBe("Forbidden");
      expect(body.message).toBe(
        "Insufficient workspace permissions: requires owner, have viewer"
      );
      expect(body.required).toBe("owner");
      expect(body.current).toBe("viewer");
      expect(handler).not.toHaveBeenCalled();
    });

    it("should call handler when access is granted", async () => {
      const handler = vi
        .fn()
        .mockResolvedValue(NextResponse.json({ success: true }));
      const wrapped = withWorkspaceAccess("editor", handler);

      const request = createMockRequest();
      const context = createMockContext();
      const response = await wrapped(request, context);
      const body = await response.json();

      expect(response.status).toBe(200);
      expect(body.success).toBe(true);
      expect(handler).toHaveBeenCalledWith(
        request,
        context,
        mockAccessGranted,
        mockUser
      );
    });

    it("should pass correct workspace name to checkWorkspaceAccess", async () => {
      const handler = vi.fn().mockResolvedValue(NextResponse.json({}));
      const wrapped = withWorkspaceAccess("viewer", handler);

      await wrapped(createMockRequest(), createMockContext("my-workspace"));

      expect(checkWorkspaceAccess).toHaveBeenCalledWith("my-workspace", "viewer");
    });
  });

  describe("withWorkspaceQuery", () => {
    const createMockRequest = (workspace?: string) => {
      const url = workspace
        ? `http://localhost:3000/api/resources?workspace=${workspace}`
        : "http://localhost:3000/api/resources";
      return new NextRequest(url);
    };

    it("should allow anonymous users with viewer access from checkWorkspaceAccess", async () => {
      (getUser as Mock).mockResolvedValue(mockAnonymousUser);
      // checkWorkspaceAccess now grants viewer access to anonymous users
      (checkWorkspaceAccess as Mock).mockResolvedValue({
        granted: true,
        role: "viewer",
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      });

      const handler = vi.fn().mockResolvedValue(NextResponse.json({ ok: true }));
      const wrapped = withWorkspaceQuery("viewer", handler);

      const response = await wrapped(createMockRequest("test-ws"));

      expect(response.status).toBe(200);
      expect(handler).toHaveBeenCalled();
    });

    it("should return 400 when workspace query param is missing", async () => {
      const handler = vi.fn().mockResolvedValue(NextResponse.json({ ok: true }));
      const wrapped = withWorkspaceQuery("viewer", handler);

      const response = await wrapped(createMockRequest());
      const body = await response.json();

      expect(response.status).toBe(400);
      expect(body.error).toBe("Bad Request");
      expect(body.message).toBe("Missing required query parameter: workspace");
      expect(handler).not.toHaveBeenCalled();
    });

    it("should return 403 when user has no role in workspace", async () => {
      (checkWorkspaceAccess as Mock).mockResolvedValue(mockAccessDeniedNoRole);

      const handler = vi.fn().mockResolvedValue(NextResponse.json({ ok: true }));
      const wrapped = withWorkspaceQuery("viewer", handler);

      const response = await wrapped(createMockRequest("test-ws"));
      const body = await response.json();

      expect(response.status).toBe(403);
      expect(body.error).toBe("Forbidden");
      expect(body.message).toBe("Access denied to workspace: test-ws");
      expect(handler).not.toHaveBeenCalled();
    });

    it("should return 403 when user has insufficient role", async () => {
      (checkWorkspaceAccess as Mock).mockResolvedValue(
        mockAccessDeniedInsufficientRole
      );

      const handler = vi.fn().mockResolvedValue(NextResponse.json({ ok: true }));
      const wrapped = withWorkspaceQuery("owner", handler);

      const response = await wrapped(createMockRequest("test-ws"));
      const body = await response.json();

      expect(response.status).toBe(403);
      expect(body.error).toBe("Forbidden");
      expect(body.required).toBe("owner");
      expect(body.current).toBe("viewer");
      expect(handler).not.toHaveBeenCalled();
    });

    it("should call handler when access is granted", async () => {
      const handler = vi
        .fn()
        .mockResolvedValue(NextResponse.json({ success: true }));
      const wrapped = withWorkspaceQuery("editor", handler);

      const request = createMockRequest("my-workspace");
      const response = await wrapped(request);
      const body = await response.json();

      expect(response.status).toBe(200);
      expect(body.success).toBe(true);
      expect(handler).toHaveBeenCalledWith(
        request,
        "my-workspace",
        mockAccessGranted,
        mockUser
      );
    });

    it("should pass correct workspace name from query to checkWorkspaceAccess", async () => {
      const handler = vi.fn().mockResolvedValue(NextResponse.json({}));
      const wrapped = withWorkspaceQuery("editor", handler);

      await wrapped(createMockRequest("query-workspace"));

      expect(checkWorkspaceAccess).toHaveBeenCalledWith(
        "query-workspace",
        "editor"
      );
    });
  });

  describe("checkWorkspace", () => {
    it("should return access and user", async () => {
      const result = await checkWorkspace("test-workspace", "viewer");

      expect(result.access).toEqual(mockAccessGranted);
      expect(result.user).toEqual(mockUser);
      expect(checkWorkspaceAccess).toHaveBeenCalledWith(
        "test-workspace",
        "viewer"
      );
    });

    it("should work without required role", async () => {
      const result = await checkWorkspace("test-workspace");

      expect(result.access).toEqual(mockAccessGranted);
      expect(checkWorkspaceAccess).toHaveBeenCalledWith(
        "test-workspace",
        undefined
      );
    });
  });

  describe("workspaceAccessDenied", () => {
    it("should return 403 with no role message when role is null", async () => {
      const response = workspaceAccessDenied("test-ws", mockAccessDeniedNoRole);
      const body = await response.json();

      expect(response.status).toBe(403);
      expect(body.error).toBe("Forbidden");
      expect(body.message).toBe("Access denied to workspace: test-ws");
      expect(body.workspace).toBe("test-ws");
    });

    it("should return 403 with insufficient role message when role exists", async () => {
      const response = workspaceAccessDenied(
        "test-ws",
        mockAccessDeniedInsufficientRole,
        "owner"
      );
      const body = await response.json();

      expect(response.status).toBe(403);
      expect(body.error).toBe("Forbidden");
      expect(body.message).toBe(
        "Insufficient workspace permissions: requires owner, have viewer"
      );
      expect(body.required).toBe("owner");
      expect(body.current).toBe("viewer");
    });

    it("should handle missing required role in message", async () => {
      const response = workspaceAccessDenied(
        "test-ws",
        mockAccessDeniedInsufficientRole
      );
      const body = await response.json();

      expect(body.message).toBe(
        "Insufficient workspace permissions: requires any, have viewer"
      );
    });
  });
});
