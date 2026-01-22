/**
 * API route for getting Arena pack content from an Arena config.
 *
 * GET /api/workspaces/:name/arena/configs/:configName/content - Get arena content
 *
 * Returns all files from the config's source with their detected types based on
 * the YAML `kind` field. Files are organized into a tree structure for navigation.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  getWorkspaceResource,
  handleK8sError,
  CRD_ARENA_CONFIGS,
  CRD_ARENA_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import { getConfigMapContent } from "@/lib/k8s/crd-operations";
import { gunzipSync } from "zlib";
import * as tar from "tar-stream";
import * as fs from "fs";
import * as path from "path";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type {
  ArenaConfig,
  ArenaSource,
  ArenaConfigContent,
  ArenaPackageFile,
  ArenaPackageTreeNode,
  ParsedPromptConfig,
  ParsedProviderConfig,
  ParsedScenario,
  ParsedTool,
  ArenaMcpServer,
  ArenaJudge,
  ArenaJudgeDefaults,
  ArenaSelfPlayConfig,
  ArenaDefaultsConfig,
} from "@/types/arena";

type RouteParams = { name: string; configName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaConfig";

// =============================================================================
// Raw YAML structures for Arena pack files
// =============================================================================

interface RawArenaYaml {
  apiVersion?: string;
  kind?: string;
  metadata?: { name?: string; namespace?: string };
  spec?: {
    prompt_configs?: Array<{ id: string; file: string; vars?: Record<string, string> }>;
    providers?: Array<{ file: string; group?: string }>;
    scenarios?: Array<{ file: string }>;
    tools?: Array<{ file: string }>;
    mcp_servers?: Record<string, { command: string; args?: string[]; env?: Record<string, string> }>;
    judges?: Record<string, { provider: string }>;
    judge_defaults?: { prompt?: string; registry_path?: string };
    self_play?: { enabled?: boolean; persona?: string; provider?: string };
    defaults?: {
      temperature?: number;
      top_p?: number;
      max_tokens?: number;
      seed?: number;
      concurrency?: number;
      timeout?: string;
      max_retries?: number;
      output?: { dir?: string; formats?: string[] };
      session?: { enabled?: boolean; dir?: string };
      fail_on?: string[];
      state?: { enabled?: boolean; max_history_turns?: number; persistence?: string; redis_url?: string };
    };
  };
}

interface RawPromptYaml {
  apiVersion?: string;
  kind?: string;
  metadata?: { name?: string };
  spec?: {
    id?: string;
    description?: string;
    task_type?: string;
    version?: string;
    variables?: Array<{
      name: string;
      type?: string;
      required?: boolean;
      default?: string;
      description?: string;
    }>;
    system_template?: string;
    allowed_tools?: string[];
    validators?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
}

interface RawProviderYaml {
  apiVersion?: string;
  kind?: string;
  metadata?: { name?: string };
  spec?: {
    id?: string;
    type?: string;
    model?: string;
    pricing?: { input_per_1k_tokens?: number; output_per_1k_tokens?: number };
    defaults?: { temperature?: number; max_tokens?: number; top_p?: number };
  };
}

interface RawScenarioYaml {
  apiVersion?: string;
  kind?: string;
  metadata?: { name?: string };
  spec?: {
    id?: string;
    description?: string;
    task_type?: string;
    turns?: Array<{ role: string; content: string }>;
    tags?: string[];
  };
}

interface RawToolYaml {
  apiVersion?: string;
  kind?: string;
  metadata?: { name?: string };
  spec?: {
    description?: string;
    input_schema?: Record<string, unknown>;
    output_schema?: Record<string, unknown>;
    config?: { mode?: string; timeout?: number; mock_result?: unknown };
  };
}

// =============================================================================
// Content fetching from filesystem or artifact
// =============================================================================

/** Base path for workspace content volume */
const WORKSPACE_CONTENT_BASE = "/workspace-content";

