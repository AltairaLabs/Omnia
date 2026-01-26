"use client";

import { useEffect, useCallback, useState } from "react";
import { useProjectEditorStore, useActiveFile, useHasUnsavedChanges } from "@/stores";
import {
  useArenaProjects,
  useArenaProject,
  useArenaProjectMutations,
  useArenaProjectFiles,
} from "@/hooks";
import { FileTree } from "./file-tree";
import { EditorTabs, EditorTabsEmptyState } from "./editor-tabs";
import { YamlEditor, YamlEditorEmptyState } from "./yaml-editor";
import { ProjectToolbar } from "./project-toolbar";
import { NewItemDialog } from "./new-item-dialog";
import { DeleteConfirmDialog } from "./delete-confirm-dialog";
import { cn } from "@/lib/utils";
import { useToast } from "@/hooks/use-toast";
import { Loader2 } from "lucide-react";
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from "@/components/ui/resizable";

interface ProjectEditorProps {
  className?: string;
  initialProjectId?: string;
}

/**
 * Main project editor component with split-pane layout.
 * Integrates file tree, tabs, Monaco editor, and toolbar.
 */
/* eslint-disable sonarjs/cognitive-complexity -- orchestration component inherently complex */
export function ProjectEditor({ className, initialProjectId }: ProjectEditorProps) {
  const { toast } = useToast();

  // Store state
  const currentProject = useProjectEditorStore((state) => state.currentProject);
  const fileTree = useProjectEditorStore((state) => state.fileTree);
  const activeFilePath = useProjectEditorStore((state) => state.activeFilePath);
  const openFiles = useProjectEditorStore((state) => state.openFiles);
  const setCurrentProject = useProjectEditorStore((state) => state.setCurrentProject);
  const setFileTree = useProjectEditorStore((state) => state.setFileTree);
  const openFile = useProjectEditorStore((state) => state.openFile);
  const updateFileContent = useProjectEditorStore((state) => state.updateFileContent);
  const markFileSaved = useProjectEditorStore((state) => state.markFileSaved);
  const setProjectLoading = useProjectEditorStore((state) => state.setProjectLoading);
  const projectLoading = useProjectEditorStore((state) => state.projectLoading);
  const projectError = useProjectEditorStore((state) => state.projectError);

  // Hooks
  const { projects, loading: projectsLoading, refetch: refetchProjects } = useArenaProjects();
  const { createProject, deleteProject } = useArenaProjectMutations();
  const { getFileContent, updateFileContent: saveFileContent, createFile, deleteFile, refreshFileTree } = useArenaProjectFiles();

  // Derived state
  const activeFile = useActiveFile();
  const hasUnsavedChanges = useHasUnsavedChanges();

  // Local state for dialogs
  const [newProjectDialogOpen, setNewProjectDialogOpen] = useState(false);
  const [deleteProjectDialogOpen, setDeleteProjectDialogOpen] = useState(false);
  const [saving, setSaving] = useState(false);

  // Fetch project data when project ID changes
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(
    initialProjectId || null
  );

  const { project: projectData, loading: projectDataLoading, refetch: refetchProject } = useArenaProject(
    selectedProjectId || undefined
  );

  // Update store when project data is fetched
  useEffect(() => {
    if (projectData) {
      setCurrentProject(projectData, projectData.tree);
    }
  }, [projectData, setCurrentProject]);

  // Set loading state
  useEffect(() => {
    setProjectLoading(projectDataLoading);
  }, [projectDataLoading, setProjectLoading]);

  // Handle project selection
  const handleProjectSelect = useCallback((projectId: string) => {
    // Check for unsaved changes
    if (hasUnsavedChanges) {
      const confirmed = window.confirm(
        "You have unsaved changes. Do you want to discard them?"
      );
      if (!confirmed) return;
    }
    setSelectedProjectId(projectId);
  }, [hasUnsavedChanges]);

  // Handle file selection
  const handleSelectFile = useCallback(
    async (path: string, name: string) => {
      if (!currentProject) return;

      // Check if already open
      const existingFile = openFiles.find((f) => f.path === path);
      if (existingFile) {
        // Just set as active
        useProjectEditorStore.getState().setActiveFile(path);
        return;
      }

      try {
        const fileData = await getFileContent(currentProject.id, path);
        openFile(path, name, fileData.content);
      } catch (err) {
        toast({
          title: "Error",
          description: err instanceof Error ? err.message : "Failed to load file",
          variant: "destructive",
        });
      }
    },
    [currentProject, openFiles, getFileContent, openFile, toast]
  );

  // Handle editor content change
  const handleEditorChange = useCallback(
    (value: string) => {
      if (activeFilePath) {
        updateFileContent(activeFilePath, value);
      }
    },
    [activeFilePath, updateFileContent]
  );

  // Handle save
  const handleSave = useCallback(async () => {
    if (!currentProject || !activeFile || !activeFile.isDirty) return;

    setSaving(true);
    try {
      await saveFileContent(currentProject.id, activeFile.path, activeFile.content);
      markFileSaved(activeFile.path);
      toast({
        title: "Saved",
        description: `${activeFile.name} saved successfully`,
      });
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof Error ? err.message : "Failed to save file",
        variant: "destructive",
      });
    } finally {
      setSaving(false);
    }
  }, [currentProject, activeFile, saveFileContent, markFileSaved, toast]);

  // Handle create file
  const handleCreateFile = useCallback(
    async (parentPath: string | null, name: string, isDirectory: boolean) => {
      if (!currentProject) return;

      try {
        await createFile(currentProject.id, parentPath, name, isDirectory);
        // Refresh file tree
        const newTree = await refreshFileTree(currentProject.id);
        setFileTree(newTree);
        toast({
          title: "Created",
          description: `${isDirectory ? "Folder" : "File"} "${name}" created`,
        });
      } catch (err) {
        toast({
          title: "Error",
          description: err instanceof Error ? err.message : "Failed to create item",
          variant: "destructive",
        });
        throw err;
      }
    },
    [currentProject, createFile, refreshFileTree, setFileTree, toast]
  );

  // Handle delete file
  const handleDeleteFile = useCallback(
    async (path: string) => {
      if (!currentProject) return;

      try {
        await deleteFile(currentProject.id, path);
        // Close file if open
        useProjectEditorStore.getState().closeFile(path);
        // Refresh file tree
        const newTree = await refreshFileTree(currentProject.id);
        setFileTree(newTree);
        toast({
          title: "Deleted",
          description: `"${path}" deleted`,
        });
      } catch (err) {
        toast({
          title: "Error",
          description: err instanceof Error ? err.message : "Failed to delete item",
          variant: "destructive",
        });
        throw err;
      }
    },
    [currentProject, deleteFile, refreshFileTree, setFileTree, toast]
  );

  // Handle new project
  const handleNewProject = useCallback(async (name: string) => {
    try {
      const project = await createProject({ name });
      await refetchProjects();
      setSelectedProjectId(project.id);
      toast({
        title: "Created",
        description: `Project "${name}" created`,
      });
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof Error ? err.message : "Failed to create project",
        variant: "destructive",
      });
      throw err;
    }
  }, [createProject, refetchProjects, toast]);

  // Handle delete project
  const handleDeleteProject = useCallback(async () => {
    if (!currentProject) return;

    try {
      await deleteProject(currentProject.id);
      setSelectedProjectId(null);
      useProjectEditorStore.getState().clearProject();
      await refetchProjects();
      toast({
        title: "Deleted",
        description: `Project "${currentProject.name}" deleted`,
      });
    } catch (err) {
      toast({
        title: "Error",
        description: err instanceof Error ? err.message : "Failed to delete project",
        variant: "destructive",
      });
      throw err;
    }
  }, [currentProject, deleteProject, refetchProjects, toast]);

  // Handle refresh
  const handleRefresh = useCallback(async () => {
    await refetchProjects();
    if (currentProject) {
      refetchProject();
    }
  }, [refetchProjects, refetchProject, currentProject]);

  const isLoading = projectsLoading || projectLoading;

  return (
    <div className={cn("flex flex-col h-full", className)}>
      {/* Toolbar */}
      <ProjectToolbar
        projects={projects}
        currentProject={currentProject}
        hasUnsavedChanges={hasUnsavedChanges}
        saving={saving}
        loading={isLoading}
        onProjectSelect={handleProjectSelect}
        onSave={handleSave}
        onNewProject={() => setNewProjectDialogOpen(true)}
        onRefresh={handleRefresh}
        onDeleteProject={currentProject ? () => setDeleteProjectDialogOpen(true) : undefined}
      />

      {/* Main content */}
      {!currentProject && !isLoading ? (
        <EmptyProjectState
          hasProjects={projects.length > 0}
          onNewProject={() => setNewProjectDialogOpen(true)}
        />
      ) : isLoading && !currentProject ? (
        <div className="flex items-center justify-center flex-1">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          <span className="ml-2 text-muted-foreground">Loading...</span>
        </div>
      ) : (
        <ResizablePanelGroup
          direction="horizontal"
          className="flex-1 min-h-0"
        >
          {/* File tree panel */}
          <ResizablePanel defaultSize={25} minSize={15} maxSize={40}>
            <div className="h-full overflow-auto border-r">
              <div className="p-2 border-b bg-muted/30">
                <h3 className="text-sm font-medium truncate">
                  {currentProject?.name || "Project Files"}
                </h3>
              </div>
              <FileTree
                tree={fileTree}
                loading={projectLoading}
                error={projectError}
                selectedPath={activeFilePath || undefined}
                onSelectFile={handleSelectFile}
                onCreateFile={handleCreateFile}
                onDeleteFile={handleDeleteFile}
              />
            </div>
          </ResizablePanel>

          <ResizableHandle withHandle />

          {/* Editor panel */}
          <ResizablePanel defaultSize={75}>
            <div className="flex flex-col h-full">
              {/* Tabs */}
              <EditorTabs />

              {/* Editor */}
              <div className="flex-1 min-h-0">
                {activeFile ? (
                  <YamlEditor
                    value={activeFile.content}
                    onChange={handleEditorChange}
                    onSave={handleSave}
                    fileType={activeFile.type}
                    loading={activeFile.loading}
                  />
                ) : openFiles.length > 0 ? (
                  <EditorTabsEmptyState />
                ) : (
                  <YamlEditorEmptyState />
                )}
              </div>
            </div>
          </ResizablePanel>
        </ResizablePanelGroup>
      )}

      {/* Dialogs */}
      <NewItemDialog
        open={newProjectDialogOpen}
        onOpenChange={setNewProjectDialogOpen}
        mode="folder"
        parentPath={null}
        onConfirm={handleNewProject}
      />

      {currentProject && (
        <DeleteConfirmDialog
          open={deleteProjectDialogOpen}
          onOpenChange={setDeleteProjectDialogOpen}
          itemName={currentProject.name}
          itemPath={currentProject.id}
          isDirectory
          onConfirm={handleDeleteProject}
        />
      )}
    </div>
  );
}
/* eslint-enable sonarjs/cognitive-complexity */

interface EmptyProjectStateProps {
  hasProjects: boolean;
  onNewProject: () => void;
}

function EmptyProjectState({ hasProjects, onNewProject }: EmptyProjectStateProps) {
  return (
    <div className="flex items-center justify-center flex-1">
      <div className="text-center">
        <h3 className="text-lg font-medium mb-2">
          {hasProjects ? "Select a project" : "No projects yet"}
        </h3>
        <p className="text-muted-foreground text-sm mb-4">
          {hasProjects
            ? "Choose a project from the dropdown above to start editing"
            : "Create your first project to get started"}
        </p>
        <button
          type="button"
          onClick={onNewProject}
          className="text-primary hover:underline text-sm"
        >
          Create a new project
        </button>
      </div>
    </div>
  );
}
