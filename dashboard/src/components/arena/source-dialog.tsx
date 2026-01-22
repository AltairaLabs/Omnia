"use client";

import { useState } from "react";
import { useArenaSourceMutations } from "@/hooks";
import { useLicense } from "@/hooks/use-license";
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
import { Badge } from "@/components/ui/badge";
import {
  GitBranch,
  Box,
  Cloud,
  FileText,
  AlertCircle,
  Loader2,
  Lock,
} from "lucide-react";
import type {
  ArenaSource,
  ArenaSourceSpec,
  ArenaSourceType,
  GitSourceSpec,
  OCISourceSpec,
  S3SourceSpec,
  ConfigMapSourceSpec,
} from "@/types/arena";

interface SourceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  source?: ArenaSource | null;
  onSuccess?: () => void;
  onClose?: () => void;
}

interface FormState {
  name: string;
  sourceType: ArenaSourceType;
  interval: string;
  secretRef: string;
  gitSpec: GitSourceSpec;
  ociSpec: OCISourceSpec;
  s3Spec: S3SourceSpec;
  configMapSpec: ConfigMapSourceSpec;
}

function getInitialFormState(source?: ArenaSource | null): FormState {
  if (source) {
    return {
      name: source.metadata?.name || "",
      sourceType: source.spec?.type || "configmap",
      interval: source.spec?.interval || "5m",
      secretRef: source.spec?.secretRef?.name || "",
      gitSpec: source.spec?.git || { url: "" },
      ociSpec: source.spec?.oci || { url: "" },
      s3Spec: source.spec?.s3 || { bucket: "" },
      configMapSpec: source.spec?.configMap || { name: "" },
    };
  }
  return {
    name: "",
    sourceType: "configmap",
    interval: "5m",
    secretRef: "",
    gitSpec: { url: "" },
    ociSpec: { url: "" },
    s3Spec: { bucket: "" },
    configMapSpec: { name: "" },
  };
}

function validateForm(
  form: FormState,
  isEnterprise: boolean
): string | null {
  if (!form.name.trim()) {
    return "Name is required";
  }
  if (form.sourceType === "git" && !form.gitSpec.url) {
    return "Git repository URL is required";
  }
  if (form.sourceType === "oci" && !form.ociSpec.url) {
    return "OCI repository URL is required";
  }
  if (form.sourceType === "s3" && !form.s3Spec.bucket) {
    return "S3 bucket name is required";
  }
  if (form.sourceType === "configmap" && !form.configMapSpec.name) {
    return "ConfigMap name is required";
  }
  if (!isEnterprise && ["git", "oci", "s3"].includes(form.sourceType)) {
    return `${form.sourceType.toUpperCase()} sources require an Enterprise license`;
  }
  return null;
}

function buildSpec(form: FormState): ArenaSourceSpec {
  const spec: ArenaSourceSpec = {
    type: form.sourceType,
    interval: form.interval,
  };

  if (form.secretRef) {
    spec.secretRef = { name: form.secretRef };
  }

  switch (form.sourceType) {
    case "git":
      spec.git = form.gitSpec;
      break;
    case "oci":
      spec.oci = form.ociSpec;
      break;
    case "s3":
      spec.s3 = form.s3Spec;
      break;
    case "configmap":
      spec.configMap = form.configMapSpec;
      break;
  }

  return spec;
}

const SOURCE_TYPES: {
  value: ArenaSourceType;
  label: string;
  icon: React.ReactNode;
  enterprise: boolean;
}[] = [
  { value: "configmap", label: "ConfigMap", icon: <FileText className="h-4 w-4" />, enterprise: false },
  { value: "git", label: "Git", icon: <GitBranch className="h-4 w-4" />, enterprise: true },
  { value: "oci", label: "OCI Registry", icon: <Box className="h-4 w-4" />, enterprise: true },
  { value: "s3", label: "S3 Bucket", icon: <Cloud className="h-4 w-4" />, enterprise: true },
];