/** Read all files from the filesystem recursively */
async function readFilesystemContent(
  workspaceName: string,
  namespace: string,
  contentPath: string
): Promise<Record<string, string> | null> {
  // Build the full path: /workspace-content/{workspace}/{namespace}/{contentPath}
  const basePath = path.join(WORKSPACE_CONTENT_BASE, workspaceName, namespace, contentPath);

  try {
    if (!fs.existsSync(basePath)) {
      console.error(`Filesystem content path not found: ${basePath}`);
      return null;
    }

    const files: Record<string, string> = {};

    const readDir = (dir: string, prefix: string = "") => {
      const entries = fs.readdirSync(dir, { withFileTypes: true });
      for (const entry of entries) {
        // Skip hidden files and directories (like .arena metadata)
        if (entry.name.startsWith(".")) continue;

        const fullPath = path.join(dir, entry.name);
        const relativePath = prefix ? `${prefix}/${entry.name}` : entry.name;

        if (entry.isDirectory()) {
          readDir(fullPath, relativePath);
        } else if (entry.isFile()) {
          try {
            const content = fs.readFileSync(fullPath, "utf-8");
            files[relativePath] = content;
          } catch (err) {
            console.error(`Error reading file ${fullPath}:`, err);
          }
        }
      }
    };

    readDir(basePath);
    return files;
  } catch (err) {
    console.error(`Error reading filesystem content at ${basePath}:`, err);
    return null;
  }
}

/** Fetch and extract tar.gz artifact from URL (fallback for legacy sources) */
async function fetchArtifactContent(url: string): Promise<Record<string, string> | null> {
  try {
    // Rewrite localhost URLs to use internal Kubernetes service
    // eslint-disable-next-line sonarjs/no-clear-text-protocols -- internal K8s service communication
    const K8S_SERVICE_URL = "http://omnia-controller-manager.omnia-system:8082";
    let fetchUrl = url;
    if (url.includes("localhost:8082")) {
      fetchUrl = url.replace("http://localhost:8082", K8S_SERVICE_URL);
    }
    const response = await fetch(fetchUrl);
    if (!response.ok) {
      console.error(`Failed to fetch artifact: ${response.status} ${response.statusText}`);
      return null;
    }

    const buffer = Buffer.from(await response.arrayBuffer());

    // Decompress gzip
    const decompressed = gunzipSync(buffer);

    // Extract tar
    const files: Record<string, string> = {};
    const extract = tar.extract();

    return new Promise((resolve) => {
      extract.on("entry", (header, stream, next) => {
        const chunks: Buffer[] = [];

        stream.on("data", (chunk: Buffer) => {
          chunks.push(chunk);
        });

        stream.on("end", () => {
          if (header.type === "file" && header.name) {
            const content = Buffer.concat(chunks).toString("utf-8");
            // Remove leading ./ or / from path
            const cleanPath = header.name.replace(/^\.?\//, "");
            if (cleanPath && !cleanPath.endsWith("/")) {
              files[cleanPath] = content;
            }
          }
          next();
        });

        stream.resume();
      });

      extract.on("finish", () => {
        resolve(files);
      });

      extract.on("error", (err) => {
        console.error("Tar extraction error:", err);
        resolve(null);
      });

      extract.end(decompressed);
    });
  } catch (err) {
    console.error("Error fetching artifact:", err);
    return null;
  }
}

// =============================================================================
// Parsing functions
// =============================================================================

async function parseYamlAsync<T>(content: string): Promise<T | null> {
  try {
    const yaml = await import("js-yaml");
    return yaml.load(content) as T;
  } catch {
    return null;
  }
}

function truncateText(text: string | undefined, maxLength: number): string | undefined {
  if (!text) return undefined;
  if (text.length <= maxLength) return text;
  return text.substring(0, maxLength) + "...";
}

/** Detect file type by parsing YAML content and checking kind field */
async function detectFileType(content: string): Promise<ArenaPackageFile["type"]> {
  try {
    const yaml = await import("js-yaml");
    const parsed = yaml.load(content) as { kind?: string; apiVersion?: string } | null;

    if (!parsed?.kind) return "other";

    // Map PromptKit kinds to file types
    const kindMap: Record<string, ArenaPackageFile["type"]> = {
      "Arena": "arena",
      "PromptConfig": "prompt",
      "Provider": "provider",
      "Scenario": "scenario",
      "Tool": "tool",
      "Persona": "persona",
    };

    return kindMap[parsed.kind] || "other";
  } catch {
    return "other";
  }
}

async function buildPackageFiles(filesData: Record<string, string>): Promise<ArenaPackageFile[]> {
  const files: ArenaPackageFile[] = [];

  for (const [path, content] of Object.entries(filesData)) {
    const type = await detectFileType(content);
    files.push({
      path,
      type,
      size: content.length,
      // Note: content is NOT included here - fetch via separate endpoint
    });
  }

  return files.sort((a, b) => a.path.localeCompare(b.path));
}

// eslint-disable-next-line sonarjs/cognitive-complexity -- tree building requires nested loops
function buildFileTree(files: ArenaPackageFile[]): ArenaPackageTreeNode[] {
  const root: ArenaPackageTreeNode[] = [];
  const nodeMap = new Map<string, ArenaPackageTreeNode>();

  // Sort files to ensure directories are processed first
  const sortedPaths = files.map(f => f.path).sort();

  for (const filePath of sortedPaths) {
    const parts = filePath.split("/");
    let currentPath = "";

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const parentPath = currentPath;
      currentPath = currentPath ? `${currentPath}/${part}` : part;
      const isFile = i === parts.length - 1;

      if (!nodeMap.has(currentPath)) {
        const file = files.find(f => f.path === currentPath);
        const node: ArenaPackageTreeNode = {
          name: part,
          path: currentPath,
          isDirectory: !isFile,
          type: isFile ? file?.type : undefined,
          children: isFile ? undefined : [],
        };

        nodeMap.set(currentPath, node);

        if (parentPath) {
          const parent = nodeMap.get(parentPath);
          if (parent?.children) {
            parent.children.push(node);
          }
        } else {
          root.push(node);
        }
      }
    }
  }

  // Sort children: directories first, then files, alphabetically
  const sortChildren = (nodes: ArenaPackageTreeNode[]) => {
    nodes.sort((a, b) => {
      if (a.isDirectory !== b.isDirectory) {
        return a.isDirectory ? -1 : 1;
      }
      return a.name.localeCompare(b.name);
    });
    for (const node of nodes) {
      if (node.children) {
        sortChildren(node.children);
      }
    }
  };
  sortChildren(root);

  return root;
}

