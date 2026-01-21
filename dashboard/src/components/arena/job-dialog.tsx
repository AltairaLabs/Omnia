"use client";

import { useState } from "react";
import { useArenaJobMutations } from "@/hooks/use-arena-jobs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Switch } from "@/components/ui/switch";
import {
  AlertCircle,
  Loader2,
  Settings,
} from "lucide-react";
import type {
  ArenaConfig,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobType,
} from "@/types/arena";

interface JobDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  configs: ArenaConfig[];
  preselectedConfig?: string;
  onSuccess?: () => void;
  onClose?: () => void;
}

interface FormState {
  name: string;
  configRef: string;
  type: ArenaJobType;
  workers: string;
  timeout: string;
  // Evaluation options
  continueOnFailure: boolean;
  passingThreshold: string;
  // Load test options
  profileType: string;
  duration: string;
  targetRPS: string;
  // Data gen options
  sampleCount: string;
  deduplicate: boolean;
}

function getInitialFormState(preselectedConfig?: string): FormState {
  return {
    name: "",
    configRef: preselectedConfig || "",
    type: "evaluation",
    workers: "2",
    timeout: "30m",
    continueOnFailure: true,
    passingThreshold: "0.8",
    profileType: "constant",
    duration: "5m",
    targetRPS: "10",
    sampleCount: "100",
    deduplicate: true,
  };
}

function validateForm(form: FormState): string | null {
  if (!form.name.trim()) {
    return "Name is required";
  }
  if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(form.name)) {
    return "Name must be lowercase alphanumeric and may contain hyphens";
  }
  if (!form.configRef) {
    return "Config is required";
  }
  const workers = parseInt(form.workers, 10);
  if (isNaN(workers) || workers < 1) {
    return "Workers must be a positive integer";
  }
  if (form.type === "evaluation") {
    const threshold = parseFloat(form.passingThreshold);
    if (isNaN(threshold) || threshold < 0 || threshold > 1) {
      return "Passing threshold must be a number between 0 and 1";
    }
  }
  if (form.type === "loadtest") {
    const rps = parseInt(form.targetRPS, 10);
    if (isNaN(rps) || rps < 1) {
      return "Target RPS must be a positive integer";
    }
  }
  if (form.type === "datagen") {
    const count = parseInt(form.sampleCount, 10);
    if (isNaN(count) || count < 1) {
      return "Sample count must be a positive integer";
    }
  }
  return null;
}

function buildSpec(form: FormState): ArenaJobSpec {
  const spec: ArenaJobSpec = {
    configRef: { name: form.configRef },
    type: form.type,
    workers: {
      replicas: parseInt(form.workers, 10),
    },
    timeout: form.timeout || undefined,
  };

  if (form.type === "evaluation") {
    spec.evaluation = {
      continueOnFailure: form.continueOnFailure,
      passingThreshold: parseFloat(form.passingThreshold),
      outputFormats: ["json", "junit"],
    };
  } else if (form.type === "loadtest") {
    spec.loadtest = {
      profileType: form.profileType as "constant" | "ramp" | "spike" | "soak",
      duration: form.duration,
      targetRPS: parseInt(form.targetRPS, 10),
    };
  } else if (form.type === "datagen") {
    spec.datagen = {
      sampleCount: parseInt(form.sampleCount, 10),
      deduplicate: form.deduplicate,
      outputFormat: "jsonl",
    };
  }

  return spec;
}

export function JobDialog({
  open,
  onOpenChange,
  configs,
  preselectedConfig,
  onSuccess,
  onClose,
}: Readonly<JobDialogProps>) {
  const { createJob, loading } = useArenaJobMutations();

  // Use preselectedConfig as key to reset form
  const formResetKey = `${preselectedConfig ?? "new"}-${open}`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <JobDialogForm
        key={formResetKey}
        configs={configs}
        preselectedConfig={preselectedConfig}
        loading={loading}
        createJob={createJob}
        onSuccess={onSuccess}
        onClose={onClose}
        onOpenChange={onOpenChange}
      />
    </Dialog>
  );
}

interface JobDialogFormProps {
  configs: ArenaConfig[];
  preselectedConfig?: string;
  loading: boolean;
  createJob: (name: string, spec: ArenaJob["spec"]) => Promise<ArenaJob>;
  onSuccess?: () => void;
  onClose?: () => void;
  onOpenChange: (open: boolean) => void;
}

