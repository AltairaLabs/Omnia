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
} from "lucide-react";
import type { ArenaProject } from "@/types/arena-project";
import { cn } from "@/lib/utils";
import { DeployButton } from "./deploy-button";
import { RunDropdown } from "./run-dropdown";

interface ProjectToolbarProps {
  projects: ArenaProject[];
  currentProject: ArenaProject | null;
  hasUnsavedChanges: boolean;
  saving: boolean;
  loading: boolean;
  validating?: boolean;
  /** Whether the user has write permissions */
  canWrite?: boolean;
  onProjectSelect: (projectId: string) => void;
  onSave: () => void;
  onNewProject: () => void;
  onRefresh: () => void;
  onDeleteProject?: () => void;
  onValidateAll?: () => void;
  className?: string;
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

        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={onNewProject}
          disabled={loading || !canWrite}
          title={canWrite ? "New Project" : "Editor access required to create projects"}
        >
          <Plus className="h-4 w-4" />
        </Button>

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
          <span className="text-xs text-amber-500 mr-2">Unsaved changes</span>
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
  onClick: () => void;
  disabled?: boolean;
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
