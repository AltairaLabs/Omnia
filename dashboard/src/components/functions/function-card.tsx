/**
 * FunctionCard renders one row of the /functions catalog. Compact
 * presentation focused on the function's contract — name, namespace,
 * input/output schema summary, recording opt-in — with click-through
 * to the detail page. Unlike AgentCard, there are no scale controls
 * or live cost spark; the detail page does the heavier read.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import type { AgentRuntime } from "@/types";

interface FunctionCardProps {
  fn: AgentRuntime;
}

/** schemaFieldCount returns the number of top-level keys under
 * schema.properties, or 0 when the schema isn't a properties-style
 * object. Used purely as a UI hint — the schema is opaque otherwise. */
function schemaFieldCount(schema: Record<string, unknown> | undefined): number {
  if (!schema) return 0;
  const props = schema.properties;
  if (props && typeof props === "object" && !Array.isArray(props)) {
    return Object.keys(props).length;
  }
  return 0;
}

export function FunctionCard({ fn }: Readonly<FunctionCardProps>) {
  const { metadata, spec } = fn;
  const namespace = metadata.namespace ?? "default";
  const recording = spec.invocationRecording?.state === "enabled";
  const inputFields = schemaFieldCount(spec.inputSchema);
  const outputFields = schemaFieldCount(spec.outputSchema);

  return (
    <Link href={`/functions/${metadata.name}?namespace=${namespace}`}>
      <Card
        className="transition-colors hover:bg-muted/50"
        data-testid="function-card"
      >
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between gap-2">
            <CardTitle className="text-base font-medium">{metadata.name}</CardTitle>
            <Badge variant={recording ? "default" : "outline"} className="shrink-0">
              {recording ? "Recording" : "Ephemeral"}
            </Badge>
          </div>
          <p className="text-xs text-muted-foreground">{namespace}</p>
        </CardHeader>
        <CardContent className="text-sm">
          <dl className="grid grid-cols-2 gap-x-3 gap-y-1">
            <dt className="text-muted-foreground">Input</dt>
            <dd>{inputFields} field{inputFields === 1 ? "" : "s"}</dd>
            <dt className="text-muted-foreground">Output</dt>
            <dd>{outputFields} field{outputFields === 1 ? "" : "s"}</dd>
          </dl>
        </CardContent>
      </Card>
    </Link>
  );
}
