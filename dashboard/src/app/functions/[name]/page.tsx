/**
 * /functions/[name] — Function detail page (#1103 PR 6).
 *
 * Shows the resolved input/output schemas and a panel of recent
 * invocations sourced from session-api's function_invocations table.
 * The workspace context provides the namespace; the URL provides
 * the function name.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { FunctionInvocationsPanel } from "@/components/functions/function-invocations-panel";
import { useAgent } from "@/hooks/agents";
import { useWorkspace } from "@/contexts/workspace-context";
import { isFunctionMode } from "@/types/agent-runtime";

type WindowPreset = "1h" | "24h" | "7d";

const WINDOW_MS: Record<WindowPreset, number> = {
  "1h": 60 * 60 * 1000,
  "24h": 24 * 60 * 60 * 1000,
  "7d": 7 * 24 * 60 * 60 * 1000,
};

export default function FunctionDetailPage() {
  const params = useParams<{ name: string }>();
  const functionName = params?.name ?? "";
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name ?? "";

  const [windowPreset, setWindowPreset] = useState<WindowPreset>("24h");

  const { data: agent, isLoading } = useAgent(functionName);

  if (isLoading) {
    return (
      <div className="flex flex-col h-full">
        <Header title={functionName} />
        <div className="p-6">
          <Skeleton className="h-[200px] w-full rounded-lg" />
        </div>
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="flex flex-col h-full">
        <Header title={functionName} description="Function not found" />
        <div className="p-6">
          <p className="text-muted-foreground">
            No AgentRuntime named <code className="font-mono">{functionName}</code> exists in
            this workspace.
          </p>
        </div>
      </div>
    );
  }

  if (!isFunctionMode(agent.spec)) {
    return (
      <div className="flex flex-col h-full">
        <Header
          title={functionName}
          description="This AgentRuntime is not a Function"
        />
        <div className="p-6">
          <p className="text-muted-foreground">
            <code className="font-mono">{functionName}</code> is mode{" "}
            <Badge variant="outline">{agent.spec.mode ?? "agent"}</Badge>. The
            functions view only surfaces runtimes with{" "}
            <code className="font-mono">spec.mode: function</code>.
          </p>
        </div>
      </div>
    );
  }

  const recording = agent.spec.invocationRecording?.state === "enabled";

  return (
    <div className="flex flex-col h-full">
      <Header
        title={functionName}
        description={`Function in ${agent.metadata.namespace ?? "default"}`}
      />

      <div className="flex-1 p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex gap-2">
            <Badge variant="default">Function</Badge>
            <Badge variant={recording ? "default" : "outline"}>
              {recording ? "Recording" : "Ephemeral"}
            </Badge>
          </div>
          <div className="flex gap-1 rounded-md border bg-muted p-1">
            {(Object.keys(WINDOW_MS) as WindowPreset[]).map((preset) => (
              <button
                key={preset}
                type="button"
                onClick={() => setWindowPreset(preset)}
                className={
                  "rounded-sm px-3 py-1 text-xs font-medium transition-colors " +
                  (windowPreset === preset
                    ? "bg-background text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground")
                }
                data-testid={`window-${preset}`}
              >
                {preset}
              </button>
            ))}
          </div>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <SchemaCard label="Input schema" schema={agent.spec.inputSchema} />
          <SchemaCard label="Output schema" schema={agent.spec.outputSchema} />
        </div>

        {recording ? (
          <FunctionInvocationsPanel
            workspace={workspace}
            functionName={functionName}
            windowMs={WINDOW_MS[windowPreset]}
          />
        ) : (
          <Card>
            <CardHeader>
              <CardTitle>Recent invocations</CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-sm text-muted-foreground">
                Invocation recording is disabled. Set{" "}
                <code className="font-mono">spec.invocationRecording.state: enabled</code>{" "}
                on this AgentRuntime to retain audit rows.
              </p>
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}

interface SchemaCardProps {
  label: string;
  schema?: Record<string, unknown>;
}

function SchemaCard({ label, schema }: Readonly<SchemaCardProps>) {
  return (
    <Card data-testid={`schema-card-${label.toLowerCase().replaceAll(/\s+/g, "-")}`}>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <pre className="overflow-auto rounded-md bg-muted p-3 text-xs">
          {schema ? JSON.stringify(schema, null, 2) : "—"}
        </pre>
      </CardContent>
    </Card>
  );
}
