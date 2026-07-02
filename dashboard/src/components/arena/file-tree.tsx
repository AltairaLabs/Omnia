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
import type { ProviderBindingInfo } from "@/hooks/arena";
import { FileContextMenu } from "./file-context-menu";
import { ProviderBindingIndicator } from "./provider-binding-indicator";
import { NewItemDialog } from "./new-item-dialog";
import { RenameDialog } from "./rename-dialog";
import { DeleteConfirmDialog } from "./delete-confirm-dialog";
import { ImportProviderDialog } from "./import-provider-dialog";
import { ImportToolDialog } from "./import-tool-dialog";
import { useToast } from "@/hooks/core";
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
  readonly providerBindingStatus?: Map<string, ProviderBindingInfo>;
  readonly onSelectFile: (path: string, name: string) => void;
  readonly onCreateFile?: (parentPath: string | null, name: string, isDirectory: boolean, content?: string) => Promise<void>;
  readonly onRenameFile?: (path: string, newName: string) => Promise<void>;
  readonly onMoveFile?: (path: string, destDir: string) => Promise<void>;
  readonly onDeleteFile?: (path: string) => Promise<void>;
  readonly onBindProvider?: (node: FileTreeNode) => void;
  readonly className?: string;
}

interface DragHandlers {
  readonly draggable: boolean;
  readonly draggedPath: string | null;
  readonly dropTargetPath: string | null;
  readonly canDropOn: (targetDir: string) => boolean;
  readonly onDragStartNode: (path: string) => void;
  readonly onDragEndNode: () => void;
  readonly onDragOverDir: (path: string, e: React.DragEvent) => void;
  readonly onDragLeaveDir: () => void;
  readonly onDropOnDir: (path: string, e: React.DragEvent) => void;
}

interface TreeNodeProps {
  readonly node: FileTreeNode;
  readonly level: number;
  readonly selectedPath?: string;
  readonly providerBindingStatus?: Map<string, ProviderBindingInfo>;
  readonly expandedPaths: Set<string>;
  readonly onToggleExpand: (path: string) => void;
  readonly onSelectFile: (path: string, name: string) => void;
  readonly onContextAction: (action: ContextAction, node: FileTreeNode, kind?: ArenaFileKind) => void;
  readonly drag: DragHandlers;
}

type ContextAction = "newFile" | "newFolder" | "newTypedFile" | "importProvider" | "importTool" | "bindProvider" | "rename" | "delete" | "copyPath";

/** Whether `src` can be dropped into directory `targetDir` ("" = project root). */
function canDropInto(src: string | null, targetDir: string): boolean {
  if (!src) return false;
  if (targetDir === src) return false; // onto itself
  if (targetDir.startsWith(`${src}/`)) return false; // into its own descendant
  const currentParent = src.includes("/") ? src.slice(0, src.lastIndexOf("/")) : "";
  return targetDir !== currentParent; // already lives there
}

/**
 * Get icon for file type
 */
function getFileIcon(type: FileType | undefined, name: string) {
  // Special case for config file
  if (name === "config.arena.yaml") {
    return <Settings className="h-4 w-4 text-category-2 flex-shrink-0" />;
  }

  switch (type) {
    case "arena":
    case "prompt":
    case "provider":
    case "scenario":
    case "tool":
    case "persona":
      return <FileCode className="h-4 w-4 text-category-1 flex-shrink-0" />;
    case "yaml":
    case "json":
      return <FileCode className="h-4 w-4 text-category-4 flex-shrink-0" />;
    case "markdown":
      return <FileText className="h-4 w-4 text-muted-foreground flex-shrink-0" />;
    default:
      return <File className="h-4 w-4 text-muted-foreground flex-shrink-0" />;
  }
}

