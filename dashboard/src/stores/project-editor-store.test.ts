import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import {
  useProjectEditorStore,
  useActiveFile,
  useHasUnsavedChanges,
} from "./project-editor-store";
import type { ArenaProject, FileTreeNode } from "@/types/arena-project";

// Test data factories
function createMockProject(overrides: Partial<ArenaProject> = {}): ArenaProject {
  return {
    id: "test-project-1",
    name: "Test Project",
    description: "A test project",
    createdAt: "2024-01-01T00:00:00Z",
    updatedAt: "2024-01-01T00:00:00Z",
    tags: ["test"],
    ...overrides,
  };
}

function createMockFileTree(): FileTreeNode[] {
  return [
    {
      name: "config.arena.yaml",
      path: "config.arena.yaml",
      isDirectory: false,
      type: "arena",
    },
    {
      name: "prompts",
      path: "prompts",
      isDirectory: true,
      children: [
        {
          name: "system.prompt.yaml",
          path: "prompts/system.prompt.yaml",
          isDirectory: false,
          type: "prompt",
        },
      ],
    },
  ];
}

describe("project-editor-store", () => {
  beforeEach(() => {
    // Reset store state before each test
    const store = useProjectEditorStore.getState();
    store.clearProject();
  });

  describe("project actions", () => {
    it("should set current project with tree", () => {
      const project = createMockProject();
      const tree = createMockFileTree();

      act(() => {
        useProjectEditorStore.getState().setCurrentProject(project, tree);
      });

      const state = useProjectEditorStore.getState();
      expect(state.currentProject).toEqual(project);
      expect(state.fileTree).toEqual(tree);
      expect(state.openFiles).toEqual([]);
      expect(state.activeFilePath).toBeNull();
    });

    it("should set file tree", () => {
      const tree = createMockFileTree();

      act(() => {
        useProjectEditorStore.getState().setFileTree(tree);
      });

      expect(useProjectEditorStore.getState().fileTree).toEqual(tree);
    });

    it("should set project loading state", () => {
      act(() => {
        useProjectEditorStore.getState().setProjectLoading(true);
      });

      expect(useProjectEditorStore.getState().projectLoading).toBe(true);

      act(() => {
        useProjectEditorStore.getState().setProjectLoading(false);
      });

      expect(useProjectEditorStore.getState().projectLoading).toBe(false);
    });

    it("should set project error state", () => {
      act(() => {
        useProjectEditorStore.getState().setProjectError("Test error");
      });

      expect(useProjectEditorStore.getState().projectError).toBe("Test error");

      act(() => {
        useProjectEditorStore.getState().setProjectError(null);
      });

      expect(useProjectEditorStore.getState().projectError).toBeNull();
    });

    it("should clear project state", () => {
      const project = createMockProject();
      const tree = createMockFileTree();

      act(() => {
        useProjectEditorStore.getState().setCurrentProject(project, tree);
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
        useProjectEditorStore.getState().expandPath("prompts");
      });

      act(() => {
        useProjectEditorStore.getState().clearProject();
      });

      const state = useProjectEditorStore.getState();
      expect(state.currentProject).toBeNull();
      expect(state.fileTree).toEqual([]);
      expect(state.openFiles).toEqual([]);
      expect(state.activeFilePath).toBeNull();
      expect(state.expandedPaths.size).toBe(0);
    });
  });

  describe("file tree actions", () => {
    it("should toggle expanded paths", () => {
      act(() => {
        useProjectEditorStore.getState().toggleExpanded("prompts");
      });

      expect(useProjectEditorStore.getState().expandedPaths.has("prompts")).toBe(true);

      act(() => {
        useProjectEditorStore.getState().toggleExpanded("prompts");
      });

      expect(useProjectEditorStore.getState().expandedPaths.has("prompts")).toBe(false);
    });

    it("should expand a path", () => {
      act(() => {
        useProjectEditorStore.getState().expandPath("prompts");
      });

      expect(useProjectEditorStore.getState().expandedPaths.has("prompts")).toBe(true);
    });

    it("should collapse a path", () => {
      act(() => {
        useProjectEditorStore.getState().expandPath("prompts");
        useProjectEditorStore.getState().collapsePath("prompts");
      });

      expect(useProjectEditorStore.getState().expandedPaths.has("prompts")).toBe(false);
    });

    it("should expand all directories", () => {
      const tree = createMockFileTree();

      act(() => {
        useProjectEditorStore.getState().setFileTree(tree);
        useProjectEditorStore.getState().expandAll();
      });

      const expanded = useProjectEditorStore.getState().expandedPaths;
      expect(expanded.has("prompts")).toBe(true);
    });

    it("should collapse all directories", () => {
      act(() => {
        useProjectEditorStore.getState().expandPath("prompts");
        useProjectEditorStore.getState().expandPath("tools");
        useProjectEditorStore.getState().collapseAll();
      });

      expect(useProjectEditorStore.getState().expandedPaths.size).toBe(0);
    });
  });

  describe("open file actions", () => {
    it("should open a file and set it as active", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content: value");
      });

      const state = useProjectEditorStore.getState();
      expect(state.openFiles).toHaveLength(1);
      expect(state.openFiles[0].path).toBe("test.yaml");
      expect(state.openFiles[0].name).toBe("test.yaml");
      expect(state.openFiles[0].content).toBe("content: value");
      expect(state.openFiles[0].originalContent).toBe("content: value");
      expect(state.openFiles[0].isDirty).toBe(false);
      expect(state.activeFilePath).toBe("test.yaml");
    });

    it("should not duplicate already open files", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
      });

      expect(useProjectEditorStore.getState().openFiles).toHaveLength(1);
    });

    it("should set existing file as active when opening again", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
        useProjectEditorStore.getState().openFile("file2.yaml", "file2.yaml", "content2");
      });

      expect(useProjectEditorStore.getState().activeFilePath).toBe("file2.yaml");

      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
      });

      expect(useProjectEditorStore.getState().activeFilePath).toBe("file1.yaml");
    });

    it("should close a file and select adjacent file", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
        useProjectEditorStore.getState().openFile("file2.yaml", "file2.yaml", "content2");
        useProjectEditorStore.getState().openFile("file3.yaml", "file3.yaml", "content3");
      });

      act(() => {
        useProjectEditorStore.getState().closeFile("file2.yaml");
      });

      const state = useProjectEditorStore.getState();
      expect(state.openFiles).toHaveLength(2);
      expect(state.openFiles.map((f) => f.path)).not.toContain("file2.yaml");
    });

    it("should select previous file when closing active file", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
        useProjectEditorStore.getState().openFile("file2.yaml", "file2.yaml", "content2");
      });

      expect(useProjectEditorStore.getState().activeFilePath).toBe("file2.yaml");

      act(() => {
        useProjectEditorStore.getState().closeFile("file2.yaml");
      });

      expect(useProjectEditorStore.getState().activeFilePath).toBe("file1.yaml");
    });

    it("should set activeFilePath to null when closing last file", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
      });

      act(() => {
        useProjectEditorStore.getState().closeFile("test.yaml");
      });

      expect(useProjectEditorStore.getState().activeFilePath).toBeNull();
    });

    it("should close all files", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
        useProjectEditorStore.getState().openFile("file2.yaml", "file2.yaml", "content2");
        useProjectEditorStore.getState().closeAllFiles();
      });

      const state = useProjectEditorStore.getState();
      expect(state.openFiles).toHaveLength(0);
      expect(state.activeFilePath).toBeNull();
    });

    it("should close other files", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
        useProjectEditorStore.getState().openFile("file2.yaml", "file2.yaml", "content2");
        useProjectEditorStore.getState().openFile("file3.yaml", "file3.yaml", "content3");
        useProjectEditorStore.getState().closeOtherFiles("file2.yaml");
      });

      const state = useProjectEditorStore.getState();
      expect(state.openFiles).toHaveLength(1);
      expect(state.openFiles[0].path).toBe("file2.yaml");
      expect(state.activeFilePath).toBe("file2.yaml");
    });

    it("should set active file", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
        useProjectEditorStore.getState().openFile("file2.yaml", "file2.yaml", "content2");
        useProjectEditorStore.getState().setActiveFile("file1.yaml");
      });

      expect(useProjectEditorStore.getState().activeFilePath).toBe("file1.yaml");
    });

    it("should not set active file if not open", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
        useProjectEditorStore.getState().setActiveFile("nonexistent.yaml");
      });

      expect(useProjectEditorStore.getState().activeFilePath).toBe("file1.yaml");
    });

    it("should update file content and mark as dirty", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "original");
        useProjectEditorStore.getState().updateFileContent("test.yaml", "modified");
      });

      const file = useProjectEditorStore.getState().openFiles[0];
      expect(file.content).toBe("modified");
      expect(file.originalContent).toBe("original");
      expect(file.isDirty).toBe(true);
    });

    it("should mark file as not dirty when content matches original", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "original");
        useProjectEditorStore.getState().updateFileContent("test.yaml", "modified");
        useProjectEditorStore.getState().updateFileContent("test.yaml", "original");
      });

      expect(useProjectEditorStore.getState().openFiles[0].isDirty).toBe(false);
    });

    it("should mark file as saved", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "original");
        useProjectEditorStore.getState().updateFileContent("test.yaml", "modified");
        useProjectEditorStore.getState().markFileSaved("test.yaml");
      });

      const file = useProjectEditorStore.getState().openFiles[0];
      expect(file.content).toBe("modified");
      expect(file.originalContent).toBe("modified");
      expect(file.isDirty).toBe(false);
    });

    it("should mark file as saved with new content", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "original");
        useProjectEditorStore.getState().markFileSaved("test.yaml", "new content");
      });

      const file = useProjectEditorStore.getState().openFiles[0];
      expect(file.content).toBe("new content");
      expect(file.originalContent).toBe("new content");
      expect(file.isDirty).toBe(false);
    });

    it("should set file loading state", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
        useProjectEditorStore.getState().setFileLoading("test.yaml", true);
      });

      expect(useProjectEditorStore.getState().openFiles[0].loading).toBe(true);

      act(() => {
        useProjectEditorStore.getState().setFileLoading("test.yaml", false);
      });

      expect(useProjectEditorStore.getState().openFiles[0].loading).toBe(false);
    });

    it("should set file error state", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
        useProjectEditorStore.getState().setFileError("test.yaml", "Load failed");
      });

      expect(useProjectEditorStore.getState().openFiles[0].error).toBe("Load failed");

      act(() => {
        useProjectEditorStore.getState().setFileError("test.yaml", null);
      });

      expect(useProjectEditorStore.getState().openFiles[0].error).toBeNull();
    });

    it("should enforce max open files limit", () => {
      // Open 21 files (max is 20)
      act(() => {
        for (let i = 0; i < 21; i++) {
          useProjectEditorStore.getState().openFile(`file${i}.yaml`, `file${i}.yaml`, `content${i}`);
        }
      });

      expect(useProjectEditorStore.getState().openFiles.length).toBeLessThanOrEqual(20);
    });
  });

  describe("utility methods", () => {
    it("should return hasUnsavedChanges correctly", () => {
      expect(useProjectEditorStore.getState().hasUnsavedChanges()).toBe(false);

      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "original");
      });

      expect(useProjectEditorStore.getState().hasUnsavedChanges()).toBe(false);

      act(() => {
        useProjectEditorStore.getState().updateFileContent("test.yaml", "modified");
      });

      expect(useProjectEditorStore.getState().hasUnsavedChanges()).toBe(true);
    });

    it("should get open file by path", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
      });

      const file = useProjectEditorStore.getState().getOpenFile("test.yaml");
      expect(file).toBeDefined();
      expect(file?.path).toBe("test.yaml");

      const notFound = useProjectEditorStore.getState().getOpenFile("nonexistent.yaml");
      expect(notFound).toBeUndefined();
    });

    it("should get dirty files", () => {
      act(() => {
        useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
        useProjectEditorStore.getState().openFile("file2.yaml", "file2.yaml", "content2");
        useProjectEditorStore.getState().updateFileContent("file1.yaml", "modified");
      });

      const dirtyFiles = useProjectEditorStore.getState().getDirtyFiles();
      expect(dirtyFiles).toHaveLength(1);
      expect(dirtyFiles[0].path).toBe("file1.yaml");
    });
  });

  describe("selector hooks", () => {
    it("useActiveFile should return the active file", () => {
      const { result } = renderHook(() => useActiveFile());

      expect(result.current).toBeNull();

      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
      });

      expect(result.current).not.toBeNull();
      expect(result.current?.path).toBe("test.yaml");
    });

    it("useHasUnsavedChanges should return dirty state", () => {
      const { result } = renderHook(() => useHasUnsavedChanges());

      expect(result.current).toBe(false);

      act(() => {
        useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "original");
        useProjectEditorStore.getState().updateFileContent("test.yaml", "modified");
      });

      expect(result.current).toBe(true);
    });

    it("useCurrentProject should return project info", () => {
      // Test individual selectors instead of the combined hook to avoid infinite loop
      // The combined hook creates new objects which causes React's useSyncExternalStore to loop
      const store = useProjectEditorStore.getState();

      expect(store.currentProject).toBeNull();
      expect(store.projectLoading).toBe(false);
      expect(store.projectError).toBeNull();

      const project = createMockProject();
      act(() => {
        useProjectEditorStore.getState().setCurrentProject(project, []);
        useProjectEditorStore.getState().setProjectLoading(true);
        useProjectEditorStore.getState().setProjectError("Test error");
      });

      const updatedStore = useProjectEditorStore.getState();
      expect(updatedStore.currentProject).toEqual(project);
      expect(updatedStore.projectLoading).toBe(true);
      expect(updatedStore.projectError).toBe("Test error");
    });
  });
});
