"use client";

import { useState, useCallback, useMemo } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Progress } from "@/components/ui/progress";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { FolderBrowser } from "./folder-browser";
import {
  K8sLabelSelector,
  type LabelSelectorValue,
} from "@/components/ui/k8s-label-selector";
import { useArenaSourceContent } from "@/hooks/use-arena-source-content";
import { useProviderPreview, useToolRegistryPreview } from "@/hooks";
import {
  ChevronLeft,
  ChevronRight,
  Rocket,
  Loader2,
  Check,
  AlertCircle,
  AlertTriangle,
  Lock,
  Settings,
  Plus,
  X,
} from "lucide-react";
import type {
  ArenaSource,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobType,
  ProviderGroupSelector,
  ToolRegistrySelector,
} from "@/types/arena";

// =============================================================================
// Types
// =============================================================================

const JOB_TYPES: {
  value: ArenaJobType;
  label: string;
  enterprise: boolean;
}[] = [
  { value: "evaluation", label: "Evaluation", enterprise: false },
  { value: "loadtest", label: "Load Test", enterprise: true },
  { value: "datagen", label: "Data Generation", enterprise: true },
];

const STEPS = [
  { title: "Basic Info", description: "Name and type" },
  { title: "Source", description: "Source and configuration" },
  { title: "Providers", description: "Provider overrides" },
  { title: "Tools", description: "Tool registry" },
  { title: "Options", description: "Workers and settings" },
  { title: "Review", description: "Create job" },
];

const DEFAULT_PROVIDER_GROUPS = ["default", "evaluation", "judge", "selfplay"];

interface JobWizardFormState {
  // Step 0: Basic Info
  name: string;
  type: ArenaJobType;

  // Step 1: Source
  sourceRef: string;
  rootPath: string;
  arenaFileName: string;

  // Step 2: Providers
  providerOverridesEnabled: boolean;
  providerOverrides: Record<string, LabelSelectorValue>;
  activeProviderGroups: string[];

  // Step 3: Tools
  toolRegistryOverrideEnabled: boolean;
  toolRegistryOverride: LabelSelectorValue;

  // Step 4: Options
  workers: string;
  timeout: string;
  verbose: boolean;

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

function getInitialFormState(preselectedSource?: string): JobWizardFormState {
  return {
    name: "",
    type: "evaluation",
    sourceRef: preselectedSource || "",
    rootPath: "",
    arenaFileName: "config.arena.yaml",
    providerOverridesEnabled: false,
    providerOverrides: {},
    activeProviderGroups: [],
    toolRegistryOverrideEnabled: false,
    toolRegistryOverride: {},
    workers: "2",
    timeout: "30m",
    verbose: false,
    continueOnFailure: true,
    passingThreshold: "0.8",
    profileType: "constant",
    duration: "5m",
    targetRPS: "10",
    sampleCount: "100",
    deduplicate: true,
  };
}

export interface JobWizardProps {
  sources: ArenaSource[];
  preselectedSource?: string;
  isEnterprise: boolean;
  maxWorkerReplicas: number;
  loading: boolean;
  onSubmit: (name: string, spec: ArenaJobSpec) => Promise<ArenaJob>;
  onSuccess?: () => void;
  onClose?: () => void;
}

// =============================================================================
// Helper Functions
// =============================================================================

function validateJobTypeOptions(form: JobWizardFormState): string | null {
  switch (form.type) {
    case "evaluation": {
      const threshold = parseFloat(form.passingThreshold);
      const isValidThreshold = !isNaN(threshold) && threshold >= 0 && threshold <= 1;
      return isValidThreshold ? null : "Passing threshold must be a number between 0 and 1";
    }
    case "loadtest": {
      const rps = Number.parseInt(form.targetRPS, 10);
      const isValidRps = !isNaN(rps) && rps >= 1;
      return isValidRps ? null : "Target RPS must be a positive integer";
    }
    case "datagen": {
      const count = Number.parseInt(form.sampleCount, 10);
      const isValidCount = !isNaN(count) && count >= 1;
      return isValidCount ? null : "Sample count must be a positive integer";
    }
    default:
      return null;
  }
}

function validateForm(
  form: JobWizardFormState,
  isEnterprise: boolean,
  maxWorkerReplicas: number
): string | null {
  if (!form.name.trim()) {
    return "Name is required";
  }
  if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(form.name)) {
    return "Name must be lowercase alphanumeric and may contain hyphens";
  }
  if (!form.sourceRef) {
    return "Source is required";
  }

