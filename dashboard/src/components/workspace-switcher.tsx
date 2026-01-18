"use client";

/**
 * Workspace switcher dropdown component.
 *
 * Displays the current workspace and allows users to switch between
 * workspaces they have access to. Shows the user's role in each workspace.
 */

import { Check, ChevronsUpDown, Building2, Loader2 } from "lucide-react";
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
import { useWorkspace } from "@/contexts/workspace-context";
import type { WorkspaceRole } from "@/types/workspace";
import { cn } from "@/lib/utils";

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
  switch (environment) {
    case "production":
      return "text-red-600 bg-red-50 border-red-200 dark:text-red-400 dark:bg-red-950 dark:border-red-800";
    case "staging":
      return "text-yellow-600 bg-yellow-50 border-yellow-200 dark:text-yellow-400 dark:bg-yellow-950 dark:border-yellow-800";
    default:
      return "text-blue-600 bg-blue-50 border-blue-200 dark:text-blue-400 dark:bg-blue-950 dark:border-blue-800";
  }
}

/**
 * Workspace switcher component.
 * Shows current workspace and dropdown to switch between workspaces.
 */
export function WorkspaceSwitcher() {
  const { workspaces, currentWorkspace, setCurrentWorkspace, isLoading, error } = useWorkspace();

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
            onClick={() => setCurrentWorkspace(workspace.name)}
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
  );
}
