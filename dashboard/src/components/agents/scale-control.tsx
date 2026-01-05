"use client";

import { useState, useCallback } from "react";
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
import { useReadOnly } from "@/hooks";

interface ScaleControlProps {
  currentReplicas: number;
  desiredReplicas: number;
  minReplicas?: number;
  maxReplicas?: number;
  autoscalingEnabled?: boolean;
  autoscalingType?: "hpa" | "keda";
  onScale: (replicas: number) => Promise<void>;
  className?: string;
  compact?: boolean;
}

export function ScaleControl({
  currentReplicas,
  desiredReplicas,
  minReplicas = 0,
  maxReplicas = 10,
  autoscalingEnabled = false,
  autoscalingType,
  onScale,
  className,
  compact = false,
}: ScaleControlProps) {
  const { isReadOnly, message: readOnlyMessage } = useReadOnly();
  const [isScaling, setIsScaling] = useState(false);
  const [pendingScale, setPendingScale] = useState<number | null>(null);
  const [showConfirmDialog, setShowConfirmDialog] = useState(false);
  const [confirmAction, setConfirmAction] = useState<{
    replicas: number;
    type: "scale-down" | "scale-to-zero";
  } | null>(null);

  const handleScale = useCallback(
    async (newReplicas: number) => {
      // Clamp to valid range
      const clampedReplicas = Math.max(minReplicas, Math.min(maxReplicas, newReplicas));

      // Check if we need confirmation
      if (clampedReplicas === 0 && desiredReplicas > 0) {
        setConfirmAction({ replicas: 0, type: "scale-to-zero" });
        setShowConfirmDialog(true);
        return;
      }

      if (clampedReplicas < desiredReplicas && clampedReplicas > 0) {
        setConfirmAction({ replicas: clampedReplicas, type: "scale-down" });
        setShowConfirmDialog(true);
        return;
      }

      // Execute scale directly for scale up
      setIsScaling(true);
      setPendingScale(clampedReplicas);
      try {
        await onScale(clampedReplicas);
      } finally {
        setIsScaling(false);
        setPendingScale(null);
      }
    },
    [desiredReplicas, minReplicas, maxReplicas, onScale]
  );

  const executeScale = useCallback(
    async (replicas: number) => {
      setIsScaling(true);
      setPendingScale(replicas);
      try {
        await onScale(replicas);
      } finally {
        setIsScaling(false);
        setPendingScale(null);
      }
    },
    [onScale]
  );

  const confirmScale = useCallback(async () => {
    if (confirmAction) {
      setShowConfirmDialog(false);
      await executeScale(confirmAction.replicas);
      setConfirmAction(null);
    }
  }, [confirmAction, executeScale]);

  const cancelConfirm = useCallback(() => {
    setShowConfirmDialog(false);
    setConfirmAction(null);
  }, []);

  const displayReplicas = pendingScale ?? desiredReplicas;
  const canScaleDown = displayReplicas > minReplicas && !isScaling && !isReadOnly;
  const canScaleUp = displayReplicas < maxReplicas && !isScaling && !isReadOnly;

  if (compact) {
    return (
      <TooltipProvider>
        <div className={cn("flex items-center gap-1", className)}>
          {autoscalingEnabled && (
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="mr-1">
                  <Zap className="h-3.5 w-3.5 text-yellow-500" />
                </div>
              </TooltipTrigger>
              <TooltipContent>
                <p>Autoscaling enabled ({autoscalingType?.toUpperCase()})</p>
                <p className="text-xs text-muted-foreground">
                  Range: {minReplicas} - {maxReplicas}
                </p>
              </TooltipContent>
            </Tooltip>
          )}

          {isReadOnly ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex items-center gap-1 text-muted-foreground">
                  <Lock className="h-3 w-3" />
                  <span className="min-w-[3rem] text-center text-sm font-medium">
                    {currentReplicas}/{displayReplicas}
                  </span>
                </div>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs">
                <p className="text-sm">{readOnlyMessage}</p>
              </TooltipContent>
            </Tooltip>
          ) : (
            <>
              <Button
                variant="outline"
                size="icon"
                className="h-6 w-6"
                onClick={() => handleScale(displayReplicas - 1)}
                disabled={!canScaleDown}
              >
                <Minus className="h-3 w-3" />
              </Button>

              <span className="min-w-[3rem] text-center text-sm font-medium">
                {isScaling ? (
                  <Loader2 className="h-4 w-4 animate-spin mx-auto" />
                ) : (
                  `${currentReplicas}/${displayReplicas}`
                )}
              </span>

              <Button
                variant="outline"
                size="icon"
                className="h-6 w-6"
                onClick={() => handleScale(displayReplicas + 1)}
                disabled={!canScaleUp}
              >
                <Plus className="h-3 w-3" />
              </Button>
            </>
          )}

          <AlertDialog open={showConfirmDialog} onOpenChange={setShowConfirmDialog}>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {confirmAction?.type === "scale-to-zero"
                    ? "Scale to Zero?"
                    : "Scale Down?"}
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {confirmAction?.type === "scale-to-zero"
                    ? "This will stop all instances of the agent. The agent will not be able to process requests until scaled back up."
                    : `This will reduce the agent from ${desiredReplicas} to ${confirmAction?.replicas} replica(s).`}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel onClick={cancelConfirm}>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={confirmScale}>
                  {confirmAction?.type === "scale-to-zero" ? "Scale to Zero" : "Scale Down"}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </TooltipProvider>
    );
  }

  return (
    <TooltipProvider>
      <div className={cn("flex flex-col gap-2", className)}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">Replicas</span>
            {autoscalingEnabled && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <div className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-yellow-500/10 text-yellow-600 dark:text-yellow-400">
                    <Zap className="h-3 w-3" />
                    <span className="text-xs font-medium">{autoscalingType?.toUpperCase()}</span>
                  </div>
                </TooltipTrigger>
                <TooltipContent>
                  <p>Autoscaling is enabled</p>
                  <p className="text-xs text-muted-foreground">
                    Range: {minReplicas} - {maxReplicas} replicas
                  </p>
                </TooltipContent>
              </Tooltip>
            )}
            {isReadOnly && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <div className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                    <Lock className="h-3 w-3" />
                    <span className="text-xs font-medium">Read Only</span>
                  </div>
                </TooltipTrigger>
                <TooltipContent className="max-w-xs">
                  <p className="text-sm">{readOnlyMessage}</p>
                </TooltipContent>
              </Tooltip>
            )}
          </div>
          <div className="text-lg font-semibold">
            {isScaling ? (
              <div className="flex items-center gap-2">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span className="text-muted-foreground">{pendingScale}</span>
              </div>
            ) : (
              <span>
                {currentReplicas}
                <span className="text-muted-foreground">/{displayReplicas}</span>
              </span>
            )}
          </div>
        </div>

        {isReadOnly ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1 opacity-50"
                  disabled
                >
                  <Minus className="h-4 w-4 mr-1" />
                  Scale Down
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1 opacity-50"
                  disabled
                >
                  <Plus className="h-4 w-4 mr-1" />
                  Scale Up
                </Button>
              </div>
            </TooltipTrigger>
            <TooltipContent className="max-w-xs">
              <p className="text-sm">{readOnlyMessage}</p>
            </TooltipContent>
          </Tooltip>
        ) : (
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="flex-1"
              onClick={() => handleScale(displayReplicas - 1)}
              disabled={!canScaleDown}
            >
              <Minus className="h-4 w-4 mr-1" />
              Scale Down
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="flex-1"
              onClick={() => handleScale(displayReplicas + 1)}
              disabled={!canScaleUp}
            >
              <Plus className="h-4 w-4 mr-1" />
              Scale Up
            </Button>
          </div>
        )}

        {autoscalingEnabled && (
          <p className="text-xs text-muted-foreground">
            Manual scaling will be overridden by autoscaler
          </p>
        )}

        <AlertDialog open={showConfirmDialog} onOpenChange={setShowConfirmDialog}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>
                {confirmAction?.type === "scale-to-zero"
                  ? "Scale to Zero?"
                  : "Confirm Scale Down"}
              </AlertDialogTitle>
              <AlertDialogDescription>
                {confirmAction?.type === "scale-to-zero" ? (
                  <>
                    This will stop all instances of the agent. The agent will not be
                    able to process any requests until scaled back up.
                    <br />
                    <br />
                    Are you sure you want to continue?
                  </>
                ) : (
                  <>
                    This will reduce the number of replicas from{" "}
                    <strong>{desiredReplicas}</strong> to{" "}
                    <strong>{confirmAction?.replicas}</strong>.
                    <br />
                    <br />
                    This may affect the agent&apos;s ability to handle traffic.
                  </>
                )}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel onClick={cancelConfirm}>Cancel</AlertDialogCancel>
              <AlertDialogAction
                onClick={confirmScale}
                className={
                  confirmAction?.type === "scale-to-zero"
                    ? "bg-destructive text-destructive-foreground hover:bg-destructive/90"
                    : ""
                }
              >
                {confirmAction?.type === "scale-to-zero" ? "Scale to Zero" : "Scale Down"}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    </TooltipProvider>
  );
}
