"use client";

import { useState } from "react";
import { useArenaConfigMutations } from "@/hooks/use-arena-configs";
import { useArenaSourceContent } from "@/hooks/use-arena-source-content";
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
import { Textarea } from "@/components/ui/textarea";
import { FolderBrowser } from "./folder-browser";
import {
  AlertCircle,
  Loader2,
  Database,
} from "lucide-react";
import type {
  ArenaConfig,
  ArenaConfigSpec,
  ArenaSource,
  ResourceRef,
  ScenarioFilter,
  ArenaDefaults,
} from "@/types/arena";

interface ConfigDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  config?: ArenaConfig | null;
  sources: ArenaSource[];
  onSuccess?: () => void;
  onClose?: () => void;
}

interface FormState {
  name: string;
  sourceRef: string;
  rootPath: string;
  arenaFileName: string;
  scenariosInclude: string;
  scenariosExclude: string;
  temperature: string;
  concurrency: string;
  timeout: string;
}

/**
 * Parse an arenaFile path into rootPath and fileName.
 * e.g., "my-project/arena.yaml" -> { rootPath: "my-project", fileName: "arena.yaml" }
 */
function parseArenaFilePath(arenaFile: string | undefined): { rootPath: string; fileName: string } {
  if (!arenaFile) {
    return { rootPath: "", fileName: "" };
  }
  const lastSlash = arenaFile.lastIndexOf("/");
  if (lastSlash === -1) {
    return { rootPath: "", fileName: arenaFile };
  }
  return {
    rootPath: arenaFile.substring(0, lastSlash),
    fileName: arenaFile.substring(lastSlash + 1),
  };
}

/** Default arena config file name */
const DEFAULT_ARENA_FILE = "config.arena.yaml";

/**
 * Combine rootPath and fileName into a full arenaFile path.
 */
function buildArenaFilePath(rootPath: string, fileName: string): string | undefined {
  const trimmedRoot = rootPath.trim();
  const trimmedFile = fileName.trim();

  if (!trimmedRoot && !trimmedFile) {
    return undefined;
  }
  if (!trimmedRoot) {
    return trimmedFile || undefined;
  }
  if (!trimmedFile) {
    // If only root path is set, default to config.arena.yaml in that folder
    return `${trimmedRoot}/${DEFAULT_ARENA_FILE}`;
  }
  return `${trimmedRoot}/${trimmedFile}`;
}

function getInitialFormState(config?: ArenaConfig | null): FormState {
  if (config) {
    const { rootPath, fileName } = parseArenaFilePath(config.spec?.arenaFile);
    return {
      name: config.metadata?.name || "",
      sourceRef: config.spec?.sourceRef?.name || "",
      rootPath,
      arenaFileName: fileName,
      scenariosInclude: config.spec?.scenarios?.include?.join(", ") || "",
      scenariosExclude: config.spec?.scenarios?.exclude?.join(", ") || "",
      temperature: config.spec?.defaults?.temperature?.toString() || "",
      concurrency: config.spec?.defaults?.concurrency?.toString() || "",
      timeout: config.spec?.defaults?.timeout || "",
    };
  }
  return {
    name: "",
    sourceRef: "",
    rootPath: "",
    arenaFileName: "",
    scenariosInclude: "",
    scenariosExclude: "",
    temperature: "",
    concurrency: "",
    timeout: "",
  };
}

function validateForm(form: FormState): string | null {
  if (!form.name.trim()) {
    return "Name is required";
  }
  if (!form.sourceRef) {
    return "Source is required";
  }
  if (form.temperature) {
    const temp = parseFloat(form.temperature);
    if (isNaN(temp) || temp < 0 || temp > 2) {
      return "Temperature must be a number between 0 and 2";
    }
  }
  if (form.concurrency) {
    const conc = parseInt(form.concurrency, 10);
    if (isNaN(conc) || conc < 1) {
      return "Concurrency must be a positive integer";
    }
  }
  return null;
}

