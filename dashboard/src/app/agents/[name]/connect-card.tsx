"use client";

import { useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AlertTriangle, Copy, ExternalLink } from "lucide-react";
import { cn } from "@/lib/utils";
import { facadeAuthHint } from "@/lib/agents/facade-auth-hint";
import { useWorkspacePermissions } from "@/hooks/use-workspace-permissions";
import { useSetAgentExpose } from "@/hooks/use-set-agent-expose";
import { primaryFacade, type AgentRuntime, type FacadeEndpoint } from "@/types/agent-runtime";

interface ConnectCardProps {
  agent: AgentRuntime;
  workspace: string;
  onExposeChange?: () => void;
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
        isInvalid && "border-warning/30 bg-warning/10",
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
          <span className="flex items-center gap-1 text-xs font-medium text-warning">
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
        <p className="text-xs text-warning">{endpoint.reason}</p>
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
 * ExposeControl is the opt-in toggle for operator-provisioned external exposure
 * (#1611). It PATCHes the primary facade's expose; the operator creates/removes the
 * HTTPRoute (#1553) and #1559 surfaces the resulting URL above. Editor-gated;
 * warns that exposure ≠ auth.
 */
function ExposeControl({ agent, workspace, onExposeChange }: Readonly<ConnectCardProps>) {
  const { isEditor } = useWorkspacePermissions();
  const { save, saving, error } = useSetAgentExpose(workspace, agent.metadata.name);

  const current = primaryFacade(agent.spec)?.expose;
  const currentEnabled = current?.enabled ?? false;
  const currentHost = current?.host ?? "";
  const [enabled, setEnabled] = useState(currentEnabled);
  const [host, setHost] = useState(currentHost);

  const dirty = enabled !== currentEnabled || host.trim() !== currentHost;
  const hasExternalAuth = Boolean(agent.spec.externalAuth);
  const hasEndpoints = (agent.status?.facade?.endpoints ?? []).length > 0;

  async function onSave() {
    if (await save(enabled, host)) onExposeChange?.();
  }

  return (
    <div className="rounded-md border p-3 space-y-3">
      <div className="flex items-center justify-between gap-3">
        <div className="space-y-0.5">
          <p className="text-sm font-medium">Expose externally</p>
          <p className="text-xs text-muted-foreground">
            Create an external route so apps outside the cluster can reach this agent.
          </p>
        </div>
        <Switch
          checked={enabled}
          onCheckedChange={setEnabled}
          disabled={!isEditor || saving}
          aria-label="Expose externally"
        />
      </div>

      {enabled && (
        <div className="space-y-1">
          <Label htmlFor="expose-host" className="text-xs">
            Host override (optional)
          </Label>
          <Input
            id="expose-host"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder="agent.example.com"
            disabled={!isEditor || saving}
            className="h-7 text-xs"
          />
        </div>
      )}

      {enabled && !hasExternalAuth && (
        <p className="flex items-start gap-1.5 text-xs text-warning">
          <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
          Exposing this agent does not authenticate it — anyone who can reach the URL
          can use it. Set spec.externalAuth to require a token.
        </p>
      )}

      {currentEnabled && !hasEndpoints && (
        <p className="text-xs text-muted-foreground">
          Waiting for the external route — the URL appears here once provisioned
          (requires a default-exposure Gateway configured by the platform).
        </p>
      )}

      {error && <p className="text-xs text-destructive">{error}</p>}

      {!isEditor && (
        <p className="text-xs text-muted-foreground">
          Editor access is required to change exposure.
        </p>
      )}
      {isEditor && dirty && (
        <Button size="sm" onClick={onSave} disabled={saving}>
          {saving ? "Saving…" : "Save"}
        </Button>
      )}
    </div>
  );
}

/**
 * ConnectCard surfaces the externally-reachable facade endpoints from
 * status.facade.endpoints, with an auth hint derived from spec.externalAuth, and
 * the opt-in external-exposure toggle (#1611).
 *
 * Distinct from the Console tab, which connects via the dashboard's internal
 * management-plane proxy — this card shows the URLs a customer app would use.
 */
export function ConnectCard({ agent, workspace, onExposeChange }: Readonly<ConnectCardProps>) {
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
      <CardContent className="space-y-3">
        <ExposeControl agent={agent} workspace={workspace} onExposeChange={onExposeChange} />
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
