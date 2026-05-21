/**
 * /functions/[name] — Function detail page.
 *
 * Shows the resolved input/output schemas for a function-mode
 * AgentRuntime alongside a panel of recent invocations sourced from
 * the standard sessions data path. Functions record as ordinary
 * sessions tagged "function"; the panel reuses the existing useSessions
 * hook + workspace session-api proxy that powers /sessions.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useParams } from "next/navigation";
import { Header } from "@/components/layout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { FunctionSessionsPanel } from "@/components/functions/function-sessions-panel";
import { useAgent } from "@/hooks/agents";
import { isFunctionMode } from "@/types/agent-runtime";

export default function FunctionDetailPage() {
  const params = useParams<{ name: string }>();
  const functionName = params?.name ?? "";

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

  return (
    <div className="flex flex-col h-full">
      <Header
        title={functionName}
        description={`Function in ${agent.metadata.namespace ?? "default"}`}
      />

      <div className="flex-1 p-6 space-y-6">
        <div className="flex gap-2">
          <Badge variant="default">Function</Badge>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <SchemaCard label="Input schema" schema={agent.spec.inputSchema} />
          <SchemaCard label="Output schema" schema={agent.spec.outputSchema} />
        </div>

        <FunctionSessionsPanel functionName={functionName} />

        <p className="text-xs text-muted-foreground">
          Function invocations are recorded as sessions tagged{" "}
          <code className="font-mono">function</code>. Click a session id above
          for the full invocation transcript, tool calls, provider calls, and
          eval results.
        </p>
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
