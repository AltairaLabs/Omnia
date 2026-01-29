"use client";

import { useEffect, useCallback, useState } from "react";
import dynamic from "next/dynamic";
import { useProjectEditorStore, useActiveFile, useHasUnsavedChanges } from "@/stores";
import {
  useArenaProjects,
  useArenaProject,
  useArenaProjectMutations,
  useArenaProjectFiles,
  useProviders,
  useDevSession,
} from "@/hooks";
import { FileTree } from "./file-tree";
import { EditorTabs, EditorTabsEmptyState } from "./editor-tabs";
import { YamlEditor, YamlEditorEmptyState } from "./yaml-editor";
import { ProjectToolbar } from "./project-toolbar";
import { useWorkspace } from "@/contexts/workspace-context";
import { getRuntimeConfig } from "@/lib/config";
import { NewItemDialog } from "./new-item-dialog";
import { DeleteConfirmDialog } from "./delete-confirm-dialog";
import {
  ValidationResultsDialog,
  type ValidationResults,
} from "./validation-results-dialog";
import { cn } from "@/lib/utils";
import { useToast } from "@/hooks/use-toast";
import { Loader2 } from "lucide-react";
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from "@/components/ui/resizable";
import { ResultsPanel, type Problem } from "./results-panel";
import { DevConsolePanel } from "./dev-console-panel";
import { useResultsPanelStore, useResultsPanelActiveTab } from "@/stores/results-panel-store";

// Dynamically import LspYamlEditor to avoid SSR issues with monaco-languageclient
// The vscode package has Node.js-specific code that doesn't work in SSR context
// In development mode with Turbopack, we skip loading the LSP editor entirely
// because Turbopack can't handle the vscode package's Node.js-specific code
const isDev = process.env.NODE_ENV === "development";

const LspYamlEditor = isDev
  ? null
  : dynamic(
      () => import("./lsp-yaml-editor").then((mod) => mod.LspYamlEditor),
      {
        ssr: false,
        loading: () => (
          <div className="flex items-center justify-center h-full">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ),
      }
    );

