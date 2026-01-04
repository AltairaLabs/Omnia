"use client";

import Link from "next/link";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { StatusBadge } from "./status-badge";
import type { AgentRuntime } from "@/types";

interface AgentCardProps {
  agent: AgentRuntime;
}

export function AgentCard({ agent }: AgentCardProps) {
  const { metadata, spec, status } = agent;

  return (
    <Link href={`/agents/${metadata.name}?namespace=${metadata.namespace}`}>
      <Card className="transition-colors hover:bg-muted/50">
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between">
            <div className="space-y-1">
              <CardTitle className="text-base font-medium">
                {metadata.name}
              </CardTitle>
              <CardDescription>{metadata.namespace}</CardDescription>
            </div>
            <StatusBadge phase={status?.phase} />
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div>
              <p className="text-muted-foreground">Provider</p>
              <p className="font-medium capitalize">{spec.provider?.type || "claude"}</p>
            </div>
            <div>
              <p className="text-muted-foreground">Model</p>
              <p className="font-medium truncate" title={spec.provider?.model}>
                {spec.provider?.model?.split("-").slice(-2).join("-") || "sonnet-4"}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground">Replicas</p>
              <p className="font-medium">
                {status?.replicas?.ready ?? 0}/{status?.replicas?.desired ?? spec.runtime?.replicas ?? 1}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground">Facade</p>
              <p className="font-medium capitalize">{spec.facade?.type || "websocket"}</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
