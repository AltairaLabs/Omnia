import { describe, it, expect } from "vitest";
import {
  ROLE_HIERARCHY,
  ROLE_PERMISSIONS,
  NO_PERMISSIONS,
} from "./workspace";

describe("workspace types", () => {
  describe("ROLE_HIERARCHY", () => {
    it("should define viewer as lowest role", () => {
      expect(ROLE_HIERARCHY.viewer).toBe(1);
    });

    it("should define editor as middle role", () => {
      expect(ROLE_HIERARCHY.editor).toBe(2);
    });

    it("should define owner as highest role", () => {
      expect(ROLE_HIERARCHY.owner).toBe(3);
    });

    it("should have owner > editor > viewer", () => {
      expect(ROLE_HIERARCHY.owner).toBeGreaterThan(ROLE_HIERARCHY.editor);
      expect(ROLE_HIERARCHY.editor).toBeGreaterThan(ROLE_HIERARCHY.viewer);
    });
  });

  describe("ROLE_PERMISSIONS", () => {
    describe("viewer permissions", () => {
      it("should allow read", () => {
        expect(ROLE_PERMISSIONS.viewer.read).toBe(true);
      });

      it("should not allow write", () => {
        expect(ROLE_PERMISSIONS.viewer.write).toBe(false);
      });

      it("should not allow delete", () => {
        expect(ROLE_PERMISSIONS.viewer.delete).toBe(false);
      });

      it("should not allow manageMembers", () => {
        expect(ROLE_PERMISSIONS.viewer.manageMembers).toBe(false);
      });
    });

    describe("editor permissions", () => {
      it("should allow read", () => {
        expect(ROLE_PERMISSIONS.editor.read).toBe(true);
      });

      it("should allow write", () => {
        expect(ROLE_PERMISSIONS.editor.write).toBe(true);
      });

      it("should allow delete", () => {
        expect(ROLE_PERMISSIONS.editor.delete).toBe(true);
      });

      it("should not allow manageMembers", () => {
        expect(ROLE_PERMISSIONS.editor.manageMembers).toBe(false);
      });
    });

    describe("owner permissions", () => {
      it("should allow read", () => {
        expect(ROLE_PERMISSIONS.owner.read).toBe(true);
      });

      it("should allow write", () => {
        expect(ROLE_PERMISSIONS.owner.write).toBe(true);
      });

      it("should allow delete", () => {
        expect(ROLE_PERMISSIONS.owner.delete).toBe(true);
      });

      it("should allow manageMembers", () => {
        expect(ROLE_PERMISSIONS.owner.manageMembers).toBe(true);
      });
    });

    it("should have progressively more permissions at higher roles", () => {
      const viewerPerms = Object.values(ROLE_PERMISSIONS.viewer).filter(Boolean).length;
      const editorPerms = Object.values(ROLE_PERMISSIONS.editor).filter(Boolean).length;
      const ownerPerms = Object.values(ROLE_PERMISSIONS.owner).filter(Boolean).length;

      expect(ownerPerms).toBeGreaterThan(editorPerms);
      expect(editorPerms).toBeGreaterThan(viewerPerms);
    });
  });

  describe("NO_PERMISSIONS", () => {
    it("should not allow read", () => {
      expect(NO_PERMISSIONS.read).toBe(false);
    });

    it("should not allow write", () => {
      expect(NO_PERMISSIONS.write).toBe(false);
    });

    it("should not allow delete", () => {
      expect(NO_PERMISSIONS.delete).toBe(false);
    });

    it("should not allow manageMembers", () => {
      expect(NO_PERMISSIONS.manageMembers).toBe(false);
    });

    it("should have all permissions set to false", () => {
      const allFalse = Object.values(NO_PERMISSIONS).every((v) => v === false);
      expect(allFalse).toBe(true);
    });
  });
});
