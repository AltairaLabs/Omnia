"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import { getFileType, type ArenaProject, type FileTreeNode, type OpenFile, type FileType } from "@/types/arena-project";

// ============================================================
// Types
// ============================================================

export interface ProjectEditorState {
  /** Currently selected project */
  currentProject: ArenaProject | null;
  /** File tree for current project */
  fileTree: FileTreeNode[];
  /** Open files in editor tabs */
  openFiles: OpenFile[];
  /** Currently active file path (shown in editor) */
  activeFilePath: string | null;
  /** Expanded paths in file tree */
  expandedPaths: Set<string>;
  /** Loading state for project */
  projectLoading: boolean;
  /** Error state for project */
  projectError: string | null;
}

export interface ProjectEditorActions {
  // Project actions
  setCurrentProject: (project: ArenaProject | null, tree?: FileTreeNode[]) => void;
  setFileTree: (tree: FileTreeNode[]) => void;
  setProjectLoading: (loading: boolean) => void;
  setProjectError: (error: string | null) => void;
  clearProject: () => void;

  // File tree actions
  toggleExpanded: (path: string) => void;
  expandPath: (path: string) => void;
  collapsePath: (path: string) => void;
  expandAll: () => void;
  collapseAll: () => void;

  // Open file actions
  openFile: (path: string, name: string, content: string, type?: FileType) => void;
  closeFile: (path: string) => void;
  closeAllFiles: () => void;
  closeOtherFiles: (path: string) => void;
  setActiveFile: (path: string | null) => void;
  updateFileContent: (path: string, content: string) => void;
  markFileSaved: (path: string, newContent?: string) => void;
  setFileLoading: (path: string, loading: boolean) => void;
  setFileError: (path: string, error: string | null) => void;

  // Utility
  hasUnsavedChanges: () => boolean;
  getOpenFile: (path: string) => OpenFile | undefined;
  getDirtyFiles: () => OpenFile[];
}

export type ProjectEditorStore = ProjectEditorState & ProjectEditorActions;

// ============================================================
// Constants
// ============================================================

const MAX_OPEN_FILES = 20;
const STORAGE_KEY = "omnia-project-editor";

// ============================================================
// Helpers
// ============================================================

function createOpenFile(
  path: string,
  name: string,
  content: string,
  type?: FileType
): OpenFile {
  return {
    path,
    name,
    content,
    originalContent: content,
    isDirty: false,
    type: type ?? getFileType(path),
    loading: false,
    error: null,
  };
}

/**
 * Collect all paths recursively from file tree for expandAll
 */
function collectAllPaths(nodes: FileTreeNode[]): string[] {
  const paths: string[] = [];
  function traverse(node: FileTreeNode) {
    if (node.isDirectory) {
      paths.push(node.path);
      node.children?.forEach(traverse);
    }
  }
  nodes.forEach(traverse);
  return paths;
}

// ============================================================
// Store
// ============================================================

