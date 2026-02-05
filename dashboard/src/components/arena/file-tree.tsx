"use client";

import { useState, useCallback } from "react";
import { cn } from "@/lib/utils";
import {
  ChevronRight,
  ChevronDown,
  Folder,
  FolderOpen,
  File,
  FileCode,
  FileText,
  Loader2,
  Settings,
} from "lucide-react";
import type { FileTreeNode, FileType } from "@/types/arena-project";
import { FileContextMenu } from "./file-context-menu";
import { NewItemDialog } from "./new-item-dialog";
import { DeleteConfirmDialog } from "./delete-confirm-dialog";
import { ImportProviderDialog } from "./import-provider-dialog";
import { ImportToolDialog } from "./import-tool-dialog";
import { useToast } from "@/hooks/use-toast";
import {
  type ArenaFileKind,
  generateUniqueBaseName,
  generateFileName,
  generateFileContent,
} from "@/lib/arena/file-templates";

interface FileTreeProps {
  readonly tree: FileTreeNode[];
  readonly loading?: boolean;
  readonly error?: string | null;
  readonly selectedPath?: string;
  readonly onSelectFile: (path: string, name: string) => void;
  readonly onCreateFile?: (parentPath: string | null, name: string, isDirectory: boolean, content?: string) => Promise<void>;
  readonly onDeleteFile?: (path: string) => Promise<void>;
  readonly className?: string;
}

interface TreeNodeProps {
  readonly node: FileTreeNode;
  readonly level: number;
  readonly selectedPath?: string;
  readonly expandedPaths: Set<string>;
  readonly onToggleExpand: (path: string) => void;
  readonly onSelectFile: (path: string, name: string) => void;
  readonly onContextAction: (action: ContextAction, node: FileTreeNode, kind?: ArenaFileKind) => void;
}

type ContextAction = "newFile" | "newFolder" | "newTypedFile" | "importProvider" | "importTool" | "rename" | "delete" | "copyPath";

/**
 * Get icon for file type
 */
function getFileIcon(type: FileType | undefined, name: string) {
  // Special case for config file
  if (name === "config.arena.yaml") {
    return <Settings className="h-4 w-4 text-purple-500 flex-shrink-0" />;
  }

  switch (type) {
    case "arena":
    case "prompt":
    case "provider":
    case "scenario":
    case "tool":
    case "persona":
      return <FileCode className="h-4 w-4 text-blue-500 flex-shrink-0" />;
    case "yaml":
    case "json":
      return <FileCode className="h-4 w-4 text-yellow-600 flex-shrink-0" />;
    case "markdown":
      return <FileText className="h-4 w-4 text-gray-500 flex-shrink-0" />;
    default:
      return <File className="h-4 w-4 text-gray-400 flex-shrink-0" />;
  }
}

