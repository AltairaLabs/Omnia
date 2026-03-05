"use client";

import { useState } from "react";
import { usePromptPackMutations } from "@/hooks/use-promptpack-mutations";
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertCircle, Loader2 } from "lucide-react";
import type { PromptPackSpec, RolloutStrategyType } from "@/types/prompt-pack";

// --- Form state ---
interface FormState {
  name: string;
  configMapName: string;
  version: string;
  rolloutType: RolloutStrategyType;
  canaryWeight: string;
  canaryStepWeight: string;
  canaryInterval: string;
}

const INITIAL_FORM: FormState = {
  name: "",
  configMapName: "",
  version: "",
  rolloutType: "immediate",
  canaryWeight: "10",
  canaryStepWeight: "10",
  canaryInterval: "5m",
};

// --- Validation ---
const DNS_NAME_RE = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;
const SEMVER_RE = /^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;

function validateForm(form: FormState): string | null {
  if (!form.name.trim()) return "Name is required";
  if (!DNS_NAME_RE.test(form.name)) {
    return "Name must be a valid DNS subdomain (lowercase alphanumeric and hyphens)";
  }
  if (!form.configMapName.trim()) return "ConfigMap reference is required";
  if (!DNS_NAME_RE.test(form.configMapName)) {
    return "ConfigMap name must be a valid DNS subdomain";
  }
  if (!form.version.trim()) return "Version is required";
  if (!SEMVER_RE.test(form.version)) {
    return "Version must be valid semver (e.g. 1.0.0)";
  }
  if (form.rolloutType === "canary") {
    const weight = Number(form.canaryWeight);
    if (Number.isNaN(weight) || weight < 0 || weight > 100) {
      return "Canary weight must be between 0 and 100";
    }
  }
  return null;
}

function buildSpec(form: FormState): PromptPackSpec {
  const spec: PromptPackSpec = {
    source: {
      type: "configmap",
      configMapRef: { name: form.configMapName },
    },
    version: form.version.replace(/^v/, ""),
    rollout: { type: form.rolloutType },
  };

  if (form.rolloutType === "canary") {
    spec.rollout.canary = {
      weight: Number(form.canaryWeight),
      stepWeight: Number(form.canaryStepWeight),
      interval: form.canaryInterval,
    };
  }

  return spec;
}

// --- Dialog props ---
interface PromptPackDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
}

// --- Inner form (remounts when dialog opens) ---
function PromptPackDialogForm({
  onOpenChange,
  onSuccess,
}: Omit<PromptPackDialogProps, "open">) {
  const [form, setForm] = useState<FormState>(INITIAL_FORM);
  const [error, setError] = useState<string | null>(null);
  const { createPromptPack, loading } = usePromptPackMutations();

  function updateForm<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function handleSubmit() {
    const validationError = validateForm(form);
    if (validationError) {
      setError(validationError);
      return;
    }

    try {
      setError(null);
      await createPromptPack(form.name, buildSpec(form));
      onSuccess?.();
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>Create PromptPack</DialogTitle>
        <DialogDescription>
          Create a new PromptPack referencing a ConfigMap with prompt definitions.
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
          <Label htmlFor="pp-name">Name</Label>
          <Input
            id="pp-name"
            placeholder="my-prompt-pack"
            value={form.name}
            onChange={(e) => updateForm("name", e.target.value)}
          />
        </div>

        {/* ConfigMap Reference */}
        <div className="space-y-2">
          <Label htmlFor="pp-configmap">ConfigMap Reference</Label>
          <Input
            id="pp-configmap"
            placeholder="my-prompts-configmap"
            value={form.configMapName}
            onChange={(e) => updateForm("configMapName", e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            Name of the ConfigMap containing the compiled PromptPack JSON.
          </p>
        </div>

        {/* Version */}
        <div className="space-y-2">
          <Label htmlFor="pp-version">Version</Label>
          <Input
            id="pp-version"
            placeholder="1.0.0"
            value={form.version}
            onChange={(e) => updateForm("version", e.target.value)}
          />
        </div>

        {/* Rollout Strategy */}
        <div className="space-y-2">
          <Label>Rollout Strategy</Label>
          <RadioGroup
            value={form.rolloutType}
            onValueChange={(v) => updateForm("rolloutType", v as RolloutStrategyType)}
          >
            <div className="flex items-center space-x-2">
              <RadioGroupItem value="immediate" id="rollout-immediate" />
              <Label htmlFor="rollout-immediate" className="font-normal">
                Immediate
              </Label>
            </div>
            <div className="flex items-center space-x-2">
              <RadioGroupItem value="canary" id="rollout-canary" />
              <Label htmlFor="rollout-canary" className="font-normal">
                Canary
              </Label>
            </div>
          </RadioGroup>
        </div>

        {/* Canary Config (conditional) */}
        {form.rolloutType === "canary" && (
          <div className="space-y-3 rounded-lg border p-3">
            <p className="text-sm font-medium">Canary Configuration</p>
            <div className="grid grid-cols-3 gap-3">
              <div className="space-y-1">
                <Label htmlFor="pp-canary-weight" className="text-xs">
                  Initial Weight (%)
                </Label>
                <Input
                  id="pp-canary-weight"
                  type="number"
                  min="0"
                  max="100"
                  value={form.canaryWeight}
                  onChange={(e) => updateForm("canaryWeight", e.target.value)}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="pp-canary-step" className="text-xs">
                  Step Weight (%)
                </Label>
                <Input
                  id="pp-canary-step"
                  type="number"
                  min="1"
                  max="100"
                  value={form.canaryStepWeight}
                  onChange={(e) => updateForm("canaryStepWeight", e.target.value)}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="pp-canary-interval" className="text-xs">
                  Interval
                </Label>
                <Input
                  id="pp-canary-interval"
                  placeholder="5m"
                  value={form.canaryInterval}
                  onChange={(e) => updateForm("canaryInterval", e.target.value)}
                />
              </div>
            </div>
          </div>
        )}
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={loading}>
          {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Create PromptPack
        </Button>
      </DialogFooter>
    </>
  );
}

// --- Main dialog ---
export function PromptPackDialog({
  open,
  onOpenChange,
  onSuccess,
}: PromptPackDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        {open && (
          <PromptPackDialogForm
            onOpenChange={onOpenChange}
            onSuccess={onSuccess}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