function GitSourceFields({
  spec,
  onChange,
}: Readonly<{
  spec: GitSourceSpec;
  onChange: (spec: GitSourceSpec) => void;
}>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="git-url">Repository URL</Label>
        <Input
          id="git-url"
          placeholder="https://github.com/org/repo.git"
          value={spec.url || ""}
          onChange={(e) => onChange({ ...spec, url: e.target.value })}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="git-branch">Branch</Label>
          <Input
            id="git-branch"
            placeholder="main"
            value={spec.ref?.branch || ""}
            onChange={(e) =>
              onChange({
                ...spec,
                ref: { ...spec.ref, branch: e.target.value || undefined },
              })
            }
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="git-path">Path</Label>
          <Input
            id="git-path"
            placeholder="prompts/"
            value={spec.path || ""}
            onChange={(e) => onChange({ ...spec, path: e.target.value || undefined })}
          />
        </div>
      </div>
    </div>
  );
}

function OCISourceFields({
  spec,
  onChange,
}: Readonly<{
  spec: OCISourceSpec;
  onChange: (spec: OCISourceSpec) => void;
}>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="oci-url">OCI Repository URL</Label>
        <Input
          id="oci-url"
          placeholder="oci://ghcr.io/org/prompts"
          value={spec.url || ""}
          onChange={(e) => onChange({ ...spec, url: e.target.value })}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="oci-tag">Tag</Label>
          <Input
            id="oci-tag"
            placeholder="latest"
            value={spec.ref?.tag || ""}
            onChange={(e) =>
              onChange({
                ...spec,
                ref: { ...spec.ref, tag: e.target.value || undefined },
              })
            }
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="oci-semver">SemVer Constraint</Label>
          <Input
            id="oci-semver"
            placeholder=">=1.0.0 <2.0.0"
            value={spec.ref?.semver || ""}
            onChange={(e) =>
              onChange({
                ...spec,
                ref: { ...spec.ref, semver: e.target.value || undefined },
              })
            }
          />
        </div>
      </div>
    </div>
  );
}

function S3SourceFields({
  spec,
  onChange,
}: Readonly<{
  spec: S3SourceSpec;
  onChange: (spec: S3SourceSpec) => void;
}>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="s3-bucket">Bucket Name</Label>
        <Input
          id="s3-bucket"
          placeholder="my-prompts-bucket"
          value={spec.bucket || ""}
          onChange={(e) => onChange({ ...spec, bucket: e.target.value })}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="s3-prefix">Prefix</Label>
          <Input
            id="s3-prefix"
            placeholder="prompts/"
            value={spec.prefix || ""}
            onChange={(e) => onChange({ ...spec, prefix: e.target.value || undefined })}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="s3-region">Region</Label>
          <Input
            id="s3-region"
            placeholder="us-east-1"
            value={spec.region || ""}
            onChange={(e) => onChange({ ...spec, region: e.target.value || undefined })}
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="s3-endpoint">Custom Endpoint (optional)</Label>
        <Input
          id="s3-endpoint"
          placeholder="https://s3.custom-endpoint.com"
          value={spec.endpoint || ""}
          onChange={(e) => onChange({ ...spec, endpoint: e.target.value || undefined })}
        />
      </div>
    </div>
  );
}

function ConfigMapSourceFields({
  spec,
  onChange,
}: Readonly<{
  spec: ConfigMapSourceSpec;
  onChange: (spec: ConfigMapSourceSpec) => void;
}>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="configmap-name">ConfigMap Name</Label>
        <Input
          id="configmap-name"
          placeholder="my-prompts-configmap"
          value={spec.name || ""}
          onChange={(e) => onChange({ ...spec, name: e.target.value })}
        />
      </div>
    </div>
  );
}

export function SourceDialog({
  open,
  onOpenChange,
  source,
  onSuccess,
  onClose,
}: Readonly<SourceDialogProps>) {
  const { createSource, updateSource, loading } = useArenaSourceMutations();
  const { license } = useLicense();
  const isEnterprise = license?.tier === "enterprise";
  const isEditing = !!source;

  // Use source name as key to reset form when source changes, and add open state
  // This ensures form resets when dialog opens with different source
  const formResetKey = `${source?.metadata?.name ?? "new"}-${open}`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <SourceDialogForm
        key={formResetKey}
        source={source}
        isEditing={isEditing}
        isEnterprise={isEnterprise}
        loading={loading}
        createSource={createSource}
        updateSource={updateSource}
        onSuccess={onSuccess}
        onClose={onClose}
        onOpenChange={onOpenChange}
      />
    </Dialog>
  );
}

interface SourceDialogFormProps {
  source?: ArenaSource | null;
  isEditing: boolean;
  isEnterprise: boolean;
  loading: boolean;
  createSource: (name: string, spec: ArenaSource["spec"]) => Promise<ArenaSource>;
  updateSource: (name: string, spec: ArenaSource["spec"]) => Promise<ArenaSource>;
  onSuccess?: () => void;
  onClose?: () => void;
  onOpenChange: (open: boolean) => void;
}

