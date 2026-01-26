"use client";

import { useState } from "react";
import { useTemplateSourceMutations } from "@/hooks/use-template-sources";
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
import {
  GitBranch,
  Box,
  FileText,
  AlertCircle,
  Loader2,
} from "lucide-react";
import type {
  ArenaTemplateSource,
  ArenaTemplateSourceSpec,
  ArenaTemplateSourceType,
} from "@/types/arena-template";
import type { GitSourceSpec, OCISourceSpec, ConfigMapSourceSpec } from "@/types/arena";

interface TemplateSourceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  source?: ArenaTemplateSource | null;
  onSuccess?: () => void;
  onClose?: () => void;
}

interface FormState {
  name: string;
  sourceType: ArenaTemplateSourceType;
  syncInterval: string;
  templatesPath: string;
  gitSpec: GitSourceSpec;
  ociSpec: OCISourceSpec;
  configMapSpec: ConfigMapSourceSpec;
}

function getInitialFormState(source?: ArenaTemplateSource | null): FormState {
  if (source) {
    return {
      name: source.metadata?.name || "",
      sourceType: source.spec?.type || "git",
      syncInterval: source.spec?.syncInterval || "1h",
      templatesPath: source.spec?.templatesPath || "templates/",
      gitSpec: source.spec?.git || { url: "" },
      ociSpec: source.spec?.oci || { url: "" },
      configMapSpec: source.spec?.configMap || { name: "" },
    };
  }
  return {
    name: "",
    sourceType: "git",
    syncInterval: "1h",
    templatesPath: "templates/",
    gitSpec: { url: "", ref: { branch: "main" } },
    ociSpec: { url: "" },
    configMapSpec: { name: "" },
  };
}

function validateForm(form: FormState): string | null {
  if (!form.name.trim()) {
    return "Name is required";
  }
  if (!/^[a-z][a-z0-9-]*$/.test(form.name)) {
    return "Name must start with a letter and contain only lowercase letters, numbers, and hyphens";
  }
  if (form.sourceType === "git" && !form.gitSpec.url) {
    return "Git repository URL is required";
  }
  if (form.sourceType === "oci" && !form.ociSpec.url) {
    return "OCI repository URL is required";
  }
  if (form.sourceType === "configmap" && !form.configMapSpec.name) {
    return "ConfigMap name is required";
  }
  return null;
}

function buildSpec(form: FormState): ArenaTemplateSourceSpec {
  const spec: ArenaTemplateSourceSpec = {
    type: form.sourceType,
    syncInterval: form.syncInterval,
    templatesPath: form.templatesPath,
  };

  switch (form.sourceType) {
    case "git":
      spec.git = form.gitSpec;
      break;
    case "oci":
      spec.oci = form.ociSpec;
      break;
    case "configmap":
      spec.configMap = form.configMapSpec;
      break;
  }

  return spec;
}

const SOURCE_TYPES: {
  value: ArenaTemplateSourceType;
  label: string;
  icon: React.ReactNode;
}[] = [
  { value: "git", label: "Git Repository", icon: <GitBranch className="h-4 w-4" /> },
  { value: "oci", label: "OCI Registry", icon: <Box className="h-4 w-4" /> },
  { value: "configmap", label: "ConfigMap", icon: <FileText className="h-4 w-4" /> },
];