// =============================================================================
// Content parsing from detected file types
// =============================================================================

function parsePromptConfig(
  raw: RawPromptYaml,
  file: string,
  vars?: Record<string, string>
): ParsedPromptConfig {
  const spec = raw.spec || {};
  return {
    id: spec.id || raw.metadata?.name || file,
    name: raw.metadata?.name || spec.id || file,
    version: spec.version,
    description: spec.description,
    taskType: spec.task_type,
    systemTemplate: truncateText(spec.system_template, 500),
    variables: spec.variables?.map((v) => ({
      name: v.name,
      type: v.type || "string",
      required: v.required,
      default: vars?.[v.name] || v.default,
      description: v.description,
    })),
    allowedTools: spec.allowed_tools,
    validators: spec.validators?.map((v) => ({
      type: v.type,
      config: v.config,
    })),
    file,
  };
}

function parseProviderConfig(raw: RawProviderYaml, file: string, group?: string): ParsedProviderConfig {
  const spec = raw.spec || {};
  return {
    id: spec.id || raw.metadata?.name || file,
    name: raw.metadata?.name || spec.id || file,
    type: spec.type || "unknown",
    model: spec.model || "unknown",
    group,
    pricing: spec.pricing
      ? {
          inputPer1kTokens: spec.pricing.input_per_1k_tokens,
          outputPer1kTokens: spec.pricing.output_per_1k_tokens,
        }
      : undefined,
    defaults: spec.defaults
      ? {
          temperature: spec.defaults.temperature,
          maxTokens: spec.defaults.max_tokens,
          topP: spec.defaults.top_p,
        }
      : undefined,
    file,
  };
}

function parseScenario(raw: RawScenarioYaml, file: string): ParsedScenario {
  const spec = raw.spec || {};
  return {
    id: spec.id || raw.metadata?.name || file,
    name: raw.metadata?.name || spec.id || file,
    description: spec.description,
    taskType: spec.task_type,
    turns: spec.turns?.map((t) => ({
      role: t.role as "user" | "assistant",
      content: t.content,
    })),
    turnCount: spec.turns?.length,
    tags: spec.tags,
    file,
  };
}

function parseTool(raw: RawToolYaml, file: string): ParsedTool {
  const spec = raw.spec || {};
  return {
    name: raw.metadata?.name || file,
    description: spec.description || "",
    mode: spec.config?.mode,
    timeout: spec.config?.timeout,
    inputSchema: spec.input_schema,
    outputSchema: spec.output_schema,
    hasMockData: spec.config?.mock_result !== undefined,
    file,
  };
}