const LspYamlEditorEmptyState = isDev
  ? null
  : dynamic(
      () => import("./lsp-yaml-editor").then((mod) => mod.LspYamlEditorEmptyState),
      { ssr: false }
    );

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
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const namespace = currentWorkspace?.namespace;

  // Enterprise feature check for LSP
  const [lspEnabled, setLspEnabled] = useState(false);
  useEffect(() => {
    getRuntimeConfig().then((config) => {
      setLspEnabled(config.enterpriseEnabled);
    });
  }, []);

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
  const { data: providers = [] } = useProviders();

  // Dev session for interactive testing (ArenaDevSession)
  const {
    session: devSession,
    isLoading: devSessionLoading,
    error: devSessionError,
    isReady: devSessionReady,
    createSession: createDevSession,
  } = useDevSession({
    workspace: workspace || "",
    projectId: currentProject?.id || "",
    autoCreate: false, // Only create when console tab is opened
  });

  // Track active results panel tab to trigger session creation
  const activeResultsTab = useResultsPanelActiveTab();

  // Derived state
  const activeFile = useActiveFile();
  const hasUnsavedChanges = useHasUnsavedChanges();

  // Local state for dialogs
  const [newProjectDialogOpen, setNewProjectDialogOpen] = useState(false);
  const [deleteProjectDialogOpen, setDeleteProjectDialogOpen] = useState(false);
  const [validationDialogOpen, setValidationDialogOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [validating, setValidating] = useState(false);
  const [validationResults, setValidationResults] = useState<ValidationResults | null>(null);
  const [problems, setProblems] = useState<Problem[]>([]);
  const [selectedProvider, setSelectedProvider] = useState<string | undefined>();

  // Provider options for dev console
  const providerOptions = providers.map((p) => ({
    id: p.metadata.name,
    name: `${p.metadata.name} (${p.spec.type})`,
  }));

  // Results panel store
  const resultsPanelOpen = useResultsPanelStore((state) => state.isOpen);
  const openResultsPanel = useResultsPanelStore((state) => state.open);

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

  // Create dev session when Console tab is activated
  useEffect(() => {
    if (
      activeResultsTab === "console" &&
      currentProject &&
      workspace &&
      !devSession &&
      !devSessionLoading
    ) {
      createDevSession().catch((err) => {
        console.error("Failed to create dev session:", err);
      });
    }
  }, [activeResultsTab, currentProject, workspace, devSession, devSessionLoading, createDevSession]);

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
    async (parentPath: string | null, name: string, isDirectory: boolean, content?: string) => {
      if (!currentProject) return;

      try {
        await createFile(currentProject.id, parentPath, name, isDirectory, content);
        // Refresh file tree
        const newTree = await refreshFileTree(currentProject.id);
        setFileTree(newTree);
        toast({
          title: "Created",
          description: `${isDirectory ? "Folder" : "File"} "${name}" created`,
        });

        // Open and focus the newly created file (not for directories)
        if (!isDirectory) {
          const fullPath = parentPath ? `${parentPath}/${name}` : name;
          await handleSelectFile(fullPath, name);
        }
      } catch (err) {
        toast({
          title: "Error",
          description: err instanceof Error ? err.message : "Failed to create item",
          variant: "destructive",
        });
        throw err;
      }
    },
    [currentProject, createFile, refreshFileTree, setFileTree, toast, handleSelectFile]
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

  // Handle validate all
  const handleValidateAll = useCallback(async () => {
    if (!currentProject || !workspace) return;

    setValidating(true);
    try {
      // Get the LSP service URL from runtime config
      const config = await getRuntimeConfig();

      // Construct the LSP API URL
      // The LSP service runs alongside the dashboard, proxied through the WebSocket proxy
      let apiUrl: string;
      if (config.wsProxyUrl) {
        const url = new URL(config.wsProxyUrl);
        url.pathname = "/api/compile";
        apiUrl = url.toString().replace(/^ws/, "http");
      } else {
        // Fallback: construct URL from current host with default port
        const protocol = window.location.protocol;
        const hostname = window.location.hostname;
        apiUrl = `${protocol}//${hostname}:3002/api/compile`;
      }

      const response = await fetch(apiUrl, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          workspace: workspace,
          project: currentProject.id,
        }),
      });

      if (!response.ok) {
        throw new Error(`Validation failed: ${response.statusText}`);
      }

      const results: ValidationResults = await response.json();
      setValidationResults(results);
      setValidationDialogOpen(true);

      // Convert validation results to problems for the panel
      const newProblems: Problem[] = [];
      if (results.diagnostics) {
        for (const [file, diagnostics] of Object.entries(results.diagnostics)) {
          for (const diag of diagnostics) {
            newProblems.push({
              severity: diag.severity === 1 ? "error" : diag.severity === 2 ? "warning" : "info",
              message: diag.message,
              file,
              line: diag.range?.start?.line ? diag.range.start.line + 1 : undefined,
              column: diag.range?.start?.character ? diag.range.start.character + 1 : undefined,
              source: diag.source,
            });
          }
        }
      }
      setProblems(newProblems);

      // Open problems panel if there are issues
      if (newProblems.length > 0) {
        openResultsPanel("problems");
      }

      // Show toast summary
      if (results.valid) {
        toast({
          title: "Validation Passed",
          description: `All ${results.summary?.totalFiles || 0} files are valid`,
        });
      } else {
        toast({
          title: "Validation Failed",
          description: `${results.summary?.errorCount || 0} errors, ${results.summary?.warningCount || 0} warnings`,
          variant: "destructive",
        });
      }
    } catch (err) {
      toast({
        title: "Validation Error",
        description: err instanceof Error ? err.message : "Failed to validate project",
        variant: "destructive",
      });
    } finally {
      setValidating(false);
    }
  }, [currentProject, workspace, toast, openResultsPanel]);

  // Handle jumping to a file from validation results
  const handleValidationFileClick = useCallback(
    // Line parameter required by ValidationResultsDialog callback signature
    async (path: string, _line?: number) => {
      if (!currentProject) return;

      // Open the file
      const fileName = path.split("/").pop() || path;
      await handleSelectFile(path, fileName);

      // Line navigation would need editor ref - for now just open the file
      setValidationDialogOpen(false);
    },
    [currentProject, handleSelectFile]
  );

  // Handle problem click in results panel
  const handleProblemClick = useCallback(
    async (problem: Problem) => {
      const fileName = problem.file.split("/").pop() || problem.file;
      await handleSelectFile(problem.file, fileName);
    },
    [handleSelectFile]
  );

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
        validating={validating}
        onProjectSelect={handleProjectSelect}
        onSave={handleSave}
        onNewProject={() => setNewProjectDialogOpen(true)}
        onRefresh={handleRefresh}
        onDeleteProject={currentProject ? () => setDeleteProjectDialogOpen(true) : undefined}
        onValidateAll={lspEnabled ? handleValidateAll : undefined}
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
          direction="vertical"
          className="flex-1 min-h-0"
        >
          {/* Main content area */}
          <ResizablePanel defaultSize={resultsPanelOpen ? 70 : 100} minSize={30}>
            <ResizablePanelGroup
              direction="horizontal"
              className="h-full"
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
                      // Use LSP editor in production when enabled, fall back to basic editor in dev
                      lspEnabled && workspace && currentProject && LspYamlEditor ? (
                        <LspYamlEditor
                          value={activeFile.content}
                          onChange={handleEditorChange}
                          onSave={handleSave}
                          fileType={activeFile.type}
                          loading={activeFile.loading}
                          workspace={workspace}
                          projectId={currentProject.id}
                          filePath={activeFile.path}
                        />
                      ) : (
                        <YamlEditor
                          value={activeFile.content}
                          onChange={handleEditorChange}
                          onSave={handleSave}
                          fileType={activeFile.type}
                          loading={activeFile.loading}
                        />
                      )
                    ) : openFiles.length > 0 ? (
                      <EditorTabsEmptyState />
                    ) : lspEnabled && LspYamlEditorEmptyState ? (
                      <LspYamlEditorEmptyState />
                    ) : (
                      <YamlEditorEmptyState />
                    )}
                  </div>
                </div>
              </ResizablePanel>
            </ResizablePanelGroup>
          </ResizablePanel>

          {/* Results panel - expanded state with resizable */}
          {resultsPanelOpen && (
            <>
              <ResizableHandle withHandle />
              <ResizablePanel defaultSize={30} minSize={15} maxSize={60}>
                <ResultsPanel
                  problems={problems}
                  onProblemClick={handleProblemClick}
                  consoleContent={
                    currentProject ? (
                      devSessionLoading ? (
                        <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
                          <Loader2 className="h-6 w-6 animate-spin mb-2" />
                          <p className="text-sm">Starting dev session...</p>
                          <p className="text-xs mt-1">This may take a moment</p>
                        </div>
                      ) : devSessionError ? (
                        <div className="flex flex-col items-center justify-center h-full text-destructive">
                          <p className="text-sm">Failed to start dev session</p>
                          <p className="text-xs mt-1">{devSessionError.message}</p>
                        </div>
                      ) : devSessionReady ? (
                        <DevConsolePanel
                          projectId={currentProject.id}
                          workspace={workspace}
                          namespace={namespace}
                          service={devSession?.status?.serviceName}
                          configPath={activeFile?.path}
                          providers={providerOptions}
                          selectedProvider={selectedProvider}
                          onProviderChange={setSelectedProvider}
                        />
                      ) : (
                        <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
                          <Loader2 className="h-6 w-6 animate-spin mb-2" />
                          <p className="text-sm">Session starting...</p>
                          <p className="text-xs mt-1">Waiting for service to be ready</p>
                        </div>
                      )
                    ) : undefined
                  }
                />
              </ResizablePanel>
            </>
          )}
        </ResizablePanelGroup>
      )}

      {/* Collapsed results panel bar - shown when panel is closed */}
      {!resultsPanelOpen && (
        <ResultsPanel
          problems={problems}
          onProblemClick={handleProblemClick}
        />
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

      <ValidationResultsDialog
        open={validationDialogOpen}
        onOpenChange={setValidationDialogOpen}
        results={validationResults}
        onFileClick={handleValidationFileClick}
      />
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
