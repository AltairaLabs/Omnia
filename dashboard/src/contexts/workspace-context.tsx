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
import {
  usePathname,
  useSearchParams,
  type ReadonlyURLSearchParams,
} from "next/navigation";
import { useWorkspaces, type WorkspaceListItem } from "@/hooks/resources";
import { isChromelessPath } from "@/lib/routes";

const STORAGE_KEY = "omnia-selected-workspace";

/** Query-string key that anchors a workspace in a URL for deep-linking. */
export const WORKSPACE_QUERY_PARAM = "workspace";

/**
 * Get the stored workspace name from localStorage.
 * Only called on the client.
 */
function getStoredWorkspaceName(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(STORAGE_KEY);
}

/**
 * Initial workspace selection: a `?workspace=` URL anchor wins over the
 * localStorage value, so a deep link / shared link resolves to the workspace
 * it names. Falls back to the stored selection, then to null (the provider
 * defaults to the first workspace once the list loads).
 */
function getInitialWorkspaceName(
  searchParams: ReadonlyURLSearchParams | null
): string | null {
  return searchParams?.get(WORKSPACE_QUERY_PARAM) ?? getStoredWorkspaceName();
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
 *
 * Note: UI refresh on workspace change is handled by WorkspaceContent
 * using React's key prop to remount the content tree.
 */
export function WorkspaceProvider({ children }: Readonly<WorkspaceProviderProps>) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const { data: workspaces = [], isLoading, error, refetch } = useWorkspaces({
    enabled: !isChromelessPath(pathname ?? ""),
  });
  // Initialize state from the URL anchor (deep link) or stored value.
  // Lazy initializer runs once on mount, capturing the first-render params.
  const [selectedWorkspaceName, setSelectedWorkspaceName] = useState<string | null>(
    () => getInitialWorkspaceName(searchParams)
  );

  // Compute the effective selected workspace name, defaulting to the first
  // workspace when there's no valid selection. Persists the chosen name so a
  // deep-linked or switched workspace sticks across subsequent loads.
  const effectiveWorkspaceName = useMemo(() => {
    if (isLoading || workspaces.length === 0) return null;

    const selectionExists =
      !!selectedWorkspaceName &&
      workspaces.some(ws => ws.name === selectedWorkspaceName);
    const chosen = selectionExists
      ? (selectedWorkspaceName as string)
      : workspaces[0].name;

    if (typeof window !== "undefined") {
      localStorage.setItem(STORAGE_KEY, chosen);
    }
    return chosen;
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
