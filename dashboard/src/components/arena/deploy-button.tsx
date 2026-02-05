"use client";

import { useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Rocket,
  ChevronDown,
  Loader2,
  CheckCircle2,
  XCircle,
  Clock,
  RefreshCw,
} from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import {
  useProjectDeployment,
  type DeployRequest,
  type DeploymentStatus,
} from "@/hooks/use-project-deployment";
import { cn } from "@/lib/utils";

interface DeployButtonProps {
  readonly projectId: string | undefined;
  readonly disabled?: boolean;
  readonly className?: string;
}

/**
 * Deploy button with dropdown for deployment options.
 * Shows deployment status indicator.
 */
export function DeployButton({ projectId, disabled, className }: DeployButtonProps) {
  const { toast } = useToast();
  const { status, loading, deploying, deploy, refetch } = useProjectDeployment(projectId);
  const [advancedDialogOpen, setAdvancedDialogOpen] = useState(false);

  const handleQuickDeploy = useCallback(async () => {
    if (!projectId) return;

    try {
      const result = await deploy();
      toast({
        title: result.isNew ? "Deployed" : "Redeployed",
        description: `Project deployed as "${result.source.metadata.name}"`,
      });
    } catch (err) {
      toast({
        title: "Deploy Failed",
        description: err instanceof Error ? err.message : "Failed to deploy project",
        variant: "destructive",
      });
    }
  }, [projectId, deploy, toast]);

  const handleAdvancedDeploy = useCallback(
    async (options: DeployRequest) => {
      if (!projectId) return;

      try {
        const result = await deploy(options);
        setAdvancedDialogOpen(false);
        toast({
          title: result.isNew ? "Deployed" : "Redeployed",
          description: `Project deployed as "${result.source.metadata.name}"`,
        });
      } catch (err) {
        toast({
          title: "Deploy Failed",
          description: err instanceof Error ? err.message : "Failed to deploy project",
          variant: "destructive",
        });
      }
    },
    [projectId, deploy, toast]
  );

  const isDisabled = disabled || !projectId || deploying;

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            disabled={isDisabled}
            className={cn("gap-2", className)}
          >
            {deploying ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Rocket className="h-4 w-4" />
            )}
            Deploy
            <DeployStatusIndicator status={status} loading={loading} />
            <ChevronDown className="h-3 w-3 ml-1" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={handleQuickDeploy} disabled={deploying}>
            <Rocket className="h-4 w-4 mr-2" />
            Quick Deploy
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => setAdvancedDialogOpen(true)}
            disabled={deploying}
          >
            <Clock className="h-4 w-4 mr-2" />
            Deploy with Options...
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={refetch} disabled={loading}>
            <RefreshCw className={cn("h-4 w-4 mr-2", loading && "animate-spin")} />
            Refresh Status
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <AdvancedDeployDialog
        open={advancedDialogOpen}
        onOpenChange={setAdvancedDialogOpen}
        status={status}
        deploying={deploying}
        onDeploy={handleAdvancedDeploy}
      />
    </>
  );
}

// =============================================================================
// Status Indicator
// =============================================================================

interface DeployStatusIndicatorProps {
  readonly status: DeploymentStatus | null;
  readonly loading: boolean;
}

function DeployStatusIndicator({ status, loading }: DeployStatusIndicatorProps) {
  if (loading) {
    return null;
  }

  if (!status?.deployed) {
    return null;
  }

  const phase = status.source?.status?.phase;

  if (phase === "Ready") {
    return (
      <span title="Deployed and ready">
        <CheckCircle2 className="h-3 w-3 text-green-500" />
      </span>
    );
  }

  if (phase === "Failed") {
    return (
      <span title="Deployment failed">
        <XCircle className="h-3 w-3 text-red-500" />
      </span>
    );
  }

  // Pending or unknown state
  return (
    <span title="Deployment pending">
      <Clock className="h-3 w-3 text-yellow-500" />
    </span>
  );
}

// =============================================================================
// Advanced Deploy Dialog
// =============================================================================

interface AdvancedDeployDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly status: DeploymentStatus | null;
  readonly deploying: boolean;
  readonly onDeploy: (options: DeployRequest) => Promise<void>;
}

function DeployButtonContent({
  deploying,
  deployed,
}: {
  readonly deploying: boolean;
  readonly deployed?: boolean;
}) {
  if (deploying) {
    return (
      <>
        <Loader2 className="h-4 w-4 mr-2 animate-spin" />
        Deploying...
      </>
    );
  }
  if (deployed) {
    return <>Redeploy</>;
  }
  return <>Deploy</>;
}

function AdvancedDeployDialog({
  open,
  onOpenChange,
  status,
  deploying,
  onDeploy,
}: AdvancedDeployDialogProps) {
  const [name, setName] = useState(status?.source?.metadata.name || "");
  const [syncInterval, setSyncInterval] = useState(
    status?.source?.spec.interval || "5m"
  );

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onDeploy({
      name: name || undefined,
      syncInterval: syncInterval || undefined,
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Deploy Project</DialogTitle>
          <DialogDescription>
            Deploy this project as an ArenaSource. This will create a ConfigMap
            with all project files and an ArenaSource referencing it.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="source-name">Source Name</Label>
            <Input
              id="source-name"
              placeholder="Auto-generated if empty"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={deploying}
            />
            <p className="text-xs text-muted-foreground">
              Name for the ArenaSource resource. Leave empty to auto-generate.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="sync-interval">Sync Interval</Label>
            <Input
              id="sync-interval"
              placeholder="5m"
              value={syncInterval}
              onChange={(e) => setSyncInterval(e.target.value)}
              disabled={deploying}
            />
            <p className="text-xs text-muted-foreground">
              How often to check for changes (e.g., &quot;5m&quot;, &quot;1h&quot;).
            </p>
          </div>

          {status?.deployed && (
            <div className="rounded-md bg-muted p-3 text-sm">
              <p className="font-medium">Currently Deployed</p>
              <p className="text-muted-foreground">
                Source: {status.source?.metadata.name}
              </p>
              <p className="text-muted-foreground">
                Phase: {status.source?.status?.phase || "Unknown"}
              </p>
              {status.configMap && (
                <p className="text-muted-foreground">
                  Files: {status.configMap.fileCount}
                </p>
              )}
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={deploying}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={deploying}>
              <DeployButtonContent deploying={deploying} deployed={status?.deployed} />
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
