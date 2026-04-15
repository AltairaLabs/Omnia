"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { cn } from "@/lib/utils";
import { YamlEditor, YamlEditorEmptyState } from "@/components/arena";
import { Markdown } from "@/components/ui/markdown";
import { Button } from "@/components/ui/button";
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
  FolderTree,
} from "lucide-react";
import type { ArenaSourceContentNode } from "@/types/arena";

/** Loaded file payload. */
export interface LoadedFile {
  content: string;
  size?: number;
}

export interface ContentExplorerLabels {
  loading?: string;
  emptyTitle?: string;
  emptyDescription?: string;
}

export interface ContentExplorerProps {
  /** File tree to render. */
  readonly tree: ArenaSourceContentNode[];
  /** Total file count for the header. */
  readonly fileCount: number;
  /** Whether the tree is being fetched. */
  readonly loading: boolean;
  /** Tree-fetch error if any. */
  readonly error: Error | null;
  /** Loads the content of a single file by its tree-relative path. */
  readonly loadFile: (path: string) => Promise<LoadedFile>;
  /** Optional sub-path to scope the tree to (e.g. "load-testing"). */
  readonly rootPath?: string;
  /** File to auto-select on mount (relative to rootPath if provided). */
  readonly defaultFile?: string;
  /** Optional file-icon override; return undefined to use the default icon. */
  readonly getFileIcon?: (name: string) => ReactNode | undefined;
  /** Strings shown for loading / empty states. */
  readonly labels?: ContentExplorerLabels;
}

/** Infer Monaco language from file extension. */
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

function isMarkdown(name: string): boolean {
  return name.endsWith(".md") || name.endsWith(".markdown");
}

function stripWrappingQuotes(value: string): string {
  if (value.length >= 2) {
    const first = value.charAt(0);
    const last = value.charAt(value.length - 1);
    if ((first === '"' || first === "'") && first === last) {
      return value.slice(1, -1);
    }
  }
  return value;
}

/**
 * Parse YAML frontmatter from a markdown document. Handles flat string keys
 * with optional indented continuation lines (good enough for SKILL.md / arena
 * frontmatter; not a full YAML parser).
 */
export function parseFrontmatter(content: string): {
  frontmatter: Record<string, string> | null;
  body: string;
} {
  if (!content.startsWith("---\n") && !content.startsWith("---\r\n")) {
    return { frontmatter: null, body: content };
  }
  const after = content.slice(content.indexOf("\n") + 1);
  const closeIdx = after.search(/^---\s*$/m);
  if (closeIdx === -1) return { frontmatter: null, body: content };

  const block = after.slice(0, closeIdx);
  const body = after.slice(closeIdx).replace(/^---\s*\n?/, "");

  const lines = block.split(/\r?\n/);
  const fm: Record<string, string> = {};
  let lastKey: string | null = null;
  for (const line of lines) {
    if (!line.trim()) continue;
    const colon = line.indexOf(":");
    const isKeyLine =
      colon > 0 &&
      !line.startsWith(" ") &&
      !line.startsWith("\t") &&
      /^[A-Za-z0-9_.-]{1,64}$/.test(line.slice(0, colon));
    if (isKeyLine) {
      const key = line.slice(0, colon);
      const value = line.slice(colon + 1).trim();
      fm[key] = stripWrappingQuotes(value);
      lastKey = key;
    } else if (lastKey) {
      const cont = line.trim();
      fm[lastKey] = fm[lastKey] ? `${fm[lastKey]} ${cont}` : cont;
    }
  }
  if (Object.keys(fm).length === 0) return { frontmatter: null, body: content };
  return { frontmatter: fm, body };
}

interface FrontmatterCardProps {
  readonly data: Record<string, string>;
}

/** Keys we promote out of the generic key/value list into hero positions. */
const TITLE_KEYS = ["name", "title"];
const SUBTITLE_KEYS = ["description", "summary"];

function pickFirst(
  data: Record<string, string>,
  keys: string[]
): { key: string; value: string } | undefined {
  for (const k of keys) {
    if (data[k]) return { key: k, value: data[k] };
  }
  return undefined;
}

