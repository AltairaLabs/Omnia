"use client";

import { useState } from "react";
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
  Box,
  FileText,
  GitBranch,
  Loader2,
} from "lucide-react";
import { useSkillSourceMutations } from "@/hooks/use-skill-sources";
import type {
  ConfigMapSourceRef,
  GitSourceRef,
  OCISourceRef,
  SkillFilter,
  SkillSource,
  SkillSourceSpec,
  SkillSourceType,
} from "@/types/skill-source";

interface SkillSourceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  source?: SkillSource | null;
  onSuccess?: () => void;
}

interface FormState {
  name: string;
  type: SkillSourceType;
  interval: string;
  timeout: string;
  targetPath: string;
  suspend: boolean;
  git: GitSourceRef;
  oci: OCISourceRef;
  configMap: ConfigMapSourceRef;
  filterInclude: string;
  filterExclude: string;
  filterNames: string;
}

const DEFAULT_FORM: FormState = {
  name: "",
  type: "configmap",
  interval: "1h",
  timeout: "60s",
  targetPath: "",
  suspend: false,
  git: { url: "", ref: { branch: "main" } },
  oci: { url: "" },
  configMap: { name: "" },
  filterInclude: "",
  filterExclude: "",
  filterNames: "",
};

function initialForm(source?: SkillSource | null): FormState {
  if (!source) return DEFAULT_FORM;
  const spec = source.spec;
  return {
    name: source.metadata.name ?? "",
    type: spec.type,
    interval: spec.interval ?? "1h",
    timeout: spec.timeout ?? "60s",
    targetPath: spec.targetPath ?? "",
    suspend: spec.suspend ?? false,
    git: spec.git ?? { url: "" },
    oci: spec.oci ?? { url: "" },
    configMap: spec.configMap ?? { name: "" },
    filterInclude: (spec.filter?.include ?? []).join(", "),
    filterExclude: (spec.filter?.exclude ?? []).join(", "),
    filterNames: (spec.filter?.names ?? []).join(", "),
  };
}

function splitCsv(s: string): string[] {
  return s
    .split(",")
    .map((v) => v.trim())
    .filter((v) => v.length > 0);
}

function buildFilter(form: FormState): SkillFilter | undefined {
  const include = splitCsv(form.filterInclude);
  const exclude = splitCsv(form.filterExclude);
  const names = splitCsv(form.filterNames);
  if (include.length === 0 && exclude.length === 0 && names.length === 0) {
    return undefined;
  }
  const filter: SkillFilter = {};
  if (include.length) filter.include = include;
  if (exclude.length) filter.exclude = exclude;
  if (names.length) filter.names = names;
  return filter;
}

function buildSpec(form: FormState): SkillSourceSpec {
  const spec: SkillSourceSpec = {
    type: form.type,
    interval: form.interval,
  };
  if (form.timeout) spec.timeout = form.timeout;
  if (form.targetPath) spec.targetPath = form.targetPath;
  if (form.suspend) spec.suspend = true;
  const filter = buildFilter(form);
  if (filter) spec.filter = filter;
  switch (form.type) {
    case "git":
      spec.git = form.git;
      break;
    case "oci":
      spec.oci = form.oci;
      break;
    case "configmap":
      spec.configMap = form.configMap;
      break;
  }
  return spec;
}

function validate(form: FormState): string | null {
  if (!form.name.trim()) return "Name is required";
  if (!/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(form.name)) {
    return "Name must be lowercase alphanumeric with dashes (DNS-1123)";
  }
  if (!form.interval.trim()) return "Interval is required";
  if (form.type === "git" && !form.git.url) {
    return "Git repository URL is required";
  }
  if (form.type === "oci" && !form.oci.url) {
    return "OCI registry URL is required";
  }
  if (form.type === "configmap" && !form.configMap.name) {
    return "ConfigMap name is required";
  }
  return null;
}

const TYPE_OPTIONS: {
  value: SkillSourceType;
  label: string;
  icon: React.ReactNode;
}[] = [
  { value: "configmap", label: "ConfigMap", icon: <FileText className="h-4 w-4" /> },
  { value: "git", label: "Git", icon: <GitBranch className="h-4 w-4" /> },
  { value: "oci", label: "OCI Registry", icon: <Box className="h-4 w-4" /> },
];

function GitFields({
  spec,
  onChange,
}: Readonly<{
  spec: GitSourceRef;
  onChange: (spec: GitSourceRef) => void;
}>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="git-url">Repository URL</Label>
        <Input
          id="git-url"
          placeholder="https://github.com/org/skills.git"
          value={spec.url}
          onChange={(e) => onChange({ ...spec, url: e.target.value })}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="git-branch">Branch</Label>
          <Input
            id="git-branch"
            placeholder="main"
            value={spec.ref?.branch ?? ""}
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
            placeholder="skills/"
            value={spec.path ?? ""}
            onChange={(e) =>
              onChange({ ...spec, path: e.target.value || undefined })
            }
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="git-secret">Secret (optional)</Label>
        <Input
          id="git-secret"
          placeholder="git-credentials"
          value={spec.secretRef?.name ?? ""}
          onChange={(e) =>
            onChange({
              ...spec,
              secretRef: e.target.value
                ? { name: e.target.value }
                : undefined,
            })
          }
        />
      </div>
    </div>
  );
}

