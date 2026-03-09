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
import { generateName } from "@/lib/name-generator";
import { FolderBrowser } from "./folder-browser";
import {
  K8sLabelSelector,
  type LabelSelectorValue,
} from "@/components/ui/k8s-label-selector";
import { useArenaSourceContent } from "@/hooks/use-arena-source-content";
import { useProviderPreview, useToolRegistryPreview, useAgents } from "@/hooks";
import { useWorkspace } from "@/contexts/workspace-context";
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
  Zap,
  Network,
} from "lucide-react";
import type {
  ArenaSource,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobType,
  ExecutionMode,
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

const ALL_STEPS = [
  { key: "basic", title: "Basic Info", description: "Name and type" },
  { key: "source", title: "Source", description: "Source and configuration" },
  { key: "execution", title: "Execution", description: "Direct or fleet mode" },
  { key: "providers", title: "Providers", description: "Provider overrides" },
  { key: "tools", title: "Tools", description: "Tool registry" },
  { key: "options", title: "Options", description: "Workers and settings" },
  { key: "review", title: "Review", description: "Create job" },
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

  // Step 2: Execution Mode
  executionMode: ExecutionMode;
  targetAgent: string;
  targetNamespace: string;

  // Step 3: Providers (direct mode only)
  providerOverridesEnabled: boolean;
  providerOverrides: Record<string, LabelSelectorValue>;
  activeProviderGroups: string[];

  // Step 4: Tools (direct mode only)
  toolRegistryOverrideEnabled: boolean;
  toolRegistryOverride: LabelSelectorValue;

  // Step 5: Options
  workers: string;
  verbose: boolean;

  // Load test options
  rampUp: string;
  duration: string;
  targetRPS: string;

  // Data gen options
  sampleCount: string;
}

function getInitialFormState(preselectedSource?: string): JobWizardFormState {
  return {
    name: generateName(),
    type: "evaluation",
    sourceRef: preselectedSource || "",
    rootPath: "",
    arenaFileName: "config.arena.yaml",
    executionMode: "direct",
    targetAgent: "",
    targetNamespace: "",
    providerOverridesEnabled: false,
    providerOverrides: {},
    activeProviderGroups: [],
    toolRegistryOverrideEnabled: false,
    toolRegistryOverride: {},
    workers: "1",
    verbose: false,
    rampUp: "30s",
    duration: "5m",
    targetRPS: "10",
    sampleCount: "100",
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
    case "evaluation":
      return null;
    case "loadtest": {
      const rps = Number.parseInt(form.targetRPS, 10);
      const isValidRps = !Number.isNaN(rps) && rps >= 1;
      return isValidRps ? null : "Target RPS must be a positive integer";
    }
    case "datagen": {
      const count = Number.parseInt(form.sampleCount, 10);
      const isValidCount = !Number.isNaN(count) && count >= 1;
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

  if (form.executionMode === "fleet" && !form.targetAgent) {
    return "Target agent is required for fleet mode";
  }

  const jobType = JOB_TYPES.find((t) => t.value === form.type);
  if (jobType?.enterprise && !isEnterprise) {
    return `${jobType.label} requires an Enterprise license`;
  }

  const workers = Number.parseInt(form.workers, 10);
  if (Number.isNaN(workers) || workers < 1) {
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
  if (form.activeProviderGroups.length === 0) return undefined;

  const overrides: Record<string, ProviderGroupSelector> = {};

  for (const group of form.activeProviderGroups) {
    const selector = form.providerOverrides[group] || {};
    // Build clean selector object (omit empty fields)
    const cleanSelector: ProviderGroupSelector["selector"] = {};

    if (selector.matchLabels && Object.keys(selector.matchLabels).length > 0) {
      cleanSelector.matchLabels = selector.matchLabels;
    }
    if (selector.matchExpressions && selector.matchExpressions.length > 0) {
      cleanSelector.matchExpressions = selector.matchExpressions;
    }

    // Include the group even with an empty selector (matches all providers)
    overrides[group] = { selector: cleanSelector };
  }

  return overrides;
}

function buildToolRegistryOverride(
  form: JobWizardFormState
): ToolRegistrySelector | undefined {
  if (!form.toolRegistryOverrideEnabled) return undefined;

  const selector = form.toolRegistryOverride || {};
  // Build clean selector object (omit empty fields)
  const cleanSelector: ToolRegistrySelector["selector"] = {};

  if (selector.matchLabels && Object.keys(selector.matchLabels).length > 0) {
    cleanSelector.matchLabels = selector.matchLabels;
  }
  if (selector.matchExpressions && selector.matchExpressions.length > 0) {
    cleanSelector.matchExpressions = selector.matchExpressions;
  }

  // Include override even with empty selector (matches all registries)
  return { selector: cleanSelector };
}

function buildSpec(form: JobWizardFormState): ArenaJobSpec {
  const arenaFile = buildArenaFilePath(form.rootPath, form.arenaFileName);
  const isFleet = form.executionMode === "fleet";

  const spec: ArenaJobSpec = {
    sourceRef: { name: form.sourceRef },
    arenaFile: arenaFile || undefined,
    type: form.type,
    workers: {
      replicas: Number.parseInt(form.workers, 10),
    },
    verbose: form.verbose || undefined,
    // Provider/tool overrides only apply in direct mode
    providerOverrides: isFleet ? undefined : buildProviderOverrides(form),
    toolRegistryOverride: isFleet ? undefined : buildToolRegistryOverride(form),
  };

  if (isFleet) {
    spec.execution = {
      mode: "fleet",
      target: {
        agentRuntimeRef: { name: form.targetAgent },
        namespace: form.targetNamespace || undefined,
      },
    };
  }

  if (form.type === "evaluation") {
    spec.evaluation = {
      outputFormats: ["json", "junit"],
    };
  } else if (form.type === "loadtest") {
    spec.loadTest = {
      rampUp: form.rampUp,
      duration: form.duration,
      targetRPS: Number.parseInt(form.targetRPS, 10),
    };
  } else if (form.type === "datagen") {
    spec.dataGen = {
      count: Number.parseInt(form.sampleCount, 10),
      format: "jsonl",
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

interface ExecutionStepProps {
  readonly formState: JobWizardFormState;
  readonly setFormState: React.Dispatch<React.SetStateAction<JobWizardFormState>>;
  readonly updateField: <K extends keyof JobWizardFormState>(
    field: K,
    value: JobWizardFormState[K]
  ) => void;
}

function ExecutionStep({
  formState,
  setFormState,
  updateField,
}: ExecutionStepProps) {
  const { workspaces, currentWorkspace } = useWorkspace();

  // Build unique namespace list from available workspaces
  const namespaces = useMemo(() => {
    const ns = workspaces.map((w) => w.namespace).filter(Boolean);
    return [...new Set(ns)];
  }, [workspaces]);

  const singleNamespace = namespaces.length <= 1;

  // Default targetNamespace to current workspace's namespace when entering fleet mode
  const effectiveNamespace = formState.targetNamespace || currentWorkspace?.namespace || "";

  // Find workspace name for the selected namespace so we can fetch its agents
  const workspaceForNamespace = useMemo(() => {
    return workspaces.find((w) => w.namespace === effectiveNamespace)?.name;
  }, [workspaces, effectiveNamespace]);

  // Fetch agents for the selected namespace's workspace
  const { data: agents, isLoading: agentsLoading } = useAgents({
    workspaceName: workspaceForNamespace,
  });

  const runningAgents = useMemo(() => {
    return (agents || []).filter((a) => a.status?.phase === "Running");
  }, [agents]);

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>Execution Mode</Label>
        <p className="text-xs text-muted-foreground">
          Choose how the job executes scenarios against LLMs
        </p>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <button
          type="button"
          className={cn(
            "flex flex-col items-start gap-2 rounded-lg border p-4 text-left transition-colors",
            formState.executionMode === "direct"
              ? "border-primary bg-primary/5"
              : "hover:bg-muted/50"
          )}
          onClick={() => {
            setFormState((prev) => ({
              ...prev,
              executionMode: "direct",
              targetAgent: "",
              targetNamespace: "",
            }));
          }}
        >
          <div className="flex items-center gap-2">
            <Zap className="h-5 w-5 text-amber-500" />
            <span className="font-medium">Direct</span>
          </div>
          <p className="text-xs text-muted-foreground">
            Workers call LLM providers directly. You can configure provider
            and tool registry overrides.
          </p>
        </button>

        <button
          type="button"
          className={cn(
            "flex flex-col items-start gap-2 rounded-lg border p-4 text-left transition-colors",
            formState.executionMode === "fleet"
              ? "border-primary bg-primary/5"
              : "hover:bg-muted/50"
          )}
          onClick={() => {
            setFormState((prev) => ({
              ...prev,
              executionMode: "fleet",
              targetNamespace: currentWorkspace?.namespace || "",
              providerOverridesEnabled: false,
              providerOverrides: {},
              activeProviderGroups: [],
              toolRegistryOverrideEnabled: false,
              toolRegistryOverride: {},
            }));
          }}
        >
          <div className="flex items-center gap-2">
            <Network className="h-5 w-5 text-blue-500" />
            <span className="font-medium">Fleet</span>
          </div>
          <p className="text-xs text-muted-foreground">
            Workers connect to a deployed agent via WebSocket. The agent uses its
            own providers and tools.
          </p>
        </button>
      </div>

      {formState.executionMode === "fleet" && (
        <div className="space-y-4 pt-2 border-t">
          <div className="space-y-2">
            <Label htmlFor="targetNamespace">Namespace</Label>
            <Select
              value={effectiveNamespace}
              onValueChange={(v) => {
                setFormState((prev) => ({
                  ...prev,
                  targetNamespace: v,
                  targetAgent: "",
                }));
              }}
              disabled={singleNamespace}
            >
              <SelectTrigger id="targetNamespace">
                <SelectValue placeholder="Select namespace" />
              </SelectTrigger>
              <SelectContent>
                {namespaces.map((ns) => (
                  <SelectItem key={ns} value={ns}>
                    {ns}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Namespace of the target agent
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="targetAgent">Target Agent</Label>
            <Select
              value={formState.targetAgent}
              onValueChange={(v) => updateField("targetAgent", v)}
              disabled={agentsLoading}
            >
              <SelectTrigger id="targetAgent">
                <SelectValue
                  placeholder={
                    agentsLoading ? "Loading agents..." : "Select an agent"
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {runningAgents.length === 0 ? (
                  <div className="flex items-center gap-2 text-muted-foreground p-2 text-sm">
                    <AlertTriangle className="h-4 w-4" />
                    No running agents available
                  </div>
                ) : (
                  runningAgents.map((agent) => (
                    <SelectItem
                      key={agent.metadata?.uid || agent.metadata?.name}
                      value={agent.metadata?.name || "unknown"}
                    >
                      {agent.metadata?.name}
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Select a running AgentRuntime to test against
            </p>
          </div>
        </div>
      )}
    </div>
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

  // Compute visible steps — fleet mode hides providers/tools
  const isFleetMode = formState.executionMode === "fleet";
  const steps = useMemo(() => {
    if (isFleetMode) {
      return ALL_STEPS.filter((s) => s.key !== "providers" && s.key !== "tools");
    }
    return ALL_STEPS;
  }, [isFleetMode]);

  const effectiveStep = Math.min(step, steps.length - 1);
  const currentStepKey = steps[effectiveStep]?.key ?? "basic";

  // Fetch source content for folder browser
  const {
    tree: sourceTree,
    loading: contentLoading,
    error: contentError,
  } = useArenaSourceContent(formState.sourceRef || undefined);

  // Get available labels for providers and tool registries
  const { availableLabels: providerLabels } = useProviderPreview(undefined);
  const { availableLabels: toolRegistryLabels } = useToolRegistryPreview(undefined);

  // Agents are fetched inside ExecutionStep based on selected namespace

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
    switch (currentStepKey) {
      case "basic":
        return formState.name.length > 0 && /^[a-z0-9-]+$/.test(formState.name);
      case "source":
        return formState.sourceRef.length > 0;
      case "execution":
        if (formState.executionMode === "fleet") {
          return formState.targetAgent.length > 0;
        }
        return true;
      case "providers":
        return true; // Optional step
      case "tools":
        return true; // Optional step
      case "options":
        return true;
      case "review":
        return true;
      default:
        return false;
    }
  }, [currentStepKey, formState]);

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
    switch (currentStepKey) {
      case "basic": // Basic Info
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
                    e.target.value.toLowerCase().replaceAll(/[^a-z0-9-]/g, "-")
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

      case "source": // Source
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

      case "execution": // Execution Mode
        return (
          <ExecutionStep
            formState={formState}
            setFormState={setFormState}
            updateField={updateField}
          />
        );

      case "providers": // Providers
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

      case "tools": // Tools
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

      case "options": // Options
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
            {formState.type === "loadtest" && (
              <div className="space-y-4 pt-2 border-t">
                <Label className="text-sm font-medium">Load Test Options</Label>
                <div className="grid grid-cols-3 gap-4">
                  <div className="space-y-2">
                    <Label
                      htmlFor="rampUp"
                      className="text-xs text-muted-foreground"
                    >
                      Ramp Up
                    </Label>
                    <Input
                      id="rampUp"
                      placeholder="30s"
                      value={formState.rampUp}
                      onChange={(e) => updateField("rampUp", e.target.value)}
                    />
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
              </div>
            )}
          </div>
        );

      case "review": // Review
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

                  <div className="text-muted-foreground">Execution Mode</div>
                  <div className="flex items-center gap-2">
                    {formState.executionMode === "fleet" ? (
                      <>
                        <Network className="h-3.5 w-3.5 text-blue-500" />
                        Fleet
                      </>
                    ) : (
                      <>
                        <Zap className="h-3.5 w-3.5 text-amber-500" />
                        Direct
                      </>
                    )}
                  </div>

                  {formState.executionMode === "fleet" && (
                    <>
                      <div className="text-muted-foreground">Target Agent</div>
                      <div>{formState.targetAgent}</div>
                      {formState.targetNamespace && (
                        <>
                          <div className="text-muted-foreground">
                            Target Namespace
                          </div>
                          <div>{formState.targetNamespace}</div>
                        </>
                      )}
                    </>
                  )}

                  <div className="text-muted-foreground">Workers</div>
                  <div>{formState.workers}</div>

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
      <Progress value={((effectiveStep + 1) / steps.length) * 100} className="h-1" />

      {/* Step indicators */}
      <div className="flex justify-between px-2 py-3">
        {steps.map((s, i) => (
          <div
            key={s.title}
            className={cn(
              "flex flex-col items-center",
              i <= effectiveStep ? "text-primary" : "text-muted-foreground"
            )}
          >
            <div
              className={cn(
                "w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium",
                getStepIndicatorClassName(i, effectiveStep)
              )}
            >
              {i < effectiveStep ? <Check className="h-3 w-3" /> : i + 1}
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
            if (effectiveStep === 0) {
              onClose?.();
            } else {
              setStep((s) => Math.max(0, s - 1));
            }
          }}
          disabled={isSubmitting || success}
        >
          <ChevronLeft className="h-4 w-4 mr-1" />
          {effectiveStep === 0 ? "Cancel" : "Back"}
        </Button>

        {effectiveStep < steps.length - 1 ? (
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