  const jobType = JOB_TYPES.find((t) => t.value === form.type);
  if (jobType?.enterprise && !isEnterprise) {
    return `${jobType.label} requires an Enterprise license`;
  }

  const workers = Number.parseInt(form.workers, 10);
  if (isNaN(workers) || workers < 1) {
    return "Workers must be a positive integer";
  }

  if (maxWorkerReplicas > 0 && workers > maxWorkerReplicas) {
    return `Open Core is limited to ${maxWorkerReplicas} worker(s)`;
  }

  return validateJobTypeOptions(form);
}

function buildArenaFilePath(rootPath: string, fileName: string): string | undefined {
  if (!rootPath && !fileName) return undefined;
  if (!rootPath) return fileName;
  if (!fileName) return `${rootPath}/config.arena.yaml`;
  return `${rootPath}/${fileName}`;
}

function buildProviderOverrides(
  form: JobWizardFormState
): Record<string, ProviderGroupSelector> | undefined {
  if (!form.providerOverridesEnabled) return undefined;

  const overrides: Record<string, ProviderGroupSelector> = {};

  for (const group of form.activeProviderGroups) {
    const selector = form.providerOverrides[group];
    if (selector && (
      (selector.matchLabels && Object.keys(selector.matchLabels).length > 0) ||
      (selector.matchExpressions && selector.matchExpressions.length > 0)
    )) {
      overrides[group] = { selector };
    }
  }

  return Object.keys(overrides).length > 0 ? overrides : undefined;
}

function buildToolRegistryOverride(
  form: JobWizardFormState
): ToolRegistrySelector | undefined {
  if (!form.toolRegistryOverrideEnabled) return undefined;

  const selector = form.toolRegistryOverride;
  if (
    (selector.matchLabels && Object.keys(selector.matchLabels).length > 0) ||
    (selector.matchExpressions && selector.matchExpressions.length > 0)
  ) {
    return { selector };
  }

  return undefined;
}

function buildSpec(form: JobWizardFormState): ArenaJobSpec {
  const arenaFile = buildArenaFilePath(form.rootPath, form.arenaFileName);
  const spec: ArenaJobSpec = {
    sourceRef: { name: form.sourceRef },
    arenaFile: arenaFile || undefined,
    type: form.type,
    workers: {
      replicas: Number.parseInt(form.workers, 10),
    },
    timeout: form.timeout || undefined,
    verbose: form.verbose || undefined,
    providerOverrides: buildProviderOverrides(form),
    toolRegistryOverride: buildToolRegistryOverride(form),
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
      targetRPS: Number.parseInt(form.targetRPS, 10),
    };
  } else if (form.type === "datagen") {
    spec.datagen = {
      sampleCount: Number.parseInt(form.sampleCount, 10),
      deduplicate: form.deduplicate,
      outputFormat: "jsonl",
    };
  }

  return spec;
}

function getStepIndicatorClassName(stepIndex: number, currentStep: number): string {
  if (stepIndex < currentStep) return "bg-primary text-primary-foreground";
  if (stepIndex === currentStep) return "border-2 border-primary";
  return "border border-muted-foreground/30";
}

// =============================================================================
// Subcomponents
// =============================================================================

interface ProviderGroupSelectorEditorProps {
  group: string;
  selector: LabelSelectorValue;
  onChange: (selector: LabelSelectorValue) => void;
  onRemove: () => void;
  availableLabels: Record<string, string[]>;
}