function OCIFields({
  spec,
  onChange,
}: Readonly<{
  spec: OCISourceRef;
  onChange: (spec: OCISourceRef) => void;
}>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="oci-url">OCI URL</Label>
        <Input
          id="oci-url"
          placeholder="oci://ghcr.io/org/skills"
          value={spec.url}
          onChange={(e) => onChange({ ...spec, url: e.target.value })}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="oci-secret">Secret (optional)</Label>
          <Input
            id="oci-secret"
            placeholder="registry-creds"
            value={spec.secretRef?.name ?? ""}
            onChange={(e) =>
              onChange({
                ...spec,
                secretRef: e.target.value
                  ? { name: e.target.value }
                  : undefined,
              })
            }
          />
        </div>
        <div className="flex items-center gap-2 pt-8">
          <Switch
            id="oci-insecure"
            checked={spec.insecure ?? false}
            onCheckedChange={(checked) =>
              onChange({ ...spec, insecure: checked })
            }
          />
          <Label htmlFor="oci-insecure" className="cursor-pointer">
            Allow insecure (HTTP) pulls
          </Label>
        </div>
      </div>
    </div>
  );
}

function ConfigMapFields({
  spec,
  onChange,
}: Readonly<{
  spec: ConfigMapSourceRef;
  onChange: (spec: ConfigMapSourceRef) => void;
}>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="cm-name">ConfigMap name</Label>
        <Input
          id="cm-name"
          placeholder="my-skills"
          value={spec.name}
          onChange={(e) => onChange({ ...spec, name: e.target.value })}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="cm-key">Key (optional)</Label>
        <Input
          id="cm-key"
          placeholder="pack.json"
          value={spec.key ?? ""}
          onChange={(e) =>
            onChange({ ...spec, key: e.target.value || undefined })
          }
        />
      </div>
    </div>
  );
}

export function SkillSourceDialog({
  open,
  onOpenChange,
  source,
  onSuccess,
}: Readonly<SkillSourceDialogProps>) {
  // Remount the inner form each time the dialog opens or the editing target
  // changes, so the form state is re-initialised from the latest source
  // without a setState-in-effect pattern.
  const formKey = open
    ? `open:${source?.metadata?.name ?? "__new__"}`
    : "closed";
  return (
    <SkillSourceDialogInner
      key={formKey}
      open={open}
      onOpenChange={onOpenChange}
      source={source}
      onSuccess={onSuccess}
    />
  );
}

function SkillSourceDialogInner({
  open,
  onOpenChange,
  source,
  onSuccess,
}: Readonly<SkillSourceDialogProps>) {
  const isEdit = !!source;
  const { createSource, updateSource, loading } = useSkillSourceMutations();
  const [form, setForm] = useState<FormState>(() => initialForm(source));
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const msg = validate(form);
    if (msg) {
      setError(msg);
      return;
    }
    setError(null);
    try {
      const spec = buildSpec(form);
      if (isEdit && source) {
        await updateSource(source.metadata.name ?? form.name, spec);
      } else {
        await createSource(form.name, spec);
      }
      onSuccess?.();
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>
              {isEdit ? "Edit SkillSource" : "Create SkillSource"}
            </DialogTitle>
            <DialogDescription>
              SkillSources pull SKILL.md content from Git, OCI registries, or
              ConfigMaps and expose it to PromptPacks via{" "}
              <code className="font-mono">spec.skills</code>.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                placeholder="anthropic-skills"
                value={form.name}
                disabled={isEdit}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="type">Source type</Label>
              <Select
                value={form.type}
                onValueChange={(value) =>
                  setForm({ ...form, type: value as SkillSourceType })
                }
              >
                <SelectTrigger id="type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TYPE_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      <div className="flex items-center gap-2">
                        {opt.icon}
                        {opt.label}
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {form.type === "git" && (
              <GitFields
                spec={form.git}
                onChange={(git) => setForm({ ...form, git })}
              />
            )}
            {form.type === "oci" && (
              <OCIFields
                spec={form.oci}
                onChange={(oci) => setForm({ ...form, oci })}
              />
            )}
            {form.type === "configmap" && (
              <ConfigMapFields
                spec={form.configMap}
                onChange={(configMap) => setForm({ ...form, configMap })}
              />
            )}

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="interval">Reconcile interval</Label>
                <Input
                  id="interval"
                  placeholder="1h"
                  value={form.interval}
                  onChange={(e) =>
                    setForm({ ...form, interval: e.target.value })
                  }
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="timeout">Fetch timeout</Label>
                <Input
                  id="timeout"
                  placeholder="60s"
                  value={form.timeout}
                  onChange={(e) =>
                    setForm({ ...form, timeout: e.target.value })
                  }
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="target-path">Target path (optional)</Label>
              <Input
                id="target-path"
                placeholder="skills/anthropic"
                value={form.targetPath}
                onChange={(e) =>
                  setForm({ ...form, targetPath: e.target.value })
                }
              />
            </div>

            <div className="space-y-2">
              <Label>Filter (comma-separated globs / names)</Label>
              <Input
                placeholder="Include: billing/*, ops/*"
                value={form.filterInclude}
                onChange={(e) =>
                  setForm({ ...form, filterInclude: e.target.value })
                }
              />
              <Input
                placeholder="Exclude: **/draft/**"
                value={form.filterExclude}
                onChange={(e) =>
                  setForm({ ...form, filterExclude: e.target.value })
                }
              />
              <Input
                placeholder="Names: refund-processing, order-lookup"
                value={form.filterNames}
                onChange={(e) =>
                  setForm({ ...form, filterNames: e.target.value })
                }
              />
            </div>

            <div className="flex items-center gap-2">
              <Switch
                id="suspend"
                checked={form.suspend}
                onCheckedChange={(checked) =>
                  setForm({ ...form, suspend: checked })
                }
              />
              <Label htmlFor="suspend" className="cursor-pointer">
                Suspend reconciliation
              </Label>
            </div>

            {error && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={loading}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={loading}>
              {loading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              {isEdit ? "Save" : "Create"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
