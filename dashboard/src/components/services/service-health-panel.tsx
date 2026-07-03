"use client";

/**
 * Service health panel — per-service health grouped by service group, with
 * in-dashboard logs. Lets an operator see e.g. "memory-api is crashlooping,
 * session-api is fine" at a glance for a workspace.
 *
 * Polls GET /api/workspaces/:name/services on mount and every 10s. Each row
 * can expand an inline log view backed by
 * GET /api/workspaces/:name/services/:group/:service/logs.
 */

import { useState, useEffect, useCallback } from "react";
import { RefreshCw, ChevronDown, ChevronRight, AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { cn } from "@/lib/utils";
import type {
  ServiceHealth,
  ServiceState,
  ServiceGroupHealth,
  WorkspaceServicesHealth,
} from "@/lib/k8s/service-health";
import type { LogEntry } from "@/lib/data/types";

const POLL_INTERVAL_MS = 10_000;

/** Sentinel group segment for workspace-level services (e.g. privacy-api). */
const WORKSPACE_LEVEL_GROUP = "__workspace__";

interface ServiceHealthPanelProps {
  workspaceName: string;
  /** Group name to auto-expand on mount — used for deep links (?group=). */
  initialExpandedGroup?: string;
}

/** Lookup table for state -> badge label/style. No nested ternaries. */
const STATE_BADGE: Record<ServiceState, { label: string; className: string }> = {
  ready: {
    label: "✔ Ready",
    className: "bg-success/15 text-success border-success/30",
  },
  crashlooping: {
    label: "✖ Crashlooping",
    className: "bg-destructive/15 text-destructive border-destructive/30",
  },
  pending: {
    label: "⏳ Pending",
    className: "bg-warning/15 text-warning border-warning/30",
  },
  notDeployed: {
    label: "○ Not deployed",
    className: "bg-muted text-muted-foreground border-border",
  },
  unknown: {
    label: "? Unknown",
    className: "bg-muted text-muted-foreground border-border",
  },
};

function StatusBadge({ state }: Readonly<{ state: ServiceState }>) {
  const badge = STATE_BADGE[state];
  return (
    <Badge variant="outline" className={badge.className}>
      {badge.label}
    </Badge>
  );
}

/** Fetches and renders a lightweight inline log view for one service. */
function ServiceLogView({
  workspaceName,
  groupName,
  service,
}: Readonly<{ workspaceName: string; groupName: string; service: string }>) {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(
      `/api/workspaces/${workspaceName}/services/${groupName}/${service}/logs?tailLines=100`
    )
      .then((res) => {
        if (!res.ok) throw new Error(`Request failed: ${res.status}`);
        return res.json() as Promise<{ logs?: LogEntry[] }>;
      })
      .then((data) => {
        if (cancelled) return;
        setLogs(data.logs ?? []);
        setError(null);
      })
      .catch(() => {
        if (!cancelled) setError("Failed to load logs");
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceName, groupName, service]);

  if (isLoading) {
    return (
      <div className="p-2 text-sm text-muted-foreground">Loading logs...</div>
    );
  }
  if (error) {
    return <div className="p-2 text-sm text-destructive">{error}</div>;
  }
  if (logs.length === 0) {
    return (
      <div className="p-2 text-sm text-muted-foreground">No logs yet...</div>
    );
  }

  return (
    <div className="max-h-64 overflow-auto rounded border bg-muted/30 p-2 font-mono text-xs">
      {logs.map((log, index) => (
        <div
          // eslint-disable-next-line react/no-array-index-key -- log entries have no stable unique ID
          key={`${index}-${log.timestamp}`}
          className="flex gap-2 py-0.5"
        >
          <span className="shrink-0 text-muted-foreground">{log.timestamp}</span>
          <span className="w-12 shrink-0 uppercase text-muted-foreground">
            {log.level}
          </span>
          <span className="break-all">{log.message}</span>
        </div>
      ))}
    </div>
  );
}

interface ServiceRowProps {
  workspaceName: string;
  groupName: string;
  service: ServiceHealth;
}

/** One service row: badge, name, restart count, reason, and a logs toggle. */
function ServiceRow({ workspaceName, groupName, service }: Readonly<ServiceRowProps>) {
  const [logsOpen, setLogsOpen] = useState(false);

  return (
    <div
      className="flex flex-col gap-2 border-b py-2 last:border-b-0"
      data-testid={`service-row-${service.service}`}
    >
      <div className="flex flex-wrap items-center gap-3">
        <StatusBadge state={service.state} />
        <span className="font-medium">{service.service}</span>
        <span className="text-sm text-muted-foreground">
          restarts: {service.restarts}
        </span>
        {service.reason && (
          <span
            className="max-w-md truncate text-sm text-muted-foreground"
            title={service.reason}
          >
            {service.reason}
          </span>
        )}
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto"
          onClick={() => setLogsOpen((open) => !open)}
        >
          [logs]
        </Button>
      </div>
      {logsOpen && (
        <ServiceLogView
          workspaceName={workspaceName}
          groupName={groupName}
          service={service.service}
        />
      )}
    </div>
  );
}

/** First non-ready member of a group — surfaced as "blocked by: <service>". */
function firstNonReadyMember(members: ServiceHealth[]): ServiceHealth | undefined {
  return members.find((member) => !member.ready);
}

interface GroupSectionProps {
  workspaceName: string;
  group: ServiceGroupHealth;
  defaultOpen: boolean;
}

/** A collapsible section for one service group (e.g. session-api + memory-api). */
function GroupSection({ workspaceName, group, defaultOpen }: Readonly<GroupSectionProps>) {
  const [open, setOpen] = useState(defaultOpen || !group.ready);
  const blocker = group.ready ? undefined : firstNonReadyMember(group.members);

  return (
    <Card>
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="flex w-full items-center justify-between px-4 py-3 text-left"
          >
            <div className="flex flex-wrap items-center gap-3">
              {open ? (
                <ChevronDown className="h-4 w-4 text-muted-foreground" />
              ) : (
                <ChevronRight className="h-4 w-4 text-muted-foreground" />
              )}
              <span className="font-medium">{group.name}</span>
              <Badge variant={group.ready ? "default" : "destructive"}>
                {group.ready ? "Ready" : "Not ready"}
              </Badge>
              {blocker && (
                <span className="text-sm text-muted-foreground">
                  blocked by: {blocker.service}
                </span>
              )}
            </div>
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <CardContent className="pt-0">
            {group.members.map((member) => (
              <ServiceRow
                key={member.service}
                workspaceName={workspaceName}
                groupName={group.name}
                service={member}
              />
            ))}
          </CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  );
}

/** Renders the workspace-level services section (e.g. privacy-api), if any. */
function WorkspaceServicesSection({
  workspaceName,
  services,
}: Readonly<{ workspaceName: string; services: ServiceHealth[] }>) {
  if (services.length === 0) return null;
  return (
    <Card>
      <CardContent className="pt-4">
        {services.map((service) => (
          <ServiceRow
            key={service.service}
            workspaceName={workspaceName}
            groupName={WORKSPACE_LEVEL_GROUP}
            service={service}
          />
        ))}
      </CardContent>
    </Card>
  );
}

export function ServiceHealthPanel({
  workspaceName,
  initialExpandedGroup,
}: Readonly<ServiceHealthPanelProps>) {
  const [data, setData] = useState<WorkspaceServicesHealth | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Fetches health without flipping isLoading back to true — used for the
  // initial load (isLoading already starts true) and background polling
  // (avoids flashing a loading state every 10s while data is refreshed).
  const fetchHealth = useCallback(() => {
    return fetch(`/api/workspaces/${workspaceName}/services`)
      .then((res) => {
        if (!res.ok) throw new Error(`Request failed: ${res.status}`);
        return res.json() as Promise<WorkspaceServicesHealth>;
      })
      .then((json) => {
        setData(json);
        setError(null);
      })
      .catch(() => {
        setError("Failed to load service health");
      })
      .finally(() => {
        setIsLoading(false);
      });
  }, [workspaceName]);

  useEffect(() => {
    fetchHealth();
    const interval = setInterval(fetchHealth, POLL_INTERVAL_MS);
    return () => clearInterval(interval);
  }, [fetchHealth]);

  // User-initiated refresh: explicitly shows the loading state, since this
  // isn't inside an effect body.
  const handleRefresh = useCallback(() => {
    setIsLoading(true);
    fetchHealth();
  }, [fetchHealth]);

  const hasNoServices =
    !!data && data.workspaceServices.length === 0 && data.groups.length === 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Service health</h2>
        <Button
          variant="outline"
          size="sm"
          onClick={handleRefresh}
          disabled={isLoading}
        >
          <RefreshCw className={cn("mr-1 h-4 w-4", isLoading && "animate-spin")} />
          Refresh
        </Button>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Could not load service health</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {hasNoServices && (
        <div className="py-8 text-center text-sm text-muted-foreground">
          No services reported for this workspace.
        </div>
      )}

      {data && (
        <WorkspaceServicesSection
          workspaceName={workspaceName}
          services={data.workspaceServices}
        />
      )}

      {data?.groups.map((group) => (
        <GroupSection
          key={group.name}
          workspaceName={workspaceName}
          group={group}
          defaultOpen={group.name === initialExpandedGroup}
        />
      ))}
    </div>
  );
}
