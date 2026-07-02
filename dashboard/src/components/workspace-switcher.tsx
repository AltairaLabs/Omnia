"use client";

/**
 * Workspace switcher dropdown component.
 *
 * Displays the current workspace and allows users to switch between
 * workspaces they have access to. Shows the user's role in each workspace.
 */

import { Check, ChevronsUpDown, Building2, Loader2, Settings } from "lucide-react";
import { useRouter, usePathname } from "next/navigation";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Badge } from "@/components/ui/badge";
import {
  useWorkspace,
  WORKSPACE_QUERY_PARAM,
} from "@/contexts/workspace-context";
import { useWorkspacePermissions } from "@/hooks/use-workspace-permissions";
import type { WorkspaceRole } from "@/types/workspace";
import { cn } from "@/lib/utils";
import { getStatusClasses, type StatusKind } from "@/lib/colors/status";

/**
 * Get badge variant based on workspace role.
 */
function getRoleBadgeVariant(role: WorkspaceRole): "default" | "secondary" | "outline" {
  switch (role) {
    case "owner":
      return "default";
    case "editor":
      return "secondary";
    default:
      return "outline";
  }
}

/**
 * Get environment badge color.
 */
function getEnvironmentColor(environment: string): string {
  const kindByEnvironment: Record<string, StatusKind> = {
    production: "error",
    staging: "warning",
  };
  const kind: StatusKind = kindByEnvironment[environment] ?? "info";
  const { text, bg, border } = getStatusClasses(kind);
  return `${text} ${bg} ${border}`;
}

/**
 * Workspace switcher component.
 * Shows current workspace and dropdown to switch between workspaces.
 */
export function WorkspaceSwitcher() {
  const { workspaces, currentWorkspace, setCurrentWorkspace, isLoading, error } = useWorkspace();
  const router = useRouter();
  const pathname = usePathname();
  const { isOwner } = useWorkspacePermissions();

  // Switch the active workspace AND anchor it in the URL (?workspace=) so the
  // current page stays copy-paste shareable. Replace, not push, to avoid
  // cluttering history with each switch. Reads the live query string from
  // window.location rather than useSearchParams() — the latter forces a CSR
  // bailout that breaks static prerendering, and this only runs on click.
  const selectWorkspace = (workspaceName: string) => {
    setCurrentWorkspace(workspaceName);
    const params = new URLSearchParams(
      typeof window === "undefined" ? "" : window.location.search
    );
    params.set(WORKSPACE_QUERY_PARAM, workspaceName);
    router.replace(`${pathname ?? "/"}?${params.toString()}`);
  };

  // Loading state
  if (isLoading) {
    return (
      <Button variant="outline" disabled className="w-[200px] justify-between">
        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        <span>Loading...</span>
      </Button>
    );
  }

  // Error state
  if (error) {
    return (
      <Button variant="outline" disabled className="w-[200px] justify-between text-destructive">
        <Building2 className="mr-2 h-4 w-4" />
        <span>Error loading workspaces</span>
      </Button>
    );
  }

  // No workspaces available
  if (workspaces.length === 0) {
    return (
      <Button variant="outline" disabled className="w-[200px] justify-between">
        <Building2 className="mr-2 h-4 w-4" />
        <span>No workspaces</span>
      </Button>
    );
  }

  return (
    <div className="flex items-center gap-1">
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" className="w-[250px] justify-between">
          <div className="flex items-center gap-2 truncate">
            <Building2 className="h-4 w-4 shrink-0" />
            <span className="truncate">
              {currentWorkspace?.displayName || "Select workspace"}
            </span>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            {currentWorkspace && (
              <Badge variant={getRoleBadgeVariant(currentWorkspace.role)} className="text-xs">
                {currentWorkspace.role}
              </Badge>
            )}
            <ChevronsUpDown className="h-4 w-4 opacity-50" />
          </div>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-[250px]" align="start">
        <DropdownMenuLabel>Workspaces</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {workspaces.map((workspace) => (
          <DropdownMenuItem
            key={workspace.name}
            onClick={() => selectWorkspace(workspace.name)}
            className="flex items-center justify-between cursor-pointer"
          >
            <div className="flex items-center gap-2 min-w-0">
              <Check
                className={cn(
                  "h-4 w-4 shrink-0",
                  currentWorkspace?.name === workspace.name ? "opacity-100" : "opacity-0"
                )}
              />
              <div className="flex flex-col min-w-0">
                <span className="truncate font-medium">{workspace.displayName}</span>
                {workspace.description && (
                  <span className="text-xs text-muted-foreground truncate">
                    {workspace.description}
                  </span>
                )}
              </div>
            </div>
            <div className="flex items-center gap-2 shrink-0 ml-2">
              <Badge
                variant="outline"
                className={cn("text-xs", getEnvironmentColor(workspace.environment))}
              >
                {workspace.environment.slice(0, 3)}
              </Badge>
              <Badge variant={getRoleBadgeVariant(workspace.role)} className="text-xs">
                {workspace.role}
              </Badge>
            </div>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
    {isOwner && currentWorkspace && (
      <button
        type="button"
        className="inline-flex items-center justify-center h-9 w-9 rounded-md border border-input hover:bg-accent"
        data-testid="workspace-settings-gear"
        onClick={() => router.push(`/workspaces/${currentWorkspace.name}/settings`)}
      >
        <Settings className="h-4 w-4 opacity-60" />
      </button>
    )}
    </div>
  );
}
