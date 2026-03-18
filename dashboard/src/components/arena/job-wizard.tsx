"use client";

import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Progress } from "@/components/ui/progress";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Checkbox } from "@/components/ui/checkbox";
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
import { useArenaSourceContent, useArenaConfigPreview, estimateWorkItems } from "@/hooks/arena";
import { useAgents } from "@/hooks/agents";
import { useProviders, useToolRegistries } from "@/hooks/resources";
import {
  ChevronLeft,
  ChevronRight,
  Rocket,
  Loader2,
  Check,
  AlertCircle,
  AlertTriangle,
  Info,
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
} from "@/types/arena";
import type { ConfigProviderRef } from "@/hooks/use-arena-config-preview";
import {
  validateForm,
  buildSpec,
  buildArenaFilePath,
  countTotalEntries,
  groupSummary,
  getStepIndicatorClassName,
  getInitialFormState,
  type ProviderGroupEntry,
  type JobWizardFormState,
} from "./job-wizard-utils";

// =============================================================================
// Types
// =============================================================================

const WIZARD_STEPS = [
  { key: "basic", title: "Basic Info", description: "Job name" },
  { key: "source", title: "Source", description: "Source and configuration" },
  { key: "providers", title: "Providers", description: "Provider groups" },
  { key: "tools", title: "Tools", description: "Tool registries" },
  { key: "options", title: "Options & Review", description: "Workers and settings" },
];

const DEFAULT_PROVIDER_GROUPS = ["default", "judge", "selfplay"];

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
// Subcomponents
// =============================================================================

interface ProviderGroupEditorProps {
  readonly group: string;
  readonly entries: ProviderGroupEntry[];
  readonly required: boolean;
  readonly onAddEntry: (entry: ProviderGroupEntry) => void;
  readonly onRemoveEntry: (index: number) => void;
  readonly onRemoveGroup: () => void;
  readonly availableProviders: { name: string }[];
  readonly availableAgents: { name: string }[];
}

