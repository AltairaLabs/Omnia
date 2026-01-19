import { describe, it, expect, beforeEach, vi, Mock } from "vitest";
import {
  checkWorkspaceAccess,
  getAccessibleWorkspaces,
  hasWorkspaceRole,
  requireWorkspaceAccess,
} from "./workspace-authz";
import { clearAuthzCache } from "./authz-cache";
import type { Workspace, WorkspaceRole } from "@/types/workspace";
import type { User } from "./types";

// Mock dependencies
vi.mock("./index", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-client", () => ({
  getWorkspace: vi.fn(),
  listWorkspaces: vi.fn(),
}));

import { getUser } from "./index";
import { getWorkspace, listWorkspaces } from "@/lib/k8s/workspace-client";

describe("workspace-authz", () => {
  const mockUser: User = {
    id: "user-123",
    username: "testuser",
    email: "testuser@example.com",
    displayName: "Test User",
    groups: ["developers@example.com", "team-alpha"],
    role: "editor",
    provider: "oauth",
  };

  const mockWorkspace: Workspace = {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Workspace",
    metadata: {
      name: "test-workspace",
      creationTimestamp: "2024-01-15T10:00:00Z",
    },
    spec: {
      displayName: "Test Workspace",
      description: "A test workspace",
      environment: "development",
      namespace: {
        name: "test-ns",
        create: true,
      },
      roleBindings: [
        {
          groups: ["owners@example.com"],
          role: "owner",
        },
        {
          groups: ["developers@example.com"],
          role: "editor",
        },
        {
          groups: ["viewers@example.com"],
          role: "viewer",
        },
      ],
      directGrants: [
        {
          user: "admin@example.com",
          role: "owner",
        },
        {
          user: "guest@example.com",
          role: "viewer",
          expires: "2099-12-31T23:59:59Z", // Far future
        },
        {
          user: "expired@example.com",
          role: "editor",
          expires: "2020-01-01T00:00:00Z", // Past date
        },
      ],
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
    clearAuthzCache();
    (getUser as Mock).mockResolvedValue(mockUser);
    (getWorkspace as Mock).mockResolvedValue(mockWorkspace);
    (listWorkspaces as Mock).mockResolvedValue([mockWorkspace]);
  });

  describe("checkWorkspaceAccess", () => {
    it("should grant viewer access to anonymous users", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        provider: "anonymous",
      });

      const access = await checkWorkspaceAccess("test-workspace");

      // Anonymous users get viewer access to existing workspaces
      expect(access.granted).toBe(true);
      expect(access.role).toBe("viewer");
    });

    it("should grant viewer access to users without email", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: undefined,
      });

      const access = await checkWorkspaceAccess("test-workspace");

      // Users without email are treated as anonymous
      expect(access.granted).toBe(true);
      expect(access.role).toBe("viewer");
    });

    it("should deny access when workspace does not exist", async () => {
      (getWorkspace as Mock).mockResolvedValue(null);

      const access = await checkWorkspaceAccess("nonexistent-workspace");

      expect(access.granted).toBe(false);
      expect(access.role).toBeNull();
    });

    it("should grant access based on group membership", async () => {
      const access = await checkWorkspaceAccess("test-workspace");

      expect(access.granted).toBe(true);
      expect(access.role).toBe("editor");
      expect(access.permissions.read).toBe(true);
      expect(access.permissions.write).toBe(true);
      expect(access.permissions.manageMembers).toBe(false);
    });

    it("should grant access based on direct grant", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: "admin@example.com",
        groups: [], // No group membership
      });

      const access = await checkWorkspaceAccess("test-workspace");

      expect(access.granted).toBe(true);
      expect(access.role).toBe("owner");
      expect(access.permissions.manageMembers).toBe(true);
    });

    it("should use highest role from multiple sources", async () => {
      // User is in developers group (editor) AND has owner direct grant
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: "admin@example.com",
        groups: ["developers@example.com"],
      });

      const access = await checkWorkspaceAccess("test-workspace");

      // Should get owner role (highest)
      expect(access.granted).toBe(true);
      expect(access.role).toBe("owner");
    });

    it("should ignore expired direct grants", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: "expired@example.com",
        groups: [],
      });

      const access = await checkWorkspaceAccess("test-workspace");

      expect(access.granted).toBe(false);
      expect(access.role).toBeNull();
    });

    it("should honor non-expired direct grants", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: "guest@example.com",
        groups: [],
      });

      const access = await checkWorkspaceAccess("test-workspace");

      expect(access.granted).toBe(true);
      expect(access.role).toBe("viewer");
    });

    it("should deny access when user has no matching groups or grants", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: "stranger@example.com",
        groups: ["unrelated-group"],
      });

      const access = await checkWorkspaceAccess("test-workspace");

      expect(access.granted).toBe(false);
      expect(access.role).toBeNull();
    });

    describe("with requiredRole", () => {
      it("should grant access when user role meets requirement", async () => {
        const access = await checkWorkspaceAccess("test-workspace", "viewer");

        expect(access.granted).toBe(true);
        expect(access.role).toBe("editor");
      });

      it("should grant access when user role exceeds requirement", async () => {
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          email: "admin@example.com",
          groups: [],
        });

        const access = await checkWorkspaceAccess("test-workspace", "editor");

        expect(access.granted).toBe(true);
        expect(access.role).toBe("owner");
      });

      it("should deny access when user role is insufficient", async () => {
        const access = await checkWorkspaceAccess("test-workspace", "owner");

        expect(access.granted).toBe(false);
        expect(access.role).toBe("editor"); // User has editor, not owner
        // Permissions should still reflect actual role
        expect(access.permissions.write).toBe(true);
      });
    });

    describe("caching", () => {
      it("should cache authorization decisions", async () => {
        // First call
        await checkWorkspaceAccess("test-workspace");

        // Second call should use cache
        await checkWorkspaceAccess("test-workspace");

        // getWorkspace should only be called once
        expect(getWorkspace).toHaveBeenCalledTimes(1);
      });

      it("should apply requiredRole to cached results", async () => {
        // First call caches result
        const firstAccess = await checkWorkspaceAccess("test-workspace");
        expect(firstAccess.granted).toBe(true);
        expect(firstAccess.role).toBe("editor");

        // Second call with stricter requirement
        const secondAccess = await checkWorkspaceAccess(
          "test-workspace",
          "owner"
        );
        expect(secondAccess.granted).toBe(false);
        expect(secondAccess.role).toBe("editor");

        // Still only one K8s call
        expect(getWorkspace).toHaveBeenCalledTimes(1);
      });
    });
  });

  describe("getAccessibleWorkspaces", () => {
    const workspace2: Workspace = {
      ...mockWorkspace,
      metadata: {
        ...mockWorkspace.metadata,
        name: "workspace-2",
      },
      spec: {
        ...mockWorkspace.spec,
        displayName: "Workspace 2",
        roleBindings: [
          {
            groups: ["team-alpha"],
            role: "viewer",
          },
        ],
      },
    };

    const workspace3: Workspace = {
      ...mockWorkspace,
      metadata: {
        ...mockWorkspace.metadata,
        name: "workspace-3",
      },
      spec: {
        ...mockWorkspace.spec,
        displayName: "Workspace 3",
        roleBindings: [
          {
            groups: ["other-team"],
            role: "owner",
          },
        ],
      },
    };

    beforeEach(() => {
      (listWorkspaces as Mock).mockResolvedValue([
        mockWorkspace,
        workspace2,
        workspace3,
      ]);
    });

    it("should return all workspaces with viewer access for anonymous users", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        provider: "anonymous",
      });

      const result = await getAccessibleWorkspaces();

      // Anonymous users get viewer access to all workspaces
      expect(result).toHaveLength(3);
      result.forEach((r) => {
        expect(r.access.granted).toBe(true);
        expect(r.access.role).toBe("viewer");
      });
    });

    it("should return workspaces user has access to", async () => {
      const result = await getAccessibleWorkspaces();

      expect(result).toHaveLength(2);
      expect(result.map((r) => r.workspace.metadata.name)).toContain(
        "test-workspace"
      );
      expect(result.map((r) => r.workspace.metadata.name)).toContain(
        "workspace-2"
      );
      expect(result.map((r) => r.workspace.metadata.name)).not.toContain(
        "workspace-3"
      );
    });

    it("should include access information for each workspace", async () => {
      const result = await getAccessibleWorkspaces();

      const testWs = result.find(
        (r) => r.workspace.metadata.name === "test-workspace"
      );
      expect(testWs?.access.role).toBe("editor");
      expect(testWs?.access.permissions.write).toBe(true);

      const ws2 = result.find(
        (r) => r.workspace.metadata.name === "workspace-2"
      );
      expect(ws2?.access.role).toBe("viewer");
      expect(ws2?.access.permissions.write).toBe(false);
    });

    it("should filter by minimum role", async () => {
      const result = await getAccessibleWorkspaces("editor");

      expect(result).toHaveLength(1);
      expect(result[0].workspace.metadata.name).toBe("test-workspace");
    });
  });

  describe("hasWorkspaceRole", () => {
    it("should return true when user has required role", async () => {
      const result = await hasWorkspaceRole("test-workspace", "editor");

      expect(result).toBe(true);
    });

    it("should return true when user has higher role", async () => {
      const result = await hasWorkspaceRole("test-workspace", "viewer");

      expect(result).toBe(true);
    });

    it("should return false when user has lower role", async () => {
      const result = await hasWorkspaceRole("test-workspace", "owner");

      expect(result).toBe(false);
    });
  });

  describe("requireWorkspaceAccess", () => {
    it("should return access when granted", async () => {
      const access = await requireWorkspaceAccess("test-workspace");

      expect(access.granted).toBe(true);
      expect(access.role).toBe("editor");
    });

    it("should throw when access denied", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: "stranger@example.com",
        groups: [],
      });

      await expect(requireWorkspaceAccess("test-workspace")).rejects.toThrow(
        "Access denied to workspace: test-workspace"
      );
    });

    it("should throw with role info when role insufficient", async () => {
      await expect(
        requireWorkspaceAccess("test-workspace", "owner")
      ).rejects.toThrow(
        "Insufficient workspace permissions: requires owner, have editor"
      );
    });
  });

  describe("role hierarchy", () => {
    const testCases: Array<{
      role: WorkspaceRole;
      canRead: boolean;
      canWrite: boolean;
      canDelete: boolean;
      canManageMembers: boolean;
    }> = [
      {
        role: "viewer",
        canRead: true,
        canWrite: false,
        canDelete: false,
        canManageMembers: false,
      },
      {
        role: "editor",
        canRead: true,
        canWrite: true,
        canDelete: true,
        canManageMembers: false,
      },
      {
        role: "owner",
        canRead: true,
        canWrite: true,
        canDelete: true,
        canManageMembers: true,
      },
    ];

    testCases.forEach(
      ({ role, canRead, canWrite, canDelete, canManageMembers }) => {
        it(`${role} should have correct permissions`, async () => {
          (getWorkspace as Mock).mockResolvedValue({
            ...mockWorkspace,
            spec: {
              ...mockWorkspace.spec,
              roleBindings: [
                {
                  groups: ["developers@example.com"],
                  role: role,
                },
              ],
            },
          });

          const access = await checkWorkspaceAccess("test-workspace");

          expect(access.role).toBe(role);
          expect(access.permissions.read).toBe(canRead);
          expect(access.permissions.write).toBe(canWrite);
          expect(access.permissions.delete).toBe(canDelete);
          expect(access.permissions.manageMembers).toBe(canManageMembers);
        });
      }
    );
  });

  describe("case-insensitive email matching", () => {
    it("should match emails case-insensitively", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: "ADMIN@EXAMPLE.COM",
        groups: [],
      });

      const access = await checkWorkspaceAccess("test-workspace");

      expect(access.granted).toBe(true);
      expect(access.role).toBe("owner");
    });
  });
});
