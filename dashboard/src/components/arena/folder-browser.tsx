"use client";

import { useState, useCallback } from "react";
import { cn } from "@/lib/utils";
import { ChevronRight, ChevronDown, Folder, FolderOpen, File, Loader2 } from "lucide-react";
import type { ArenaSourceContentNode } from "@/types/arena";

interface FolderBrowserProps {
  /** Content tree to display */
  tree: ArenaSourceContentNode[];
  /** Whether content is loading */
  loading?: boolean;
  /** Error message if any */
  error?: string | null;
  /** Currently selected folder path */
  selectedPath?: string;
  /** Callback when a folder is selected */
  onSelectFolder: (path: string) => void;
  /** Callback when a file is selected (optional) */
  onSelectFile?: (filePath: string, folderPath: string, fileName: string) => void;
  /** Optional className for the container */
  className?: string;
  /** Maximum height for the browser */
  maxHeight?: string;
}

interface TreeNodeProps {
  node: ArenaSourceContentNode;
  level: number;
  selectedPath?: string;
  expandedPaths: Set<string>;
  onToggleExpand: (path: string) => void;
  onSelectFolder: (path: string) => void;
  onSelectFile?: (filePath: string, folderPath: string, fileName: string) => void;
}

function TreeNode({
  node,
  level,
  selectedPath,
  expandedPaths,
  onToggleExpand,
  onSelectFolder,
  onSelectFile,
}: TreeNodeProps) {
  const isExpanded = expandedPaths.has(node.path);
  const isSelected = selectedPath === node.path;
  const hasChildren = node.children && node.children.length > 0;
  const paddingLeft = `${level * 16 + 8}px`;

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (hasChildren) {
      onToggleExpand(node.path);
    }
  };

  // Render file node
  if (!node.isDirectory) {
    const fileContent = (
      <>
        <span className="w-4" />
        <File className="h-4 w-4 flex-shrink-0" />
        <span className="truncate text-sm">{node.name}</span>
      </>
    );

    // Non-clickable file (no onSelectFile provided)
    if (!onSelectFile) {
      return (
        <div
          className="flex items-center gap-1 py-1 px-2 text-muted-foreground"
          style={{ paddingLeft }}
        >
          {fileContent}
        </div>
      );
    }

    // Clickable file
    const handleFileClick = () => {
      // Extract folder path and file name from the full path
      const lastSlash = node.path.lastIndexOf("/");
      const folderPath = lastSlash === -1 ? "" : node.path.substring(0, lastSlash);
      const fileName = lastSlash === -1 ? node.path : node.path.substring(lastSlash + 1);
      onSelectFile(node.path, folderPath, fileName);
    };

    const handleFileKeyDown = (e: React.KeyboardEvent) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        handleFileClick();
      }
    };

    return (
      <div
        role="button"
        tabIndex={0}
        className="flex items-center gap-1 py-1 px-2 text-muted-foreground cursor-pointer hover:bg-muted/50 hover:text-foreground"
        style={{ paddingLeft }}
        onClick={handleFileClick}
        onKeyDown={handleFileKeyDown}
      >
        {fileContent}
      </div>
    );
  }

  // Render directory node (interactive)
  const handleClick = () => {
    onSelectFolder(node.path);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      handleClick();
    }
  };

  return (
    <div>
      <div
        role="button"
        tabIndex={0}
        className={cn(
          "flex items-center gap-1 py-1 px-2 rounded-sm transition-colors cursor-pointer hover:bg-muted/50",
          isSelected && "bg-primary/10 text-primary font-medium"
        )}
        style={{ paddingLeft }}
        onClick={handleClick}
        onKeyDown={handleKeyDown}
      >
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
        {isExpanded ? (
          <FolderOpen className="h-4 w-4 text-amber-500 flex-shrink-0" />
        ) : (
          <Folder className="h-4 w-4 text-amber-500 flex-shrink-0" />
        )}
        <span className="truncate text-sm">{node.name}</span>
      </div>

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
              onSelectFolder={onSelectFolder}
              onSelectFile={onSelectFile}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * A file/folder browser component for selecting a root folder from source content.
 * Folders are selectable to set the root path.
 * Files can optionally be clicked to set both folder and file name.
 */
export function FolderBrowser({
  tree,
  loading = false,
  error = null,
  selectedPath = "",
  onSelectFolder,
  onSelectFile,
  className,
  maxHeight = "200px",
}: Readonly<FolderBrowserProps>) {
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());

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

  const handleSelectRoot = () => {
    onSelectFolder("");
  };

  if (loading) {
    return (
      <div className={cn("flex items-center justify-center py-8", className)}>
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        <span className="ml-2 text-sm text-muted-foreground">Loading content...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className={cn("text-sm text-destructive py-4", className)}>
        {error}
      </div>
    );
  }

  if (tree.length === 0) {
    return (
      <div className={cn("text-sm text-muted-foreground py-4", className)}>
        No content available. The source may need to be synced.
      </div>
    );
  }

  return (
    <div className={cn("border rounded-md", className)}>
      <div className="overflow-y-auto" style={{ maxHeight }}>
        <div className="py-1">
          {/* Root folder option */}
          <div
            role="button"
            tabIndex={0}
            className={cn(
              "flex items-center gap-1 py-1 px-2 cursor-pointer rounded-sm hover:bg-muted/50 transition-colors mx-1",
              selectedPath === "" && "bg-primary/10 text-primary font-medium"
            )}
            onClick={handleSelectRoot}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                handleSelectRoot();
              }
            }}
          >
            <span className="w-4" />
            <Folder className="h-4 w-4 text-amber-500 flex-shrink-0" />
            <span className="text-sm">/ (root)</span>
          </div>

          {/* Tree nodes */}
          {tree.map((node) => (
            <TreeNode
              key={node.path}
              node={node}
              level={0}
              selectedPath={selectedPath}
              expandedPaths={expandedPaths}
              onToggleExpand={handleToggleExpand}
              onSelectFolder={onSelectFolder}
              onSelectFile={onSelectFile}
            />
          ))}
        </div>
      </div>

      {/* Selected path indicator */}
      <div className="border-t px-3 py-2 bg-muted/30">
        <div className="flex items-center justify-between">
          <span className="text-xs text-muted-foreground">Selected:</span>
          <code className="text-xs bg-muted px-1.5 py-0.5 rounded truncate ml-2">
            {selectedPath || "/"}
          </code>
        </div>
      </div>
    </div>
  );
}
