"use client";

/**
 * Workspace content wrapper that remounts children when workspace changes.
 *
 * Uses React's key prop to force a complete remount of the content tree
 * when the workspace changes. This ensures all component state is reset
 * and data is refetched for the new workspace.
 *
 * The key prop approach combined with staleTime: 0 on queries ensures
 * fresh data is fetched when children remount.
 */

import { type ReactNode } from "react";
import { useWorkspace } from "@/contexts/workspace-context";

interface WorkspaceContentProps {
  children: ReactNode;
}

export function WorkspaceContent({ children }: Readonly<WorkspaceContentProps>) {
  const { currentWorkspace, isLoading } = useWorkspace();

  // Use workspace name as key - when it changes, React remounts all children
  // This resets all component state and triggers fresh data fetches
  // Use "loading" as key while workspaces are loading to prevent flash
  const contentKey = isLoading ? "loading" : (currentWorkspace?.name ?? "none");

  return (
    <div key={contentKey} className="h-full">
      {children}
    </div>
  );
}
