"use client";

import { useState, useCallback, useEffect } from "react";
import { Minus, Plus, Loader2, Zap, Lock } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { useReadOnly, usePermissions, Permission } from "@/hooks";

interface ScaleControlProps {
  currentReplicas: number;
  desiredReplicas: number;
  minReplicas?: number;
  maxReplicas?: number;
  autoscalingEnabled?: boolean;
  autoscalingType?: "hpa" | "keda";
  onScale: (replicas: number) => Promise<void>;
  /** Optional refetch function to poll for updates while in optimistic mode */
  refetch?: () => void;
  className?: string;
  compact?: boolean;
}

interface ConfirmAction {
  replicas: number;
  type: "scale-down" | "scale-to-zero";
}

/** Helper to compute the disabled message */
function getDisabledMessage(isReadOnly: boolean, readOnlyMessage: string, canScale: boolean): string {
  if (isReadOnly) return readOnlyMessage;
  if (!canScale) return "You don't have permission to scale agents";
  return "";
}

/** Autoscaling indicator tooltip */
function AutoscalingIndicator({
  type,
  minReplicas,
  maxReplicas,
  compact,
}: Readonly<{
  type?: string;
  minReplicas: number;
  maxReplicas: number;
  compact: boolean;
}>) {
  if (compact) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <div className="mr-1">
            <Zap className="h-3.5 w-3.5 text-yellow-500" />
          </div>
        </TooltipTrigger>
        <TooltipContent>
          <p>Autoscaling enabled ({type?.toUpperCase()})</p>
          <p className="text-xs text-muted-foreground">Range: {minReplicas} - {maxReplicas}</p>
        </TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-yellow-500/10 text-yellow-600 dark:text-yellow-400">
          <Zap className="h-3 w-3" />
          <span className="text-xs font-medium">{type?.toUpperCase()}</span>
        </div>
      </TooltipTrigger>
      <TooltipContent>
        <p>Autoscaling is enabled</p>
        <p className="text-xs text-muted-foreground">Range: {minReplicas} - {maxReplicas} replicas</p>
      </TooltipContent>
    </Tooltip>
  );
}

/** Get the dialog title based on action type and compact mode */
function getDialogTitle(isScaleToZero: boolean, compact: boolean): string {
  if (isScaleToZero) return "Scale to Zero?";
  if (compact) return "Scale Down?";
  return "Confirm Scale Down";
}

/** Get the dialog description based on action type and compact mode */
function getDialogDescription(
  isScaleToZero: boolean,
  compact: boolean,
  desiredReplicas: number,
  targetReplicas: number | undefined
): React.ReactNode {
  if (isScaleToZero && compact) {
    return "This will stop all instances of the agent. The agent will not be able to process requests until scaled back up.";
  }
  if (isScaleToZero) {
    return (
      <>
        This will stop all instances of the agent. The agent will not be
        able to process any requests until scaled back up.
        <br /><br />
        Are you sure you want to continue?
      </>
    );
  }
  if (compact) {
    return `This will reduce the agent from ${desiredReplicas} to ${targetReplicas} replica(s).`;
  }
  return (
    <>
      This will reduce the number of replicas from{" "}
      <strong>{desiredReplicas}</strong> to{" "}
      <strong>{targetReplicas}</strong>.
      <br /><br />
      This may affect the agent&apos;s ability to handle traffic.
    </>
  );
}