function TreeNode({
  node,
  level,
  selectedPath,
  providerBindingStatus,
  expandedPaths,
  onToggleExpand,
  onSelectFile,
  onContextAction,
  drag,
}: TreeNodeProps) {
  const isExpanded = expandedPaths.has(node.path);
  const isSelected = selectedPath === node.path;
  const hasChildren = node.children && node.children.length > 0;
  const paddingLeft = `${level * 16 + 8}px`;

  // Check if this is the project config file (should not be deletable or moved).
  const isConfigFile = node.name === "config.arena.yaml" && level === 0;
  const isDraggable = drag.draggable && !isConfigFile;
  const isDropTarget =
    node.isDirectory && drag.dropTargetPath === node.path && drag.canDropOn(node.path);

  const dirDropProps = node.isDirectory
    ? {
        onDragOver: (e: React.DragEvent) => drag.onDragOverDir(node.path, e),
        onDragLeave: () => drag.onDragLeaveDir(),
        onDrop: (e: React.DragEvent) => drag.onDropOnDir(node.path, e),
      }
    : {};

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

  const content = (
    <div
      role="button"
      tabIndex={0}
      draggable={isDraggable}
      onDragStart={(e) => {
        e.dataTransfer.effectAllowed = "move";
        e.dataTransfer.setData("text/plain", node.path);
        drag.onDragStartNode(node.path);
      }}
      onDragEnd={() => drag.onDragEndNode()}
      className={cn(
        "flex items-center gap-1 py-1 px-2 rounded-sm transition-colors cursor-pointer",
        "hover:bg-muted/50",
        isSelected && "bg-primary/10 text-primary font-medium",
        isDropTarget && "ring-1 ring-primary bg-primary/10"
      )}
      style={{ paddingLeft }}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
      {...dirDropProps}
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
        <FolderOpen className="h-4 w-4 text-category-4 flex-shrink-0" />
      )}
      {node.isDirectory && !isExpanded && (
        <Folder className="h-4 w-4 text-category-4 flex-shrink-0" />
      )}
      {!node.isDirectory && getFileIcon(node.type, node.name)}

      {/* Name */}
      <span className="truncate text-sm">{node.name}</span>

      {/* Provider binding indicator */}
      {!node.isDirectory && node.type === "provider" && providerBindingStatus?.has(node.path) && (
        <ProviderBindingIndicator bindingInfo={providerBindingStatus.get(node.path)!} />
      )}
    </div>
  );

  const isProviderFile = !node.isDirectory && node.type === "provider";

  return (
    <div>
      <FileContextMenu
        isDirectory={node.isDirectory}
        isRoot={isConfigFile}
        isProviderFile={isProviderFile}
        onNewFile={() => handleContextAction("newFile")}
        onNewFolder={() => handleContextAction("newFolder")}
        onNewTypedFile={(kind) => handleContextAction("newTypedFile", kind)}
        onImportProvider={() => handleContextAction("importProvider")}
        onImportTool={() => handleContextAction("importTool")}
        onBindProvider={() => handleContextAction("bindProvider")}
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
              providerBindingStatus={providerBindingStatus}
              expandedPaths={expandedPaths}
              onToggleExpand={onToggleExpand}
              onSelectFile={onSelectFile}
              onContextAction={onContextAction}
              drag={drag}
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
  providerBindingStatus,
  onSelectFile,
  onCreateFile,
  onRenameFile,
  onMoveFile,
  onDeleteFile,
  onBindProvider,
  className,
}: Readonly<FileTreeProps>) {
  const { toast } = useToast();
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [draggedPath, setDraggedPath] = useState<string | null>(null);
  const [dropTargetPath, setDropTargetPath] = useState<string | null>(null);
  const [renameDialog, setRenameDialog] = useState<{
    open: boolean;
    node: FileTreeNode | null;
  }>({ open: false, node: null });
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

  const performMove = useCallback(
    async (src: string, destDir: string) => {
      if (!onMoveFile || !canDropInto(src, destDir)) return;
      try {
        await onMoveFile(src, destDir);
      } catch {
        // Error toast is handled by the parent component.
      }
    },
    [onMoveFile]
  );

  const drag: DragHandlers = {
    draggable: !!onMoveFile,
    draggedPath,
    dropTargetPath,
    canDropOn: (targetDir: string) => canDropInto(draggedPath, targetDir),
    onDragStartNode: (path: string) => setDraggedPath(path),
    onDragEndNode: () => {
      setDraggedPath(null);
      setDropTargetPath(null);
    },
    onDragOverDir: (path: string, e: React.DragEvent) => {
      if (!canDropInto(draggedPath, path)) return;
      e.preventDefault();
      e.stopPropagation();
      e.dataTransfer.dropEffect = "move";
      if (dropTargetPath !== path) setDropTargetPath(path);
    },
    onDragLeaveDir: () => setDropTargetPath(null),
    onDropOnDir: (path: string, e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const src = draggedPath;
      setDraggedPath(null);
      setDropTargetPath(null);
      if (src) void performMove(src, path);
    },
  };

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
        case "bindProvider":
          onBindProvider?.(node);
          break;
        case "rename":
          if (onRenameFile) setRenameDialog({ open: true, node });
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
    [toast, createTypedFile, onBindProvider, onRenameFile]
  );

  const handleRenameItem = async (newName: string) => {
    if (!onRenameFile || !renameDialog.node) return;
    await onRenameFile(renameDialog.node.path, newName);
  };

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
        {/* Root drop zone for moving items to the project root. A drag-and-drop
            target has no native interactive element / keyboard equivalent here;
            the same move is reachable via the (future) context menu. */}
        {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions */}
        <div
          className={cn(
            "min-h-[100px] rounded-sm",
            dropTargetPath === "" && draggedPath && canDropInto(draggedPath, "") && "ring-1 ring-primary ring-inset bg-primary/5"
          )}
          onDragOver={(e) => {
            if (!canDropInto(draggedPath, "")) return;
            e.preventDefault();
            e.dataTransfer.dropEffect = "move";
            if (dropTargetPath !== "") setDropTargetPath("");
          }}
          onDrop={(e) => {
            e.preventDefault();
            const src = draggedPath;
            setDraggedPath(null);
            setDropTargetPath(null);
            if (src) void performMove(src, "");
          }}
        >
          {tree.map((node) => (
            <TreeNode
              key={node.path}
              node={node}
              level={0}
              selectedPath={selectedPath}
              providerBindingStatus={providerBindingStatus}
              expandedPaths={expandedPaths}
              onToggleExpand={handleToggleExpand}
              onSelectFile={onSelectFile}
              onContextAction={handleContextAction}
              drag={drag}
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

      <RenameDialog
        open={renameDialog.open}
        onOpenChange={(open) => setRenameDialog((prev) => ({ ...prev, open }))}
        currentName={renameDialog.node?.name || ""}
        isDirectory={renameDialog.node?.isDirectory || false}
        onConfirm={handleRenameItem}
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