function FrontmatterCard({ data }: FrontmatterCardProps) {
  const title = pickFirst(data, TITLE_KEYS);
  const subtitle = pickFirst(data, SUBTITLE_KEYS);
  const skipKeys = new Set(
    [title?.key, subtitle?.key].filter((k): k is string => Boolean(k))
  );
  const rest = Object.entries(data).filter(([k]) => !skipKeys.has(k));

  return (
    <div className="mb-8 rounded-lg border border-primary/20 bg-primary/5 px-8 py-7 shadow-sm">
      {title && (
        <h1 className="text-2xl font-semibold tracking-tight text-foreground">
          {title.value}
        </h1>
      )}
      {subtitle && (
        <p className="mt-2 text-base leading-relaxed text-muted-foreground">
          {subtitle.value}
        </p>
      )}
      {rest.length > 0 && (
        <dl className="mt-4 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-2 border-t border-primary/10 pt-4 text-sm">
          {rest.map(([key, value]) => (
            <div key={key} className="contents">
              <dt className="font-semibold text-foreground self-start">
                {key}
              </dt>
              <dd className="break-words text-muted-foreground">{value}</dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  );
}

/** Default file-type icons; consumers can override specific filenames. */
function defaultFileIcon(name: string): ReactNode {
  if (name.endsWith(".yaml") || name.endsWith(".yml") || name.endsWith(".json")) {
    return <FileCode className="h-4 w-4 text-yellow-600 flex-shrink-0" />;
  }
  if (isMarkdown(name)) {
    return <FileText className="h-4 w-4 text-gray-500 flex-shrink-0" />;
  }
  if (
    name.endsWith(".go") ||
    name.endsWith(".ts") ||
    name.endsWith(".js") ||
    name.endsWith(".py")
  ) {
    return <FileCode className="h-4 w-4 text-blue-500 flex-shrink-0" />;
  }
  return <File className="h-4 w-4 text-gray-400 flex-shrink-0" />;
}

interface TreeNodeProps {
  readonly node: ArenaSourceContentNode;
  readonly level: number;
  readonly selectedPath?: string;
  readonly expandedPaths: Set<string>;
  readonly onToggleExpand: (path: string) => void;
  readonly onSelectFile: (path: string, name: string) => void;
  readonly resolveIcon: (name: string) => ReactNode;
}

function TreeNode({
  node,
  level,
  selectedPath,
  expandedPaths,
  onToggleExpand,
  onSelectFile,
  resolveIcon,
}: TreeNodeProps) {
  const isExpanded = expandedPaths.has(node.path);
  const isSelected = selectedPath === node.path;
  const hasChildren = node.children && node.children.length > 0;
  const paddingLeft = `${level * 16 + 8}px`;

  const handleClick = () => {
    if (node.isDirectory) onToggleExpand(node.path);
    else onSelectFile(node.path, node.name);
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
            onClick={(e) => {
              e.stopPropagation();
              onToggleExpand(node.path);
            }}
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
        {!node.isDirectory && resolveIcon(node.name)}

        <span className="truncate text-sm">{node.name}</span>
      </div>

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
              resolveIcon={resolveIcon}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface FileViewerProps {
  readonly selectedName: string | null;
  readonly selectedPath: string | null;
  readonly fileContent: string;
  readonly fileLoading: boolean;
  readonly fileError: string | null;
}

function MarkdownPreviewPane({
  fileContent,
  fileLoading,
}: {
  fileContent: string;
  fileLoading: boolean;
}) {
  if (fileLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }
  const { frontmatter, body } = parseFrontmatter(fileContent);
  return (
    <div className="p-4">
      {frontmatter && <FrontmatterCard data={frontmatter} />}
      <Markdown content={body} />
    </div>
  );
}

function MarkdownFileViewer({
  fileContent,
  fileLoading,
}: {
  fileContent: string;
  fileLoading: boolean;
}) {
  const [view, setView] = useState<"preview" | "source">("preview");

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-1 px-3 py-1.5 border-b bg-muted/30">
        <Button
          size="sm"
          variant={view === "preview" ? "secondary" : "ghost"}
          className="h-7 px-2 text-xs"
          onClick={() => setView("preview")}
        >
          Preview
        </Button>
        <Button
          size="sm"
          variant={view === "source" ? "secondary" : "ghost"}
          className="h-7 px-2 text-xs"
          onClick={() => setView("source")}
        >
          Source
        </Button>
      </div>
      <div className="flex-1 min-h-0 overflow-auto">
        {view === "preview" ? (
          <MarkdownPreviewPane fileContent={fileContent} fileLoading={fileLoading} />
        ) : (
          <YamlEditor
            value={fileContent}
            readOnly
            language="markdown"
            loading={fileLoading}
          />
        )}
      </div>
    </div>
  );
}

function FileViewer({
  selectedName,
  selectedPath,
  fileContent,
  fileLoading,
  fileError,
}: FileViewerProps) {
  if (fileError) {
    return (
      <div className="flex items-center justify-center h-full text-destructive">
        <p className="text-sm">{fileError}</p>
      </div>
    );
  }
  if (!selectedPath) return <YamlEditorEmptyState />;

  if (selectedName && isMarkdown(selectedName)) {
    return <MarkdownFileViewer fileContent={fileContent} fileLoading={fileLoading} />;
  }

  return (
    <YamlEditor
      value={fileContent}
      readOnly
      language={getLanguageForFile(selectedName || "")}
      loading={fileLoading}
    />
  );
}

/**
 * Read-only file browser: tree on the left, content on the right.
 * For .md / .markdown files, renders a Preview/Source toggle and
 * defaults to a rendered preview using the shared Markdown component.
 *
 * Consumers supply `tree` (typically from a domain-specific hook) and
 * `loadFile` (which should hit the appropriate API endpoint).
 */
export function ContentExplorer({
  tree,
  fileCount,
  loading,
  error,
  loadFile,
  rootPath,
  defaultFile,
  getFileIcon,
  labels,
}: ContentExplorerProps) {
  const resolveIcon = useCallback(
    (name: string): ReactNode =>
      getFileIcon?.(name) ?? defaultFileIcon(name),
    [getFileIcon]
  );

  // Scope tree to rootPath if provided.
  const scopedTree = useMemo(() => {
    if (!rootPath || !tree.length) return tree;
    const parts = rootPath.split("/").filter(Boolean);
    let current = tree;
    for (const part of parts) {
      const found = current.find((n) => n.name === part && n.isDirectory);
      if (!found?.children) return tree;
      current = found.children;
    }
    return current;
  }, [tree, rootPath]);

  // Initial expansion path for defaultFile.
  const initialExpanded = useMemo(() => {
    if (!defaultFile) return new Set<string>();
    const fullPath = rootPath ? `${rootPath}/${defaultFile}` : defaultFile;
    const parts = fullPath.split("/");
    const paths = new Set<string>();
    for (let i = 1; i < parts.length; i++) {
      paths.add(parts.slice(0, i).join("/"));
    }
    return paths;
  }, [rootPath, defaultFile]);

  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(initialExpanded);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [hasAutoSelected, setHasAutoSelected] = useState(false);
  const [fileContent, setFileContent] = useState<string>("");
  const [fileLoading, setFileLoading] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

  const handleToggleExpand = useCallback((path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  }, []);

  const handleSelectFile = useCallback(
    async (path: string, name: string) => {
      setSelectedPath(path);
      setSelectedName(name);
      setFileLoading(true);
      setFileError(null);
      try {
        const result = await loadFile(path);
        setFileContent(result.content);
      } catch (err) {
        setFileError(err instanceof Error ? err.message : "Failed to load file");
        setFileContent("");
      } finally {
        setFileLoading(false);
      }
    },
    [loadFile]
  );

  // Auto-select default file once the tree is available.
  useEffect(() => {
    if (hasAutoSelected || !defaultFile || !scopedTree.length) return;
    const fullPath = rootPath ? `${rootPath}/${defaultFile}` : defaultFile;
    const fileName = defaultFile.split("/").pop() || defaultFile;
    setHasAutoSelected(true);
    handleSelectFile(fullPath, fileName);
  }, [hasAutoSelected, defaultFile, rootPath, scopedTree, handleSelectFile]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        <span className="ml-2 text-muted-foreground">
          {labels?.loading ?? "Loading content..."}
        </span>
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
        <p className="text-lg font-medium mb-1">
          {labels?.emptyTitle ?? "No content available"}
        </p>
        <p className="text-sm">
          {labels?.emptyDescription ??
            "Content will appear here once it has been synced."}
        </p>
      </div>
    );
  }

  return (
    <ResizablePanelGroup orientation="horizontal" className="h-full rounded-lg border">
      <ResizablePanel defaultSize={25} minSize={15} maxSize={40}>
        <div className="h-full overflow-auto">
          <div className="p-2 border-b bg-muted/30">
            <h3 className="text-sm font-medium truncate">
              Files
              {rootPath && (
                <span className="text-xs text-muted-foreground ml-1">/{rootPath}</span>
              )}
              <span className="text-xs text-muted-foreground ml-2">({fileCount})</span>
            </h3>
          </div>
          <div className="py-1">
            {scopedTree.map((node) => (
              <TreeNode
                key={node.path}
                node={node}
                level={0}
                selectedPath={selectedPath || undefined}
                expandedPaths={expandedPaths}
                onToggleExpand={handleToggleExpand}
                onSelectFile={handleSelectFile}
                resolveIcon={resolveIcon}
              />
            ))}
          </div>
        </div>
      </ResizablePanel>

      <ResizableHandle withHandle />

      <ResizablePanel defaultSize={75}>
        <div className="flex flex-col h-full">
          {selectedName && (
            <div className="px-3 py-1.5 border-b bg-muted/30 text-xs text-muted-foreground font-mono truncate">
              {selectedPath}
            </div>
          )}
          <div className="flex-1 min-h-0">
            <FileViewer
              key={selectedPath ?? "__empty__"}
              selectedName={selectedName}
              selectedPath={selectedPath}
              fileContent={fileContent}
              fileLoading={fileLoading}
              fileError={fileError}
            />
          </div>
        </div>
      </ResizablePanel>
    </ResizablePanelGroup>
  );
}