function ProviderGroupSelectorEditor({
  group,
  selector,
  onChange,
  onRemove,
  availableLabels,
}: ProviderGroupSelectorEditorProps) {
  const { matchCount, totalCount, isLoading } = useProviderPreview(selector);

  return (
    <div className="space-y-2 rounded-md border p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="font-mono">
            {group}
          </Badge>
          {isLoading ? (
            <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
          ) : (
            <span className="text-xs text-muted-foreground">
              {matchCount} of {totalCount} providers match
            </span>
          )}
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onRemove}
          className="h-6 w-6 p-0"
        >
          <X className="h-4 w-4" />
        </Button>
      </div>
      <K8sLabelSelector
        value={selector}
        onChange={onChange}
        availableLabels={availableLabels}
        description={`Select providers to use for the "${group}" provider group`}
      />
    </div>
  );
}

interface ToolRegistryPreviewBadgeProps {
  selector: LabelSelectorValue | undefined;
}

function ToolRegistryPreviewBadge({ selector }: ToolRegistryPreviewBadgeProps) {
  const { matchCount, totalToolsCount, isLoading } = useToolRegistryPreview(selector);

  if (isLoading) {
    return (
      <Badge variant="secondary">
        <Loader2 className="h-3 w-3 animate-spin mr-1" />
        Loading...
      </Badge>
    );
  }

  return (
    <Badge variant={matchCount > 0 ? "default" : "secondary"}>
      {matchCount} registries ({totalToolsCount} tools)
    </Badge>
  );
}

// =============================================================================
// Main Component
// =============================================================================

