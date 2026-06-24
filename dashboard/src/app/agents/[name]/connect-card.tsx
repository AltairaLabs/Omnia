"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { AlertTriangle, Copy, ExternalLink } from "lucide-react";
import { cn } from "@/lib/utils";
import { facadeAuthHint } from "@/lib/agents/facade-auth-hint";
import type { AgentRuntime, FacadeEndpoint } from "@/types/agent-runtime";

interface ConnectCardProps {
  agent: AgentRuntime;
}

interface EndpointRowProps {
  endpoint: FacadeEndpoint;
  authLabel: string;
  authDetail?: string;
}

function protocolBadgeVariant(protocol: string): "default" | "secondary" | "outline" {
  if (protocol === "websocket") return "default";
  if (protocol === "a2a") return "secondary";
  return "outline";
}

function EndpointRow({ endpoint, authLabel, authDetail }: Readonly<EndpointRowProps>) {
  function copyUrl() {
    navigator.clipboard.writeText(endpoint.url);
  }

  const isInvalid = !endpoint.valid;

  return (
    <div
      className={cn(
        "rounded-md border p-3 space-y-2",
        isInvalid && "border-amber-400 bg-amber-50 dark:bg-amber-950/30",
      )}
    >
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <div className="flex items-center gap-2">
          <Badge variant={protocolBadgeVariant(endpoint.protocol)}>
            {endpoint.scheme.toUpperCase()}
          </Badge>
          <Badge variant="outline" className="text-xs">
            {endpoint.protocol}
          </Badge>
        </div>
        {isInvalid && (
          <span className="flex items-center gap-1 text-xs font-medium text-amber-700 dark:text-amber-400">
            <AlertTriangle className="h-3 w-3" />
            Not connectable
          </span>
        )}
      </div>

      <div className="flex items-center gap-2">
        <code className="flex-1 text-xs font-mono bg-muted px-2 py-1 rounded truncate">
          {endpoint.url}
        </code>
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0"
          onClick={copyUrl}
          aria-label={`Copy URL ${endpoint.url}`}
        >
          <Copy className="h-3.5 w-3.5" />
        </Button>
      </div>

      {isInvalid && endpoint.reason && (
        <p className="text-xs text-amber-700 dark:text-amber-400">{endpoint.reason}</p>
      )}

      <p className="text-xs text-muted-foreground">
        Auth: <span className="font-medium">{authLabel}</span>
        {authDetail && <span className="ml-1 font-mono">{authDetail}</span>}
      </p>
    </div>
  );
}

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center py-6 text-center gap-2">
      <p className="text-sm text-muted-foreground">
        No external endpoints — this agent is reachable only inside the cluster.
      </p>
      <a
        href="https://omnia.altairalabs.ai/how-to/expose-agents/"
        target="_blank"
        rel="noopener noreferrer"
        className="flex items-center gap-1 text-xs text-primary hover:underline"
      >
        How to expose agents externally
        <ExternalLink className="h-3 w-3" />
      </a>
    </div>
  );
}

/**
 * ConnectCard surfaces the externally-reachable facade endpoints from
 * status.facade.endpoints, with an auth hint derived from spec.externalAuth.
 *
 * Distinct from the Console tab, which connects via the dashboard's internal
 * management-plane proxy — this card shows the URLs a customer app would use.
 */
export function ConnectCard({ agent }: Readonly<ConnectCardProps>) {
  const endpoints = agent.status?.facade?.endpoints ?? [];
  const hint = facadeAuthHint(agent.spec.externalAuth);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Connect (external)</CardTitle>
        <CardDescription>
          External WebSocket / A2A / MCP URLs for customer apps — distinct from
          the Console tab, which connects via the dashboard&apos;s internal proxy.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {endpoints.length === 0 ? (
          <EmptyState />
        ) : (
          <div className="space-y-2">
            {endpoints.map((ep) => (
              <EndpointRow
                key={`${ep.routeNamespace}/${ep.routeName}/${ep.protocol}`}
                endpoint={ep}
                authLabel={hint.label}
                authDetail={hint.detail}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