export const useProjectEditorStore = create<ProjectEditorStore>()(
  persist(
    (set, get) => ({
      // Initial state
      currentProject: null,
      fileTree: [],
      openFiles: [],
      activeFilePath: null,
      expandedPaths: new Set<string>(),
      projectLoading: false,
      projectError: null,

      // Project actions
      setCurrentProject: (project, tree = []) => {
        set({
          currentProject: project,
          fileTree: tree,
          openFiles: [],
          activeFilePath: null,
          expandedPaths: new Set<string>(),
          projectError: null,
        });
      },

      setFileTree: (tree) => {
        set({ fileTree: tree });
      },

      setProjectLoading: (loading) => {
        set({ projectLoading: loading });
      },

      setProjectError: (error) => {
        set({ projectError: error });
      },

      clearProject: () => {
        set({
          currentProject: null,
          fileTree: [],
          openFiles: [],
          activeFilePath: null,
          expandedPaths: new Set<string>(),
          projectError: null,
        });
      },

      // File tree actions
      toggleExpanded: (path) => {
        const state = get();
        const next = new Set(state.expandedPaths);
        if (next.has(path)) {
          next.delete(path);
        } else {
          next.add(path);
        }
        set({ expandedPaths: next });
      },

      expandPath: (path) => {
        const state = get();
        const next = new Set(state.expandedPaths);
        next.add(path);
        set({ expandedPaths: next });
      },

      collapsePath: (path) => {
        const state = get();
        const next = new Set(state.expandedPaths);
        next.delete(path);
        set({ expandedPaths: next });
      },

      expandAll: () => {
        const state = get();
        const allPaths = collectAllPaths(state.fileTree);
        set({ expandedPaths: new Set(allPaths) });
      },

      collapseAll: () => {
        set({ expandedPaths: new Set() });
      },

      // Open file actions
      openFile: (path, name, content, type) => {
        const state = get();

        // Check if file is already open
        const existingIndex = state.openFiles.findIndex((f) => f.path === path);
        if (existingIndex !== -1) {
          // Already open, just set as active
          set({ activeFilePath: path });
          return;
        }

        // Enforce max open files - close oldest non-dirty file
        const openFiles = [...state.openFiles];
        if (openFiles.length >= MAX_OPEN_FILES) {
          const nonDirtyIndex = openFiles.findIndex((f) => f.isDirty === false);
          if (nonDirtyIndex >= 0) {
            openFiles.splice(nonDirtyIndex, 1);
          } else {
            // All files are dirty, close the first one anyway
            openFiles.shift();
          }
        }

        const newFile = createOpenFile(path, name, content, type);
        openFiles.push(newFile);

        set({
          openFiles,
          activeFilePath: path,
        });
      },

      closeFile: (path) => {
        const state = get();
        const newOpenFiles = state.openFiles.filter((f) => f.path !== path);

        let newActivePath = state.activeFilePath;
        if (state.activeFilePath === path) {
          // Select adjacent file
          const closedIndex = state.openFiles.findIndex((f) => f.path === path);
          if (newOpenFiles.length > 0) {
            const nextIndex = Math.min(closedIndex, newOpenFiles.length - 1);
            newActivePath = newOpenFiles[nextIndex].path;
          } else {
            newActivePath = null;
          }
        }

        set({
          openFiles: newOpenFiles,
          activeFilePath: newActivePath,
        });
      },

      closeAllFiles: () => {
        set({
          openFiles: [],
          activeFilePath: null,
        });
      },

      closeOtherFiles: (path) => {
        const state = get();
        const keepFile = state.openFiles.find((f) => f.path === path);
        set({
          openFiles: keepFile ? [keepFile] : [],
          activeFilePath: keepFile ? path : null,
        });
      },

      setActiveFile: (path) => {
        const state = get();
        // Only set active if file is open
        if (path === null || state.openFiles.some((f) => f.path === path)) {
          set({ activeFilePath: path });
        }
      },

      updateFileContent: (path, content) => {
        const state = get();
        const fileIndex = state.openFiles.findIndex((f) => f.path === path);
        if (fileIndex === -1) return;

        const newOpenFiles = [...state.openFiles];
        const file = newOpenFiles[fileIndex];
        newOpenFiles[fileIndex] = {
          ...file,
          content,
          isDirty: content !== file.originalContent,
        };

        set({ openFiles: newOpenFiles });
      },

      markFileSaved: (path, newContent) => {
        const state = get();
        const fileIndex = state.openFiles.findIndex((f) => f.path === path);
        if (fileIndex === -1) return;

        const newOpenFiles = [...state.openFiles];
        const file = newOpenFiles[fileIndex];
        const savedContent = newContent ?? file.content;
        newOpenFiles[fileIndex] = {
          ...file,
          content: savedContent,
          originalContent: savedContent,
          isDirty: false,
        };

        set({ openFiles: newOpenFiles });
      },

      setFileLoading: (path, loading) => {
        const state = get();
        const fileIndex = state.openFiles.findIndex((f) => f.path === path);
        if (fileIndex === -1) return;

        const newOpenFiles = [...state.openFiles];
        newOpenFiles[fileIndex] = {
          ...newOpenFiles[fileIndex],
          loading,
        };

        set({ openFiles: newOpenFiles });
      },

      setFileError: (path, error) => {
        const state = get();
        const fileIndex = state.openFiles.findIndex((f) => f.path === path);
        if (fileIndex === -1) return;

        const newOpenFiles = [...state.openFiles];
        newOpenFiles[fileIndex] = {
          ...newOpenFiles[fileIndex],
          error,
        };

        set({ openFiles: newOpenFiles });
      },

      // Utility
      hasUnsavedChanges: () => {
        const state = get();
        return state.openFiles.some((f) => f.isDirty);
      },

      getOpenFile: (path) => {
        const state = get();
        return state.openFiles.find((f) => f.path === path);
      },

      getDirtyFiles: () => {
        const state = get();
        return state.openFiles.filter((f) => f.isDirty);
      },
    }),
    {
      name: STORAGE_KEY,
      // Only persist expanded paths and current project ID, not file contents
      partialize: (state) => ({
        expandedPaths: Array.from(state.expandedPaths),
        currentProjectId: state.currentProject?.id ?? null,
      }),
      // Custom merge to handle Set serialization
      merge: (persistedState, currentState) => {
        const persisted = persistedState as {
          expandedPaths?: string[];
          currentProjectId?: string | null;
        };
        return {
          ...currentState,
          expandedPaths: new Set(persisted?.expandedPaths ?? []),
          // Note: We don't restore currentProject from persistence
          // It should be loaded fresh from the API
        };
      },
    }
  )
);

// ============================================================
// Selector hooks for convenience
// ============================================================

/**
 * Get the currently active open file.
 */
export function useActiveFile(): OpenFile | null {
  return useProjectEditorStore((state) =>
    state.activeFilePath
      ? state.openFiles.find((f) => f.path === state.activeFilePath) ?? null
      : null
  );
}

/**
 * Get whether any files have unsaved changes.
 */
export function useHasUnsavedChanges(): boolean {
  return useProjectEditorStore((state) => state.openFiles.some((f) => f.isDirty));
}

/**
 * Get current project info.
 */
export function useCurrentProject() {
  return useProjectEditorStore((state) => ({
    project: state.currentProject,
    loading: state.projectLoading,
    error: state.projectError,
  }));
}
