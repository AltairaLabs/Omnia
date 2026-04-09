import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import type { ServiceGroupStatus, Workspace } from "@/types/workspace";
import { Copy, Info } from "lucide-react";

interface ServicesTabProps {
  workspace: Workspace;
}

interface StatusDotProps {
  ready: boolean;
}

function StatusDot({ ready }: StatusDotProps) {
  return (
    <span
      data-testid={ready ? "status-ready" : "status-not-ready"}
      className={`inline-block h-2 w-2 rounded-full ${ready ? "bg-green-500" : "bg-red-500"}`}
    />
  );
}

interface ServiceCardProps {
  label: string;
  url: string;
  ready: boolean;
  secretRefName: string | undefined;
}

function ServiceCard({ label, url, ready, secretRefName }: ServiceCardProps) {
  function copyUrl() {
    navigator.clipboard.writeText(url);
  }

  return (
    <div className="rounded-md border p-4 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{label}</span>
        <StatusDot ready={ready} />
      </div>
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground truncate flex-1 font-mono">
          {url}
        </span>
        <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" onClick={copyUrl} aria-label={`Copy ${label} URL`}>
          <Copy className="h-3 w-3" />
        </Button>
      </div>
      {secretRefName && (
        <div className="text-xs text-muted-foreground">
          Secret: <span className="font-mono">{secretRefName}</span>
        </div>
      )}
    </div>
  );
}

export function ServicesTab({ workspace }: ServicesTabProps) {
  const specServices = workspace.spec.services;
  const statusServices = workspace.status?.services ?? [];

  if (!specServices || specServices.length === 0) {
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

  if (statusServices.length === 0) {
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

  const statusByName = new Map<string, ServiceGroupStatus>(
    statusServices.map((s) => [s.name, s])
  );

  return (
    <div className="space-y-6">
      {specServices.map((group) => {
        const groupStatus = statusByName.get(group.name);
        const sessionURL = groupStatus?.sessionURL ?? "";
        const memoryURL = groupStatus?.memoryURL ?? "";
        const ready = groupStatus?.ready ?? false;

        return (
          <Card key={group.name}>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                {group.name}
                <Badge variant="outline">{group.mode}</Badge>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <ServiceCard
                  label="Session API"
                  url={sessionURL}
                  ready={ready}
                  secretRefName={group.session?.database?.secretRef?.name}
                />
                <ServiceCard
                  label="Memory API"
                  url={memoryURL}
                  ready={ready}
                  secretRefName={group.memory?.database?.secretRef?.name}
                />
              </div>
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
