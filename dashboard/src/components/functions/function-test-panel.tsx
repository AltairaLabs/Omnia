/**
 * FunctionTestPanel — invoke a function-mode AgentRuntime and show its output.
 *
 * Mirrors the ToolRegistry Test tab, but targets the function invoke proxy
 * (`POST /api/workspaces/:ws/functions/:name/invoke`), which reaches the
 * function's facade exactly like the agent Console does. The input editor
 * offers a schema-driven Form view (toggleable to raw JSON) seeded from the
 * function's inputSchema; the result card pretty-prints the facade response.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useCallback } from "react";
import Link from "next/link";
import { Play, CheckCircle, XCircle, Loader2, Clock, ArrowUpRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { JsonBlock } from "@/components/ui/json-block";
import { JsonEditor } from "@/components/editors/json-editor";
import { SchemaForm, isRenderableObjectSchema } from "@/components/tools/schema-form";
import { getSampleArgs } from "@/components/tools/tool-test-panel";
import { formatCost } from "@/lib/pricing";

interface FunctionTestPanelProps {
  functionName: string;
  workspace: string;
  inputSchema?: Record<string, unknown>;
  /** Whether the function is serving requests. When false, Run is disabled. */
  ready?: boolean;
  /** Human-readable reason shown when the function isn't ready (e.g. phase). */
  unavailableReason?: string;
}

interface InvokeUsage {
  inputTokens?: number;
  outputTokens?: number;
  costUsd?: number;
}

interface InvokeResult {
  ok: boolean;
  status: number;
  /** Server-reported duration when present, else the client round-trip. */
  durationMs: number;
  /** The function output (envelope `output`) or raw body / error raw_output. */
  output: unknown;
  outputIsJson: boolean;
  /** Recorded session id (envelope `invocation_id`), for deep-linking. */
  invocationId?: string;
  usage?: InvokeUsage;
  error?: string;
}

/** Parses a JSON string; returns the value or an error message. */
function parseJson(raw: string): { value?: unknown; error?: string } {
  const cleaned = raw.trim().replace(/^﻿/, "");
  try {
    return { value: JSON.parse(cleaned) };
  } catch (e) {
    const detail = e instanceof SyntaxError ? `: ${e.message}` : "";
    return { error: `Invalid JSON${detail}` };
  }
}

/** Coerces the args JSON string into a plain object for the Form view. */
function parseArgsObject(raw: string): Record<string, unknown> {
  const { value } = parseJson(raw);
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  return {};
}

/** Raw JSON input editor (Monaco, self-validated). */
function JsonInput({
  args,
  jsonError,
  onChange,
}: Readonly<{ args: string; jsonError: string | null; onChange: (v: string) => void }>) {
  return (
    <div className="space-y-2">
      <Label>Input (JSON)</Label>
      <JsonEditor value={args} onChange={onChange} ariaLabel="Input (JSON)" />
      {jsonError && <p className="text-sm text-red-500">{jsonError}</p>}
    </div>
  );
}

/** Input editor with a Form/JSON toggle when the schema is renderable. */
function InputEditor({
  inputSchema,
  args,
  jsonError,
  onChange,
}: Readonly<{
  inputSchema?: Record<string, unknown>;
  args: string;
  jsonError: string | null;
  onChange: (v: string) => void;
}>) {
  const canForm = isRenderableObjectSchema(inputSchema);
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
          schema={inputSchema}
          value={parseArgsObject(args)}
          onChange={(v) => onChange(JSON.stringify(v, null, 2))}
          idPrefix="fn-args"
        />
      ) : (
        <JsonInput args={args} jsonError={jsonError} onChange={onChange} />
      )}
    </div>
  );
}

/** Token + cost chips from the facade usage block. */
function UsageChips({ usage }: Readonly<{ usage: InvokeUsage }>) {
  const hasTokens = usage.inputTokens !== undefined || usage.outputTokens !== undefined;
  if (!hasTokens && usage.costUsd === undefined) return null;
  return (
    <>
      {hasTokens && (
        <span className="flex items-center gap-1" title="Input / output tokens">
          {usage.inputTokens ?? 0} in / {usage.outputTokens ?? 0} out
        </span>
      )}
      {usage.costUsd !== undefined && <span title="Estimated cost">{formatCost(usage.costUsd)}</span>}
    </>
  );
}

