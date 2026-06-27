"use client";

import { useState } from "react";
import { usePromptPackMutations } from "@/hooks/resources";
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
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertCircle, Loader2 } from "lucide-react";
import type { PromptPackSpec } from "@/types/prompt-pack";
import { useFieldValidation } from "@/hooks/use-field-validation";
import { FieldError } from "@/components/ui/field-error";
import { crdConstraints } from "@/types/generated/crd-constraints";

// --- Form state ---
interface FormState {
  name: string;
  configMapName: string;
  version: string;
}

const INITIAL_FORM: FormState = {
  name: "",
  configMapName: "",
  version: "",
};

function buildSpec(form: FormState): PromptPackSpec {
  return {
    source: {
      type: "configmap",
      configMapRef: { name: form.configMapName },
    },
    version: form.version.replace(/^v/, ""),
  };
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

  const { errors, validate, validateAll, hasErrors } = useFieldValidation(
    crdConstraints.PromptPack
  );

  function updateForm<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function handleSubmit() {
    const valid = validateAll([
      { path: "metadata.name", value: form.name },
      { path: "spec.version", value: form.version },
    ]);
    if (!valid) return;

    // Cross-field: configMap name is required (no constraint in CRD map)
    if (!form.configMapName.trim()) {
      setError("ConfigMap reference is required");
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
            aria-invalid={!!errors["metadata.name"]}
            aria-describedby={errors["metadata.name"] ? "pp-name-error" : undefined}
            value={form.name}
            onChange={(e) => {
              updateForm("name", e.target.value);
              validate("metadata.name", e.target.value);
            }}
          />
          <FieldError id="pp-name-error" message={errors["metadata.name"]} />
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
            aria-invalid={!!errors["spec.version"]}
            aria-describedby={errors["spec.version"] ? "pp-version-error" : undefined}
            value={form.version}
            onChange={(e) => {
              updateForm("version", e.target.value);
              validate("spec.version", e.target.value);
            }}
          />
          <FieldError id="pp-version-error" message={errors["spec.version"]} />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={hasErrors || loading}>
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