function ProviderGroupEditor({
  group,
  entries,
  required,
  onAddEntry,
  onRemoveEntry,
  onRemoveGroup,
  availableProviders,
  availableAgents,
}: ProviderGroupEditorProps) {
  // Build combined picker options, excluding already-added entries
  const pickerOptions = useMemo(() => {
    const addedNames = new Set(entries.map((e) => `${e.type}:${e.name}`));
    const opts: { type: "provider" | "agent"; name: string }[] = [];
    for (const p of availableProviders) {
      if (!addedNames.has(`provider:${p.name}`)) {
        opts.push({ type: "provider", name: p.name });
      }
    }
    for (const a of availableAgents) {
      if (!addedNames.has(`agent:${a.name}`)) {
        opts.push({ type: "agent", name: a.name });
      }
    }
    return opts;
  }, [availableProviders, availableAgents, entries]);

  const handleAdd = useCallback(
    (value: string) => {
      const [type, name] = value.split(":") as ["provider" | "agent", string];
      if (type && name) {
        onAddEntry({ type, name });
      }
    },
    [onAddEntry]
  );

  return (
    <div className="space-y-2 rounded-md border p-3">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Badge variant="outline" className="font-mono">
            {group}
          </Badge>
          {required && (
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
              required
            </Badge>
          )}
          {entries.length === 0 ? (
            required ? (
              <Badge variant="destructive" className="text-[10px] px-1.5 py-0">
                <AlertCircle className="h-3 w-3 mr-0.5" />
                empty
              </Badge>
            ) : (
              <span className="text-xs text-muted-foreground">No entries</span>
            )
          ) : (
            <span className="text-xs text-muted-foreground">
              {groupSummary(entries)}
            </span>
          )}
        </div>
        {!required && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onRemoveGroup}
            className="h-6 w-6 p-0"
          >
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>

      {/* Existing entries */}
      {entries.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {entries.map((entry, i) => (
            <Badge
              key={`${entry.type}-${entry.name}`}
              variant="secondary"
              className="flex items-center gap-1"
            >
              {entry.type === "agent" ? (
                <Network className="h-3 w-3 text-blue-500" />
              ) : (
                <Zap className="h-3 w-3 text-amber-500" />
              )}
              {entry.name}
              <button
                type="button"
                onClick={() => onRemoveEntry(i)}
                className="ml-1 hover:text-destructive"
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}

      {/* Add entry picker */}
      {pickerOptions.length > 0 && (
        <Select value="" onValueChange={handleAdd}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Add provider or agent..." />
          </SelectTrigger>
          <SelectContent>
            {pickerOptions.map((opt) => (
              <SelectItem
                key={`${opt.type}:${opt.name}`}
                value={`${opt.type}:${opt.name}`}
              >
                <span className="flex items-center gap-2">
                  {opt.type === "agent" ? (
                    <Network className="h-3.5 w-3.5 text-blue-500" />
                  ) : (
                    <Zap className="h-3.5 w-3.5 text-amber-500" />
                  )}
                  {opt.name}
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}

interface ProviderMappingEditorProps {
  readonly group: string;
  readonly mapping: Record<string, ProviderGroupEntry | null>;
  readonly availableProviders: { name: string }[];
  readonly availableAgents: { name: string }[];
  readonly onSetMapping: (group: string, configID: string, entry: ProviderGroupEntry | null) => void;
}

function ProviderMappingEditor({
  group,
  mapping,
  availableProviders,
  availableAgents,
  onSetMapping,
}: ProviderMappingEditorProps) {
  const handleChange = useCallback(
    (configID: string, v: string) => {
      if (!v) {
        onSetMapping(group, configID, null);
        return;
      }
      const [type, name] = v.split(":") as ["provider" | "agent", string];
      if (type && name) {
        onSetMapping(group, configID, { type, name });
      }
    },
    [group, onSetMapping]
  );

  return (
    <div className="space-y-2 rounded-md border p-3">
      <div className="flex items-center gap-2">
        <Badge variant="outline" className="font-mono">
          {group}
        </Badge>
        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
          mapped
        </Badge>
      </div>
      {Object.entries(mapping).map(([configID, entry]) => (
        <div key={configID} className="flex items-center gap-2">
          <code className="text-xs bg-muted px-2 py-1 rounded min-w-[120px]">
            {configID}
          </code>
          <Select
            value={entry ? `${entry.type}:${entry.name}` : ""}
            onValueChange={(v) => handleChange(configID, v)}
          >
            <SelectTrigger className="flex-1">
              <SelectValue placeholder="Select provider or agent..." />
            </SelectTrigger>
            <SelectContent>
              {availableProviders.map((p) => (
                <SelectItem key={`provider:${p.name}`} value={`provider:${p.name}`}>
                  <span className="flex items-center gap-2">
                    <Zap className="h-3.5 w-3.5 text-amber-500" />
                    {p.name}
                  </span>
                </SelectItem>
              ))}
              {availableAgents.map((a) => (
                <SelectItem key={`agent:${a.name}`} value={`agent:${a.name}`}>
                  <span className="flex items-center gap-2">
                    <Network className="h-3.5 w-3.5 text-blue-500" />
                    {a.name}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {!entry && (
            <AlertCircle className="h-4 w-4 text-destructive shrink-0" />
          )}
        </div>
      ))}
    </div>
  );
}

// =============================================================================
// Main Component
// =============================================================================

export function JobWizard({
  sources,
  preselectedSource,
  maxWorkerReplicas,
  loading,
  onSubmit,
  onSuccess,
  onClose,
}: Readonly<JobWizardProps>) {
  const [step, setStep] = useState(0);
  const [formState, setFormState] = useState<JobWizardFormState>(() =>
    getInitialFormState(preselectedSource, generateName())
  );
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const steps = WIZARD_STEPS;
  const effectiveStep = Math.min(step, steps.length - 1);
  const currentStepKey = steps[effectiveStep]?.key ?? "basic";

  // Fetch source content for folder browser
  const {
    tree: sourceTree,
    loading: contentLoading,
    error: contentError,
  } = useArenaSourceContent(formState.sourceRef || undefined);

  // Fetch providers and agents for picker
  const { data: providerList, isLoading: providersLoading } = useProviders();
  const { data: agentList, isLoading: agentsLoading } = useAgents({});
  const { data: toolRegistryList, isLoading: toolRegistriesLoading } = useToolRegistries();

  const availableProviders = useMemo(
    () => (providerList || []).map((p) => ({ name: p.metadata.name })),
    [providerList]
  );

  const availableAgents = useMemo(
    () => (agentList || []).map((a) => ({ name: a.metadata?.name || "" })).filter((a) => a.name),
    [agentList]
  );

  // Fetch arena config for work item estimation
  const arenaFilePath = buildArenaFilePath(formState.rootPath, formState.arenaFileName);
  const configPreview = useArenaConfigPreview(
    formState.sourceRef || undefined,
    arenaFilePath
  );

  const totalProviderEntries = countTotalEntries(formState.providerGroups);
  const workEstimate = useMemo(
    () => estimateWorkItems(configPreview, totalProviderEntries, maxWorkerReplicas),
    [configPreview, totalProviderEntries, maxWorkerReplicas]
  );

  // Determine which groups should be map-mode based on providerRefs
  const mapModeGroups = useMemo(() => {
    if (!configPreview.loaded) return new Map<string, ConfigProviderRef[]>();
    // Group providerRefs by their source category to determine map-mode groups
    const grouped = new Map<string, ConfigProviderRef[]>();
    for (const ref of configPreview.providerRefs) {
      // Use the source as the group name (judges, judge_specs, self_play → selfplay, judges)
      const groupName = ref.source === "self_play" ? "selfplay" : ref.source;
      if (!grouped.has(groupName)) {
        grouped.set(groupName, []);
      }
      grouped.get(groupName)!.push(ref);
    }
    return grouped;
  }, [configPreview.loaded, configPreview.providerRefs]);

  // Auto-populate provider groups required by the arena config
  const prevRequiredGroups = useRef<string[]>([]);
  const prevProviderRefs = useRef<ConfigProviderRef[]>([]);
  useEffect(() => {
    if (!configPreview.loaded) return;

    const required = configPreview.requiredGroups;
    const refs = configPreview.providerRefs;

    // Skip if nothing changed
    const refsChanged = refs.length !== prevProviderRefs.current.length ||
      refs.some((r, i) => r.id !== prevProviderRefs.current[i]?.id);
    const groupsChanged = required.length !== prevRequiredGroups.current.length ||
      !required.every((g) => prevRequiredGroups.current.includes(g));

    if (!refsChanged && !groupsChanged) return;
    prevRequiredGroups.current = required;
    prevProviderRefs.current = refs;

    setFormState((prev) => {
      const updatedGroups = { ...prev.providerGroups };
      const updatedMappings = { ...prev.providerMappings };

      for (const group of required) {
        // Check if this group should be map-mode
        const mapRefs = mapModeGroups.get(group);
        if (mapRefs && mapRefs.length > 0) {
          // Map-mode: create mapping entries for each provider ref
          if (!(group in updatedMappings)) {
            const mapping: Record<string, ProviderGroupEntry | null> = {};
            for (const ref of mapRefs) {
              mapping[ref.id] = null;
            }
            updatedMappings[group] = mapping;
          }
        } else {
          // Array-mode: ensure group exists
          if (!(group in updatedGroups)) {
            updatedGroups[group] = [];
          }
        }
      }

      return { ...prev, providerGroups: updatedGroups, providerMappings: updatedMappings };
    });
  }, [configPreview.loaded, configPreview.requiredGroups, configPreview.providerRefs, mapModeGroups]);

  // Auto-update workers when the recommended count changes
  const prevRecommended = useRef(workEstimate.recommendedWorkers);
  useEffect(() => {
    if (
      configPreview.loaded &&
      workEstimate.recommendedWorkers !== prevRecommended.current
    ) {
      prevRecommended.current = workEstimate.recommendedWorkers;
      setFormState((prev) => ({
        ...prev,
        workers: String(workEstimate.recommendedWorkers),
      }));
    }
  }, [configPreview.loaded, workEstimate.recommendedWorkers]);

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
      providerGroups: {
        ...prev.providerGroups,
        [group]: [],
      },
    }));
  }, []);

  const handleRemoveProviderGroup = useCallback((group: string) => {
    setFormState((prev) => {
      const newGroups = { ...prev.providerGroups };
      delete newGroups[group];
      return { ...prev, providerGroups: newGroups };
    });
  }, []);

  const handleAddEntryToGroup = useCallback((group: string, entry: ProviderGroupEntry) => {
    setFormState((prev) => ({
      ...prev,
      providerGroups: {
        ...prev.providerGroups,
        [group]: [...(prev.providerGroups[group] || []), entry],
      },
    }));
  }, []);

  const handleRemoveEntryFromGroup = useCallback((group: string, index: number) => {
    setFormState((prev) => {
      const entries = [...(prev.providerGroups[group] || [])];
      entries.splice(index, 1);
      return {
        ...prev,
        providerGroups: {
          ...prev.providerGroups,
          [group]: entries,
        },
      };
    });
  }, []);

  // Tool registry selection
  const handleToggleToolRegistry = useCallback((name: string) => {
    setFormState((prev) => {
      const selected = prev.selectedToolRegistries.includes(name)
        ? prev.selectedToolRegistries.filter((n) => n !== name)
        : [...prev.selectedToolRegistries, name];
      return { ...prev, selectedToolRegistries: selected };
    });
  }, []);

  // Available groups for adding (exclude already active ones)
  const activeGroupNames = Object.keys(formState.providerGroups);
  const availableGroupNames = useMemo(() => {
    return DEFAULT_PROVIDER_GROUPS.filter((g) => !activeGroupNames.includes(g));
  }, [activeGroupNames]);

  const [newGroupName, setNewGroupName] = useState("");

  const canProceed = useCallback(() => {
    switch (currentStepKey) {
      case "basic":
        return formState.name.length > 0 && /^[a-z0-9-]+$/.test(formState.name);
      case "source":
        return formState.sourceRef.length > 0;
      case "providers":
        return true; // Optional step
      case "tools":
        return true; // Optional step
      case "options":
        return true;
      default:
        return false;
    }
  }, [currentStepKey, formState]);

  const handleSubmit = async () => {
    try {
      setError(null);
      setIsSubmitting(true);

      const validationError = validateForm(formState, maxWorkerReplicas, configPreview.requiredGroups);
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
      case "basic":
        return renderBasicStep();
      case "source":
        return renderSourceStep();
      case "providers":
        return renderProvidersStep();
      case "tools":
        return renderToolsStep();
      case "options":
        return renderOptionsAndReviewStep();
      default:
        return null;
    }
  };

  const renderBasicStep = () => (
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
    </div>
  );

  const renderSourceStep = () => (
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

  // Provider mapping management
  const handleSetMapping = useCallback((group: string, configID: string, entry: ProviderGroupEntry | null) => {
    setFormState((prev) => ({
      ...prev,
      providerMappings: {
        ...prev.providerMappings,
        [group]: {
          ...prev.providerMappings[group],
          [configID]: entry,
        },
      },
    }));
  }, []);

  const activeMappingGroups = Object.keys(formState.providerMappings);

  const renderProvidersStep = () => (
    <div className="space-y-4">
      {(providersLoading || agentsLoading) && (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading providers and agents...
        </div>
      )}

      {/* Map-mode groups: Provider Mappings */}
      {activeMappingGroups.length > 0 && (
        <>
          <div className="space-y-0.5">
            <Label>Provider Mappings</Label>
            <p className="text-xs text-muted-foreground">
              Select a CRD provider or agent for each config-referenced provider ID.
            </p>
          </div>

          {activeMappingGroups.map((group) => (
            <ProviderMappingEditor
              key={group}
              group={group}
              mapping={formState.providerMappings[group]}
              availableProviders={availableProviders}
              availableAgents={availableAgents}
              onSetMapping={handleSetMapping}
            />
          ))}
        </>
      )}

      {/* Array-mode groups: Test Providers */}
      <div className="space-y-0.5">
        <Label>Test Providers</Label>
        <p className="text-xs text-muted-foreground">
          Configure which providers and agents to use for each group.
          Leave empty to use defaults from the arena config.
        </p>
      </div>

      {/* Active provider groups */}
      {activeGroupNames.map((group) => (
        <ProviderGroupEditor
          key={group}
          group={group}
          entries={formState.providerGroups[group] || []}
          required={configPreview.requiredGroups.includes(group)}
          onAddEntry={(entry) => handleAddEntryToGroup(group, entry)}
          onRemoveEntry={(index) => handleRemoveEntryFromGroup(group, index)}
          onRemoveGroup={() => handleRemoveProviderGroup(group)}
          availableProviders={availableProviders}
          availableAgents={availableAgents}
        />
      ))}

      {/* Add new group */}
      <div className="flex items-center gap-2">
        <Select
          value=""
          onValueChange={(v) => {
            if (v !== "__custom__") {
              handleAddProviderGroup(v);
            }
          }}
        >
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="Add group" />
          </SelectTrigger>
          <SelectContent>
            {availableGroupNames.map((group) => (
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

      {activeGroupNames.length === 0 && activeMappingGroups.length === 0 && (
        <p className="text-xs text-muted-foreground italic">
          No provider groups configured. The arena config defaults will be used.
        </p>
      )}
    </div>
  );

  const renderToolsStep = () => (
    <div className="space-y-4">
      <div className="space-y-0.5">
        <Label>Tool Registries</Label>
        <p className="text-xs text-muted-foreground">
          Select ToolRegistry CRDs to include in this job.
          Leave empty to use defaults from the arena config.
        </p>
      </div>

      {toolRegistriesLoading && (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading tool registries...
        </div>
      )}

      {!toolRegistriesLoading && toolRegistryList && toolRegistryList.length === 0 && (
        <p className="text-xs text-muted-foreground italic">
          No tool registries found in this workspace.
        </p>
      )}

      {toolRegistryList && toolRegistryList.length > 0 && (
        <div className="space-y-2">
          {toolRegistryList.map((registry) => {
            const regName = registry.metadata.name;
            const toolCount = registry.status?.discoveredToolsCount ?? 0;
            const checked = formState.selectedToolRegistries.includes(regName);
            return (
              <div
                key={regName}
                className="flex items-center space-x-3 rounded-md border p-3"
              >
                <Checkbox
                  id={`tr-${regName}`}
                  checked={checked}
                  onCheckedChange={() => handleToggleToolRegistry(regName)}
                />
                <div className="flex-1">
                  <Label
                    htmlFor={`tr-${regName}`}
                    className="text-sm font-medium cursor-pointer"
                  >
                    {regName}
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    {toolCount} tool{toolCount === 1 ? "" : "s"} discovered
                  </p>
                </div>
                <Badge variant={registry.status?.phase === "Ready" ? "default" : "secondary"}>
                  {registry.status?.phase || "Unknown"}
                </Badge>
              </div>
            );
          })}
        </div>
      )}

      {formState.selectedToolRegistries.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {formState.selectedToolRegistries.length} registr{formState.selectedToolRegistries.length === 1 ? "y" : "ies"} selected
        </p>
      )}
    </div>
  );

  const renderOptionsAndReviewStep = () => (
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
          {/* Workers & verbose */}
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
              {configPreview.loaded && workEstimate.description && (
                <p className="text-xs text-muted-foreground flex items-center gap-1">
                  <Info className="h-3 w-3 shrink-0" />
                  {workEstimate.workItems === 1
                    ? `${workEstimate.description}. Additional workers won\u2019t improve speed.`
                    : `${workEstimate.workItems} work items (${workEstimate.description}). Workers beyond ${workEstimate.workItems} won\u2019t improve speed.`}
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

          {/* Review summary */}
          <div className="border-t pt-4 mt-4">
            <div className="flex items-center justify-between mb-3">
              <h3 className="font-medium">Review Configuration</h3>
              <Badge variant="outline">{formState.name}</Badge>
            </div>

            <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
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

              {(activeGroupNames.length > 0 || activeMappingGroups.length > 0) && (
                <>
                  <div className="text-muted-foreground">Provider Groups</div>
                  <div className="space-y-1">
                    {activeMappingGroups.map((group) => {
                      const mapping = formState.providerMappings[group];
                      return (
                        <div key={group} className="flex items-center gap-1 flex-wrap">
                          <Badge variant="outline" className="font-mono text-xs">
                            {group}
                          </Badge>
                          <Badge variant="secondary" className="text-[10px] px-1 py-0">
                            mapped
                          </Badge>
                          {Object.entries(mapping).map(([configID, entry]) => (
                            <span key={configID} className="text-xs">
                              {configID}→{entry ? entry.name : "?"}
                            </span>
                          ))}
                        </div>
                      );
                    })}
                    {activeGroupNames.map((group) => {
                      const entries = formState.providerGroups[group] || [];
                      return (
                        <div key={group} className="flex items-center gap-1 flex-wrap">
                          <Badge variant="outline" className="font-mono text-xs">
                            {group}
                          </Badge>
                          {entries.map((entry) => (
                            <Badge
                              key={`${entry.type}-${entry.name}`}
                              variant="secondary"
                              className="text-xs flex items-center gap-0.5"
                            >
                              {entry.type === "agent" ? (
                                <Network className="h-2.5 w-2.5" />
                              ) : (
                                <Zap className="h-2.5 w-2.5" />
                              )}
                              {entry.name}
                            </Badge>
                          ))}
                          {entries.length === 0 && (
                            <span className="text-xs text-muted-foreground">empty</span>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </>
              )}

              {formState.selectedToolRegistries.length > 0 && (
                <>
                  <div className="text-muted-foreground">Tool Registries</div>
                  <div className="flex flex-wrap gap-1">
                    {formState.selectedToolRegistries.map((name) => (
                      <Badge key={name} variant="secondary" className="text-xs">
                        {name}
                      </Badge>
                    ))}
                  </div>
                </>
              )}
            </div>
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
