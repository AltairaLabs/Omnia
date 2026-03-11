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
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ToolRegistry, HandlerDefinition, DiscoveredTool } from "@/types";

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

/**
 * Get a sample arguments JSON from a tool's input schema.
 */
function getSampleArgs(tool?: DiscoveredTool | null, handler?: HandlerDefinition): string {
  const schema = tool?.inputSchema ?? handler?.tool?.inputSchema;
  if (!schema || typeof schema !== "object") return "{}";

  const props = (schema as Record<string, unknown>).properties;
  if (!props || typeof props !== "object") return "{}";

  const sample: Record<string, unknown> = {};
  for (const [key, def] of Object.entries(props as Record<string, Record<string, unknown>>)) {
    const type = def?.type;
    if (type === "string") sample[key] = "";
    else if (type === "number" || type === "integer") sample[key] = 0;
    else if (type === "boolean") sample[key] = false;
    else if (type === "array") sample[key] = [];
    else if (type === "object") sample[key] = {};
    else sample[key] = null;
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

  const currentHandler = handlers.find((h) => h.name === selectedHandler);
  const availableTools = currentHandler
    ? getToolsForHandler(currentHandler, discoveredTools)
    : [];
  // For HTTP/gRPC with inline tool, include it even if not discovered
  const hasInlineTool = currentHandler?.tool?.name != null;

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
    // Validate JSON
    try {
      JSON.parse(args);
      setJsonError(null);
    } catch {
      setJsonError("Invalid JSON");
      return;
    }

    setIsRunning(true);
    setResult(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspaceName}/toolregistries/${registry.metadata.name}/test`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            handlerName: selectedHandler,
            toolName: selectedTool || undefined,
            arguments: JSON.parse(args),
          }),
        }
      );

      const data: ToolTestResult = await response.json();
      setResult(data);
    } catch (err) {
      setResult({
        success: false,
        error: err instanceof Error ? err.message : "Request failed",
        durationMs: 0,
        handlerType: currentHandler?.type || "unknown",
      });
    } finally {
      setIsRunning(false);
    }
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

          {/* Tool Select — show when handler has multiple tools */}
          {(availableTools.length > 1 || (!hasInlineTool && availableTools.length > 0)) && (
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

          {/* Arguments */}
          <div className="space-y-2">
            <Label htmlFor="args-input">Arguments (JSON)</Label>
            <Textarea
              id="args-input"
              value={args}
              onChange={(e) => {
                setArgs(e.target.value);
                setJsonError(null);
              }}
              className="font-mono text-sm min-h-[120px]"
              placeholder='{"key": "value"}'
            />
            {jsonError && (
              <p className="text-sm text-red-500">{jsonError}</p>
            )}
          </div>

          {/* Run Button */}
          <Button
            onClick={handleRun}
            disabled={isRunning || !selectedHandler}
            className="w-full"
          >
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
        </CardContent>
      </Card>

      {/* Result Card */}
      {result && (
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
      )}
    </div>
  );
}