function parseGlobPatterns(value: string): string[] | undefined {
  if (!value.trim()) return undefined;
  return value
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

function buildSpec(form: FormState): ArenaConfigSpec {
  const sourceRef: ResourceRef = { name: form.sourceRef };

  const scenarios: ScenarioFilter | undefined = (() => {
    const include = parseGlobPatterns(form.scenariosInclude);
    const exclude = parseGlobPatterns(form.scenariosExclude);
    if (!include && !exclude) return undefined;
    return { include, exclude };
  })();

  const defaults: ArenaDefaults | undefined = (() => {
    const temp = form.temperature ? parseFloat(form.temperature) : undefined;
    const conc = form.concurrency ? parseInt(form.concurrency, 10) : undefined;
    const timeout = form.timeout || undefined;
    if (temp === undefined && conc === undefined && timeout === undefined) {
      return undefined;
    }
    return {
      temperature: temp,
      concurrency: conc,
      timeout,
    };
  })();

  // Build the full arenaFile path from rootPath and fileName
  const arenaFile = buildArenaFilePath(form.rootPath, form.arenaFileName);

  const spec: ArenaConfigSpec = {
    sourceRef,
    arenaFile,
    scenarios,
    defaults,
  };

  return spec;
}

export function ConfigDialog({
  open,
  onOpenChange,
  config,
  sources,
  onSuccess,
  onClose,
}: Readonly<ConfigDialogProps>) {
  const { createConfig, updateConfig, loading } = useArenaConfigMutations();
  const isEditing = !!config;

  // Use config name as key to reset form when config changes
  const formResetKey = `${config?.metadata?.name ?? "new"}-${open}`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <ConfigDialogForm
        key={formResetKey}
        config={config}
        sources={sources}
        isEditing={isEditing}
        loading={loading}
        createConfig={createConfig}
        updateConfig={updateConfig}
        onSuccess={onSuccess}
        onClose={onClose}
        onOpenChange={onOpenChange}
      />
    </Dialog>
  );
}

interface ConfigDialogFormProps {
  config?: ArenaConfig | null;
  sources: ArenaSource[];
  isEditing: boolean;
  loading: boolean;
  createConfig: (name: string, spec: ArenaConfig["spec"]) => Promise<ArenaConfig>;
  updateConfig: (name: string, spec: ArenaConfig["spec"]) => Promise<ArenaConfig>;
  onSuccess?: () => void;
  onClose?: () => void;
  onOpenChange: (open: boolean) => void;
}

