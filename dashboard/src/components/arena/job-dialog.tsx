"use client";

import { useArenaJobMutations } from "@/hooks/use-arena-jobs";
import { useLicense } from "@/hooks/use-license";
import { JobWizard } from "./job-wizard";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { ArenaSource } from "@/types/arena";

interface JobDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sources: ArenaSource[];
  preselectedSource?: string;
  onSuccess?: () => void;
  onClose?: () => void;
}

/**
 * Job creation dialog - thin wrapper around JobWizard for backward compatibility.
 * Handles dialog state and reset logic.
 */
export function JobDialog({
  open,
  onOpenChange,
  sources,
  preselectedSource,
  onSuccess,
  onClose,
}: Readonly<JobDialogProps>) {
  const { createJob, loading } = useArenaJobMutations();
  const { license, isEnterprise } = useLicense();

  // Use preselectedSource as key to reset form when it changes
  const formResetKey = `${preselectedSource ?? "new"}-${open}`;

  const handleClose = () => {
    onClose?.();
    onOpenChange(false);
  };

  const handleSuccess = () => {
    onSuccess?.();
    // Close dialog after a short delay to show success state
    setTimeout(() => {
      handleClose();
    }, 1500);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[560px] max-h-[90vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>Create Job</DialogTitle>
          <DialogDescription>
            Create a new Arena job to run evaluations, load tests, or generate data.
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 overflow-hidden">
          <JobWizard
            key={formResetKey}
            sources={sources}
            preselectedSource={preselectedSource}
            isEnterprise={isEnterprise}
            maxWorkerReplicas={license.limits.maxWorkerReplicas}
            loading={loading}
            onSubmit={createJob}
            onSuccess={handleSuccess}
            onClose={handleClose}
          />
        </div>
      </DialogContent>
    </Dialog>
  );
}
