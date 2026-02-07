/**
 * Arena Project Editor types for the project-based YAML editing workflow.
 *
 * Projects are stored in workspace volumes at:
 * /workspace-content/{workspace}/{namespace}/arena/projects/{project-id}/
 */

// =============================================================================
// Project Types
// =============================================================================

/** Project metadata stored in config.arena.yaml */
export interface ArenaProject {
  /** Unique project identifier (directory name) */
  id: string;
  /** Human-readable project name */
  name: string;
  /** Project description */
  description?: string;
  /** Creation timestamp */
  createdAt: string;
  /** Last modification timestamp */
  updatedAt: string;
  /** Project tags for organization */
  tags?: string[];
}

/** Project with file tree structure for editor */
export interface ArenaProjectWithTree extends ArenaProject {
  /** File tree structure */
  tree: FileTreeNode[];
}

// =============================================================================
// File Tree Types
// =============================================================================

/** Provider binding annotation data extracted from YAML files */
export interface ProviderBinding {
  providerName?: string;
  providerNamespace?: string;
}

/** A node in the file tree (file or directory) */
export interface FileTreeNode {
  /** Node name (file or directory name) */
  name: string;
  /** Full path relative to project root */
  path: string;
  /** Whether this is a directory */
  isDirectory: boolean;
  /** Children nodes (for directories only) */
  children?: FileTreeNode[];
  /** File size in bytes (for files only) */
  size?: number;
  /** Last modification time */
  modifiedAt?: string;
  /** File type based on extension or YAML kind */
  type?: FileType;
  /** Provider binding annotations (for provider files only) */
  providerBinding?: ProviderBinding;
}

/** File type classification for icons and behavior */
export type FileType =
  | "arena"
  | "prompt"
  | "provider"
  | "scenario"
  | "tool"
  | "persona"
  | "yaml"
  | "json"
  | "markdown"
  | "other";

// =============================================================================
// Open File Types (Editor State)
// =============================================================================

/** Represents a file open in the editor */
export interface OpenFile {
  /** File path relative to project root */
  path: string;
  /** File name for display */
  name: string;
  /** Current content in editor */
  content: string;
  /** Original content when loaded (for dirty detection) */
  originalContent: string;
  /** Whether the file has unsaved changes */
  isDirty: boolean;
  /** File type for syntax highlighting */
  type: FileType;
  /** Loading state */
  loading?: boolean;
  /** Error state */
  error?: string | null;
}

// =============================================================================
// API Response Types
// =============================================================================

/** Response from listing projects */
export interface ProjectListResponse {
  projects: ArenaProject[];
}

/** Response from getting a project with its tree */
export type ProjectGetResponse = ArenaProjectWithTree;

/** Request body for creating a project */
export interface ProjectCreateRequest {
  name: string;
  description?: string;
  tags?: string[];
}

/** Response from creating a project */
export type ProjectCreateResponse = ArenaProject;

/** Response from getting file content */
export interface FileContentResponse {
  path: string;
  content: string;
  size: number;
  modifiedAt: string;
  encoding: "utf-8" | "base64";
}

/** Request body for updating file content */
export interface FileUpdateRequest {
  content: string;
}

/** Response from updating file content */
export interface FileUpdateResponse {
  path: string;
  size: number;
  modifiedAt: string;
}

/** Request body for creating a file or directory */
export interface FileCreateRequest {
  name: string;
  isDirectory: boolean;
  content?: string;
}

/** Response from creating a file or directory */
export interface FileCreateResponse {
  path: string;
  name: string;
  isDirectory: boolean;
  size?: number;
  modifiedAt: string;
}

/** Request body for renaming a file or directory */
export interface FileRenameRequest {
  newName: string;
}

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Determine file type from path/extension
 */
export function getFileType(filePath: string): FileType {
  const name = filePath.toLowerCase();

  // Check for Arena-specific file types
  if (name.endsWith(".arena.yaml") || name.endsWith(".arena.yml")) {
    return "arena";
  }
  if (name.endsWith(".prompt.yaml") || name.endsWith(".prompt.yml")) {
    return "prompt";
  }
  if (name.endsWith(".provider.yaml") || name.endsWith(".provider.yml")) {
    return "provider";
  }
  if (name.endsWith(".scenario.yaml") || name.endsWith(".scenario.yml")) {
    return "scenario";
  }
  if (name.endsWith(".tool.yaml") || name.endsWith(".tool.yml")) {
    return "tool";
  }
  if (name.endsWith(".persona.yaml") || name.endsWith(".persona.yml")) {
    return "persona";
  }

  // Check for general file types
  if (name.endsWith(".yaml") || name.endsWith(".yml")) {
    return "yaml";
  }
  if (name.endsWith(".json")) {
    return "json";
  }
  if (name.endsWith(".md") || name.endsWith(".markdown")) {
    return "markdown";
  }

  return "other";
}

/**
 * Get display label for file type
 */
export function getFileTypeLabel(type: FileType): string {
  const labels: Record<FileType, string> = {
    arena: "Arena Config",
    prompt: "Prompt",
    provider: "Provider",
    scenario: "Scenario",
    tool: "Tool",
    persona: "Persona",
    yaml: "YAML",
    json: "JSON",
    markdown: "Markdown",
    other: "File",
  };
  return labels[type];
}
