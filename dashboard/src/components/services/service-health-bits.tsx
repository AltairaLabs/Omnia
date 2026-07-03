"use client";

/**
 * Shared per-service health UI: a state->badge lookup, a status badge, a
 * service row (badge + restarts + reason + logs toggle), and an inline log
 * view. Used by the workspace settings Services tab to show session-api /
 * memory-api / privacy-api health without duplicating this rendering logic.
 */

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import type { ServiceHealth, ServiceState } from "@/lib/k8s/service-health";
import type { LogEntry } from "@/lib/data/types";

/** Sentinel group segment for workspace-level services (e.g. privacy-api). */
export const WORKSPACE_LEVEL_GROUP = "__workspace__";

/** Lookup table for state -> badge label/style. No nested ternaries. */
export const STATE_BADGE: Record<ServiceState, { label: string; className: string }> = {
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

export function StatusBadge({ state }: Readonly<{ state: ServiceState }>) {
  const badge = STATE_BADGE[state];
  return (
    <Badge variant="outline" className={badge.className}>
      {badge.label}
    </Badge>
  );
}

/** Fetches and renders a lightweight inline log view for one service. */
export function ServiceLogView({
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

export interface ServiceRowProps {
  workspaceName: string;
  groupName: string;
  service: ServiceHealth;
}

/** One service row: badge, name, restart count, reason, and a logs toggle. */
export function ServiceRow({ workspaceName, groupName, service }: Readonly<ServiceRowProps>) {
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