function JobDialogForm({
  configs,
  preselectedConfig,
  loading,
  createJob,
  onSuccess,
  onClose,
  onOpenChange,
}: Readonly<JobDialogFormProps>) {
  const [formState, setFormState] = useState<FormState>(() =>
    getInitialFormState(preselectedConfig)
  );
  const [error, setError] = useState<string | null>(null);

  const updateForm = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setFormState((prev) => ({ ...prev, [key]: value }));
  };

  const handleSubmit = async () => {
    try {
      setError(null);

      const validationError = validateForm(formState);
      if (validationError) {
        setError(validationError);
        return;
      }

      const spec = buildSpec(formState);
      await createJob(formState.name, spec);
      onSuccess?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create job");
    }
  };

  const handleClose = () => {
    onClose?.();
    onOpenChange(false);
  };

  const readyConfigs = configs.filter(
    (c) => c.status?.phase === "Ready"
  );

  return (
    <DialogContent className="sm:max-w-[500px]">
      <DialogHeader>
        <DialogTitle>Create Job</DialogTitle>
        <DialogDescription>
          Create a new Arena job to run evaluations, load tests, or generate data.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-4 py-4">
        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {/* Name */}
        <div className="space-y-2">
          <Label htmlFor="name">Name</Label>
          <Input
            id="name"
            placeholder="my-job"
            value={formState.name}
            onChange={(e) => updateForm("name", e.target.value)}
          />
        </div>

        {/* Config Reference */}
        <div className="space-y-2">
          <Label htmlFor="config">Config</Label>
          <Select
            value={formState.configRef}
            onValueChange={(v) => updateForm("configRef", v)}
          >
            <SelectTrigger id="config">
              <SelectValue placeholder="Select a config" />
            </SelectTrigger>
            <SelectContent>
              {readyConfigs.length === 0 ? (
                <div className="flex items-center gap-2 text-muted-foreground p-2 text-sm">
                  <Settings className="h-4 w-4" />
                  No ready configs available
                </div>
              ) : (
                readyConfigs.map((config) => (
                  <SelectItem key={config.metadata?.name} value={config.metadata?.name || "unknown"}>
                    {config.metadata?.name}
                  </SelectItem>
                ))
              )}
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            Select the config containing scenarios to run
          </p>
        </div>

        {/* Job Type */}
        <div className="space-y-2">
          <Label htmlFor="type">Job Type</Label>
          <Select
            value={formState.type}
            onValueChange={(v) => updateForm("type", v as ArenaJobType)}
          >
            <SelectTrigger id="type">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="evaluation">Evaluation</SelectItem>
              <SelectItem value="loadtest">Load Test</SelectItem>
              <SelectItem value="datagen">Data Generation</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* Workers */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="workers">Workers</Label>
            <Input
              id="workers"
              type="number"
              min="1"
              value={formState.workers}
              onChange={(e) => updateForm("workers", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="timeout">Timeout</Label>
            <Input
              id="timeout"
              placeholder="30m"
              value={formState.timeout}
              onChange={(e) => updateForm("timeout", e.target.value)}
            />
          </div>
        </div>

        {/* Type-specific options */}
        {formState.type === "evaluation" && (
          <div className="space-y-4 pt-2 border-t">
            <Label className="text-sm font-medium">Evaluation Options</Label>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="threshold" className="text-xs text-muted-foreground">
                  Passing Threshold
                </Label>
                <Input
                  id="threshold"
                  type="number"
                  step="0.1"
                  min="0"
                  max="1"
                  value={formState.passingThreshold}
                  onChange={(e) => updateForm("passingThreshold", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label className="text-xs text-muted-foreground">
                  Continue on Failure
                </Label>
                <div className="flex items-center h-10">
                  <Switch
                    checked={formState.continueOnFailure}
                    onCheckedChange={(checked) => updateForm("continueOnFailure", checked)}
                  />
                </div>
              </div>
            </div>
          </div>
        )}

        {formState.type === "loadtest" && (
          <div className="space-y-4 pt-2 border-t">
            <Label className="text-sm font-medium">Load Test Options</Label>
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="profile" className="text-xs text-muted-foreground">
                  Profile Type
                </Label>
                <Select
                  value={formState.profileType}
                  onValueChange={(v) => updateForm("profileType", v)}
                >
                  <SelectTrigger id="profile">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="constant">Constant</SelectItem>
                    <SelectItem value="ramp">Ramp</SelectItem>
                    <SelectItem value="spike">Spike</SelectItem>
                    <SelectItem value="soak">Soak</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="duration" className="text-xs text-muted-foreground">
                  Duration
                </Label>
                <Input
                  id="duration"
                  placeholder="5m"
                  value={formState.duration}
                  onChange={(e) => updateForm("duration", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="rps" className="text-xs text-muted-foreground">
                  Target RPS
                </Label>
                <Input
                  id="rps"
                  type="number"
                  min="1"
                  value={formState.targetRPS}
                  onChange={(e) => updateForm("targetRPS", e.target.value)}
                />
              </div>
            </div>
          </div>
        )}

        {formState.type === "datagen" && (
          <div className="space-y-4 pt-2 border-t">
            <Label className="text-sm font-medium">Data Generation Options</Label>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="samples" className="text-xs text-muted-foreground">
                  Sample Count
                </Label>
                <Input
                  id="samples"
                  type="number"
                  min="1"
                  value={formState.sampleCount}
                  onChange={(e) => updateForm("sampleCount", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label className="text-xs text-muted-foreground">
                  Deduplicate
                </Label>
                <div className="flex items-center h-10">
                  <Switch
                    checked={formState.deduplicate}
                    onCheckedChange={(checked) => updateForm("deduplicate", checked)}
                  />
                </div>
              </div>
            </div>
          </div>
        )}
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={handleClose}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={loading}>
          {loading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
          Create Job
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
