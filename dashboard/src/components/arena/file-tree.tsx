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
import { useToast } from "@/hooks/use-toast";

interface FileTreeProps {
  tree: FileTreeNode[];
  loading?: boolean;
  error?: string | null;
  selectedPath?: string;
  onSelectFile: (path: string, name: string) => void;
  onCreateFile?: (parentPath: string | null, name: string, isDirectory: boolean) => Promise<void>;
  onDeleteFile?: (path: string) => Promise<void>;
  className?: string;
}

interface TreeNodeProps {
  node: FileTreeNode;
  level: number;
  selectedPath?: string;
  expandedPaths: Set<string>;
  onToggleExpand: (path: string) => void;
  onSelectFile: (path: string, name: string) => void;
  onContextAction: (action: ContextAction, node: FileTreeNode) => void;
}

type ContextAction = "newFile" | "newFolder" | "rename" | "delete" | "copyPath";

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

  const handleContextAction = (action: ContextAction) => {
    onContextAction(action, node);
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
      {node.isDirectory ? (
        isExpanded ? (
          <FolderOpen className="h-4 w-4 text-amber-500 flex-shrink-0" />
        ) : (
          <Folder className="h-4 w-4 text-amber-500 flex-shrink-0" />
        )
      ) : (
        getFileIcon(node.type, node.name)
      )}

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

  const handleContextAction = useCallback(
    (action: ContextAction, node: FileTreeNode) => {
      switch (action) {
        case "newFile":
          setNewItemDialog({
            open: true,
            mode: "file",
            parentPath: node.isDirectory ? node.path : null,
          });
          break;
        case "newFolder":
          setNewItemDialog({
            open: true,
            mode: "folder",
            parentPath: node.isDirectory ? node.path : null,
          });
          break;
        case "rename":
          // Note: Rename dialog to be added in future iteration
          toast({
            title: "Not implemented",
            description: "Rename functionality coming soon",
          });
          break;
        case "delete":
          setDeleteDialog({ open: true, node });
          break;
        case "copyPath":
          navigator.clipboard.writeText(node.path);
          toast({
            title: "Path copied",
            description: node.path,
          });
          break;
      }
    },
    [toast]
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
    </div>
  );
}
