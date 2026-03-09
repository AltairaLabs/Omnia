"use client";

import { useState, useCallback } from "react";
import { cn } from "@/lib/utils";
import { useArenaSourceContent } from "@/hooks/use-arena-source-content";
import { useWorkspace } from "@/contexts/workspace-context";
import { YamlEditor, YamlEditorEmptyState } from "./yaml-editor";
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from "@/components/ui/resizable";
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
  FolderTree,
} from "lucide-react";
import type { ArenaSourceContentNode } from "@/types/arena";

interface SourceExplorerProps {
  readonly sourceName: string;
}

/** Infer Monaco language from file extension */
function getLanguageForFile(name: string): string {
  if (name.endsWith(".yaml") || name.endsWith(".yml")) return "yaml";
  if (name.endsWith(".json")) return "json";
  if (name.endsWith(".md") || name.endsWith(".markdown")) return "markdown";
  if (name.endsWith(".go")) return "go";
  if (name.endsWith(".ts") || name.endsWith(".tsx")) return "typescript";
  if (name.endsWith(".js") || name.endsWith(".jsx")) return "javascript";
  if (name.endsWith(".sh") || name.endsWith(".bash")) return "shell";
  if (name.endsWith(".py")) return "python";
  return "plaintext";
}

/** Get icon for a file based on its name */
function getFileIcon(name: string) {
  if (name === "config.arena.yaml") {
    return <Settings className="h-4 w-4 text-purple-500 flex-shrink-0" />;
  }
  if (name.endsWith(".yaml") || name.endsWith(".yml") || name.endsWith(".json")) {
    return <FileCode className="h-4 w-4 text-yellow-600 flex-shrink-0" />;
  }
  if (name.endsWith(".md") || name.endsWith(".markdown")) {
    return <FileText className="h-4 w-4 text-gray-500 flex-shrink-0" />;
  }
  if (name.endsWith(".go") || name.endsWith(".ts") || name.endsWith(".js") || name.endsWith(".py")) {
    return <FileCode className="h-4 w-4 text-blue-500 flex-shrink-0" />;
  }
  return <File className="h-4 w-4 text-gray-400 flex-shrink-0" />;
}

interface ReadOnlyTreeNodeProps {
  readonly node: ArenaSourceContentNode;
  readonly level: number;
  readonly selectedPath?: string;
  readonly expandedPaths: Set<string>;
  readonly onToggleExpand: (path: string) => void;
  readonly onSelectFile: (path: string, name: string) => void;
}

function ReadOnlyTreeNode({
  node,
  level,
  selectedPath,
  expandedPaths,
  onToggleExpand,
  onSelectFile,
}: ReadOnlyTreeNodeProps) {
  const isExpanded = expandedPaths.has(node.path);
  const isSelected = selectedPath === node.path;
  const hasChildren = node.children && node.children.length > 0;
  const paddingLeft = `${level * 16 + 8}px`;

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

  return (
    <div>
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
        {node.isDirectory ? (
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onToggleExpand(node.path); }}
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

        {node.isDirectory && isExpanded && (
          <FolderOpen className="h-4 w-4 text-amber-500 flex-shrink-0" />
        )}
        {node.isDirectory && !isExpanded && (
          <Folder className="h-4 w-4 text-amber-500 flex-shrink-0" />
        )}
        {!node.isDirectory && getFileIcon(node.name)}

        <span className="truncate text-sm">{node.name}</span>
      </div>

      {isExpanded && node.children && (
        <div>
          {node.children.map((child) => (
            <ReadOnlyTreeNode
              key={child.path}
              node={child}
              level={level + 1}
              selectedPath={selectedPath}
              expandedPaths={expandedPaths}
              onToggleExpand={onToggleExpand}
              onSelectFile={onSelectFile}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * Read-only file explorer for ArenaSource content.
 * Split-pane layout: file tree on left, Monaco viewer on right.
 */
export function SourceExplorer({ sourceName }: SourceExplorerProps) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const { tree, fileCount, loading, error } = useArenaSourceContent(sourceName);

  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string>("");
  const [fileLoading, setFileLoading] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

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

  const handleSelectFile = useCallback(
    async (path: string, name: string) => {
      if (!workspace) return;

      setSelectedPath(path);
      setSelectedName(name);
      setFileLoading(true);
      setFileError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/sources/${sourceName}/file?path=${encodeURIComponent(path)}`
        );

        if (!response.ok) {
          const data = await response.json().catch(() => null);
          throw new Error(data?.error || `Failed to load file: ${response.statusText}`);
        }

        const data = await response.json();
        setFileContent(data.content);
      } catch (err) {
        setFileError(err instanceof Error ? err.message : "Failed to load file");
        setFileContent("");
      } finally {
        setFileLoading(false);
      }
    },
    [workspace, sourceName]
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <span className="ml-2 text-muted-foreground">Loading source content...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <FolderTree className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">Unable to load content</p>
        <p className="text-sm">{error.message}</p>
      </div>
    );
  }

  if (tree.length === 0) {
    return (
      <div className="text-center py-12 text-muted-foreground">
        <FolderTree className="h-12 w-12 mx-auto mb-4 opacity-50" />
        <p className="text-lg font-medium mb-1">No content available</p>
        <p className="text-sm">
          Source content will appear here once it has been synced.
        </p>
      </div>
    );
  }

  return (
    <ResizablePanelGroup orientation="horizontal" className="h-full rounded-lg border">
      {/* File tree panel */}
      <ResizablePanel defaultSize={25} minSize={15} maxSize={40}>
        <div className="h-full overflow-auto">
          <div className="p-2 border-b bg-muted/30">
            <h3 className="text-sm font-medium truncate">
              Files
              <span className="text-xs text-muted-foreground ml-2">({fileCount})</span>
            </h3>
          </div>
          <div className="py-1">
            {tree.map((node) => (
              <ReadOnlyTreeNode
                key={node.path}
                node={node}
                level={0}
                selectedPath={selectedPath || undefined}
                expandedPaths={expandedPaths}
                onToggleExpand={handleToggleExpand}
                onSelectFile={handleSelectFile}
              />
            ))}
          </div>
        </div>
      </ResizablePanel>

      <ResizableHandle withHandle />

      {/* Editor panel */}
      <ResizablePanel defaultSize={75}>
        <div className="flex flex-col h-full">
          {/* File path bar */}
          {selectedName && (
            <div className="px-3 py-1.5 border-b bg-muted/30 text-xs text-muted-foreground font-mono truncate">
              {selectedPath}
            </div>
          )}

          {/* Content */}
          <div className="flex-1 min-h-0">
            {fileError && (
              <div className="flex items-center justify-center h-full text-destructive">
                <p className="text-sm">{fileError}</p>
              </div>
            )}
            {!fileError && selectedPath && (
              <YamlEditor
                value={fileContent}
                readOnly
                language={getLanguageForFile(selectedName || "")}
                loading={fileLoading}
              />
            )}
            {!fileError && !selectedPath && <YamlEditorEmptyState />}
          </div>
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  );
}