function parseDefaults(raw: RawArenaYaml["spec"]): ArenaDefaultsConfig | undefined {
  const defaults = raw?.defaults;
  if (!defaults) return undefined;

  return {
    temperature: defaults.temperature,
    topP: defaults.top_p,
    maxTokens: defaults.max_tokens,
    seed: defaults.seed,
    concurrency: defaults.concurrency,
    timeout: defaults.timeout,
    maxRetries: defaults.max_retries,
    output: defaults.output
      ? { dir: defaults.output.dir, formats: defaults.output.formats }
      : undefined,
    session: defaults.session
      ? { enabled: defaults.session.enabled, dir: defaults.session.dir }
      : undefined,
    failOn: defaults.fail_on,
    state: defaults.state
      ? {
          enabled: defaults.state.enabled,
          maxHistoryTurns: defaults.state.max_history_turns,
          persistence: defaults.state.persistence,
          redisUrl: defaults.state.redis_url,
        }
      : undefined,
  };
}

function createEmptyContent(name: string): ArenaConfigContent {
  return {
    metadata: { name },
    files: [],
    fileTree: [],
    promptConfigs: [],
    providers: [],
    scenarios: [],
    tools: [],
    mcpServers: {},
    judges: {},
  };
}

// =============================================================================
// Parse all files by their detected type
// =============================================================================

interface ArenaConfigParsed {
  mcpServers: Record<string, ArenaMcpServer>;
  judges: Record<string, ArenaJudge>;
  judgeDefaults?: ArenaJudgeDefaults;
  selfPlay?: ArenaSelfPlayConfig;
  defaults?: ArenaDefaultsConfig;
  arenaMetadata?: { name?: string; namespace?: string };
}

/** Parse arena config file and extract its contents */
function parseArenaConfig(parsed: RawArenaYaml): ArenaConfigParsed {
  const result: ArenaConfigParsed = {
    mcpServers: {},
    judges: {},
    arenaMetadata: parsed.metadata,
    defaults: parseDefaults(parsed.spec),
  };

  if (!parsed.spec) return result;

  // Parse MCP servers
  if (parsed.spec.mcp_servers) {
    for (const [serverName, server] of Object.entries(parsed.spec.mcp_servers)) {
      result.mcpServers[serverName] = {
        command: server.command,
        args: server.args,
        env: server.env,
      };
    }
  }

  // Parse judges
  if (parsed.spec.judges) {
    for (const [judgeName, judge] of Object.entries(parsed.spec.judges)) {
      result.judges[judgeName] = { provider: judge.provider };
    }
  }

  // Parse judge defaults
  if (parsed.spec.judge_defaults) {
    result.judgeDefaults = {
      prompt: parsed.spec.judge_defaults.prompt,
      registryPath: parsed.spec.judge_defaults.registry_path,
    };
  }

  // Parse self-play config
  if (parsed.spec.self_play) {
    result.selfPlay = {
      enabled: parsed.spec.self_play.enabled || false,
      persona: parsed.spec.self_play.persona,
      provider: parsed.spec.self_play.provider,
    };
  }

  return result;
}

// eslint-disable-next-line sonarjs/cognitive-complexity -- switch on file types with parsing
async function parseAllFiles(
  files: ArenaPackageFile[],
  filesData: Record<string, string>
): Promise<{
  promptConfigs: ParsedPromptConfig[];
  providers: ParsedProviderConfig[];
  scenarios: ParsedScenario[];
  tools: ParsedTool[];
  mcpServers: Record<string, ArenaMcpServer>;
  judges: Record<string, ArenaJudge>;
  judgeDefaults?: ArenaJudgeDefaults;
  selfPlay?: ArenaSelfPlayConfig;
  defaults?: ArenaDefaultsConfig;
  arenaMetadata?: { name?: string; namespace?: string };
}> {
  const promptConfigs: ParsedPromptConfig[] = [];
  const providers: ParsedProviderConfig[] = [];
  const scenarios: ParsedScenario[] = [];
  const tools: ParsedTool[] = [];
  let arenaConfig: ArenaConfigParsed | undefined;

  for (const file of files) {
    const content = filesData[file.path];
    if (!content) continue;

    switch (file.type) {
      case "arena": {
        const parsed = await parseYamlAsync<RawArenaYaml>(content);
        if (parsed?.spec) {
          arenaConfig = parseArenaConfig(parsed);
        }
        break;
      }
      case "prompt": {
        const parsed = await parseYamlAsync<RawPromptYaml>(content);
        if (parsed) {
          promptConfigs.push(parsePromptConfig(parsed, file.path));
        }
        break;
      }
      case "provider": {
        const parsed = await parseYamlAsync<RawProviderYaml>(content);
        if (parsed) {
          providers.push(parseProviderConfig(parsed, file.path));
        }
        break;
      }
      case "scenario": {
        const parsed = await parseYamlAsync<RawScenarioYaml>(content);
        if (parsed) {
          scenarios.push(parseScenario(parsed, file.path));
        }
        break;
      }
      case "tool": {
        const parsed = await parseYamlAsync<RawToolYaml>(content);
        if (parsed) {
          tools.push(parseTool(parsed, file.path));
        }
        break;
      }
    }
  }

  return {
    promptConfigs,
    providers,
    scenarios,
    tools,
    mcpServers: arenaConfig?.mcpServers ?? {},
    judges: arenaConfig?.judges ?? {},
    judgeDefaults: arenaConfig?.judgeDefaults,
    selfPlay: arenaConfig?.selfPlay,
    defaults: arenaConfig?.defaults,
    arenaMetadata: arenaConfig?.arenaMetadata,
  };
}

