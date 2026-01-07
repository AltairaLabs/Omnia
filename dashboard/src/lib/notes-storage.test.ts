import { describe, it, expect, beforeEach, vi } from "vitest";
import {
  loadNotes,
  saveNotes,
  getNote,
  setNote,
  deleteNote,
  getNotesForNamespaces,
  type NotesMap,
  type ResourceNote,
} from "./notes-storage";

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] || null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key];
    }),
    clear: vi.fn(() => {
      store = {};
    }),
  };
})();

Object.defineProperty(globalThis, "localStorage", {
  value: localStorageMock,
});

describe("notes-storage", () => {
  beforeEach(() => {
    localStorageMock.clear();
    vi.clearAllMocks();
  });

  describe("loadNotes", () => {
    it("should return empty object when no notes stored", () => {
      const notes = loadNotes();
      expect(notes).toEqual({});
    });

    it("should return stored notes", () => {
      const storedNotes: NotesMap = {
        "agent/default/test-agent": {
          resourceType: "agent",
          namespace: "default",
          name: "test-agent",
          note: "Test note",
          updatedAt: "2024-01-01T00:00:00.000Z",
        },
      };
      localStorageMock.setItem(
        "omnia-topology-notes",
        JSON.stringify(storedNotes)
      );

      const notes = loadNotes();
      expect(notes).toEqual(storedNotes);
    });

    it("should return empty object on parse error", () => {
      localStorageMock.setItem("omnia-topology-notes", "invalid json");

      const notes = loadNotes();
      expect(notes).toEqual({});
    });
  });

  describe("saveNotes", () => {
    it("should save notes to localStorage", () => {
      const notes: NotesMap = {
        "agent/default/test-agent": {
          resourceType: "agent",
          namespace: "default",
          name: "test-agent",
          note: "Test note",
          updatedAt: "2024-01-01T00:00:00.000Z",
        },
      };

      saveNotes(notes);

      expect(localStorageMock.setItem).toHaveBeenCalledWith(
        "omnia-topology-notes",
        JSON.stringify(notes)
      );
    });

    it("should handle save errors gracefully", () => {
      const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      localStorageMock.setItem.mockImplementationOnce(() => {
        throw new Error("Storage full");
      });

      const notes: NotesMap = {};
      saveNotes(notes);

      expect(consoleSpy).toHaveBeenCalled();
      consoleSpy.mockRestore();
    });
  });

  describe("getNote", () => {
    it("should return note for existing resource", () => {
      const storedNote: ResourceNote = {
        resourceType: "agent",
        namespace: "default",
        name: "test-agent",
        note: "My test note",
        updatedAt: "2024-01-01T00:00:00.000Z",
      };
      localStorageMock.setItem(
        "omnia-topology-notes",
        JSON.stringify({ "agent/default/test-agent": storedNote })
      );

      const note = getNote("agent", "default", "test-agent");
      expect(note).toEqual(storedNote);
    });

    it("should return undefined for non-existent resource", () => {
      const note = getNote("agent", "default", "non-existent");
      expect(note).toBeUndefined();
    });
  });

  describe("setNote", () => {
    it("should create a new note", () => {
      setNote("agent", "default", "test-agent", "New note content");

      const notes = loadNotes();
      expect(notes["agent/default/test-agent"]).toBeDefined();
      expect(notes["agent/default/test-agent"].note).toBe("New note content");
      expect(notes["agent/default/test-agent"].resourceType).toBe("agent");
      expect(notes["agent/default/test-agent"].namespace).toBe("default");
      expect(notes["agent/default/test-agent"].name).toBe("test-agent");
    });

    it("should update an existing note", () => {
      setNote("agent", "default", "test-agent", "Original note");
      setNote("agent", "default", "test-agent", "Updated note");

      const notes = loadNotes();
      expect(notes["agent/default/test-agent"].note).toBe("Updated note");
    });

    it("should delete note when empty string provided", () => {
      setNote("agent", "default", "test-agent", "Some note");
      setNote("agent", "default", "test-agent", "");

      const notes = loadNotes();
      expect(notes["agent/default/test-agent"]).toBeUndefined();
    });

    it("should delete note when whitespace-only string provided", () => {
      setNote("agent", "default", "test-agent", "Some note");
      setNote("agent", "default", "test-agent", "   ");

      const notes = loadNotes();
      expect(notes["agent/default/test-agent"]).toBeUndefined();
    });

    it("should trim whitespace from note content", () => {
      setNote("agent", "default", "test-agent", "  Note with spaces  ");

      const notes = loadNotes();
      expect(notes["agent/default/test-agent"].note).toBe("Note with spaces");
    });

    it("should set updatedAt timestamp", () => {
      const before = new Date().toISOString();
      setNote("agent", "default", "test-agent", "Note");
      const after = new Date().toISOString();

      const notes = loadNotes();
      const updatedAt = notes["agent/default/test-agent"].updatedAt;
      expect(updatedAt >= before).toBe(true);
      expect(updatedAt <= after).toBe(true);
    });
  });

  describe("deleteNote", () => {
    it("should delete an existing note", () => {
      setNote("agent", "default", "test-agent", "Note to delete");
      deleteNote("agent", "default", "test-agent");

      const notes = loadNotes();
      expect(notes["agent/default/test-agent"]).toBeUndefined();
    });

    it("should handle deleting non-existent note", () => {
      // Should not throw
      deleteNote("agent", "default", "non-existent");

      const notes = loadNotes();
      expect(Object.keys(notes)).toHaveLength(0);
    });
  });

  describe("getNotesForNamespaces", () => {
    beforeEach(() => {
      // Set up test notes across multiple namespaces
      setNote("agent", "default", "agent1", "Note 1");
      setNote("promptpack", "production", "pack1", "Note 2");
      setNote("toolregistry", "default", "tools1", "Note 3");
      setNote("agent", "staging", "agent2", "Note 4");
    });

    it("should return all notes when empty namespace array provided", () => {
      const notes = getNotesForNamespaces([]);
      expect(notes).toHaveLength(4);
    });

    it("should filter notes by single namespace", () => {
      const notes = getNotesForNamespaces(["default"]);
      expect(notes).toHaveLength(2);
      expect(notes.every((n) => n.namespace === "default")).toBe(true);
    });

    it("should filter notes by multiple namespaces", () => {
      const notes = getNotesForNamespaces(["default", "production"]);
      expect(notes).toHaveLength(3);
      expect(
        notes.every((n) => ["default", "production"].includes(n.namespace))
      ).toBe(true);
    });

    it("should return empty array for non-existent namespace", () => {
      const notes = getNotesForNamespaces(["non-existent"]);
      expect(notes).toHaveLength(0);
    });
  });
});
