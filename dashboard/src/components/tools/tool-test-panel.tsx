"use client";

import { useState, useCallback, useMemo } from "react";
import {
  Play,
  CheckCircle,
  XCircle,
  Loader2,
  Clock,
  ShieldCheck,
  ShieldAlert,
  AlertTriangle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { JsonEditor } from "@/components/editors/json-editor";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { SchemaForm, isRenderableObjectSchema } from "@/components/tools/schema-form";
import { computeToolLints } from "@/lib/tools/openapi-lints";
import { useOpenAPIToolPreview } from "@/hooks/use-openapi-tool-preview";
import type {
  ToolRegistry,
  HandlerDefinition,
  DiscoveredTool,
  OpenAPIToolPreviewItem,
} from "@/types";

interface SchemaCheck {
  valid: boolean;
  errors?: string[];
}

interface ValidationResult {
  request?: SchemaCheck;
  response?: SchemaCheck;
}

interface ToolTestResult {
  success: boolean;
  result?: unknown;
  error?: string;
  durationMs: number;
  handlerType: string;
  validation?: ValidationResult;
}

interface ToolTestPanelProps {
  registry: ToolRegistry;
  workspaceName: string;
}

/** A tool-like value with an optional input schema (discovered or live preview). */
type SchemaTool = { inputSchema?: unknown } | null | undefined;

/**
 * Get the available tools for a handler from discovered tools.
 */
function getToolsForHandler(
  handler: HandlerDefinition,
  discoveredTools: DiscoveredTool[]
): DiscoveredTool[] {
  return discoveredTools.filter((t) => t.handlerName === handler.name);
}

/**
 * Get a default tool name for a handler.
 * For HTTP/gRPC handlers with an inline tool, use that name.
 * For self-describing handlers (MCP/OpenAPI), use the first discovered tool.
 */
function getDefaultToolName(
  handler: HandlerDefinition,
  discoveredTools: DiscoveredTool[]
): string {
  if (handler.tool?.name) return handler.tool.name;
  const tools = getToolsForHandler(handler, discoveredTools);
  return tools.length > 0 ? tools[0].name : "";
}

/** Type-zero value for a JSON Schema property type. */
function typeZeroValue(type: unknown): unknown {
  if (type === "string") return "";
  if (type === "number" || type === "integer") return 0;
  if (type === "boolean") return false;
  if (type === "array") return [];
  if (type === "object") return {};
  return null;
}

/**
 * Sample value for one property: prefer an authored `example`, then
 * `examples[0]`, then `default`, falling back to the type-zero value. This
 * lets a function/tool with a documented example seed a runnable input.
 */
function sampleValueForProperty(def: Record<string, unknown> | undefined): unknown {
  if (def?.example !== undefined) return def.example;
  const examples = def?.examples;
  if (Array.isArray(examples) && examples.length > 0) return examples[0];
  if (def?.default !== undefined) return def.default;
  return typeZeroValue(def?.type);
}

/**
 * Get a sample arguments JSON from a tool's input schema.
 */
export function getSampleArgs(tool?: SchemaTool, handler?: HandlerDefinition): string {
  let schema = tool?.inputSchema ?? handler?.tool?.inputSchema;
  if (!schema) return "{}";

  // Handle double-encoded JSON strings (schema stored as string instead of object)
  if (typeof schema === "string") {
    try {
      schema = JSON.parse(schema);
    } catch {
      return "{}";
    }
  }

  if (typeof schema !== "object") return "{}";

  const props = (schema as Record<string, unknown>).properties;
  if (!props || typeof props !== "object") return "{}";

  const sample: Record<string, unknown> = {};
  for (const [key, def] of Object.entries(props as Record<string, Record<string, unknown>>)) {
    sample[key] = sampleValueForProperty(def);
  }
  return JSON.stringify(sample, null, 2);
}

function ValidationBadge({ label, check }: Readonly<{ label: string; check: SchemaCheck }>) {
  return (
    <div className="flex items-start gap-2">
      {check.valid ? (
        <ShieldCheck className="h-4 w-4 text-green-500 mt-0.5 shrink-0" />
      ) : (
        <ShieldAlert className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" />
      )}
      <div className="min-w-0">
        <span className="text-sm font-medium">
          {label}:{" "}
          {check.valid ? (
            <span className="text-green-600 dark:text-green-400">Valid</span>
          ) : (
            <span className="text-amber-600 dark:text-amber-400">Invalid</span>
          )}
        </span>
        {check.errors && check.errors.length > 0 && (
          <ul className="mt-1 space-y-0.5">
            {check.errors.map((err) => (
              <li key={err} className="text-xs text-muted-foreground font-mono break-all">
                {err}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

/** Renders the result card (shared by the discovered-tools and OpenAPI flows). */
function ResultCard({ result }: Readonly<{ result: ToolTestResult }>) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          {result.success ? (
            <CheckCircle className="h-4 w-4 text-green-500" />
          ) : (
            <XCircle className="h-4 w-4 text-red-500" />
          )}
          {result.success ? "Success" : "Failed"}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Metadata */}
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <Clock className="h-3 w-3" />
            {result.durationMs}ms
          </span>
          <Badge variant="outline" className="text-xs capitalize">
            {result.handlerType}
          </Badge>
        </div>

        {/* Error */}
        {result.error && (
          <div className="bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-900/50 rounded-lg p-3">
            <p className="text-sm text-red-700 dark:text-red-400 font-mono break-all">
              {result.error}
            </p>
          </div>
        )}

        {/* Schema Validation */}
        {result.validation && (
          <div className="space-y-3">
            <Label>Schema Validation</Label>
            {result.validation.request && (
              <ValidationBadge label="Request" check={result.validation.request} />
            )}
            {result.validation.response && (
              <ValidationBadge label="Response" check={result.validation.response} />
            )}
          </div>
        )}

        {/* Result */}
        {result.result != null && (
          <div className="space-y-2">
            <Label>Response</Label>
            <pre className="text-xs bg-muted p-3 rounded-lg overflow-auto max-h-[400px] font-mono">
              {typeof result.result === "string"
                ? result.result
                : JSON.stringify(result.result, null, 2)}
            </pre>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

/** Renders the Run button (shared). */
function RunButton({
  isRunning,
  disabled,
  onClick,
}: Readonly<{ isRunning: boolean; disabled: boolean; onClick: () => void }>) {
  return (
    <Button onClick={onClick} disabled={disabled} className="w-full">
      {isRunning ? (
        <>
          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
          Running...
        </>
      ) : (
        <>
          <Play className="mr-2 h-4 w-4" />
          Run Test
        </>
      )}
    </Button>
  );
}

interface RunTestArgs {
  workspaceName: string;
  registryName: string;
  handlerName: string;
  toolName: string;
  parsedArgs: unknown;
  handlerType: string;
}

/** POSTs a tool-test request and returns the result (or a failure result on error). */
async function runTest(args: RunTestArgs): Promise<ToolTestResult> {
  try {
    const response = await fetch(
      `/api/workspaces/${args.workspaceName}/toolregistries/${args.registryName}/test`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          handlerName: args.handlerName,
          toolName: args.toolName || undefined,
          arguments: args.parsedArgs,
        }),
      }
    );
    return (await response.json()) as ToolTestResult;
  } catch (err) {
    return {
      success: false,
      error: err instanceof Error ? err.message : "Request failed",
      durationMs: 0,
      handlerType: args.handlerType,
    };
  }
}

/** Parses the arguments textarea; returns parsed value or an error message. */
function parseArgs(args: string): { value?: unknown; error?: string } {
  const cleaned = args.trim().replace(/^\uFEFF/, "");
  try {
    return { value: JSON.parse(cleaned) };
  } catch (e) {
    const detail = e instanceof SyntaxError ? `: ${e.message}` : "";
    return { error: `Invalid JSON${detail}` };
  }
}

/** Arguments editor (Monaco, self-validated) + JSON error message (shared). */
function ArgsInput({
  args,
  jsonError,
  onChange,
}: Readonly<{ args: string; jsonError: string | null; onChange: (v: string) => void }>) {
  return (
    <div className="space-y-2">
      <Label>Arguments (JSON)</Label>
      <JsonEditor value={args} onChange={onChange} ariaLabel="Arguments (JSON)" />
      {jsonError && <p className="text-sm text-red-500">{jsonError}</p>}
    </div>
  );
}

/** Parses the args JSON string into a plain object, or {} when invalid/non-object. */
function parseArgsObject(args: string): Record<string, unknown> {
  const parsed = parseArgs(args);
  const v = parsed.value;
  if (v && typeof v === "object" && !Array.isArray(v)) {
    return v as Record<string, unknown>;
  }
  return {};
}

/** Threshold above which the operation picker shows a search box. */
const SEARCH_THRESHOLD = 8;

/** Dropdown picker used when there are few operations. */
function OperationSelect({
  tools,
  selectedTool,
  onChange,
}: Readonly<{
  tools: OpenAPIToolPreviewItem[];
  selectedTool: string;
  onChange: (toolName: string) => void;
}>) {
  return (
    <Select value={selectedTool} onValueChange={onChange}>
      <SelectTrigger id="tool-select">
        <SelectValue placeholder="Select tool" />
      </SelectTrigger>
      <SelectContent>
        {tools.map((t) => (
          <SelectItem key={t.name} value={t.name}>
            {t.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Search box + filtered, clickable list used when there are many operations. */
function OperationSearchList({
  tools,
  selectedTool,
  onChange,
}: Readonly<{
  tools: OpenAPIToolPreviewItem[];
  selectedTool: string;
  onChange: (toolName: string) => void;
}>) {
  const [query, setQuery] = useState("");
  const q = query.trim().toLowerCase();
  const filtered = q
    ? tools.filter(
        (t) =>
          t.name.toLowerCase().includes(q) ||
          (t.description ?? "").toLowerCase().includes(q)
      )
    : tools;

  return (
    <>
      <Input
        placeholder="Search operations…"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        className="text-sm"
      />
      <div className="max-h-56 overflow-auto rounded-md border divide-y">
        {filtered.map((t) => (
          <button
            key={t.name}
            type="button"
            onClick={() => onChange(t.name)}
            className={`w-full text-left px-3 py-2 text-sm hover:bg-muted ${
              t.name === selectedTool ? "bg-muted font-medium" : ""
            }`}
          >
            {t.name}
          </button>
        ))}
      </div>
    </>
  );
}

/** Searchable Tool picker. Shows a search box only when there are many operations. */
function OperationPicker({
  tools,
  selectedTool,
  onChange,
}: Readonly<{
  tools: OpenAPIToolPreviewItem[];
  selectedTool: string;
  onChange: (toolName: string) => void;
}>) {
  const useSearch = tools.length > SEARCH_THRESHOLD;
  return (
    <div className="space-y-2">
      <Label htmlFor="tool-select">Tool</Label>
      {useSearch ? (
        <OperationSearchList tools={tools} selectedTool={selectedTool} onChange={onChange} />
      ) : (
        <OperationSelect tools={tools} selectedTool={selectedTool} onChange={onChange} />
      )}
    </div>
  );
}

/** Shows the selected tool's description and any lint warnings. */
function ToolInspector({ tool }: Readonly<{ tool: OpenAPIToolPreviewItem | undefined }>) {
  if (!tool) return null;
  const lints = computeToolLints(tool);
  return (
    <div className="space-y-2 rounded-lg border bg-muted/40 p-3">
      <p className="text-sm font-medium">{tool.name}</p>
      {tool.description && (
        <p className="text-sm text-muted-foreground break-words">{tool.description}</p>
      )}
      {lints.map((lint) => (
        <div key={lint.id} className="flex items-start gap-2">
          <AlertTriangle className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" />
          <p className="text-xs text-amber-700 dark:text-amber-400">{lint.message}</p>
        </div>
      ))}
    </div>
  );
}

/**
 * Argument editor with a Form/JSON toggle. `args` (a JSON string) is the single
 * source of truth; Form mode round-trips through it via parse/stringify.
 */
function ArgEditor({
  tool,
  args,
  jsonError,
  onChange,
}: Readonly<{
  tool: OpenAPIToolPreviewItem | undefined;
  args: string;
  jsonError: string | null;
  onChange: (v: string) => void;
}>) {
  const canForm = isRenderableObjectSchema(tool?.inputSchema);
  const [mode, setMode] = useState<"form" | "json">(canForm ? "form" : "json");
  const useForm = canForm && mode === "form";

  return (
    <div className="space-y-2">
      {canForm && (
        <div className="flex items-center gap-1">
          <Button
            type="button"
            size="sm"
            variant={mode === "form" ? "default" : "outline"}
            onClick={() => setMode("form")}
          >
            Form
          </Button>
          <Button
            type="button"
            size="sm"
            variant={mode === "json" ? "default" : "outline"}
            onClick={() => setMode("json")}
          >
            JSON
          </Button>
        </div>
      )}
      {useForm ? (
        <SchemaForm
          schema={tool?.inputSchema}
          value={parseArgsObject(args)}
          onChange={(v) => onChange(JSON.stringify(v, null, 2))}
          idPrefix="openapi-args"
        />
      ) : (
        <ArgsInput args={args} jsonError={jsonError} onChange={onChange} />
      )}
    </div>
  );
}

interface OpenAPIToolRunnerProps {
  tools: OpenAPIToolPreviewItem[];
  handlerName: string;
  workspaceName: string;
  registryName: string;
}

/**
 * Drives the Tool select + arguments + run for an OpenAPI handler from the LIVE
 * preview tools. State is initialized from the loaded tools via useState
 * initializers; the parent remounts this via a `key` when the tools change, so
 * no setState-in-effect is needed.
 */
export function OpenAPIToolRunner({
  tools,
  handlerName,
  workspaceName,
  registryName,
}: Readonly<OpenAPIToolRunnerProps>) {
  const [selectedTool, setSelectedTool] = useState<string>(tools[0]?.name ?? "");
  const [args, setArgs] = useState<string>(() => getSampleArgs(tools[0]));
  const [isRunning, setIsRunning] = useState(false);
  const [result, setResult] = useState<ToolTestResult | null>(null);
  const [jsonError, setJsonError] = useState<string | null>(null);

  const handleToolChange = useCallback(
    (toolName: string) => {
      setSelectedTool(toolName);
      setResult(null);
      setArgs(getSampleArgs(tools.find((t) => t.name === toolName)));
    },
    [tools]
  );

  const handleArgsChange = useCallback((v: string) => {
    setArgs(v);
    setJsonError(null);
  }, []);

  const handleRun = useCallback(async () => {
    const parsed = parseArgs(args);
    if (parsed.error) {
      setJsonError(parsed.error);
      return;
    }
    setJsonError(null);
    setIsRunning(true);
    setResult(null);
    const res = await runTest({
      workspaceName,
      registryName,
      handlerName,
      toolName: selectedTool,
      parsedArgs: parsed.value,
      handlerType: "openapi",
    });
    setResult(res);
    setIsRunning(false);
  }, [args, selectedTool, handlerName, workspaceName, registryName]);

  const currentTool = tools.find((t) => t.name === selectedTool);

  return (
    <>
      <OperationPicker tools={tools} selectedTool={selectedTool} onChange={handleToolChange} />

      <ToolInspector tool={currentTool} />

      <ArgEditor
        key={selectedTool}
        tool={currentTool}
        args={args}
        jsonError={jsonError}
        onChange={handleArgsChange}
      />

      <RunButton isRunning={isRunning} disabled={isRunning || !selectedTool} onClick={handleRun} />

      {result && <ResultCard result={result} />}
    </>
  );
}

/** Loading / error / runner UI for the OpenAPI live-preview flow. */
function OpenAPISection({
  preview,
  handlerName,
  workspaceName,
  registryName,
}: Readonly<{
  preview: ReturnType<typeof useOpenAPIToolPreview>;
  handlerName: string;
  workspaceName: string;
  registryName: string;
}>) {
  if (preview.loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading tools…
      </div>
    );
  }

  if (preview.error) {
    return (
      <div className="rounded-lg border border-red-200 dark:border-red-900/50 bg-red-50 dark:bg-red-950/20 p-3 space-y-1">
        <p className="text-sm font-medium text-red-700 dark:text-red-400">
          Couldn&apos;t load tools from spec
        </p>
        <p className="text-sm text-red-700 dark:text-red-400 font-mono break-all">
          {preview.error}
        </p>
        {preview.specURL && (
          <p className="text-xs text-muted-foreground font-mono break-all">{preview.specURL}</p>
        )}
      </div>
    );
  }

  const sig = `${handlerName}:${preview.tools.map((t) => t.name).join(",")}`;
  return (
    <OpenAPIToolRunner
      key={sig}
      tools={preview.tools}
      handlerName={handlerName}
      workspaceName={workspaceName}
      registryName={registryName}
    />
  );
}

export function ToolTestPanel({ registry, workspaceName }: Readonly<ToolTestPanelProps>) {
  const handlers = useMemo(() => registry.spec.handlers || [], [registry.spec.handlers]);
  const discoveredTools = useMemo(
    () => registry.status?.discoveredTools || [],
    [registry.status?.discoveredTools]
  );

  const [selectedHandler, setSelectedHandler] = useState<string>(
    handlers.length > 0 ? handlers[0].name : ""
  );
  const [selectedTool, setSelectedTool] = useState<string>(() => {
    if (handlers.length === 0) return "";
    return getDefaultToolName(handlers[0], discoveredTools);
  });
  const [args, setArgs] = useState<string>(() => {
    if (handlers.length === 0) return "{}";
    const handler = handlers[0];
    const toolName = getDefaultToolName(handler, discoveredTools);
    const discovered = discoveredTools.find((t) => t.name === toolName);
    return getSampleArgs(discovered, handler);
  });
  const [isRunning, setIsRunning] = useState(false);
  const [result, setResult] = useState<ToolTestResult | null>(null);
  const [jsonError, setJsonError] = useState<string | null>(null);

  const currentHandler = useMemo(
    () => handlers.find((h) => h.name === selectedHandler),
    [handlers, selectedHandler]
  );
  const isOpenAPI = currentHandler?.type === "openapi";
  const availableTools = currentHandler
    ? getToolsForHandler(currentHandler, discoveredTools)
    : [];
  // For HTTP/gRPC with inline tool, include it even if not discovered
  const hasInlineTool = currentHandler?.tool?.name != null;

  // Live OpenAPI preview — only fetches for OpenAPI handlers (empty handler name no-ops).
  const preview = useOpenAPIToolPreview(
    workspaceName,
    registry.metadata.name,
    isOpenAPI ? selectedHandler : ""
  );

  const handleHandlerChange = useCallback(
    (handlerName: string) => {
      setSelectedHandler(handlerName);
      setResult(null);
      const handler = handlers.find((h) => h.name === handlerName);
      if (handler) {
        const toolName = getDefaultToolName(handler, discoveredTools);
        setSelectedTool(toolName);
        const discovered = discoveredTools.find((t) => t.name === toolName);
        setArgs(getSampleArgs(discovered, handler));
      }
    },
    [handlers, discoveredTools]
  );

  const handleToolChange = useCallback(
    (toolName: string) => {
      setSelectedTool(toolName);
      setResult(null);
      const discovered = discoveredTools.find((t) => t.name === toolName);
      setArgs(getSampleArgs(discovered, currentHandler));
    },
    [discoveredTools, currentHandler]
  );

  const handleRun = useCallback(async () => {
    const parsed = parseArgs(args);
    if (parsed.error) {
      setJsonError(parsed.error);
      return;
    }
    setJsonError(null);
    setIsRunning(true);
    setResult(null);
    const res = await runTest({
      workspaceName,
      registryName: registry.metadata.name,
      handlerName: selectedHandler,
      toolName: selectedTool,
      parsedArgs: parsed.value,
      handlerType: currentHandler?.type || "unknown",
    });
    setResult(res);
    setIsRunning(false);
  }, [args, selectedHandler, selectedTool, workspaceName, registry.metadata.name, currentHandler]);

  if (handlers.length === 0) {
    return (
      <Card>
        <CardContent className="py-8">
          <p className="text-sm text-muted-foreground text-center">
            No handlers configured in this ToolRegistry
          </p>
        </CardContent>
      </Card>
    );
  }

  const showToolSelect =
    availableTools.length > 1 || (!hasInlineTool && availableTools.length > 0);

  return (
    <div className="space-y-6">
      {/* Input Card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Test Tool Call</CardTitle>
          <CardDescription>
            Execute a tool call using the configured handler to verify connectivity and behavior
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Handler Select */}
          <div className="space-y-2">
            <Label htmlFor="handler-select">Handler</Label>
            <Select value={selectedHandler} onValueChange={handleHandlerChange}>
              <SelectTrigger id="handler-select">
                <SelectValue placeholder="Select handler" />
              </SelectTrigger>
              <SelectContent>
                {handlers.map((h) => (
                  <SelectItem key={h.name} value={h.name}>
                    <span className="flex items-center gap-2">
                      {h.name}
                      <Badge variant="outline" className="text-xs capitalize ml-1">
                        {h.type}
                      </Badge>
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {isOpenAPI ? (
            <OpenAPISection
              preview={preview}
              handlerName={selectedHandler}
              workspaceName={workspaceName}
              registryName={registry.metadata.name}
            />
          ) : (
            <>
              {/* Tool Select — show when handler has multiple tools */}
              {showToolSelect && (
                <div className="space-y-2">
                  <Label htmlFor="tool-select">Tool</Label>
                  <Select value={selectedTool} onValueChange={handleToolChange}>
                    <SelectTrigger id="tool-select">
                      <SelectValue placeholder="Select tool" />
                    </SelectTrigger>
                    <SelectContent>
                      {availableTools.map((t) => (
                        <SelectItem key={t.name} value={t.name}>
                          {t.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}

              <ArgsInput
                args={args}
                jsonError={jsonError}
                onChange={(v) => {
                  setArgs(v);
                  setJsonError(null);
                }}
              />

              {/* Client tool notice */}
              {currentHandler?.type === "client" && (
                <div className="rounded-lg border border-amber-200 dark:border-amber-900/50 bg-amber-50 dark:bg-amber-950/20 p-3">
                  <p className="text-sm text-amber-700 dark:text-amber-400">
                    Client tools are executed in the browser during an active agent session. They
                    cannot be tested from this page — use the Console to test them through a live
                    agent conversation.
                  </p>
                </div>
              )}

              <RunButton
                isRunning={isRunning}
                disabled={isRunning || !selectedHandler || currentHandler?.type === "client"}
                onClick={handleRun}
              />
            </>
          )}
        </CardContent>
      </Card>

      {/* Result Card (discovered-tools flow; OpenAPI renders its own) */}
      {!isOpenAPI && result && <ResultCard result={result} />}
    </div>
  );
}