function TreeNode({
  node,
  level,
  selectedPath,
  expandedPaths,
  onToggleExpand,
  onSelectFile,
  onContextAction,
}: TreeNodeProps) {
  const isExpanded = expandedPaths.has(node.path);
  const isSelected = selectedPath === node.path;
  const hasChildren = node.children && node.children.length > 0;
  const paddingLeft = `${level * 16 + 8}px`;

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (node.isDirectory) {
      onToggleExpand(node.path);
    }
  };

  const handleClick = () => {
    if (node.isDirectory) {
      onToggleExpand(node.path);
    } else {
      onSelectFile(node.path, node.name);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      handleClick();
    }
  };

  const handleContextAction = (action: ContextAction, kind?: ArenaFileKind) => {
    onContextAction(action, node, kind);
  };

  // Check if this is the project config file (should not be deletable)
  const isConfigFile = node.name === "config.arena.yaml" && level === 0;

  const content = (
    <div
      role="button"
      tabIndex={0}
      className={cn(
        "flex items-center gap-1 py-1 px-2 rounded-sm transition-colors cursor-pointer",
        "hover:bg-muted/50",
        isSelected && "bg-primary/10 text-primary font-medium"
      )}
      style={{ paddingLeft }}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      {/* Expand/collapse chevron for directories */}
      {node.isDirectory ? (
        <button
          type="button"
          onClick={handleToggle}
          className={cn(
            "p-0.5 hover:bg-muted rounded-sm transition-colors",
            !hasChildren && "invisible"
          )}
        >
          {isExpanded ? (
            <ChevronDown className="h-3.5 w-3.5" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5" />
          )}
        </button>
      ) : (
        <span className="w-4" />
      )}

      {/* Icon */}
      {node.isDirectory && isExpanded && (
        <FolderOpen className="h-4 w-4 text-amber-500 flex-shrink-0" />
      )}
      {node.isDirectory && !isExpanded && (
        <Folder className="h-4 w-4 text-amber-500 flex-shrink-0" />
      )}
      {!node.isDirectory && getFileIcon(node.type, node.name)}

      {/* Name */}
      <span className="truncate text-sm">{node.name}</span>
    </div>
  );

  return (
    <div>
      <FileContextMenu
        isDirectory={node.isDirectory}
        isRoot={isConfigFile}
        onNewFile={() => handleContextAction("newFile")}
        onNewFolder={() => handleContextAction("newFolder")}
        onNewTypedFile={(kind) => handleContextAction("newTypedFile", kind)}
        onImportProvider={() => handleContextAction("importProvider")}
        onImportTool={() => handleContextAction("importTool")}
        onRename={() => handleContextAction("rename")}
        onDelete={() => handleContextAction("delete")}
        onCopyPath={() => handleContextAction("copyPath")}
      >
        {content}
      </FileContextMenu>

      {/* Children (if expanded) */}
      {isExpanded && node.children && (
        <div>
          {node.children.map((child) => (
            <TreeNode
              key={child.path}
              node={child}
              level={level + 1}
              selectedPath={selectedPath}
              expandedPaths={expandedPaths}
              onToggleExpand={onToggleExpand}
              onSelectFile={onSelectFile}
              onContextAction={onContextAction}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * File tree component for navigating project files.
 * Supports context menu for file operations.
 */
export function FileTree({
  tree,
  loading = false,
  error = null,
  selectedPath,
  onSelectFile,
  onCreateFile,
  onDeleteFile,
  className,
}: Readonly<FileTreeProps>) {
  const { toast } = useToast();
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [newItemDialog, setNewItemDialog] = useState<{
    open: boolean;
    mode: "file" | "folder";
    parentPath: string | null;
  }>({ open: false, mode: "file", parentPath: null });
  const [deleteDialog, setDeleteDialog] = useState<{
    open: boolean;
    node: FileTreeNode | null;
  }>({ open: false, node: null });
  const [importProviderDialog, setImportProviderDialog] = useState<{
    open: boolean;
    parentPath: string | null;
  }>({ open: false, parentPath: null });
  const [importToolDialog, setImportToolDialog] = useState<{
    open: boolean;
    parentPath: string | null;
  }>({ open: false, parentPath: null });

  const handleToggleExpand = useCallback((path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }, []);

  const createTypedFile = useCallback(
    async (kind: ArenaFileKind, parentPath: string | null) => {
      if (!onCreateFile) return;
      const baseName = generateUniqueBaseName(kind);
      const fileName = generateFileName(baseName, kind);
      const content = generateFileContent(baseName, kind);
      try {
        await onCreateFile(parentPath, fileName, false, content);
      } catch {
        // Error toast is handled by the parent component
      }
    },
    [onCreateFile]
  );

  const handleContextAction = useCallback(
    async (action: ContextAction, node: FileTreeNode, kind?: ArenaFileKind) => {
      const parentPath = node.isDirectory ? node.path : null;

      switch (action) {
        case "newFile":
          setNewItemDialog({ open: true, mode: "file", parentPath });
          break;
        case "newFolder":
          setNewItemDialog({ open: true, mode: "folder", parentPath });
          break;
        case "newTypedFile":
          if (kind) await createTypedFile(kind, parentPath);
          break;
        case "importProvider":
          setImportProviderDialog({ open: true, parentPath });
          break;
        case "importTool":
          setImportToolDialog({ open: true, parentPath });
          break;
        case "rename":
          toast({ title: "Not implemented", description: "Rename functionality coming soon" });
          break;
        case "delete":
          setDeleteDialog({ open: true, node });
          break;
        case "copyPath":
          navigator.clipboard.writeText(node.path);
          toast({ title: "Path copied", description: node.path });
          break;
      }
    },
    [toast, createTypedFile]
  );

  const handleCreateItem = async (name: string) => {
    if (!onCreateFile) return;
    await onCreateFile(
      newItemDialog.parentPath,
      name,
      newItemDialog.mode === "folder"
    );
  };

  const handleDeleteItem = async () => {
    if (!onDeleteFile || !deleteDialog.node) return;
    await onDeleteFile(deleteDialog.node.path);
  };

  // Handle root-level context menu (when clicking empty space)
  const handleRootNewFile = () => {
    setNewItemDialog({ open: true, mode: "file", parentPath: null });
  };

  const handleRootNewFolder = () => {
    setNewItemDialog({ open: true, mode: "folder", parentPath: null });
  };

  const handleRootNewTypedFile = async (kind: ArenaFileKind) => {
    if (!onCreateFile) return;
    const baseName = generateUniqueBaseName(kind);
    const fileName = generateFileName(baseName, kind);
    const content = generateFileContent(baseName, kind);
    try {
      await onCreateFile(null, fileName, false, content);
    } catch {
      // Error toast is handled by the parent component
    }
  };

  const handleRootImportProvider = () => {
    setImportProviderDialog({ open: true, parentPath: null });
  };

  const handleRootImportTool = () => {
    setImportToolDialog({ open: true, parentPath: null });
  };

  /**
   * Handle importing multiple files from a dialog.
   * Creates each file in sequence at the specified parent path.
   */
  const handleImportFiles = async (
    parentPath: string | null,
    files: { name: string; content: string }[]
  ) => {
    if (!onCreateFile || files.length === 0) return;

    let successCount = 0;
    let lastError: Error | null = null;

    for (const file of files) {
      try {
        await onCreateFile(parentPath, file.name, false, file.content);
        successCount++;
      } catch (err) {
        lastError = err instanceof Error ? err : new Error("Failed to create file");
      }
    }

    if (successCount > 0) {
      toast({
        title: "Import complete",
        description: `Successfully imported ${successCount} of ${files.length} file${files.length === 1 ? "" : "s"}`,
      });
    }

    if (lastError) {
      throw lastError;
    }
  };

  if (loading) {
    return (
      <div className={cn("flex items-center justify-center py-8", className)}>
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        <span className="ml-2 text-sm text-muted-foreground">Loading...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className={cn("text-sm text-destructive py-4 px-2", className)}>
        {error}
      </div>
    );
  }

  if (tree.length === 0) {
    return (
      <div className={cn("text-sm text-muted-foreground py-4 px-2", className)}>
        <p>No files in project.</p>
        {onCreateFile && (
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              onClick={handleRootNewFile}
              className="text-primary hover:underline"
            >
              Create a file
            </button>
            <span>or</span>
            <button
              type="button"
              onClick={handleRootNewFolder}
              className="text-primary hover:underline"
            >
              folder
            </button>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className={cn("py-1", className)}>
      {/* File tree */}
      <FileContextMenu
        isDirectory
        isRoot
        onNewFile={handleRootNewFile}
        onNewFolder={handleRootNewFolder}
        onNewTypedFile={handleRootNewTypedFile}
        onImportProvider={handleRootImportProvider}
        onImportTool={handleRootImportTool}
        onCopyPath={() => {
          navigator.clipboard.writeText("/");
          toast({ title: "Path copied", description: "/" });
        }}
      >
        <div className="min-h-[100px]">
          {tree.map((node) => (
            <TreeNode
              key={node.path}
              node={node}
              level={0}
              selectedPath={selectedPath}
              expandedPaths={expandedPaths}
              onToggleExpand={handleToggleExpand}
              onSelectFile={onSelectFile}
              onContextAction={handleContextAction}
            />
          ))}
        </div>
      </FileContextMenu>

      {/* Dialogs */}
      <NewItemDialog
        open={newItemDialog.open}
        onOpenChange={(open) => setNewItemDialog((prev) => ({ ...prev, open }))}
        mode={newItemDialog.mode}
        parentPath={newItemDialog.parentPath}
        onConfirm={handleCreateItem}
      />

      <DeleteConfirmDialog
        open={deleteDialog.open}
        onOpenChange={(open) => setDeleteDialog((prev) => ({ ...prev, open }))}
        itemName={deleteDialog.node?.name || ""}
        itemPath={deleteDialog.node?.path || ""}
        isDirectory={deleteDialog.node?.isDirectory || false}
        onConfirm={handleDeleteItem}
      />

      <ImportProviderDialog
        open={importProviderDialog.open}
        onOpenChange={(open) =>
          setImportProviderDialog((prev) => ({ ...prev, open }))
        }
        parentPath={importProviderDialog.parentPath}
        onImport={(files) =>
          handleImportFiles(importProviderDialog.parentPath, files)
        }
      />

      <ImportToolDialog
        open={importToolDialog.open}
        onOpenChange={(open) =>
          setImportToolDialog((prev) => ({ ...prev, open }))
        }
        parentPath={importToolDialog.parentPath}
        onImport={(files) =>
          handleImportFiles(importToolDialog.parentPath, files)
        }
      />
    </div>
  );
}