function ConfigDialogForm({
  config,
  sources,
  isEditing,
  loading,
  createConfig,
  updateConfig,
  onSuccess,
  onClose,
  onOpenChange,
}: Readonly<ConfigDialogFormProps>) {
  const [formState, setFormState] = useState<FormState>(() =>
    getInitialFormState(config)
  );
  const [error, setError] = useState<string | null>(null);

  // Fetch source content when a source is selected
  const {
    tree: sourceTree,
    loading: contentLoading,
    error: contentError,
  } = useArenaSourceContent(formState.sourceRef || undefined);

  const updateForm = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setFormState((prev) => ({ ...prev, [key]: value }));
  };

  const handleSourceChange = (sourceRef: string) => {
    // Reset root path when source changes
    setFormState((prev) => ({
      ...prev,
      sourceRef,
      rootPath: "",
    }));
  };

  const handleFolderSelect = (path: string) => {
    updateForm("rootPath", path);
  };

  const handleFileSelect = (_filePath: string, folderPath: string, fileName: string) => {
    // When a file is clicked, set both the root path and the arena file name
    setFormState((prev) => ({
      ...prev,
      rootPath: folderPath,
      arenaFileName: fileName,
    }));
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

      if (isEditing) {
        await updateConfig(formState.name, spec);
      } else {
        await createConfig(formState.name, spec);
      }

      onSuccess?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save config");
    }
  };

  const handleClose = () => {
    onClose?.();
    onOpenChange(false);
  };

  const readySources = sources.filter(
    (s) => s.status?.phase === "Ready"
  );

  // Show the computed full path for user feedback
  const fullArenaPath = buildArenaFilePath(formState.rootPath, formState.arenaFileName) || DEFAULT_ARENA_FILE;

  return (
    <DialogContent className="sm:max-w-[550px] max-h-[90vh] flex flex-col overflow-hidden">
      <DialogHeader className="flex-shrink-0">
        <DialogTitle>{isEditing ? "Edit Config" : "Create Config"}</DialogTitle>
        <DialogDescription>
          {isEditing
            ? "Update the configuration for this Arena config."
            : "Configure a new Arena evaluation configuration."}
        </DialogDescription>
      </DialogHeader>

      {/* Fixed section: error and name */}
      <div className="flex-shrink-0 space-y-4 pt-4">
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
            placeholder="my-config"
            value={formState.name}
            onChange={(e) => updateForm("name", e.target.value)}
            disabled={isEditing}
          />
        </div>
      </div>

      {/* Scrollable section */}
      <div className="flex-1 min-h-0 overflow-y-auto pr-2">
        <div className="space-y-4 py-4">
          {/* Source Reference */}
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
                    <Database className="h-4 w-4" />
                    No ready sources available
                  </div>
                ) : (
                  readySources.map((source) => (
                    <SelectItem key={source.metadata?.name} value={source.metadata?.name || "unknown"}>
                      {source.metadata?.name}
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Select the source containing your PromptKit scenarios
            </p>
          </div>

          {/* Root Folder Browser - only show when source is selected */}
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
                Select the folder containing your config file, or click a file to select it directly
              </p>
            </div>
          )}

          {/* Arena File Name */}
          <div className="space-y-2">
            <Label htmlFor="arena-file">Arena File Name (optional)</Label>
            <Input
              id="arena-file"
              placeholder="config.arena.yaml"
              value={formState.arenaFileName}
              onChange={(e) => updateForm("arenaFileName", e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Arena config file name (default: config.arena.yaml).
              {formState.sourceRef && (
                <span className="block mt-1">
                  Full path: <code className="bg-muted px-1 rounded">{fullArenaPath}</code>
                </span>
              )}
            </p>
          </div>

          {/* Scenario Filters */}
          <div className="space-y-2">
            <Label>Scenario Filters</Label>
            <div className="space-y-2">
              <Textarea
                placeholder="scenarios/*.yaml, tests/**/*.yaml"
                value={formState.scenariosInclude}
                onChange={(e) => updateForm("scenariosInclude", e.target.value)}
                rows={2}
              />
              <p className="text-xs text-muted-foreground">
                Include patterns (comma-separated globs)
              </p>
            </div>
            <div className="space-y-2">
              <Textarea
                placeholder="scenarios/*-wip.yaml, draft/**"
                value={formState.scenariosExclude}
                onChange={(e) => updateForm("scenariosExclude", e.target.value)}
                rows={2}
              />
              <p className="text-xs text-muted-foreground">
                Exclude patterns (comma-separated globs)
              </p>
            </div>
          </div>

          {/* Default Values */}
          <div className="space-y-2">
            <Label>Default Values</Label>
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="temperature" className="text-xs text-muted-foreground">
                  Temperature
                </Label>
                <Input
                  id="temperature"
                  type="number"
                  step="0.1"
                  min="0"
                  max="2"
                  placeholder="0.7"
                  value={formState.temperature}
                  onChange={(e) => updateForm("temperature", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="concurrency" className="text-xs text-muted-foreground">
                  Concurrency
                </Label>
                <Input
                  id="concurrency"
                  type="number"
                  min="1"
                  placeholder="10"
                  value={formState.concurrency}
                  onChange={(e) => updateForm("concurrency", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="timeout" className="text-xs text-muted-foreground">
                  Timeout
                </Label>
                <Input
                  id="timeout"
                  placeholder="30s"
                  value={formState.timeout}
                  onChange={(e) => updateForm("timeout", e.target.value)}
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <DialogFooter className="flex-shrink-0">
        <Button variant="outline" onClick={handleClose}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={loading}>
          {loading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
          {isEditing ? "Save Changes" : "Create Config"}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
