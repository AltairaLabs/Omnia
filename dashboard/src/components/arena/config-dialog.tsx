"use client";

import { useState } from "react";
import { useArenaConfigMutations } from "@/hooks/use-arena-configs";
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
  arenaFile: string;
  scenariosInclude: string;
  scenariosExclude: string;
  temperature: string;
  concurrency: string;
  timeout: string;
}

function getInitialFormState(config?: ArenaConfig | null): FormState {
  if (config) {
    return {
      name: config.metadata?.name || "",
      sourceRef: config.spec?.sourceRef?.name || "",
      arenaFile: config.spec?.arenaFile || "",
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
    arenaFile: "",
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

  const spec: ArenaConfigSpec = {
    sourceRef,
    arenaFile: form.arenaFile || undefined,
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

  const updateForm = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setFormState((prev) => ({ ...prev, [key]: value }));
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

  return (
    <DialogContent className="sm:max-w-[500px]">
      <DialogHeader>
        <DialogTitle>{isEditing ? "Edit Config" : "Create Config"}</DialogTitle>
        <DialogDescription>
          {isEditing
            ? "Update the configuration for this Arena config."
            : "Configure a new Arena evaluation configuration."}
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
            placeholder="my-config"
            value={formState.name}
            onChange={(e) => updateForm("name", e.target.value)}
            disabled={isEditing}
          />
        </div>

        {/* Source Reference */}
        <div className="space-y-2">
          <Label htmlFor="source">Source</Label>
          <Select
            value={formState.sourceRef}
            onValueChange={(v) => updateForm("sourceRef", v)}
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

        {/* Arena File */}
        <div className="space-y-2">
          <Label htmlFor="arena-file">Arena File Path (optional)</Label>
          <Input
            id="arena-file"
            placeholder="arena.yaml"
            value={formState.arenaFile}
            onChange={(e) => updateForm("arenaFile", e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            Path to the arena configuration file within the source (default: arena.yaml)
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

      <DialogFooter>
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