/** Result card: status, duration, usage, invocation link, and the output. */
function ResultCard({ result }: Readonly<{ result: InvokeResult }>) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          {result.ok ? (
            <CheckCircle className="h-4 w-4 text-green-500" />
          ) : (
            <XCircle className="h-4 w-4 text-red-500" />
          )}
          {result.ok ? "Success" : "Failed"}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <Clock className="h-3 w-3" />
            {result.durationMs}ms
          </span>
          <Badge variant="outline" className="text-xs">
            HTTP {result.status}
          </Badge>
          {result.usage && <UsageChips usage={result.usage} />}
          {result.invocationId && (
            <Link
              href={`/sessions/${result.invocationId}`}
              className="flex items-center gap-0.5 text-foreground hover:underline"
            >
              View invocation
              <ArrowUpRight className="h-3 w-3" />
            </Link>
          )}
        </div>

        {result.error && (
          <div className="bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-900/50 rounded-lg p-3">
            <p className="text-sm text-red-700 dark:text-red-400 font-mono break-all">
              {result.error}
            </p>
          </div>
        )}

        {result.output != null && (
          <div className="space-y-2">
            <Label>{result.ok ? "Output" : "Raw output"}</Label>
            {result.outputIsJson ? (
              <JsonBlock data={result.output} className="max-h-[400px]" />
            ) : (
              <pre className="text-xs bg-muted p-3 rounded-lg overflow-auto max-h-[400px] font-mono break-all whitespace-pre-wrap">
                {String(result.output)}
              </pre>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

/** Reads a string field from a parsed object, or undefined. */
function strField(obj: Record<string, unknown>, key: string): string | undefined {
  const v = obj[key];
  return typeof v === "string" ? v : undefined;
}

/** Maps the facade `usage` block to the panel's camelCase shape. */
function parseUsage(raw: unknown): InvokeUsage | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const u = raw as Record<string, unknown>;
  const num = (k: string) => (typeof u[k] === "number" ? (u[k] as number) : undefined);
  return { inputTokens: num("input_tokens"), outputTokens: num("output_tokens"), costUsd: num("cost_usd") };
}

const isJsonObject = (v: unknown): v is Record<string, unknown> =>
  v !== null && typeof v === "object" && !Array.isArray(v);

/** Shapes a successful response, unwrapping the facade envelope when present. */
function shapeSuccess(status: number, text: string, clientMs: number): InvokeResult {
  const parsed = parseJson(text);
  const bodyIsJson = parsed.error === undefined && text.trim() !== "";
  const envelope = isJsonObject(parsed.value) ? parsed.value : null;

  // Facade success envelope: { output, invocation_id, duration_ms, usage }.
  if (envelope && "output" in envelope) {
    return {
      ok: true,
      status,
      durationMs: typeof envelope.duration_ms === "number" ? envelope.duration_ms : clientMs,
      output: envelope.output,
      outputIsJson: isJsonObject(envelope.output) || Array.isArray(envelope.output),
      invocationId: strField(envelope, "invocation_id"),
      usage: parseUsage(envelope.usage),
    };
  }
  // Non-envelope (proxy passthrough / bare value): show the whole body.
  return {
    ok: true,
    status,
    durationMs: clientMs,
    output: bodyIsJson ? parsed.value : text,
    outputIsJson: bodyIsJson,
  };
}

/** Shapes a non-2xx response, surfacing the facade error envelope. */
function shapeError(status: number, text: string, clientMs: number): InvokeResult {
  const parsed = parseJson(text);
  const envelope = isJsonObject(parsed.value) ? parsed.value : null;
  const error = envelope
    ? [strField(envelope, "error"), strField(envelope, "detail")].filter(Boolean).join(": ")
    : text.trim();
  const raw = envelope?.raw_output;
  return {
    ok: false,
    status,
    durationMs: clientMs,
    output: raw ?? null,
    outputIsJson: isJsonObject(raw) || Array.isArray(raw),
    error: error || `HTTP ${status}`,
  };
}

/** POSTs the input to the function invoke proxy and shapes the result. */
async function invoke(
  workspace: string,
  functionName: string,
  parsed: unknown,
): Promise<InvokeResult> {
  const started = performance.now();
  let response: Response;
  try {
    response = await fetch(`/api/workspaces/${workspace}/functions/${functionName}/invoke`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(parsed),
    });
  } catch (err) {
    return {
      ok: false,
      status: 0,
      durationMs: Math.round(performance.now() - started),
      output: null,
      outputIsJson: false,
      error: err instanceof Error ? err.message : "Request failed",
    };
  }
  const clientMs = Math.round(performance.now() - started);
  const text = await response.text();
  return response.status < 400
    ? shapeSuccess(response.status, text, clientMs)
    : shapeError(response.status, text, clientMs);
}

export function FunctionTestPanel({
  functionName,
  workspace,
  inputSchema,
  ready = true,
  unavailableReason,
}: Readonly<FunctionTestPanelProps>) {
  const [args, setArgs] = useState<string>(() =>
    inputSchema ? getSampleArgs({ inputSchema }) : "{}",
  );
  const [isRunning, setIsRunning] = useState(false);
  const [result, setResult] = useState<InvokeResult | null>(null);
  const [jsonError, setJsonError] = useState<string | null>(null);

  const handleArgsChange = useCallback((v: string) => {
    setArgs(v);
    setJsonError(null);
  }, []);

  const handleRun = useCallback(async () => {
    if (!ready) return;
    const parsed = parseJson(args);
    if (parsed.error) {
      setJsonError(parsed.error);
      return;
    }
    setJsonError(null);
    setIsRunning(true);
    setResult(null);
    const res = await invoke(workspace, functionName, parsed.value);
    setResult(res);
    setIsRunning(false);
  }, [args, workspace, functionName, ready]);

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Invoke Function</CardTitle>
          <CardDescription>
            Send structured input to the function&apos;s facade and inspect the response. Input is
            validated against the function&apos;s inputSchema by the facade.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <InputEditor
            inputSchema={inputSchema}
            args={args}
            jsonError={jsonError}
            onChange={handleArgsChange}
          />

          {!ready && (
            <div className="rounded-lg border border-amber-200 dark:border-amber-900/50 bg-amber-50 dark:bg-amber-950/20 p-3">
              <p className="text-sm text-amber-700 dark:text-amber-400">
                This function is not ready
                {unavailableReason ? ` (${unavailableReason})` : ""} — its runtime or provider may
                still be starting. The Run button is disabled until it&apos;s serving requests.
              </p>
            </div>
          )}

          <Button onClick={handleRun} disabled={isRunning || !ready} className="w-full">
            {isRunning ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Running...
              </>
            ) : (
              <>
                <Play className="mr-2 h-4 w-4" />
                Run
              </>
            )}
          </Button>
        </CardContent>
      </Card>

      {result && <ResultCard result={result} />}
    </div>
  );
}
