import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { usePermissions, Permission, Permissions } from "./use-permissions";

// Mock useAuth
vi.mock("./use-auth", () => ({
  useAuth: vi.fn().mockReturnValue({
    role: "editor",
  }),
}));

// Import the mock to control it
import { useAuth } from "./use-auth";
const mockUseAuth = vi.mocked(useAuth);

describe("usePermissions", () => {
  describe("with viewer role", () => {
    it("should have view permissions", () => {
      mockUseAuth.mockReturnValue({ role: "viewer" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.can(Permission.AGENTS_VIEW)).toBe(true);
      expect(result.current.can(Permission.PROMPTPACKS_VIEW)).toBe(true);
      expect(result.current.can(Permission.TOOLS_VIEW)).toBe(true);
    });

    it("should not have write permissions", () => {
      mockUseAuth.mockReturnValue({ role: "viewer" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.can(Permission.AGENTS_DEPLOY)).toBe(false);
      expect(result.current.can(Permission.PROMPTPACKS_CREATE)).toBe(false);
      expect(result.current.can(Permission.TOOLS_EDIT)).toBe(false);
    });

    it("should not have admin permissions", () => {
      mockUseAuth.mockReturnValue({ role: "viewer" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.can(Permission.USERS_MANAGE)).toBe(false);
      expect(result.current.can(Permission.SETTINGS_EDIT)).toBe(false);
    });
  });

  describe("with editor role", () => {
    it("should have view permissions", () => {
      mockUseAuth.mockReturnValue({ role: "editor" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.can(Permission.AGENTS_VIEW)).toBe(true);
      expect(result.current.can(Permission.PROMPTPACKS_VIEW)).toBe(true);
    });

    it("should have write permissions", () => {
      mockUseAuth.mockReturnValue({ role: "editor" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.can(Permission.AGENTS_DEPLOY)).toBe(true);
      expect(result.current.can(Permission.AGENTS_SCALE)).toBe(true);
      expect(result.current.can(Permission.PROMPTPACKS_CREATE)).toBe(true);
      expect(result.current.can(Permission.TOOLS_EDIT)).toBe(true);
    });

    it("should not have admin permissions", () => {
      mockUseAuth.mockReturnValue({ role: "editor" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.can(Permission.USERS_MANAGE)).toBe(false);
      expect(result.current.can(Permission.SETTINGS_EDIT)).toBe(false);
    });
  });

  describe("with admin role", () => {
    it("should have all permissions", () => {
      mockUseAuth.mockReturnValue({ role: "admin" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.can(Permission.AGENTS_VIEW)).toBe(true);
      expect(result.current.can(Permission.AGENTS_DEPLOY)).toBe(true);
      expect(result.current.can(Permission.USERS_MANAGE)).toBe(true);
      expect(result.current.can(Permission.SETTINGS_EDIT)).toBe(true);
    });
  });

  describe("canAll", () => {
    it("should return true when user has all permissions", () => {
      mockUseAuth.mockReturnValue({ role: "editor" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(
        result.current.canAll([Permission.AGENTS_VIEW, Permission.AGENTS_DEPLOY])
      ).toBe(true);
    });

    it("should return false when user is missing a permission", () => {
      mockUseAuth.mockReturnValue({ role: "editor" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(
        result.current.canAll([Permission.AGENTS_DEPLOY, Permission.USERS_MANAGE])
      ).toBe(false);
    });

    it("should return true for empty array", () => {
      mockUseAuth.mockReturnValue({ role: "viewer" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.canAll([])).toBe(true);
    });
  });

  describe("canAny", () => {
    it("should return true when user has at least one permission", () => {
      mockUseAuth.mockReturnValue({ role: "editor" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(
        result.current.canAny([Permission.USERS_MANAGE, Permission.AGENTS_DEPLOY])
      ).toBe(true);
    });

    it("should return false when user has none of the permissions", () => {
      mockUseAuth.mockReturnValue({ role: "viewer" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(
        result.current.canAny([Permission.USERS_MANAGE, Permission.AGENTS_DEPLOY])
      ).toBe(false);
    });

    it("should return false for empty array", () => {
      mockUseAuth.mockReturnValue({ role: "admin" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.canAny([])).toBe(false);
    });
  });

  describe("permissions set", () => {
    it("should return a set of all permissions for the role", () => {
      mockUseAuth.mockReturnValue({ role: "viewer" } as ReturnType<typeof useAuth>);
      const { result } = renderHook(() => usePermissions());

      expect(result.current.permissions).toBeInstanceOf(Set);
      expect(result.current.permissions.has(Permission.AGENTS_VIEW)).toBe(true);
      expect(result.current.permissions.has(Permission.AGENTS_DEPLOY)).toBe(false);
    });
  });
});

describe("Permissions shortcuts", () => {
  it("should have correct agent permission mappings", () => {
    expect(Permissions.canViewAgents).toBe(Permission.AGENTS_VIEW);
    expect(Permissions.canScaleAgents).toBe(Permission.AGENTS_SCALE);
    expect(Permissions.canDeployAgents).toBe(Permission.AGENTS_DEPLOY);
    expect(Permissions.canDeleteAgents).toBe(Permission.AGENTS_DELETE);
  });

  it("should have correct prompt pack permission mappings", () => {
    expect(Permissions.canViewPromptPacks).toBe(Permission.PROMPTPACKS_VIEW);
    expect(Permissions.canCreatePromptPacks).toBe(Permission.PROMPTPACKS_CREATE);
    expect(Permissions.canEditPromptPacks).toBe(Permission.PROMPTPACKS_EDIT);
    expect(Permissions.canDeletePromptPacks).toBe(Permission.PROMPTPACKS_DELETE);
  });

  it("should have correct tool permission mappings", () => {
    expect(Permissions.canViewTools).toBe(Permission.TOOLS_VIEW);
    expect(Permissions.canCreateTools).toBe(Permission.TOOLS_CREATE);
    expect(Permissions.canEditTools).toBe(Permission.TOOLS_EDIT);
    expect(Permissions.canDeleteTools).toBe(Permission.TOOLS_DELETE);
  });

  it("should have correct admin permission mappings", () => {
    expect(Permissions.canManageUsers).toBe(Permission.USERS_MANAGE);
    expect(Permissions.canEditSettings).toBe(Permission.SETTINGS_EDIT);
  });
});
