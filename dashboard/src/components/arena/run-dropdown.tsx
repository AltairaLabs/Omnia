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
import { Play, ChevronDown, Loader2, Beaker, Zap, Database } from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { useProjectJobsWithRun } from "@/hooks/use-project-jobs";
import { useProjectDeployment } from "@/hooks/use-project-deployment";
import type { ArenaJobType } from "@/types/arena";
import { cn } from "@/lib/utils";
import { QuickRunDialog } from "./quick-run-dialog";

interface RunDropdownProps {
  readonly projectId: string | undefined;
  readonly disabled?: boolean;
  readonly className?: string;
  readonly onJobCreated?: (jobName: string) => void;
}

const JOB_TYPE_ICONS: Record<ArenaJobType, React.ReactNode> = {
  evaluation: <Beaker className="h-4 w-4" />,
  loadtest: <Zap className="h-4 w-4" />,
  datagen: <Database className="h-4 w-4" />,
};

const JOB_TYPE_LABELS: Record<ArenaJobType, string> = {
  evaluation: "Evaluation",
  loadtest: "Load Test",
  datagen: "Data Generation",
};

/**
 * Run button with dropdown for quick-run options.
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
  const [selectedType, setSelectedType] = useState<ArenaJobType | null>(null);

  const handleQuickRun = useCallback(
    async (type: ArenaJobType) => {
      if (!projectId) return;

      // Auto-deploy if not deployed (use && so that if either endpoint
      // confirms deployment we skip the unnecessary re-deploy)
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
        const result = await run({ type });
        toast({
          title: "Job Started",
          description: `${JOB_TYPE_LABELS[type]} job "${result.job.metadata.name}" created`,
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

  const handleOpenDialog = useCallback((type: ArenaJobType) => {
    setSelectedType(type);
    setDialogOpen(true);
  }, []);

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
          <DropdownMenuItem onClick={() => handleQuickRun("evaluation")}>
            {JOB_TYPE_ICONS.evaluation}
            <span className="ml-2">Quick Evaluation</span>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => handleQuickRun("loadtest")}>
            {JOB_TYPE_ICONS.loadtest}
            <span className="ml-2">Quick Load Test</span>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => handleQuickRun("datagen")}>
            {JOB_TYPE_ICONS.datagen}
            <span className="ml-2">Quick Data Gen</span>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => handleOpenDialog("evaluation")}>
            {JOB_TYPE_ICONS.evaluation}
            <span className="ml-2">Evaluation with Options...</span>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => handleOpenDialog("loadtest")}>
            {JOB_TYPE_ICONS.loadtest}
            <span className="ml-2">Load Test with Options...</span>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => handleOpenDialog("datagen")}>
            {JOB_TYPE_ICONS.datagen}
            <span className="ml-2">Data Gen with Options...</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {selectedType && (
        <QuickRunDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          projectId={projectId}
          type={selectedType}
          onJobCreated={onJobCreated}
        />
      )}
    </>
  );
}
