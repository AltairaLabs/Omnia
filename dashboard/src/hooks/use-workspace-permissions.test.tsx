import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useWorkspacePermissions, hasPermission } from "./use-workspace-permissions";
import type { WorkspacePermissions } from "@/types/workspace";

// Mock the workspace context
const mockUseWorkspace = vi.fn();
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => mockUseWorkspace(),
}));

describe("useWorkspacePermissions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("when no workspace is selected", () => {
    beforeEach(() => {
      mockUseWorkspace.mockReturnValue({ currentWorkspace: null });
    });

    it("should return all permissions as false", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.canRead).toBe(false);
      expect(result.current.canWrite).toBe(false);
      expect(result.current.canDelete).toBe(false);
      expect(result.current.canManageMembers).toBe(false);
    });

    it("should return hasWorkspace as false", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.hasWorkspace).toBe(false);
    });

    it("should return role as null", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.role).toBeNull();
    });

    it("should return all role checks as false", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.isViewer).toBe(false);
      expect(result.current.isEditor).toBe(false);
      expect(result.current.isOwner).toBe(false);
    });
  });

  describe("when workspace with viewer role is selected", () => {
    beforeEach(() => {
      mockUseWorkspace.mockReturnValue({
        currentWorkspace: {
          name: "test-workspace",
          role: "viewer",
          permissions: {
            read: true,
            write: false,
            delete: false,
            manageMembers: false,
          },
        },
      });
    });

    it("should return read permission as true", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.canRead).toBe(true);
      expect(result.current.canWrite).toBe(false);
      expect(result.current.canDelete).toBe(false);
      expect(result.current.canManageMembers).toBe(false);
    });

    it("should return hasWorkspace as true", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.hasWorkspace).toBe(true);
    });

    it("should return viewer role checks correctly", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.role).toBe("viewer");
      expect(result.current.isViewer).toBe(true);
      expect(result.current.isEditor).toBe(false);
      expect(result.current.isOwner).toBe(false);
    });
  });

  describe("when workspace with editor role is selected", () => {
    beforeEach(() => {
      mockUseWorkspace.mockReturnValue({
        currentWorkspace: {
          name: "test-workspace",
          role: "editor",
          permissions: {
            read: true,
            write: true,
            delete: true,
            manageMembers: false,
          },
        },
      });
    });

    it("should return write and delete permissions as true", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.canRead).toBe(true);
      expect(result.current.canWrite).toBe(true);
      expect(result.current.canDelete).toBe(true);
      expect(result.current.canManageMembers).toBe(false);
    });

    it("should return editor role checks correctly", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.role).toBe("editor");
      expect(result.current.isViewer).toBe(false);
      expect(result.current.isEditor).toBe(true);
      expect(result.current.isOwner).toBe(false);
    });
  });

  describe("when workspace with owner role is selected", () => {
    beforeEach(() => {
      mockUseWorkspace.mockReturnValue({
        currentWorkspace: {
          name: "test-workspace",
          role: "owner",
          permissions: {
            read: true,
            write: true,
            delete: true,
            manageMembers: true,
          },
        },
      });
    });

    it("should return all permissions as true", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.canRead).toBe(true);
      expect(result.current.canWrite).toBe(true);
      expect(result.current.canDelete).toBe(true);
      expect(result.current.canManageMembers).toBe(true);
    });

    it("should return owner role checks correctly", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.role).toBe("owner");
      expect(result.current.isViewer).toBe(false);
      expect(result.current.isEditor).toBe(true); // owner is also considered an editor
      expect(result.current.isOwner).toBe(true);
    });

    it("should return the permissions object", () => {
      const { result } = renderHook(() => useWorkspacePermissions());

      expect(result.current.permissions).toEqual({
        read: true,
        write: true,
        delete: true,
        manageMembers: true,
      });
    });
  });
});

describe("hasPermission", () => {
  const fullPermissions: WorkspacePermissions = {
    read: true,
    write: true,
    delete: true,
    manageMembers: true,
  };

  const limitedPermissions: WorkspacePermissions = {
    read: true,
    write: false,
    delete: false,
    manageMembers: false,
  };

  it("should return true for granted permissions", () => {
    expect(hasPermission(fullPermissions, "read")).toBe(true);
    expect(hasPermission(fullPermissions, "write")).toBe(true);
    expect(hasPermission(fullPermissions, "delete")).toBe(true);
    expect(hasPermission(fullPermissions, "manageMembers")).toBe(true);
  });

  it("should return false for denied permissions", () => {
    expect(hasPermission(limitedPermissions, "write")).toBe(false);
    expect(hasPermission(limitedPermissions, "delete")).toBe(false);
    expect(hasPermission(limitedPermissions, "manageMembers")).toBe(false);
  });

  it("should return true for read permission on limited permissions", () => {
    expect(hasPermission(limitedPermissions, "read")).toBe(true);
  });
});
