"use client";

import { useState, useCallback } from "react";
import { Button } from "@/components/ui/button";
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
import { Checkbox } from "@/components/ui/checkbox";
import { Loader2 } from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { useProjectJobsWithRun, type QuickRunRequest } from "@/hooks/use-project-jobs";
import { useProjectDeployment } from "@/hooks/use-project-deployment";
import type { ArenaJobType } from "@/types/arena";

interface QuickRunDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string | undefined;
  type: ArenaJobType;
  onJobCreated?: (jobName: string) => void;
}

const JOB_TYPE_LABELS: Record<ArenaJobType, string> = {
  evaluation: "Evaluation",
  loadtest: "Load Test",
  datagen: "Data Generation",
};

const JOB_TYPE_DESCRIPTIONS: Record<ArenaJobType, string> = {
  evaluation: "Run evaluation scenarios to assess agent quality, correctness, and safety.",
  loadtest: "Stress test your agent with concurrent requests to validate performance.",
  datagen: "Generate synthetic conversation data for training and testing.",
};

/**
 * Parse comma-separated patterns into an array, filtering empty values.
 */
function parsePatterns(input: string): string[] {
  return input
    .split(",")
    .map((p) => p.trim())
    .filter(Boolean);
}

/**
 * Build scenario filter object from include/exclude patterns.
 */
function buildScenarioFilter(include: string[], exclude: string[]): { include?: string[]; exclude?: string[] } | undefined {
  if (include.length === 0 && exclude.length === 0) return undefined;
  const filter: { include?: string[]; exclude?: string[] } = {};
  if (include.length > 0) filter.include = include;
  if (exclude.length > 0) filter.exclude = exclude;
  return filter;
}

/**
 * Dialog for running jobs with custom options.
 */
export function QuickRunDialog({
  open,
  onOpenChange,
  projectId,
  type,
  onJobCreated,
}: QuickRunDialogProps) {
  const { toast } = useToast();
  const { deployed, running, run } = useProjectJobsWithRun(projectId);
  const { status: deploymentStatus, deploying, deploy } = useProjectDeployment(projectId);

  // Form state
  const [name, setName] = useState("");
  const [includePatterns, setIncludePatterns] = useState("");
  const [excludePatterns, setExcludePatterns] = useState("");
  const [verbose, setVerbose] = useState(false);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();

      if (!projectId) return;

      // Auto-deploy if not deployed
      if (!deployed || !deploymentStatus?.deployed) {
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

      // Build request
      const trimmedName = name.trim();
      const include = parsePatterns(includePatterns);
      const exclude = parsePatterns(excludePatterns);
      const scenarioFilter = buildScenarioFilter(include, exclude);

      const request: QuickRunRequest = {
        type,
        verbose,
        name: trimmedName || undefined,
        scenarios: scenarioFilter,
      };

      try {
        const result = await run(request);
        toast({
          title: "Job Started",
          description: `${JOB_TYPE_LABELS[type]} job "${result.job.metadata.name}" created`,
        });
        onOpenChange(false);
        onJobCreated?.(result.job.metadata.name);

        // Reset form
        setName("");
        setIncludePatterns("");
        setExcludePatterns("");
        setVerbose(false);
      } catch (err) {
        toast({
          title: "Run Failed",
          description: err instanceof Error ? err.message : "Failed to start job",
          variant: "destructive",
        });
      }
    },
    [
      projectId,
      deployed,
      deploymentStatus,
      deploy,
      run,
      type,
      name,
      includePatterns,
      excludePatterns,
      verbose,
      toast,
      onOpenChange,
      onJobCreated,
    ]
  );

  const isSubmitting = running || deploying;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Run {JOB_TYPE_LABELS[type]}</DialogTitle>
          <DialogDescription>{JOB_TYPE_DESCRIPTIONS[type]}</DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="job-name">Job Name</Label>
            <Input
              id="job-name"
              placeholder="Auto-generated if empty"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isSubmitting}
            />
            <p className="text-xs text-muted-foreground">
              Custom name for the job. Leave empty to auto-generate.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="include-patterns">Include Scenarios</Label>
            <Input
              id="include-patterns"
              placeholder="scenarios/*.yaml, critical/**"
              value={includePatterns}
              onChange={(e) => setIncludePatterns(e.target.value)}
              disabled={isSubmitting}
            />
            <p className="text-xs text-muted-foreground">
              Comma-separated glob patterns for scenarios to include.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="exclude-patterns">Exclude Scenarios</Label>
            <Input
              id="exclude-patterns"
              placeholder="scenarios/*-wip.yaml"
              value={excludePatterns}
              onChange={(e) => setExcludePatterns(e.target.value)}
              disabled={isSubmitting}
            />
            <p className="text-xs text-muted-foreground">
              Comma-separated glob patterns for scenarios to exclude.
            </p>
          </div>

          <div className="flex items-center space-x-2">
            <Checkbox
              id="verbose"
              checked={verbose}
              onCheckedChange={(checked) => setVerbose(checked === true)}
              disabled={isSubmitting}
            />
            <Label htmlFor="verbose" className="text-sm font-normal">
              Enable verbose logging
            </Label>
          </div>

          {!deployed && !deploymentStatus?.deployed && (
            <div className="rounded-md bg-amber-50 border border-amber-200 p-3 text-sm">
              <p className="font-medium text-amber-800">Project Not Deployed</p>
              <p className="text-amber-700">
                The project will be automatically deployed before running.
              </p>
            </div>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isSubmitting}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  {deploying ? "Deploying..." : "Running..."}
                </>
              ) : (
                `Run ${JOB_TYPE_LABELS[type]}`
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