function SourceDialogForm({
  source,
  isEditing,
  isEnterprise,
  loading,
  createSource,
  updateSource,
  onSuccess,
  onClose,
  onOpenChange,
}: Readonly<SourceDialogFormProps>) {

  // Initialize form state once when component mounts
  const [formState, setFormState] = useState<FormState>(() => getInitialFormState(source));
  const [error, setError] = useState<string | null>(null);

  const updateForm = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setFormState((prev) => ({ ...prev, [key]: value }));
  };

  const handleSubmit = async () => {
    try {
      setError(null);

      const validationError = validateForm(formState, isEnterprise);
      if (validationError) {
        setError(validationError);
        return;
      }

      const spec = buildSpec(formState);

      if (isEditing) {
        await updateSource(formState.name, spec);
      } else {
        await createSource(formState.name, spec);
      }

      onSuccess?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save source");
    }
  };

  const handleClose = () => {
    onClose?.();
    onOpenChange(false);
  };

  const isTypeDisabled = (type: ArenaSourceType) => {
    const typeConfig = SOURCE_TYPES.find((t) => t.value === type);
    return typeConfig?.enterprise && !isEnterprise;
  };

  return (
    <DialogContent className="sm:max-w-[500px]">
      <DialogHeader>
        <DialogTitle>{isEditing ? "Edit Source" : "Create Source"}</DialogTitle>
          <DialogDescription>
            {isEditing
              ? "Update the configuration for this Arena source."
              : "Configure a new source for PromptKit bundles."}
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
              placeholder="my-source"
              value={formState.name}
              onChange={(e) => updateForm("name", e.target.value)}
              disabled={isEditing}
            />
          </div>

          {/* Source Type */}
          <div className="space-y-2">
            <Label>Source Type</Label>
            <Select
              value={formState.sourceType}
              onValueChange={(v) => updateForm("sourceType", v as ArenaSourceType)}
              disabled={isEditing}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SOURCE_TYPES.map((type) => (
                  <SelectItem
                    key={type.value}
                    value={type.value}
                    disabled={isTypeDisabled(type.value)}
                  >
                    <div className="flex items-center gap-2">
                      {type.icon}
                      <span>{type.label}</span>
                      {type.enterprise && !isEnterprise && (
                        <Badge variant="outline" className="ml-2 text-xs">
                          <Lock className="h-3 w-3 mr-1" />
                          Enterprise
                        </Badge>
                      )}
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Type-specific fields */}
          {formState.sourceType === "git" && (
            <GitSourceFields spec={formState.gitSpec} onChange={(spec) => updateForm("gitSpec", spec)} />
          )}
          {formState.sourceType === "oci" && (
            <OCISourceFields spec={formState.ociSpec} onChange={(spec) => updateForm("ociSpec", spec)} />
          )}
          {formState.sourceType === "s3" && (
            <S3SourceFields spec={formState.s3Spec} onChange={(spec) => updateForm("s3Spec", spec)} />
          )}
          {formState.sourceType === "configmap" && (
            <ConfigMapSourceFields spec={formState.configMapSpec} onChange={(spec) => updateForm("configMapSpec", spec)} />
          )}

          {/* Common fields */}
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="interval">Sync Interval</Label>
              <Select value={formState.interval} onValueChange={(v) => updateForm("interval", v)}>
                <SelectTrigger id="interval">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="1m">1 minute</SelectItem>
                  <SelectItem value="5m">5 minutes</SelectItem>
                  <SelectItem value="10m">10 minutes</SelectItem>
                  <SelectItem value="30m">30 minutes</SelectItem>
                  <SelectItem value="1h">1 hour</SelectItem>
                  <SelectItem value="6h">6 hours</SelectItem>
                  <SelectItem value="12h">12 hours</SelectItem>
                  <SelectItem value="24h">24 hours</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="secret-ref">Credentials Secret (optional)</Label>
              <Input
                id="secret-ref"
                placeholder="my-credentials"
                value={formState.secretRef}
                onChange={(e) => updateForm("secretRef", e.target.value)}
              />
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={loading}>
            {loading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
            {isEditing ? "Save Changes" : "Create Source"}
          </Button>
      </DialogFooter>
    </DialogContent>
  );
}