export function JobWizard({
  sources,
  preselectedSource,
  isEnterprise,
  maxWorkerReplicas,
  loading,
  onSubmit,
  onSuccess,
  onClose,
}: Readonly<JobWizardProps>) {
  const [step, setStep] = useState(0);
  const [formState, setFormState] = useState<JobWizardFormState>(() =>
    getInitialFormState(preselectedSource)
  );
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Fetch source content for folder browser
  const {
    tree: sourceTree,
    loading: contentLoading,
    error: contentError,
  } = useArenaSourceContent(formState.sourceRef || undefined);

  // Get available labels for providers and tool registries
  const { availableLabels: providerLabels } = useProviderPreview(undefined);
  const { availableLabels: toolRegistryLabels } = useToolRegistryPreview(undefined);

  const updateField = useCallback(<K extends keyof JobWizardFormState>(
    field: K,
    value: JobWizardFormState[K]
  ) => {
    setFormState((prev) => ({ ...prev, [field]: value }));
  }, []);

  // Handle source change - reset paths when source changes
  const handleSourceChange = useCallback((newSourceRef: string) => {
    setFormState((prev) => ({
      ...prev,
      sourceRef: newSourceRef,
      rootPath: "",
      arenaFileName: "config.arena.yaml",
    }));
  }, []);

  const handleFolderSelect = useCallback((path: string) => {
    updateField("rootPath", path);
  }, [updateField]);

  const handleFileSelect = useCallback((filePath: string, folderPath: string, fileName: string) => {
    if (fileName.endsWith(".arena.yaml") || fileName.endsWith(".arena.yml")) {
      setFormState((prev) => ({
        ...prev,
        rootPath: folderPath,
        arenaFileName: fileName,
      }));
    } else {
      updateField("rootPath", folderPath);
    }
  }, [updateField]);

  // Provider group management
  const handleAddProviderGroup = useCallback((group: string) => {
    setFormState((prev) => ({
      ...prev,
      activeProviderGroups: [...prev.activeProviderGroups, group],
      providerOverrides: {
        ...prev.providerOverrides,
        [group]: {},
      },
    }));
  }, []);

  const handleRemoveProviderGroup = useCallback((group: string) => {
    setFormState((prev) => {
      const newGroups = prev.activeProviderGroups.filter((g) => g !== group);
      const newOverrides = { ...prev.providerOverrides };
      delete newOverrides[group];
      return {
        ...prev,
        activeProviderGroups: newGroups,
        providerOverrides: newOverrides,
      };
    });
  }, []);

  const handleProviderGroupSelectorChange = useCallback(
    (group: string, selector: LabelSelectorValue) => {
      setFormState((prev) => ({
        ...prev,
        providerOverrides: {
          ...prev.providerOverrides,
          [group]: selector,
        },
      }));
    },
    []
  );

  // Available groups for adding (exclude already active ones)
  const availableProviderGroups = useMemo(() => {
    const allGroups = new Set([
      ...DEFAULT_PROVIDER_GROUPS,
      ...formState.activeProviderGroups,
    ]);
    return Array.from(allGroups).filter(
      (g) => !formState.activeProviderGroups.includes(g)
    );
  }, [formState.activeProviderGroups]);

  const [newGroupName, setNewGroupName] = useState("");

  const canProceed = useCallback(() => {
    switch (step) {
      case 0: // Basic Info
        return formState.name.length > 0 && /^[a-z0-9-]+$/.test(formState.name);
      case 1: // Source
        return formState.sourceRef.length > 0;
      case 2: // Providers
        return true; // Optional step
      case 3: // Tools
        return true; // Optional step
      case 4: // Options
        return true;
      case 5: // Review
        return true;
      default:
        return false;
    }
  }, [step, formState]);

  const handleSubmit = async () => {
    try {
      setError(null);
      setIsSubmitting(true);

      const validationError = validateForm(formState, isEnterprise, maxWorkerReplicas);
      if (validationError) {
        setError(validationError);
        setIsSubmitting(false);
        return;
      }

      const spec = buildSpec(formState);
      await onSubmit(formState.name, spec);
      setSuccess(true);
      onSuccess?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create job");
    } finally {
      setIsSubmitting(false);
    }
  };

  const readySources = sources.filter((s) => s.status?.phase === "Ready");

  // Step rendering
  const renderStep = () => {
    switch (step) {
      case 0: // Basic Info
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Job Name</Label>
              <Input
                id="name"
                placeholder="my-job"
                value={formState.name}
                onChange={(e) =>
                  updateField(
                    "name",
                    e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-")
                  )
                }
              />
              <p className="text-xs text-muted-foreground">
                Lowercase letters, numbers, and hyphens only
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="type">Job Type</Label>
              <Select
                value={formState.type}
                onValueChange={(v) => updateField("type", v as ArenaJobType)}
              >
                <SelectTrigger id="type">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {JOB_TYPES.map((jobType) => {
                    const isLocked = jobType.enterprise && !isEnterprise;
                    return (
                      <SelectItem
                        key={jobType.value}
                        value={jobType.value}
                        disabled={isLocked}
                      >
                        <div className="flex items-center gap-2">
                          {isLocked && (
                            <Lock className="h-3 w-3 text-muted-foreground" />
                          )}
                          {jobType.label}
                          {isLocked && (
                            <Badge variant="outline" className="ml-1 text-xs">
                              Enterprise
                            </Badge>
                          )}
                        </div>
                      </SelectItem>
                    );
                  })}
                </SelectContent>
              </Select>
            </div>
          </div>
        );

      case 1: // Source
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="source">Source</Label>
              <Select
                value={formState.sourceRef}
                onValueChange={handleSourceChange}
              >
                <SelectTrigger id="source">
                  <SelectValue placeholder="Select a source" />
                </SelectTrigger>
                <SelectContent>
                  {readySources.length === 0 ? (
                    <div className="flex items-center gap-2 text-muted-foreground p-2 text-sm">
                      <Settings className="h-4 w-4" />
                      No ready sources available
                    </div>
                  ) : (
                    readySources.map((source) => (
                      <SelectItem
                        key={source.metadata?.name}
                        value={source.metadata?.name || "unknown"}
                      >
                        {source.metadata?.name}
                      </SelectItem>
                    ))
                  )}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Select the source containing arena configuration and scenarios
              </p>
            </div>

            {formState.sourceRef && (
              <div className="space-y-2">
                <Label>Root Folder</Label>
                <FolderBrowser
                  tree={sourceTree}
                  loading={contentLoading}
                  error={contentError?.message}
                  selectedPath={formState.rootPath}
                  onSelectFolder={handleFolderSelect}
                  onSelectFile={handleFileSelect}
                  maxHeight="180px"
                />
                <p className="text-xs text-muted-foreground">
                  Select a root folder or click an arena config file to auto-fill both
                </p>
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="arenaFileName">Arena Config File</Label>
              <div className="flex items-center gap-2">
                {formState.rootPath && (
                  <code className="text-xs bg-muted px-2 py-1 rounded text-muted-foreground">
                    {formState.rootPath}/
                  </code>
                )}
                <Input
                  id="arenaFileName"
                  placeholder="config.arena.yaml"
                  value={formState.arenaFileName}
                  onChange={(e) => updateField("arenaFileName", e.target.value)}
                  className="flex-1"
                />
              </div>
              <p className="text-xs text-muted-foreground">
                Full path:{" "}
                {buildArenaFilePath(formState.rootPath, formState.arenaFileName) ||
                  "config.arena.yaml"}
              </p>
            </div>
          </div>
        );

      case 2: // Providers
        return (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>Provider Overrides</Label>
                <p className="text-xs text-muted-foreground">
                  Override which providers to use for each provider group
                </p>
              </div>
              <Switch
                checked={formState.providerOverridesEnabled}
                onCheckedChange={(checked) =>
                  updateField("providerOverridesEnabled", checked)
                }
              />
            </div>

            {formState.providerOverridesEnabled && (
              <div className="space-y-4">
                {/* Active provider groups */}
                {formState.activeProviderGroups.map((group) => (
                  <ProviderGroupSelectorEditor
                    key={group}
                    group={group}
                    selector={formState.providerOverrides[group] || {}}
                    onChange={(selector) =>
                      handleProviderGroupSelectorChange(group, selector)
                    }
                    onRemove={() => handleRemoveProviderGroup(group)}
                    availableLabels={providerLabels}
                  />
                ))}

                {/* Add new group */}
                <div className="flex items-center gap-2">
                  <Select
                    value=""
                    onValueChange={(v) => {
                      if (v === "__custom__") {
                        // Do nothing, handled by custom input
                      } else {
                        handleAddProviderGroup(v);
                      }
                    }}
                  >
                    <SelectTrigger className="w-[180px]">
                      <SelectValue placeholder="Add provider group" />
                    </SelectTrigger>
                    <SelectContent>
                      {availableProviderGroups.map((group) => (
                        <SelectItem key={group} value={group}>
                          {group}
                        </SelectItem>
                      ))}
                      <SelectItem value="__custom__">Custom...</SelectItem>
                    </SelectContent>
                  </Select>

                  <Input
                    value={newGroupName}
                    onChange={(e) => setNewGroupName(e.target.value)}
                    placeholder="Custom group name"
                    className="w-[160px]"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => {
                      if (newGroupName.trim()) {
                        handleAddProviderGroup(newGroupName.trim());
                        setNewGroupName("");
                      }
                    }}
                    disabled={!newGroupName.trim()}
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                </div>

                {formState.activeProviderGroups.length === 0 && (
                  <p className="text-xs text-muted-foreground italic">
                    No provider groups configured. Add a group to override its
                    provider selection.
                  </p>
                )}
              </div>
            )}
          </div>
        );

      case 3: // Tools
        return (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label>Tool Registry Override</Label>
                <p className="text-xs text-muted-foreground">
                  Override which tool registries to use for this job
                </p>
              </div>
              <Switch
                checked={formState.toolRegistryOverrideEnabled}
                onCheckedChange={(checked) =>
                  updateField("toolRegistryOverrideEnabled", checked)
                }
              />
            </div>

            {formState.toolRegistryOverrideEnabled && (
              <div className="space-y-4">
                <K8sLabelSelector
                  value={formState.toolRegistryOverride}
                  onChange={(selector) =>
                    updateField("toolRegistryOverride", selector)
                  }
                  availableLabels={toolRegistryLabels}
                  description="Select tool registries by labels"
                  previewComponent={
                    <ToolRegistryPreviewBadge
                      selector={formState.toolRegistryOverride}
                    />
                  }
                />
              </div>
            )}
          </div>
        );

      case 4: // Options
        return (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="workers">Workers</Label>
                <Input
                  id="workers"
                  type="number"
                  min="1"
                  max={maxWorkerReplicas > 0 ? maxWorkerReplicas : undefined}
                  value={formState.workers}
                  onChange={(e) => updateField("workers", e.target.value)}
                />
                {maxWorkerReplicas > 0 && (
                  <p className="text-xs text-muted-foreground flex items-center gap-1">
                    <AlertTriangle className="h-3 w-3" />
                    Limited to {maxWorkerReplicas} worker
                    {maxWorkerReplicas === 1 ? "" : "s"} (upgrade for more)
                  </p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="timeout">Timeout</Label>
                <Input
                  id="timeout"
                  placeholder="30m"
                  value={formState.timeout}
                  onChange={(e) => updateField("timeout", e.target.value)}
                />
              </div>
            </div>

            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label htmlFor="verbose">Verbose Logging</Label>
                <p className="text-xs text-muted-foreground">
                  Enable debug output from promptarena for troubleshooting
                </p>
              </div>
              <Switch
                id="verbose"
                checked={formState.verbose}
                onCheckedChange={(checked) => updateField("verbose", checked)}
              />
            </div>

            {/* Type-specific options */}
            {formState.type === "evaluation" && (
              <div className="space-y-4 pt-2 border-t">
                <Label className="text-sm font-medium">Evaluation Options</Label>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label
                      htmlFor="threshold"
                      className="text-xs text-muted-foreground"
                    >
                      Passing Threshold
                    </Label>
                    <Input
                      id="threshold"
                      type="number"
                      step="0.1"
                      min="0"
                      max="1"
                      value={formState.passingThreshold}
                      onChange={(e) =>
                        updateField("passingThreshold", e.target.value)
                      }
                    />
                  </div>
                  <div className="space-y-2">
                    <Label className="text-xs text-muted-foreground">
                      Continue on Failure
                    </Label>
                    <div className="flex items-center h-10">
                      <Switch
                        checked={formState.continueOnFailure}
                        onCheckedChange={(checked) =>
                          updateField("continueOnFailure", checked)
                        }
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
                    <Label
                      htmlFor="profile"
                      className="text-xs text-muted-foreground"
                    >
                      Profile Type
                    </Label>
                    <Select
                      value={formState.profileType}
                      onValueChange={(v) => updateField("profileType", v)}
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
                    <Label
                      htmlFor="duration"
                      className="text-xs text-muted-foreground"
                    >
                      Duration
                    </Label>
                    <Input
                      id="duration"
                      placeholder="5m"
                      value={formState.duration}
                      onChange={(e) => updateField("duration", e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label
                      htmlFor="rps"
                      className="text-xs text-muted-foreground"
                    >
                      Target RPS
                    </Label>
                    <Input
                      id="rps"
                      type="number"
                      min="1"
                      value={formState.targetRPS}
                      onChange={(e) => updateField("targetRPS", e.target.value)}
                    />
                  </div>
                </div>
              </div>
            )}

            {formState.type === "datagen" && (
              <div className="space-y-4 pt-2 border-t">
                <Label className="text-sm font-medium">
                  Data Generation Options
                </Label>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label
                      htmlFor="samples"
                      className="text-xs text-muted-foreground"
                    >
                      Sample Count
                    </Label>
                    <Input
                      id="samples"
                      type="number"
                      min="1"
                      value={formState.sampleCount}
                      onChange={(e) =>
                        updateField("sampleCount", e.target.value)
                      }
                    />
                  </div>
                  <div className="space-y-2">
                    <Label className="text-xs text-muted-foreground">
                      Deduplicate
                    </Label>
                    <div className="flex items-center h-10">
                      <Switch
                        checked={formState.deduplicate}
                        onCheckedChange={(checked) =>
                          updateField("deduplicate", checked)
                        }
                      />
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>
        );

      case 5: // Review
        return (
          <div className="space-y-4">
            {success ? (
              <div className="flex flex-col items-center justify-center py-8 text-center">
                <div className="rounded-full bg-green-500/10 p-3 mb-4">
                  <Check className="h-8 w-8 text-green-500" />
                </div>
                <h3 className="text-lg font-semibold">Job Created!</h3>
                <p className="text-sm text-muted-foreground mt-1">
                  {formState.name} is being created
                </p>
              </div>
            ) : (
              <>
                <div className="flex items-center justify-between">
                  <h3 className="font-medium">Review Configuration</h3>
                  <Badge variant="outline">{formState.name}</Badge>
                </div>

                <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
                  <div className="text-muted-foreground">Job Type</div>
                  <div>
                    {JOB_TYPES.find((t) => t.value === formState.type)?.label}
                  </div>

                  <div className="text-muted-foreground">Source</div>
                  <div>{formState.sourceRef}</div>

                  <div className="text-muted-foreground">Arena File</div>
                  <div className="font-mono text-xs">
                    {buildArenaFilePath(
                      formState.rootPath,
                      formState.arenaFileName
                    ) || "config.arena.yaml"}
                  </div>

                  <div className="text-muted-foreground">Workers</div>
                  <div>{formState.workers}</div>

                  <div className="text-muted-foreground">Timeout</div>
                  <div>{formState.timeout || "30m"}</div>

                  {formState.providerOverridesEnabled &&
                    formState.activeProviderGroups.length > 0 && (
                      <>
                        <div className="text-muted-foreground">
                          Provider Overrides
                        </div>
                        <div>
                          {formState.activeProviderGroups.map((group) => (
                            <Badge
                              key={group}
                              variant="secondary"
                              className="mr-1 mb-1"
                            >
                              {group}
                            </Badge>
                          ))}
                        </div>
                      </>
                    )}

                  {formState.toolRegistryOverrideEnabled && (
                    <>
                      <div className="text-muted-foreground">
                        Tool Registry Override
                      </div>
                      <div>
                        <ToolRegistryPreviewBadge
                          selector={formState.toolRegistryOverride}
                        />
                      </div>
                    </>
                  )}
                </div>

                {error && (
                  <Alert variant="destructive">
                    <AlertCircle className="h-4 w-4" />
                    <AlertDescription>{error}</AlertDescription>
                  </Alert>
                )}
              </>
            )}
          </div>
        );

      default:
        return null;
    }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Progress */}
      <Progress value={((step + 1) / STEPS.length) * 100} className="h-1" />

      {/* Step indicators */}
      <div className="flex justify-between px-2 py-3">
        {STEPS.map((s, i) => (
          <div
            key={s.title}
            className={cn(
              "flex flex-col items-center",
              i <= step ? "text-primary" : "text-muted-foreground"
            )}
          >
            <div
              className={cn(
                "w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium",
                getStepIndicatorClassName(i, step)
              )}
            >
              {i < step ? <Check className="h-3 w-3" /> : i + 1}
            </div>
            <span className="text-[10px] mt-1 hidden sm:block">{s.title}</span>
          </div>
        ))}
      </div>

      {/* Form content */}
      <div className="flex-1 overflow-y-auto px-1 py-4 min-h-[300px]">
        {renderStep()}
      </div>

      {/* Navigation */}
      <div className="flex justify-between pt-4 border-t">
        <Button
          variant="outline"
          onClick={() => {
            if (step === 0) {
              onClose?.();
            } else {
              setStep((s) => Math.max(0, s - 1));
            }
          }}
          disabled={isSubmitting || success}
        >
          <ChevronLeft className="h-4 w-4 mr-1" />
          {step === 0 ? "Cancel" : "Back"}
        </Button>

        {step < STEPS.length - 1 ? (
          <Button onClick={() => setStep((s) => s + 1)} disabled={!canProceed()}>
            Next
            <ChevronRight className="h-4 w-4 ml-1" />
          </Button>
        ) : (
          <Button
            onClick={handleSubmit}
            disabled={isSubmitting || success || loading}
          >
            {isSubmitting || loading ? (
              <>
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                Creating...
              </>
            ) : success ? (
              <>
                <Check className="h-4 w-4 mr-2" />
                Created
              </>
            ) : (
              <>
                <Rocket className="h-4 w-4 mr-2" />
                Create Job
              </>
            )}
          </Button>
        )}
      </div>
    </div>
  );
}
