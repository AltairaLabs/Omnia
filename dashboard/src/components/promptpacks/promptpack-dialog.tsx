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
  return null;
}

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
