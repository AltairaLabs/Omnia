"use client";

/**
 * Workspace context for managing current workspace selection.
 *
 * Usage:
 *   import { useWorkspace, WorkspaceProvider } from "@/contexts/workspace-context";
 *
 *   function MyComponent() {
 *     const { currentWorkspace, setCurrentWorkspace, workspaces } = useWorkspace();
 *     ...
 *   }
 */

import {
  createContext,
  useContext,
  useState,
  useMemo,
  useCallback,
  type ReactNode,
} from "react";
import { useWorkspaces, type WorkspaceListItem } from "@/hooks/use-workspaces";

const STORAGE_KEY = "omnia-selected-workspace";

/**
 * Get the stored workspace name from localStorage.
 * Only called on the client.
 */
function getStoredWorkspaceName(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(STORAGE_KEY);
}

interface WorkspaceContextValue {
  /** List of workspaces the user has access to */
  workspaces: WorkspaceListItem[];
  /** Currently selected workspace */
  currentWorkspace: WorkspaceListItem | null;
  /** Set the current workspace by name */
  setCurrentWorkspace: (workspaceName: string | null) => void;
  /** Whether workspaces are loading */
  isLoading: boolean;
  /** Error if workspace fetch failed */
  error: Error | null;
  /** Refetch workspaces */
  refetch: () => void;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

interface WorkspaceProviderProps {
  children: ReactNode;
}

/**
 * Workspace provider - manages workspace selection state.
 * Persists the selected workspace to localStorage.
 */
export function WorkspaceProvider({ children }: Readonly<WorkspaceProviderProps>) {
  const { data: workspaces = [], isLoading, error, refetch } = useWorkspaces();
  // Initialize state with stored value (lazy initializer runs once on mount)
  const [selectedWorkspaceName, setSelectedWorkspaceName] = useState<string | null>(
    () => getStoredWorkspaceName()
  );

  // Compute the effective selected workspace name, defaulting to first workspace
  // if no selection or the selected workspace doesn't exist
  const effectiveWorkspaceName = useMemo(() => {
    if (isLoading || workspaces.length === 0) return null;

    // If we have a selection and it exists in the workspace list, use it
    if (selectedWorkspaceName) {
      const exists = workspaces.some(ws => ws.name === selectedWorkspaceName);
      if (exists) return selectedWorkspaceName;
    }

    // Default to first workspace and persist it
    const firstWorkspace = workspaces[0].name;
    if (typeof window !== "undefined") {
      localStorage.setItem(STORAGE_KEY, firstWorkspace);
    }
    return firstWorkspace;
  }, [selectedWorkspaceName, workspaces, isLoading]);

  // Find the current workspace object
  const currentWorkspace = useMemo(() => {
    if (!effectiveWorkspaceName || workspaces.length === 0) return null;
    return workspaces.find(ws => ws.name === effectiveWorkspaceName) || null;
  }, [effectiveWorkspaceName, workspaces]);

  const setCurrentWorkspace = useCallback((workspaceName: string | null) => {
    setSelectedWorkspaceName(workspaceName);
    if (typeof window !== "undefined") {
      if (workspaceName) {
        localStorage.setItem(STORAGE_KEY, workspaceName);
      } else {
        localStorage.removeItem(STORAGE_KEY);
      }
    }
  }, []);

  const value = useMemo<WorkspaceContextValue>(
    () => ({
      workspaces,
      currentWorkspace,
      setCurrentWorkspace,
      isLoading,
      error: error as Error | null,
      refetch,
    }),
    [workspaces, currentWorkspace, setCurrentWorkspace, isLoading, error, refetch]
  );

  return (
    <WorkspaceContext.Provider value={value}>
      {children}
    </WorkspaceContext.Provider>
  );
}

/**
 * Hook to access workspace context.
 */
export function useWorkspace(): WorkspaceContextValue {
  const context = useContext(WorkspaceContext);
  if (!context) {
    throw new Error("useWorkspace must be used within a WorkspaceProvider");
  }
  return context;
}