// =============================================================================
// Route handler
// =============================================================================

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, configName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<ArenaConfig>(
        name,
        access.role!,
        CRD_ARENA_CONFIGS,
        configName,
        "Arena config"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      // Get the source reference from the config
      const sourceRef = result.resource.spec?.sourceRef?.name;
      if (!sourceRef) {
        auditSuccess(auditCtx, "get", configName, { subresource: "content" });
        return NextResponse.json(createEmptyContent("No source"));
      }

      // Fetch the ArenaSource to get the ConfigMap reference
      const sourceResult = await getWorkspaceResource<ArenaSource>(
        name,
        access.role!,
        CRD_ARENA_SOURCES,
        sourceRef,
        "Arena source"
      );
      if (!sourceResult.ok) {
        auditSuccess(auditCtx, "get", configName, { subresource: "content" });
        return NextResponse.json(createEmptyContent("Source not found"));
      }

      // Try to get content from shared filesystem first (preferred)
      // Fall back to artifact URL, then ConfigMap for legacy sources
      let packageFiles: Record<string, string> | null = null;

      const artifact = sourceResult.resource.status?.artifact;
      const namespace = result.workspace.spec.namespace.name;

      // Priority 1: Filesystem content (new shared workspace volume)
      if (artifact?.contentPath) {
        packageFiles = await readFilesystemContent(name, namespace, artifact.contentPath);
      }

      // Priority 2: Artifact URL (legacy tar.gz serving)
      if (!packageFiles && artifact?.url) {
        packageFiles = await fetchArtifactContent(artifact.url);
      }

      // Priority 3: ConfigMap (legacy local sources)
      if (!packageFiles) {
        const configMapName = sourceResult.resource.spec?.configMap?.name;
        if (configMapName) {
          packageFiles = await getConfigMapContent(sourceResult.clientOptions, configMapName);
        }
      }

      if (!packageFiles || Object.keys(packageFiles).length === 0) {
        auditSuccess(auditCtx, "get", configName, { subresource: "content" });
        return NextResponse.json(createEmptyContent("No content available"));
      }

      // Build file list with detected types and tree structure
      const files = await buildPackageFiles(packageFiles);
      const fileTree = buildFileTree(files);

      // Find the entry point (arena config file)
      const entryPoint = files.find(f => f.type === "arena")?.path;

      // Parse all files based on their detected types
      const parsed = await parseAllFiles(files, packageFiles);

      // Build the response
      const content: ArenaConfigContent = {
        metadata: {
          name: parsed.arenaMetadata?.name || configName,
          namespace: parsed.arenaMetadata?.namespace,
        },
        files,
        fileTree,
        entryPoint,
        promptConfigs: parsed.promptConfigs,
        providers: parsed.providers,
        scenarios: parsed.scenarios,
        tools: parsed.tools,
        mcpServers: parsed.mcpServers,
        judges: parsed.judges,
        judgeDefaults: parsed.judgeDefaults,
        selfPlay: parsed.selfPlay,
        defaults: parsed.defaults,
      };

      auditSuccess(auditCtx, "get", configName, {
        subresource: "content",
        fileCount: files.length,
        promptCount: content.promptConfigs.length,
        providerCount: content.providers.length,
        scenarioCount: content.scenarios.length,
        toolCount: content.tools.length,
      });
      return NextResponse.json(content);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", configName, error, 500);
      }
      return handleK8sError(error, "get content for this arena config");
    }
  }
);
