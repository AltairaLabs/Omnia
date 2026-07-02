"use client";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Save,
  Loader2,
  FolderOpen,
  Plus,
  RefreshCw,
  Trash2,
  CheckCircle2,
  FilePlus2,
  LayoutTemplate,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import type { ArenaProject } from "@/types/arena-project";
import { cn } from "@/lib/utils";
import { DeployButton } from "./deploy-button";
import { RunDropdown } from "./run-dropdown";

interface ProjectToolbarProps {
  readonly projects: ArenaProject[];
  readonly currentProject: ArenaProject | null;
  readonly hasUnsavedChanges: boolean;
  readonly saving: boolean;
  readonly loading: boolean;
  readonly validating?: boolean;
  /** Whether the user has write permissions */
  readonly canWrite?: boolean;
  readonly onProjectSelect: (projectId: string) => void;
  readonly onSave: () => void;
  readonly onNewProject: () => void;
  readonly onNewFromTemplate: () => void;
  readonly onRefresh: () => void;
  readonly onDeleteProject?: () => void;
  readonly onValidateAll?: () => void;
  readonly className?: string;
}

/**
 * Toolbar for the project editor.
 * Contains project selector, save button, and other actions.
 */
export function ProjectToolbar({
  projects,
  currentProject,
  hasUnsavedChanges,
  saving,
  loading,
  validating = false,
  canWrite = true,
  onProjectSelect,
  onSave,
  onNewProject,
  onNewFromTemplate,
  onRefresh,
  onDeleteProject,
  onValidateAll,
  className,
}: ProjectToolbarProps) {
  return (
    <div
      className={cn(
        "flex items-center justify-between px-4 py-2 border-b bg-muted/30",
        className
      )}
    >
      {/* Left side: Project selector */}
      <div className="flex items-center gap-2">
        <FolderOpen className="h-4 w-4 text-muted-foreground" />
        <Select
          value={currentProject?.id || ""}
          onValueChange={onProjectSelect}
          disabled={loading}
        >
          <SelectTrigger className="w-[200px] h-8">
            <SelectValue placeholder="Select a project" />
          </SelectTrigger>
          <SelectContent>
            {projects.map((project) => (
              <SelectItem key={project.id} value={project.id}>
                {project.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              disabled={loading || !canWrite}
              title={canWrite ? "New Project" : "Editor access required to create projects"}
            >
              <Plus className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={onNewProject}>
              <FilePlus2 className="h-4 w-4 mr-2" />
              New blank project
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onNewFromTemplate}>
              <LayoutTemplate className="h-4 w-4 mr-2" />
              From template…
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={onRefresh}
          disabled={loading}
          title="Refresh"
        >
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
        </Button>
      </div>

      {/* Right side: Actions */}
      <div className="flex items-center gap-2">
        {/* Unsaved indicator */}
        {hasUnsavedChanges && (
          <span className="text-xs text-warning mr-2">Unsaved changes</span>
        )}

        {/* Validate All button */}
        {currentProject && onValidateAll && (
          <Button
            variant="outline"
            size="sm"
            onClick={onValidateAll}
            disabled={loading || validating}
            className="gap-2"
            title="Validate all files in the project"
          >
            {validating ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <CheckCircle2 className="h-4 w-4" />
            )}
            Validate All
          </Button>
        )}

        {/* Deploy button */}
        {currentProject && (
          <DeployButton
            projectId={currentProject.id}
            disabled={loading || saving || !canWrite}
          />
        )}

        {/* Run button */}
        {currentProject && (
          <RunDropdown
            projectId={currentProject.id}
            disabled={loading || saving || !canWrite}
          />
        )}

        {/* Save button */}
        <Button
          variant={hasUnsavedChanges ? "default" : "outline"}
          size="sm"
          onClick={onSave}
          disabled={!hasUnsavedChanges || saving || !canWrite}
          className="gap-2"
          title={canWrite ? "Save changes" : "Editor access required to save"}
        >
          {saving ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Save className="h-4 w-4" />
          )}
          Save
        </Button>

        {/* Delete project button */}
        {currentProject && onDeleteProject && (
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 text-destructive hover:text-destructive"
            onClick={onDeleteProject}
            disabled={loading || saving || !canWrite}
            title={canWrite ? "Delete Project" : "Editor access required to delete"}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        )}
      </div>
    </div>
  );
}

interface NewProjectButtonProps {
  readonly onClick: () => void;
  readonly disabled?: boolean;
}

/**
 * Standalone button that opens the new project dialog
 */
export function NewProjectButton({
  onClick,
  disabled,
}: NewProjectButtonProps) {
  return (
    <Button
      variant="outline"
      size="sm"
      onClick={onClick}
      disabled={disabled}
      className="gap-2"
    >
      <Plus className="h-4 w-4" />
      New Project
    </Button>
  );
}
