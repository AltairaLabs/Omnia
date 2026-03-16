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
import { Play, ChevronDown, Loader2, Beaker, Settings } from "lucide-react";
import { useToast } from "@/hooks/core";
import { useProjectJobsWithRun, useProjectDeployment } from "@/hooks/arena";
import { cn } from "@/lib/utils";
import { QuickRunDialog } from "./quick-run-dialog";

interface RunDropdownProps {
  readonly projectId: string | undefined;
  readonly disabled?: boolean;
  readonly className?: string;
  readonly onJobCreated?: (jobName: string) => void;
}

/**
 * Run button with dropdown for quick-run and run-with-options.
 * Always runs as an evaluation job.
 */
export function RunDropdown({
  projectId,
  disabled,
  className,
  onJobCreated,
}: RunDropdownProps) {
  const { toast } = useToast();
  const { deployed, running, run } = useProjectJobsWithRun(projectId);
  const { status: deploymentStatus, deploying, deploy } = useProjectDeployment(projectId);
  const [dialogOpen, setDialogOpen] = useState(false);

  const handleQuickRun = useCallback(
    async () => {
      if (!projectId) return;

      if (!deployed && !deploymentStatus?.deployed) {
        try {
          toast({
            title: "Deploying",
            description: "Project not deployed. Deploying first...",
          });
          await deploy();
        } catch (err) {
          toast({
            title: "Deploy Failed",
            description: err instanceof Error ? err.message : "Failed to deploy project",
            variant: "destructive",
          });
          return;
        }
      }

      try {
        const result = await run({ type: "evaluation" });
        toast({
          title: "Job Started",
          description: `Evaluation job "${result.job.metadata.name}" created`,
        });
        onJobCreated?.(result.job.metadata.name);
      } catch (err) {
        toast({
          title: "Run Failed",
          description: err instanceof Error ? err.message : "Failed to start job",
          variant: "destructive",
        });
      }
    },
    [projectId, deployed, deploymentStatus, deploy, run, toast, onJobCreated]
  );

  const isDisabled = disabled || !projectId || running || deploying;

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
            {running || deploying ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Play className="h-4 w-4" />
            )}
            Run
            <ChevronDown className="h-3 w-3 ml-1" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-[200px]">
          <DropdownMenuItem onClick={handleQuickRun}>
            <Beaker className="h-4 w-4" />
            <span className="ml-2">Quick Run</span>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => setDialogOpen(true)}>
            <Settings className="h-4 w-4" />
            <span className="ml-2">Run with Options...</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <QuickRunDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        projectId={projectId}
        onJobCreated={onJobCreated}
      />
    </>
  );
}
