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
import { Play, CheckCircle, XCircle, Loader2, Clock } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { JsonBlock } from "@/components/ui/json-block";
import { SchemaForm, isRenderableObjectSchema } from "@/components/tools/schema-form";
import { getSampleArgs } from "@/components/tools/tool-test-panel";

interface FunctionTestPanelProps {
  functionName: string;
  workspace: string;
  inputSchema?: Record<string, unknown>;
  /** Whether the function is serving requests. When false, Run is disabled. */
  ready?: boolean;
  /** Human-readable reason shown when the function isn't ready (e.g. phase). */
  unavailableReason?: string;
}

interface InvokeResult {
  ok: boolean;
  status: number;
  durationMs: number;
  /** Parsed JSON body when the response was JSON, else the raw text. */
  body: unknown;
  isJson: boolean;
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

/** Raw JSON input editor. */
function JsonInput({
  args,
  jsonError,
  onChange,
}: Readonly<{ args: string; jsonError: string | null; onChange: (v: string) => void }>) {
  return (
    <div className="space-y-2">
      <Label htmlFor="fn-input">Input (JSON)</Label>
      <Textarea
        id="fn-input"
        value={args}
        onChange={(e) => onChange(e.target.value)}
        className="font-mono text-sm min-h-[120px]"
        placeholder='{"key": "value"}'
      />
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

/** Result card: status, duration, and the pretty-printed response body. */
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
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <Clock className="h-3 w-3" />
            {result.durationMs}ms
          </span>
          <Badge variant="outline" className="text-xs">
            HTTP {result.status}
          </Badge>
        </div>

        {result.error && (
          <div className="bg-red-50 dark:bg-red-950/20 border border-red-200 dark:border-red-900/50 rounded-lg p-3">
            <p className="text-sm text-red-700 dark:text-red-400 font-mono break-all">
              {result.error}
            </p>
          </div>
        )}

        {result.body != null && (
          <div className="space-y-2">
            <Label>Response</Label>
            {result.isJson ? (
              <JsonBlock data={result.body} className="max-h-[400px]" />
            ) : (
              <pre className="text-xs bg-muted p-3 rounded-lg overflow-auto max-h-[400px] font-mono break-all whitespace-pre-wrap">
                {String(result.body)}
              </pre>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

/** POSTs the input to the function invoke proxy and shapes the result. */
async function invoke(
  workspace: string,
  functionName: string,
  parsed: unknown,
): Promise<InvokeResult> {
  const started = performance.now();
  try {
    const response = await fetch(
      `/api/workspaces/${workspace}/functions/${functionName}/invoke`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(parsed),
      },
    );
    const durationMs = Math.round(performance.now() - started);
    const text = await response.text();
    const json = parseJson(text);
    const isJson = json.error === undefined && text.trim() !== "";
    return {
      ok: response.status < 400,
      status: response.status,
      durationMs,
      body: isJson ? json.value : text,
      isJson,
    };
  } catch (err) {
    return {
      ok: false,
      status: 0,
      durationMs: Math.round(performance.now() - started),
      body: null,
      isJson: false,
      error: err instanceof Error ? err.message : "Request failed",
    };
  }
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
