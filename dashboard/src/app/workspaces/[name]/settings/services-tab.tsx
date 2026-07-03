"use client";

/**
 * Workspace settings "Services" tab.
 *
 * Renders one card per configured service group (session-api + memory-api),
 * plus a workspace-level privacy-api card. Each service gets its OWN health
 * badge/restarts/reason/logs — driven by GET /api/workspaces/:name/services
 * — instead of a single group-level ready flag applied to every member.
 */

import { useEffect, useState } from "react";
import { AlertCircle, Copy, Info } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  ServiceRow,
  WORKSPACE_LEVEL_GROUP,
} from "@/components/services/service-health-bits";
import type {
  ServiceGroupHealth,
  ServiceHealth,
  WorkspaceServicesHealth,
} from "@/lib/k8s/service-health";
import type { ServiceGroupStatus, Workspace, WorkspaceSpec } from "@/types/workspace";

const SESSION_COMPONENT = "session-api";
const MEMORY_COMPONENT = "memory-api";

type ServiceGroupSpec = NonNullable<WorkspaceSpec["services"]>[number];

interface ServicesTabProps {
  workspace: Workspace;
}

/** Fetches per-service health for a workspace. No-op while `enabled` is false. */
function useServiceHealth(workspaceName: string, enabled: boolean) {
  const [data, setData] = useState<WorkspaceServicesHealth | null>(null);
  const [isLoading, setIsLoading] = useState(enabled);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) return undefined;

    let cancelled = false;
    fetch(`/api/workspaces/${workspaceName}/services`)
      .then((res) => {
        if (!res.ok) throw new Error(`Request failed: ${res.status}`);
        return res.json() as Promise<WorkspaceServicesHealth>;
      })
      .then((json) => {
        if (cancelled) return;
        setData(json);
        setError(null);
      })
      .catch(() => {
        if (!cancelled) setError("Failed to load service health");
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [workspaceName, enabled]);

  return { data, isLoading, error };
}

/** Looks up a member's health within a group, falling back to "unknown". */
function resolveHealth(
  group: ServiceGroupHealth | undefined,
  serviceName: string,
  url: string | undefined
): ServiceHealth {
  const member = group?.members.find((m) => m.service === serviceName);
  if (member) return member;
  return { service: serviceName, url, state: "unknown", ready: false, restarts: 0 };
}

interface ServiceDetailProps {
  workspaceName: string;
  groupName: string;
  label: string;
  url: string;
  secretRefName: string | undefined;
  health: ServiceHealth;
}

/** URL + copy button + secret ref + the shared per-service health row. */
function ServiceDetail({
  workspaceName,
  groupName,
  label,
  url,
  secretRefName,
  health,
}: Readonly<ServiceDetailProps>) {
  function copyUrl() {
    navigator.clipboard.writeText(url);
  }

  return (
    <div className="rounded-md border p-4 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{label}</span>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground truncate flex-1 font-mono">
          {url}
        </span>
        <Button
          variant="ghost"
          size="icon"
          className="h-6 w-6 shrink-0"
          onClick={copyUrl}
          aria-label={`Copy ${label} URL`}
        >
          <Copy className="h-3 w-3" />
        </Button>
      </div>
      {secretRefName && (
        <div className="text-xs text-muted-foreground">
          Secret: <span className="font-mono">{secretRefName}</span>
        </div>
      )}
      <ServiceRow workspaceName={workspaceName} groupName={groupName} service={health} />
    </div>
  );
}

interface ServiceGroupCardProps {
  workspaceName: string;
  group: ServiceGroupSpec;
  groupStatus: ServiceGroupStatus | undefined;
  groupHealth: ServiceGroupHealth | undefined;
}

/** One service-group card: group name/mode header + per-service details. */
function ServiceGroupCard({
  workspaceName,
  group,
  groupStatus,
  groupHealth,
}: Readonly<ServiceGroupCardProps>) {
  const sessionHealth = resolveHealth(groupHealth, SESSION_COMPONENT, groupStatus?.sessionURL);
  const memoryHealth = resolveHealth(groupHealth, MEMORY_COMPONENT, groupStatus?.memoryURL);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {group.name}
          <Badge variant="outline">{group.mode}</Badge>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-2 gap-4">
          <ServiceDetail
            workspaceName={workspaceName}
            groupName={group.name}
            label="Session API"
            url={sessionHealth.url ?? groupStatus?.sessionURL ?? ""}
            secretRefName={group.session?.database?.secretRef?.name}
            health={sessionHealth}
          />
          <ServiceDetail
            workspaceName={workspaceName}
            groupName={group.name}
            label="Memory API"
            url={memoryHealth.url ?? groupStatus?.memoryURL ?? ""}
            secretRefName={group.memory?.database?.secretRef?.name}
            health={memoryHealth}
          />
        </div>
      </CardContent>
    </Card>
  );
}

/** Workspace-level services (currently just privacy-api), if any were reported. */
function WorkspaceServicesCard({
  workspaceName,
  services,
}: Readonly<{ workspaceName: string; services: ServiceHealth[] }>) {
  if (services.length === 0) return null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Workspace services</CardTitle>
      </CardHeader>
      <CardContent>
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

function NoServiceGroupsAlert() {
  return (
    <Alert>
      <Info className="h-4 w-4" />
      <AlertTitle>No service groups configured</AlertTitle>
      <AlertDescription>
        Add service groups to the Workspace CRD spec to provision session-api
        and memory-api for this workspace.
      </AlertDescription>
    </Alert>
  );
}

function ProvisioningAlert() {
  return (
    <Alert>
      <Info className="h-4 w-4" />
      <AlertTitle>Services being provisioned</AlertTitle>
      <AlertDescription>
        Service groups are defined but not yet ready. The controller is
        provisioning session-api and memory-api.
      </AlertDescription>
    </Alert>
  );
}

export function ServicesTab({ workspace }: Readonly<ServicesTabProps>) {
  const specServices = workspace.spec.services;
  const statusServices = workspace.status?.services ?? [];
  const workspaceName = workspace.metadata.name;
  const hasServices = !!specServices && specServices.length > 0 && statusServices.length > 0;

  const { data: health, isLoading, error } = useServiceHealth(workspaceName, hasServices);

  if (!specServices || specServices.length === 0) {
    return <NoServiceGroupsAlert />;
  }
  if (statusServices.length === 0) {
    return <ProvisioningAlert />;
  }

  const statusByName = new Map<string, ServiceGroupStatus>(
    statusServices.map((s) => [s.name, s])
  );
  const healthGroupsByName = new Map<string, ServiceGroupHealth>(
    (health?.groups ?? []).map((g) => [g.name, g])
  );

  return (
    <div className="space-y-6">
      {isLoading && (
        <div className="space-y-4" data-testid="services-health-loading">
          <Skeleton className="h-48 rounded-lg" />
        </div>
      )}

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Could not load service health</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {!isLoading &&
        specServices.map((group) => (
          <ServiceGroupCard
            key={group.name}
            workspaceName={workspaceName}
            group={group}
            groupStatus={statusByName.get(group.name)}
            groupHealth={healthGroupsByName.get(group.name)}
          />
        ))}

      {!isLoading && (
        <WorkspaceServicesCard
          workspaceName={workspaceName}
          services={health?.workspaceServices ?? []}
        />
      )}
    </div>
  );
}