/** Confirmation dialog for scale actions */
function ScaleConfirmDialog({
  open,
  onOpenChange,
  confirmAction,
  desiredReplicas,
  onConfirm,
  onCancel,
  compact,
}: Readonly<{
  open: boolean;
  onOpenChange: (open: boolean) => void;
  confirmAction: ConfirmAction | null;
  desiredReplicas: number;
  onConfirm: () => void;
  onCancel: () => void;
  compact: boolean;
}>) {
  const isScaleToZero = confirmAction?.type === "scale-to-zero";
  const title = getDialogTitle(isScaleToZero, compact);
  const buttonLabel = isScaleToZero ? "Scale to Zero" : "Scale Down";
  const description = getDialogDescription(isScaleToZero, compact, desiredReplicas, confirmAction?.replicas);

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>
            {description}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={onCancel}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className={isScaleToZero && !compact ? "bg-destructive text-destructive-foreground hover:bg-destructive/90" : ""}
          >
            {buttonLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function ScaleControl({
  currentReplicas,
  desiredReplicas,
  minReplicas = 0,
  maxReplicas = 10,
  autoscalingEnabled = false,
  autoscalingType,
  onScale,
  refetch,
  className,
  compact = false,
}: Readonly<ScaleControlProps>) {
  const { isReadOnly, message: readOnlyMessage } = useReadOnly();
  const { can } = usePermissions();
  const canScale = can(Permission.AGENTS_SCALE);
  const isDisabled = isReadOnly || !canScale;
  const disabledMessage = getDisabledMessage(isReadOnly, readOnlyMessage, canScale);

  const [isScaling, setIsScaling] = useState(false);
  // Optimistic value: what we want the desired replicas to be
  // This persists until the actual desiredReplicas prop matches it
  const [optimisticDesired, setOptimisticDesired] = useState<number | null>(null);
  const [showConfirmDialog, setShowConfirmDialog] = useState(false);
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null);

  // Clear optimistic state when the actual desiredReplicas catches up
  useEffect(() => {
    if (optimisticDesired !== null && desiredReplicas === optimisticDesired) {
      setOptimisticDesired(null);
    }
  }, [desiredReplicas, optimisticDesired]);

  // Poll for updates while in optimistic mode
  useEffect(() => {
    if (optimisticDesired === null || !refetch) return;

    const interval = setInterval(() => {
      refetch();
    }, 1000); // Poll every second

    return () => clearInterval(interval);
  }, [optimisticDesired, refetch]);

  const executeScale = useCallback(async (replicas: number) => {
    // Immediately show optimistic update
    setOptimisticDesired(replicas);
    setIsScaling(true);
    try {
      await onScale(replicas);
    } finally {
      setIsScaling(false);
      // Don't clear optimisticDesired here - let it persist until CRD updates
    }
  }, [onScale]);

  const handleScale = useCallback(async (newReplicas: number) => {
    const clampedReplicas = Math.max(minReplicas, Math.min(maxReplicas, newReplicas));

    // Scale to zero needs confirmation
    if (clampedReplicas === 0 && desiredReplicas > 0) {
      setConfirmAction({ replicas: 0, type: "scale-to-zero" });
      setShowConfirmDialog(true);
      return;
    }

    // Scale down needs confirmation
    if (clampedReplicas < desiredReplicas && clampedReplicas > 0) {
      setConfirmAction({ replicas: clampedReplicas, type: "scale-down" });
      setShowConfirmDialog(true);
      return;
    }

    // Scale up executes directly
    await executeScale(clampedReplicas);
  }, [desiredReplicas, minReplicas, maxReplicas, executeScale]);

  const confirmScale = useCallback(async () => {
    if (!confirmAction) return;
    setShowConfirmDialog(false);
    await executeScale(confirmAction.replicas);
    setConfirmAction(null);
  }, [confirmAction, executeScale]);

  const cancelConfirm = useCallback(() => {
    setShowConfirmDialog(false);
    setConfirmAction(null);
  }, []);

  // Use optimistic value if set, otherwise use actual desired replicas
  const displayDesired = optimisticDesired ?? desiredReplicas;
  const isOptimistic = optimisticDesired !== null;
  const canScaleDown = displayDesired > minReplicas && !isScaling && !isDisabled;
  const canScaleUp = displayDesired < maxReplicas && !isScaling && !isDisabled;

  // Build the replica display with optimistic styling
  const renderReplicaCount = (showSpinner: boolean = false) => {
    if (showSpinner && isScaling) {
      return <Loader2 className="h-4 w-4 animate-spin mx-auto" />;
    }
    return (
      <span className="min-w-[3rem] text-center text-sm font-medium">
        {currentReplicas}/
        <span className={isOptimistic ? "text-muted-foreground/60" : ""}>
          {displayDesired}
        </span>
      </span>
    );
  };

  // Compact view
  if (compact) {
    // When autoscaling is enabled, show only the autoscaling indicator and replica count
    if (autoscalingEnabled) {
      return (
        <TooltipProvider>
          <div className={cn("flex items-center gap-1", className)}>
            <AutoscalingIndicator type={autoscalingType} minReplicas={minReplicas} maxReplicas={maxReplicas} compact />
            {renderReplicaCount()}
          </div>
        </TooltipProvider>
      );
    }

    return (
      <TooltipProvider>
        <div className={cn("flex items-center gap-1", className)}>
          {isDisabled ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex items-center gap-1 text-muted-foreground">
                  <Lock className="h-3 w-3" />
                  {renderReplicaCount()}
                </div>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs">
                <p className="text-sm">{disabledMessage}</p>
              </TooltipContent>
            </Tooltip>
          ) : (
            <>
              <Button variant="outline" size="icon" className="h-6 w-6" onClick={() => handleScale(displayDesired - 1)} disabled={!canScaleDown}>
                <Minus className="h-3 w-3" />
              </Button>
              {renderReplicaCount()}
              <Button variant="outline" size="icon" className="h-6 w-6" onClick={() => handleScale(displayDesired + 1)} disabled={!canScaleUp}>
                <Plus className="h-3 w-3" />
              </Button>
            </>
          )}

          <ScaleConfirmDialog
            open={showConfirmDialog}
            onOpenChange={setShowConfirmDialog}
            confirmAction={confirmAction}
            desiredReplicas={desiredReplicas}
            onConfirm={confirmScale}
            onCancel={cancelConfirm}
            compact
          />
        </div>
      </TooltipProvider>
    );
  }

  // Full view - when autoscaling is enabled, show label without buttons
  if (autoscalingEnabled) {
    return (
      <TooltipProvider>
        <div className={cn("flex flex-col gap-2", className)}>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className="text-sm text-muted-foreground">Replicas</span>
              <AutoscalingIndicator type={autoscalingType} minReplicas={minReplicas} maxReplicas={maxReplicas} compact={false} />
            </div>
            <div className="text-lg font-semibold">
              <span>
                {currentReplicas}
                <span className={cn("text-muted-foreground", isOptimistic && "opacity-50")}>
                  /{displayDesired}
                </span>
              </span>
            </div>
          </div>
          <p className="text-xs text-muted-foreground">Scaling is managed by {autoscalingType?.toUpperCase() || "autoscaler"}</p>
        </div>
      </TooltipProvider>
    );
  }

  // Full view - manual scaling
  return (
    <TooltipProvider>
      <div className={cn("flex flex-col gap-2", className)}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Replicas</span>
            {isDisabled && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <div className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                    <Lock className="h-3 w-3" />
                    <span className="text-xs font-medium">{isReadOnly ? "Read Only" : "No Permission"}</span>
                  </div>
                </TooltipTrigger>
                <TooltipContent className="max-w-xs">
                  <p className="text-sm">{disabledMessage}</p>
                </TooltipContent>
              </Tooltip>
            )}
          </div>
          <div className="text-lg font-semibold">
            <span>
              {currentReplicas}
              <span className={cn("text-muted-foreground", isOptimistic && "opacity-50")}>
                /{displayDesired}
              </span>
              {isScaling && <Loader2 className="h-4 w-4 animate-spin inline ml-2" />}
            </span>
          </div>
        </div>

        {isDisabled ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" className="flex-1 opacity-50" disabled>
                  <Minus className="h-4 w-4 mr-1" />Scale Down
                </Button>
                <Button variant="outline" size="sm" className="flex-1 opacity-50" disabled>
                  <Plus className="h-4 w-4 mr-1" />Scale Up
                </Button>
              </div>
            </TooltipTrigger>
            <TooltipContent className="max-w-xs">
              <p className="text-sm">{disabledMessage}</p>
            </TooltipContent>
          </Tooltip>
        ) : (
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" className="flex-1" onClick={() => handleScale(displayDesired - 1)} disabled={!canScaleDown}>
              <Minus className="h-4 w-4 mr-1" />Scale Down
            </Button>
            <Button variant="outline" size="sm" className="flex-1" onClick={() => handleScale(displayDesired + 1)} disabled={!canScaleUp}>
              <Plus className="h-4 w-4 mr-1" />Scale Up
            </Button>
          </div>
        )}

        <ScaleConfirmDialog
          open={showConfirmDialog}
          onOpenChange={setShowConfirmDialog}
          confirmAction={confirmAction}
          desiredReplicas={desiredReplicas}
          onConfirm={confirmScale}
          onCancel={cancelConfirm}
          compact={false}
        />
      </div>
    </TooltipProvider>
  );
}