export function TemplateSourceDialog({
  open,
  onOpenChange,
  source,
  onSuccess,
  onClose,
}: TemplateSourceDialogProps) {
  const [form, setForm] = useState<FormState>(() => getInitialFormState(source));
  const [error, setError] = useState<string | null>(null);
  const { createSource, updateSource, loading } = useTemplateSourceMutations();

  const isEditing = !!source;

  const handleSubmit = async () => {
    setError(null);

    const validationError = validateForm(form);
    if (validationError) {
      setError(validationError);
      return;
    }

    try {
      const spec = buildSpec(form);

      if (isEditing) {
        await updateSource(form.name, spec);
      } else {
        await createSource(form.name, spec);
      }

      onSuccess?.();
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save template source");
    }
  };

  const handleClose = () => {
    setError(null);
    setForm(getInitialFormState(source));
    onClose?.();
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEditing ? "Edit Template Source" : "Add Template Source"}
          </DialogTitle>
          <DialogDescription>
            {isEditing
              ? "Update the template source configuration."
              : "Add a new source for project templates."}
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
              value={form.name}
              onChange={(e) => setForm((prev) => ({ ...prev, name: e.target.value }))}
              placeholder="my-templates"
              disabled={isEditing}
            />
          </div>

          {/* Source type */}
          <div className="space-y-2">
            <Label>Source Type</Label>
            <Select
              value={form.sourceType}
              onValueChange={(value: ArenaTemplateSourceType) =>
                setForm((prev) => ({ ...prev, sourceType: value }))
              }
              disabled={isEditing}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SOURCE_TYPES.map((type) => (
                  <SelectItem key={type.value} value={type.value}>
                    <div className="flex items-center gap-2">
                      {type.icon}
                      {type.label}
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Git-specific fields */}
          {form.sourceType === "git" && (
            <>
              <div className="space-y-2">
                <Label htmlFor="gitUrl">Repository URL</Label>
                <Input
                  id="gitUrl"
                  value={form.gitSpec.url}
                  onChange={(e) =>
                    setForm((prev) => ({
                      ...prev,
                      gitSpec: { ...prev.gitSpec, url: e.target.value },
                    }))
                  }
                  placeholder="https://github.com/org/templates"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="gitBranch">Branch</Label>
                <Input
                  id="gitBranch"
                  value={form.gitSpec.ref?.branch || ""}
                  onChange={(e) =>
                    setForm((prev) => ({
                      ...prev,
                      gitSpec: {
                        ...prev.gitSpec,
                        ref: { ...prev.gitSpec.ref, branch: e.target.value },
                      },
                    }))
                  }
                  placeholder="main"
                />
              </div>
            </>
          )}

          {/* OCI-specific fields */}
          {form.sourceType === "oci" && (
            <div className="space-y-2">
              <Label htmlFor="ociUrl">OCI URL</Label>
              <Input
                id="ociUrl"
                value={form.ociSpec.url}
                onChange={(e) =>
                  setForm((prev) => ({
                    ...prev,
                    ociSpec: { ...prev.ociSpec, url: e.target.value },
                  }))
                }
                placeholder="oci://ghcr.io/org/templates:latest"
              />
            </div>
          )}

          {/* ConfigMap-specific fields */}
          {form.sourceType === "configmap" && (
            <div className="space-y-2">
              <Label htmlFor="configMapName">ConfigMap Name</Label>
              <Input
                id="configMapName"
                value={form.configMapSpec.name}
                onChange={(e) =>
                  setForm((prev) => ({
                    ...prev,
                    configMapSpec: { ...prev.configMapSpec, name: e.target.value },
                  }))
                }
                placeholder="templates-config"
              />
            </div>
          )}

          {/* Templates path */}
          <div className="space-y-2">
            <Label htmlFor="templatesPath">Templates Path</Label>
            <Input
              id="templatesPath"
              value={form.templatesPath}
              onChange={(e) =>
                setForm((prev) => ({ ...prev, templatesPath: e.target.value }))
              }
              placeholder="templates/"
            />
            <p className="text-xs text-muted-foreground">
              Path within the source where templates are located
            </p>
          </div>

          {/* Sync interval */}
          <div className="space-y-2">
            <Label htmlFor="syncInterval">Sync Interval</Label>
            <Select
              value={form.syncInterval}
              onValueChange={(value) =>
                setForm((prev) => ({ ...prev, syncInterval: value }))
              }
            >
              <SelectTrigger id="syncInterval">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="5m">5 minutes</SelectItem>
                <SelectItem value="15m">15 minutes</SelectItem>
                <SelectItem value="30m">30 minutes</SelectItem>
                <SelectItem value="1h">1 hour</SelectItem>
                <SelectItem value="6h">6 hours</SelectItem>
                <SelectItem value="24h">24 hours</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={loading}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={loading}>
            {loading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            {isEditing ? "Update" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
