import { describe, it, expect } from "vitest";
import {
  Permission,
  getPermissionsForRole,
  roleHasPermission,
  userHasPermission,
  userHasAllPermissions,
  userHasAnyPermission,
  PermissionGroups,
} from "./permissions";
import type { User } from "./types";

describe("permissions", () => {
  describe("Permission constants", () => {
    it("should have agent permissions", () => {
      expect(Permission.AGENTS_VIEW).toBe("agents:view");
      expect(Permission.AGENTS_SCALE).toBe("agents:scale");
      expect(Permission.AGENTS_DEPLOY).toBe("agents:deploy");
      expect(Permission.AGENTS_DELETE).toBe("agents:delete");
    });

    it("should have promptpack permissions", () => {
      expect(Permission.PROMPTPACKS_VIEW).toBe("promptpacks:view");
      expect(Permission.PROMPTPACKS_CREATE).toBe("promptpacks:create");
      expect(Permission.PROMPTPACKS_EDIT).toBe("promptpacks:edit");
      expect(Permission.PROMPTPACKS_DELETE).toBe("promptpacks:delete");
    });

    it("should have tool permissions", () => {
      expect(Permission.TOOLS_VIEW).toBe("tools:view");
      expect(Permission.TOOLS_CREATE).toBe("tools:create");
      expect(Permission.TOOLS_EDIT).toBe("tools:edit");
      expect(Permission.TOOLS_DELETE).toBe("tools:delete");
    });

    it("should have admin permissions", () => {
      expect(Permission.USERS_VIEW).toBe("users:view");
      expect(Permission.USERS_MANAGE).toBe("users:manage");
      expect(Permission.SETTINGS_VIEW).toBe("settings:view");
      expect(Permission.SETTINGS_EDIT).toBe("settings:edit");
    });

    it("should have API key permissions", () => {
      expect(Permission.API_KEYS_VIEW_OWN).toBe("apikeys:view:own");
      expect(Permission.API_KEYS_MANAGE_OWN).toBe("apikeys:manage:own");
      expect(Permission.API_KEYS_VIEW_ALL).toBe("apikeys:view:all");
      expect(Permission.API_KEYS_MANAGE_ALL).toBe("apikeys:manage:all");
    });
  });

  describe("getPermissionsForRole", () => {
    it("should return viewer permissions for viewer role", () => {
      const permissions = getPermissionsForRole("viewer");

      expect(permissions.has(Permission.AGENTS_VIEW)).toBe(true);
      expect(permissions.has(Permission.PROMPTPACKS_VIEW)).toBe(true);
      expect(permissions.has(Permission.TOOLS_VIEW)).toBe(true);
      expect(permissions.has(Permission.LOGS_VIEW)).toBe(true);
      expect(permissions.has(Permission.METRICS_VIEW)).toBe(true);
      expect(permissions.has(Permission.SESSIONS_VIEW)).toBe(true);
      expect(permissions.has(Permission.API_KEYS_VIEW_OWN)).toBe(true);
      expect(permissions.has(Permission.API_KEYS_MANAGE_OWN)).toBe(true);

      // Should NOT have write permissions
      expect(permissions.has(Permission.AGENTS_SCALE)).toBe(false);
      expect(permissions.has(Permission.AGENTS_DEPLOY)).toBe(false);
      expect(permissions.has(Permission.USERS_MANAGE)).toBe(false);
    });

    it("should return editor permissions including viewer permissions", () => {
      const permissions = getPermissionsForRole("editor");

      // Inherited viewer permissions
      expect(permissions.has(Permission.AGENTS_VIEW)).toBe(true);
      expect(permissions.has(Permission.PROMPTPACKS_VIEW)).toBe(true);

      // Editor-specific permissions
      expect(permissions.has(Permission.AGENTS_SCALE)).toBe(true);
      expect(permissions.has(Permission.AGENTS_DEPLOY)).toBe(true);
      expect(permissions.has(Permission.AGENTS_DELETE)).toBe(true);
      expect(permissions.has(Permission.PROMPTPACKS_CREATE)).toBe(true);
      expect(permissions.has(Permission.PROMPTPACKS_EDIT)).toBe(true);
      expect(permissions.has(Permission.PROMPTPACKS_DELETE)).toBe(true);
      expect(permissions.has(Permission.TOOLS_CREATE)).toBe(true);
      expect(permissions.has(Permission.SESSIONS_DELETE)).toBe(true);

      // Should NOT have admin permissions
      expect(permissions.has(Permission.USERS_MANAGE)).toBe(false);
      expect(permissions.has(Permission.SETTINGS_EDIT)).toBe(false);
    });

    it("should return admin permissions including all lower role permissions", () => {
      const permissions = getPermissionsForRole("admin");

      // Inherited viewer permissions
      expect(permissions.has(Permission.AGENTS_VIEW)).toBe(true);
      expect(permissions.has(Permission.API_KEYS_VIEW_OWN)).toBe(true);

      // Inherited editor permissions
      expect(permissions.has(Permission.AGENTS_SCALE)).toBe(true);
      expect(permissions.has(Permission.PROMPTPACKS_CREATE)).toBe(true);

      // Admin-specific permissions
      expect(permissions.has(Permission.USERS_VIEW)).toBe(true);
      expect(permissions.has(Permission.USERS_MANAGE)).toBe(true);
      expect(permissions.has(Permission.SETTINGS_VIEW)).toBe(true);
      expect(permissions.has(Permission.SETTINGS_EDIT)).toBe(true);
      expect(permissions.has(Permission.API_KEYS_VIEW_ALL)).toBe(true);
      expect(permissions.has(Permission.API_KEYS_MANAGE_ALL)).toBe(true);
    });

    it("should have more permissions at higher roles", () => {
      const viewerPerms = getPermissionsForRole("viewer");
      const editorPerms = getPermissionsForRole("editor");
      const adminPerms = getPermissionsForRole("admin");

      expect(viewerPerms.size).toBeLessThan(editorPerms.size);
      expect(editorPerms.size).toBeLessThan(adminPerms.size);
    });
  });

  describe("roleHasPermission", () => {
    it("should return true for permissions the role has", () => {
      expect(roleHasPermission("viewer", Permission.AGENTS_VIEW)).toBe(true);
      expect(roleHasPermission("editor", Permission.AGENTS_SCALE)).toBe(true);
      expect(roleHasPermission("admin", Permission.USERS_MANAGE)).toBe(true);
    });

    it("should return false for permissions the role lacks", () => {
      expect(roleHasPermission("viewer", Permission.AGENTS_SCALE)).toBe(false);
      expect(roleHasPermission("editor", Permission.USERS_MANAGE)).toBe(false);
    });

    it("should respect role hierarchy", () => {
      // Editor has viewer permissions
      expect(roleHasPermission("editor", Permission.AGENTS_VIEW)).toBe(true);
      // Admin has both viewer and editor permissions
      expect(roleHasPermission("admin", Permission.AGENTS_VIEW)).toBe(true);
      expect(roleHasPermission("admin", Permission.AGENTS_SCALE)).toBe(true);
    });
  });

  describe("userHasPermission", () => {
    const createUser = (role: "viewer" | "editor" | "admin"): User => ({
      id: "test-user",
      username: "testuser",
      displayName: "Test User",
      email: "test@example.com",
      groups: [],
      role,
      provider: "builtin",
    });

    it("should check user role for permission", () => {
      const viewer = createUser("viewer");
      const editor = createUser("editor");
      const admin = createUser("admin");

      expect(userHasPermission(viewer, Permission.AGENTS_VIEW)).toBe(true);
      expect(userHasPermission(viewer, Permission.AGENTS_SCALE)).toBe(false);

      expect(userHasPermission(editor, Permission.AGENTS_SCALE)).toBe(true);
      expect(userHasPermission(editor, Permission.USERS_MANAGE)).toBe(false);

      expect(userHasPermission(admin, Permission.USERS_MANAGE)).toBe(true);
    });
  });

  describe("userHasAllPermissions", () => {
    const createUser = (role: "viewer" | "editor" | "admin"): User => ({
      id: "test-user",
      username: "testuser",
      displayName: "Test User",
      email: "test@example.com",
      groups: [],
      role,
      provider: "builtin",
    });

    it("should return true if user has all permissions", () => {
      const editor = createUser("editor");
      expect(
        userHasAllPermissions(editor, [
          Permission.AGENTS_VIEW,
          Permission.AGENTS_SCALE,
          Permission.AGENTS_DEPLOY,
        ])
      ).toBe(true);
    });

    it("should return false if user lacks any permission", () => {
      const viewer = createUser("viewer");
      expect(
        userHasAllPermissions(viewer, [
          Permission.AGENTS_VIEW,
          Permission.AGENTS_SCALE, // viewer doesn't have this
        ])
      ).toBe(false);
    });

    it("should return true for empty permissions array", () => {
      const viewer = createUser("viewer");
      expect(userHasAllPermissions(viewer, [])).toBe(true);
    });
  });

  describe("userHasAnyPermission", () => {
    const createUser = (role: "viewer" | "editor" | "admin"): User => ({
      id: "test-user",
      username: "testuser",
      displayName: "Test User",
      email: "test@example.com",
      groups: [],
      role,
      provider: "builtin",
    });

    it("should return true if user has any of the permissions", () => {
      const viewer = createUser("viewer");
      expect(
        userHasAnyPermission(viewer, [
          Permission.USERS_MANAGE, // viewer doesn't have this
          Permission.AGENTS_VIEW, // viewer has this
        ])
      ).toBe(true);
    });

    it("should return false if user has none of the permissions", () => {
      const viewer = createUser("viewer");
      expect(
        userHasAnyPermission(viewer, [
          Permission.USERS_MANAGE,
          Permission.SETTINGS_EDIT,
        ])
      ).toBe(false);
    });

    it("should return false for empty permissions array", () => {
      const viewer = createUser("viewer");
      expect(userHasAnyPermission(viewer, [])).toBe(false);
    });
  });

  describe("PermissionGroups", () => {
    it("should have AGENT_WRITE group with correct permissions", () => {
      expect(PermissionGroups.AGENT_WRITE).toContain(Permission.AGENTS_SCALE);
      expect(PermissionGroups.AGENT_WRITE).toContain(Permission.AGENTS_DEPLOY);
      expect(PermissionGroups.AGENT_WRITE).toContain(Permission.AGENTS_DELETE);
      expect(PermissionGroups.AGENT_WRITE).toHaveLength(3);
    });

    it("should have PROMPTPACK_WRITE group with correct permissions", () => {
      expect(PermissionGroups.PROMPTPACK_WRITE).toContain(
        Permission.PROMPTPACKS_CREATE
      );
      expect(PermissionGroups.PROMPTPACK_WRITE).toContain(
        Permission.PROMPTPACKS_EDIT
      );
      expect(PermissionGroups.PROMPTPACK_WRITE).toContain(
        Permission.PROMPTPACKS_DELETE
      );
      expect(PermissionGroups.PROMPTPACK_WRITE).toHaveLength(3);
    });

    it("should have TOOLS_WRITE group with correct permissions", () => {
      expect(PermissionGroups.TOOLS_WRITE).toContain(Permission.TOOLS_CREATE);
      expect(PermissionGroups.TOOLS_WRITE).toContain(Permission.TOOLS_EDIT);
      expect(PermissionGroups.TOOLS_WRITE).toContain(Permission.TOOLS_DELETE);
      expect(PermissionGroups.TOOLS_WRITE).toHaveLength(3);
    });

    it("should have ADMIN_OPS group with correct permissions", () => {
      expect(PermissionGroups.ADMIN_OPS).toContain(Permission.USERS_MANAGE);
      expect(PermissionGroups.ADMIN_OPS).toContain(Permission.SETTINGS_EDIT);
      expect(PermissionGroups.ADMIN_OPS).toHaveLength(2);
    });

    it("should allow editor to have AGENT_WRITE permissions", () => {
      const editorPerms = getPermissionsForRole("editor");
      const hasAllAgentWrite = PermissionGroups.AGENT_WRITE.every((p) =>
        editorPerms.has(p)
      );
      expect(hasAllAgentWrite).toBe(true);
    });

    it("should not allow viewer to have AGENT_WRITE permissions", () => {
      const viewerPerms = getPermissionsForRole("viewer");
      const hasAnyAgentWrite = PermissionGroups.AGENT_WRITE.some((p) =>
        viewerPerms.has(p)
      );
      expect(hasAnyAgentWrite).toBe(false);
    });
  });
});
