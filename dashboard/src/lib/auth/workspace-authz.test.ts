import { describe, it, expect, beforeEach, vi, Mock } from "vitest";
import {
  checkWorkspaceAccess,
  getAccessibleWorkspaces,
  hasWorkspaceRole,
  requireWorkspaceAccess,
  isPlatformAdmin,
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
      // Anonymous access enabled with default viewer role
      anonymousAccess: {
        enabled: true,
      },
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
    describe("anonymous access", () => {
      it("should grant viewer access to anonymous users when enabled", async () => {
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });

        const access = await checkWorkspaceAccess("test-workspace");

        expect(access.granted).toBe(true);
        expect(access.role).toBe("viewer");
      });

      it("should resolve real role via UPN when the email claim is absent", async () => {
        // Entra members with no `mail` attribute authenticate with a UPN
        // (carried in `username`) but no email claim. They must resolve their
        // real role via group membership — NOT be treated as anonymous.
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          email: undefined,
        });

        const access = await checkWorkspaceAccess("test-workspace");

        // mockUser is in developers@example.com → editor (not anonymous viewer)
        expect(access.granted).toBe(true);
        expect(access.role).toBe("editor");
      });

      it("should still treat the anonymous provider as anonymous even with a username", async () => {
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
          email: undefined,
        });

        const access = await checkWorkspaceAccess("test-workspace");

        // anonymousAccess is enabled (default viewer) — anonymous provider
        // never resolves a group/grant role regardless of username.
        expect(access.granted).toBe(true);
        expect(access.role).toBe("viewer");
      });

      it("should deny anonymous access when not configured", async () => {
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });
        (getWorkspace as Mock).mockResolvedValue({
          ...mockWorkspace,
          spec: {
            ...mockWorkspace.spec,
            anonymousAccess: undefined, // No anonymous access config
          },
        });

        const access = await checkWorkspaceAccess("test-workspace");

        expect(access.granted).toBe(false);
        expect(access.role).toBeNull();
      });

      it("should deny anonymous access when explicitly disabled", async () => {
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });
        (getWorkspace as Mock).mockResolvedValue({
          ...mockWorkspace,
          spec: {
            ...mockWorkspace.spec,
            anonymousAccess: { enabled: false },
          },
        });

        const access = await checkWorkspaceAccess("test-workspace");

        expect(access.granted).toBe(false);
        expect(access.role).toBeNull();
      });

      it("should grant configured role to anonymous users", async () => {
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });
        (getWorkspace as Mock).mockResolvedValue({
          ...mockWorkspace,
          spec: {
            ...mockWorkspace.spec,
            anonymousAccess: { enabled: true, role: "editor" },
          },
        });

        const access = await checkWorkspaceAccess("test-workspace");

        expect(access.granted).toBe(true);
        expect(access.role).toBe("editor");
        expect(access.permissions.write).toBe(true);
      });

      it("should log warning for editor anonymous access", async () => {
        const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });
        (getWorkspace as Mock).mockResolvedValue({
          ...mockWorkspace,
          spec: {
            ...mockWorkspace.spec,
            anonymousAccess: { enabled: true, role: "editor" },
          },
        });

        await checkWorkspaceAccess("test-workspace");

        expect(warnSpy).toHaveBeenCalledWith(
          expect.stringContaining("[SECURITY WARNING]")
        );
        expect(warnSpy).toHaveBeenCalledWith(
          expect.stringContaining("EDITOR access to anonymous users")
        );
        warnSpy.mockRestore();
      });

      it("should log warning for owner anonymous access", async () => {
        const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });
        (getWorkspace as Mock).mockResolvedValue({
          ...mockWorkspace,
          spec: {
            ...mockWorkspace.spec,
            anonymousAccess: { enabled: true, role: "owner" },
          },
        });

        await checkWorkspaceAccess("test-workspace");

        expect(warnSpy).toHaveBeenCalledWith(
          expect.stringContaining("[SECURITY WARNING]")
        );
        expect(warnSpy).toHaveBeenCalledWith(
          expect.stringContaining("OWNER access to anonymous users")
        );
        warnSpy.mockRestore();
      });

      it("should not log warning for viewer anonymous access", async () => {
        const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });

        await checkWorkspaceAccess("test-workspace");

        expect(warnSpy).not.toHaveBeenCalled();
        warnSpy.mockRestore();
      });

      it("should respect requiredRole for anonymous users", async () => {
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });

        // Anonymous has viewer, but editor is required
        const access = await checkWorkspaceAccess("test-workspace", "editor");

        expect(access.granted).toBe(false);
        expect(access.role).toBe("viewer");
        expect(access.permissions.read).toBe(true);
      });

      it("should report notFound for anonymous users when workspace is missing", async () => {
        (getUser as Mock).mockResolvedValue({
          ...mockUser,
          provider: "anonymous",
        });
        (getWorkspace as Mock).mockResolvedValue(null);

        const access = await checkWorkspaceAccess("nonexistent-workspace");

        expect(access.granted).toBe(false);
        expect(access.notFound).toBe(true);
      });
    });

    it("should report notFound when workspace does not exist", async () => {
      (getWorkspace as Mock).mockResolvedValue(null);

      const access = await checkWorkspaceAccess("nonexistent-workspace");

      expect(access.granted).toBe(false);
      expect(access.role).toBeNull();
      // Distinct from "exists but denied" so guards can return 404, not 403.
      expect(access.notFound).toBe(true);
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

    it("should match directGrants by UPN when the email claim is absent", async () => {
      // No email claim (e.g. Entra user without a mailbox); the UPN lives in
      // `username` and must satisfy a directGrant keyed on that address.
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        email: undefined,
        username: "admin@example.com",
        groups: [],
      });

      const access = await checkWorkspaceAccess("test-workspace");

      expect(access.granted).toBe(true);
      expect(access.role).toBe("owner");
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
        anonymousAccess: undefined, // No anonymous access
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
        anonymousAccess: undefined, // No anonymous access
      },
    };

    beforeEach(() => {
      (listWorkspaces as Mock).mockResolvedValue([
        mockWorkspace,
        workspace2,
        workspace3,
      ]);
    });

    it("should return only workspaces with anonymous access enabled for anonymous users", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        provider: "anonymous",
      });

      // Only mockWorkspace has anonymousAccess enabled
      const result = await getAccessibleWorkspaces();

      // Anonymous users only get access to workspaces with anonymousAccess enabled
      expect(result).toHaveLength(1);
      expect(result[0].workspace.metadata.name).toBe("test-workspace");
      expect(result[0].access.granted).toBe(true);
      expect(result[0].access.role).toBe("viewer");
    });

    it("should respect configured anonymous role when listing workspaces", async () => {
      (getUser as Mock).mockResolvedValue({
        ...mockUser,
        provider: "anonymous",
      });
      // Add anonymousAccess with editor role to workspace2
      const workspace2WithAnon = {
        ...workspace2,
        spec: {
          ...workspace2.spec,
          anonymousAccess: { enabled: true, role: "editor" as WorkspaceRole },
        },
      };
      (listWorkspaces as Mock).mockResolvedValue([
        mockWorkspace,
        workspace2WithAnon,
        workspace3,
      ]);

      const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
      const result = await getAccessibleWorkspaces();

      expect(result).toHaveLength(2);

      const testWs = result.find(r => r.workspace.metadata.name === "test-workspace");
      expect(testWs?.access.role).toBe("viewer");

      const ws2 = result.find(r => r.workspace.metadata.name === "workspace-2");
      expect(ws2?.access.role).toBe("editor");

      // Should have logged warning for editor access
      expect(warnSpy).toHaveBeenCalled();
      warnSpy.mockRestore();
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

describe("isPlatformAdmin", () => {
  const base: User = {
    id: "u",
    username: "admin",
    email: "admin@example.com",
    groups: [],
    role: "admin",
    provider: "builtin",
  };

  it("is true for an authenticated admin (builtin)", () => {
    expect(isPlatformAdmin(base)).toBe(true);
  });

  it("is true for an oauth admin", () => {
    expect(isPlatformAdmin({ ...base, provider: "oauth" })).toBe(true);
  });

  it("is false for an anonymous user even with the admin role", () => {
    // An unauthenticated visitor must never get manage-all-workspaces, even
    // when dev sets anonymousRole=admin.
    expect(isPlatformAdmin({ ...base, provider: "anonymous" })).toBe(false);
  });

  it("is false for a non-admin (editor)", () => {
    expect(isPlatformAdmin({ ...base, role: "editor" })).toBe(false);
  });

  it("is false for a viewer", () => {
    expect(isPlatformAdmin({ ...base, role: "viewer" })).toBe(false);
  });
});

describe("checkWorkspaceAccess platform-admin override", () => {
  // platadmin matches no roleBinding (no groups) and no directGrant (email not
  // listed) in mockWorkspace, so computeWorkspaceRole returns null for them.
  const adminUser: User = {
    id: "pa",
    username: "platadmin",
    email: "platadmin@example.com",
    groups: [],
    role: "admin",
    provider: "builtin",
  };

  const wsFixture: Workspace = {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Workspace",
    metadata: { name: "test-workspace", creationTimestamp: "2024-01-15T10:00:00Z" },
    spec: {
      displayName: "Test",
      description: "",
      environment: "development",
      namespace: { name: "test-ns", create: true },
      roleBindings: [{ groups: ["owners@example.com"], role: "owner" }],
      directGrants: [{ user: "admin@example.com", role: "owner" }],
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
    clearAuthzCache();
    (getWorkspace as Mock).mockResolvedValue(wsFixture);
    (listWorkspaces as Mock).mockResolvedValue([wsFixture]);
  });

  it("grants manage-only access to an admin with no explicit grant", async () => {
    (getUser as Mock).mockResolvedValue(adminUser);
    const access = await checkWorkspaceAccess("test-workspace");
    expect(access.granted).toBe(true);
    expect(access.role).toBeNull();
    expect(access.permissions.manageMembers).toBe(true);
    expect(access.permissions.read).toBe(false);
    expect(access.permissions.write).toBe(false);
    expect(access.permissions.delete).toBe(false);
  });

  it("denies an admin a DATA role they have not self-granted", async () => {
    (getUser as Mock).mockResolvedValue(adminUser);
    const access = await checkWorkspaceAccess("test-workspace", "viewer");
    expect(access.granted).toBe(false);
  });

  it("does NOT grant manage access to a non-admin with no grant", async () => {
    (getUser as Mock).mockResolvedValue({ ...adminUser, role: "viewer" as const });
    const access = await checkWorkspaceAccess("test-workspace");
    expect(access.granted).toBe(false);
    expect(access.permissions.manageMembers).toBe(false);
  });

  it("uses the real grant for an admin who has self-granted (not the override)", async () => {
    // admin@example.com holds a directGrant of owner in mockWorkspace.
    (getUser as Mock).mockResolvedValue({ ...adminUser, email: "admin@example.com" });
    const access = await checkWorkspaceAccess("test-workspace");
    expect(access.granted).toBe(true);
    expect(access.role).toBe("owner");
    expect(access.permissions.read).toBe(true);
  });

  it("lists EVERY workspace for an admin with no grant (no minimumRole)", async () => {
    (getUser as Mock).mockResolvedValue(adminUser);
    const result = await getAccessibleWorkspaces();
    expect(result).toHaveLength(1);
    expect(result[0].workspace.metadata.name).toBe("test-workspace");
    expect(result[0].access.permissions.manageMembers).toBe(true);
    expect(result[0].access.role).toBeNull();
  });

  it("excludes manage-only workspaces when a DATA minimumRole is requested", async () => {
    (getUser as Mock).mockResolvedValue(adminUser);
    const result = await getAccessibleWorkspaces("editor");
    expect(result).toHaveLength(0);
  });

  it("does not list ungranted workspaces for a non-admin", async () => {
    (getUser as Mock).mockResolvedValue({ ...adminUser, role: "viewer" as const });
    const result = await getAccessibleWorkspaces();
    expect(result).toHaveLength(0);
  });
});

describe("API-key workspace scope (#1561)", () => {
  const mockWorkspace: Workspace = {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Workspace",
    metadata: { name: "test-workspace", creationTimestamp: "2024-01-15T10:00:00Z" },
    spec: {
      displayName: "Test Workspace",
      description: "A test workspace",
      environment: "development",
      namespace: { name: "test-ns", create: true },
      roleBindings: [
        { groups: ["developers@example.com"], role: "editor" },
      ],
      directGrants: [],
    },
  };

  beforeEach(() => {
    vi.clearAllMocks();
    clearAuthzCache();
    (getWorkspace as Mock).mockResolvedValue(mockWorkspace);
    (listWorkspaces as Mock).mockResolvedValue([mockWorkspace]);
  });

  const keyUser = (over: Partial<User>): User => ({
    id: "u1", username: "apikey:ci", email: "alice@example.com",
    groups: ["developers@example.com"], role: "editor", provider: "proxy", ...over,
  });

  it("grants the owner's role when the workspace is in the allowlist", async () => {
    (getUser as Mock).mockResolvedValue(keyUser({ apiKeyScope: { workspaces: ["test-workspace"] } }));
    const access = await checkWorkspaceAccess("test-workspace");
    expect(access.granted).toBe(true);
    expect(access.role).toBe("editor");
  });

  it("denies a workspace outside the allowlist even if the role would match", async () => {
    (getUser as Mock).mockResolvedValue(keyUser({ apiKeyScope: { workspaces: ["other-ws"] } }));
    const access = await checkWorkspaceAccess("test-workspace");
    expect(access.granted).toBe(false);
  });

  it("is unrestricted when the allowlist is empty/undefined", async () => {
    (getUser as Mock).mockResolvedValue(keyUser({ apiKeyScope: { workspaces: undefined } }));
    const access = await checkWorkspaceAccess("test-workspace");
    expect(access.granted).toBe(true);
    expect(access.role).toBe("editor");
  });

  it("suppresses platform-admin for a scoped admin key", async () => {
    (getUser as Mock).mockResolvedValue(keyUser({
      role: "admin", groups: [], email: "admin-key", apiKeyScope: { workspaces: ["test-workspace"] },
    }));
    const access = await checkWorkspaceAccess("test-workspace");
    expect(access.granted).toBe(false); // no group/grant match + platform-admin suppressed
  });

  it("keeps platform-admin for an unscoped admin key", async () => {
    (getUser as Mock).mockResolvedValue(keyUser({
      role: "admin", groups: [], email: "admin-key", apiKeyScope: { workspaces: undefined },
    }));
    const access = await checkWorkspaceAccess("test-workspace");
    expect(access.granted).toBe(true); // platform admin manage-only (no requiredRole)
  });
});
