"use client";

import { useState, useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { YamlBlock } from "@/components/ui/yaml-block";
import { Progress } from "@/components/ui/progress";
import { cn } from "@/lib/utils";
import { usePromptPacks, useToolRegistries, useReadOnly, usePermissions, Permission } from "@/hooks";
import { useDataService } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import {
  ChevronLeft,
  ChevronRight,
  Rocket,
  Loader2,
  Check,
  AlertCircle,
  Lock,
} from "lucide-react";

interface DeployWizardProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type FrameworkType = "promptkit" | "langchain" | "autogen" | "custom";
type ProviderType = "claude" | "openai" | "gemini" | "ollama";
type FacadeType = "websocket" | "grpc";
type SessionType = "memory" | "redis";

interface WizardFormData {
  // Step 1: Basic Info
  name: string;
  // Step 2: Framework
  framework: FrameworkType;
  frameworkVersion: string;
  customImage: string;
  // Step 3: PromptPack
  promptPackName: string;
  promptPackTrack: string;
  // Step 4: Provider
  providerType: ProviderType;
  providerModel: string;
  providerSecretName: string;
  // Step 5: Tools & Session
  toolRegistryName: string;
  toolRegistryNamespace: string;
  sessionType: SessionType;
  sessionTtl: string;
  // Step 6: Runtime
  replicas: number;
  cpuRequest: string;
  cpuLimit: string;
  memoryRequest: string;
  memoryLimit: string;
  facadeType: FacadeType;
  facadePort: number;
}

const INITIAL_FORM_DATA: WizardFormData = {
  name: "",
  framework: "promptkit",
  frameworkVersion: "",
  customImage: "",
  promptPackName: "",
  promptPackTrack: "stable",
  providerType: "claude",
  providerModel: "",
  providerSecretName: "",
  toolRegistryName: "",
  toolRegistryNamespace: "",
  sessionType: "memory",
  sessionTtl: "24h",
  replicas: 1,
  cpuRequest: "100m",
  cpuLimit: "500m",
  memoryRequest: "128Mi",
  memoryLimit: "512Mi",
  facadeType: "websocket",
  facadePort: 8080,
};

const STEPS = [
  { title: "Basic Info", description: "Name" },
  { title: "Framework", description: "Agent framework" },
  { title: "PromptPack", description: "Select prompts" },
  { title: "Provider", description: "LLM configuration" },
  { title: "Options", description: "Tools & session" },
  { title: "Runtime", description: "Resources & scaling" },
  { title: "Review", description: "Deploy agent" },
];

const FRAMEWORKS: { value: FrameworkType; label: string; description: string }[] = [
  { value: "promptkit", label: "PromptKit", description: "AltairaLabs' native framework" },
  { value: "langchain", label: "LangChain", description: "Popular Python framework" },
  { value: "autogen", label: "AutoGen", description: "Microsoft's agent framework" },
  { value: "custom", label: "Custom", description: "Your own container image" },
];

const PROVIDERS: { value: ProviderType; label: string; models: string[] }[] = [
  { value: "claude", label: "Anthropic Claude", models: ["claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-3-5-sonnet-20241022"] },
  { value: "openai", label: "OpenAI", models: ["gpt-4o", "gpt-4o-mini", "gpt-4-turbo"] },
  { value: "gemini", label: "Google Gemini", models: ["gemini-2.0-flash", "gemini-1.5-pro"] },
  { value: "ollama", label: "Ollama (Local)", models: ["llama3.2", "llava:7b", "mistral"] },
];

/**
 * Get the disabled message based on read-only and permission status.
 */
function getDisabledMessage(
  isReadOnly: boolean,
  canDeploy: boolean,
  readOnlyMessage: string
): string {
  if (isReadOnly) return readOnlyMessage;
  if (!canDeploy) return "You don't have permission to deploy agents";
  return "";
}

/**
 * Get step indicator class based on current step.
 */
function getStepIndicatorClassName(stepIndex: number, currentStep: number): string {
  if (stepIndex < currentStep) return "bg-primary text-primary-foreground";
  if (stepIndex === currentStep) return "border-2 border-primary";
  return "border border-muted-foreground/30";
}

/**
 * Render deploy button content based on state.
 */
function renderDeployButtonContent(isSubmitting: boolean, success: boolean) {
  if (isSubmitting) {
    return (
      <>
        <Loader2 className="h-4 w-4 mr-2 animate-spin" />
        Deploying...
      </>
    );
  }
  if (success) {
    return (
      <>
        <Check className="h-4 w-4 mr-2" />
        Deployed
      </>
    );
  }
  return (
    <>
      <Rocket className="h-4 w-4 mr-2" />
      Deploy Agent
    </>
  );
}

export function DeployWizard({ open, onOpenChange }: Readonly<DeployWizardProps>) {
  const [step, setStep] = useState(0);
  const [formData, setFormData] = useState<WizardFormData>(INITIAL_FORM_DATA);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const { isReadOnly, message: readOnlyMessage } = useReadOnly();
  const { can } = usePermissions();
  const canDeploy = can(Permission.AGENTS_DEPLOY);
  const isDisabled = isReadOnly || !canDeploy;
  const disabledMessage = getDisabledMessage(isReadOnly, canDeploy, readOnlyMessage);
  const queryClient = useQueryClient();
  const dataService = useDataService();
  const { currentWorkspace } = useWorkspace();
  // Get PromptPacks from current workspace
  const { data: promptPacks } = usePromptPacks();
  // Get all ToolRegistries (shared resources)
  const { data: toolRegistries } = useToolRegistries();

  const updateField = useCallback(<K extends keyof WizardFormData>(
    field: K,
    value: WizardFormData[K]
  ) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  }, []);

  const canProceed = useCallback(() => {
    switch (step) {
      case 0: // Basic Info
        return formData.name.length > 0 && /^[a-z0-9-]+$/.test(formData.name);
      case 1: // Framework
        return formData.framework !== "custom" || formData.customImage.length > 0;
      case 2: // PromptPack
        return formData.promptPackName.length > 0;
      case 3: // Provider
        return true; // Auto-detect is always valid
      case 4: // Options
        return true; // All optional
      case 5: // Runtime
        return formData.replicas >= 0;
      case 6: // Review
        return true;
      default:
        return false;
    }
  }, [step, formData]);

  const generateYaml = useCallback(() => {
    const yaml: Record<string, unknown> = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "AgentRuntime",
      metadata: {
        name: formData.name,
        namespace: currentWorkspace?.namespace || "default",
      },
      spec: {
        promptPackRef: {
          name: formData.promptPackName,
          track: formData.promptPackTrack,
        },
        facade: {
          type: formData.facadeType,
          port: formData.facadePort,
        },
      },
    };

    // Framework (only if not default promptkit)
    if (formData.framework !== "promptkit" || formData.customImage) {
      (yaml.spec as Record<string, unknown>).framework = {
        type: formData.framework,
        ...(formData.frameworkVersion && { version: formData.frameworkVersion }),
        ...(formData.customImage && { image: formData.customImage }),
      };
    }

    // Provider (type is required)
    (yaml.spec as Record<string, unknown>).provider = {
      type: formData.providerType,
      ...(formData.providerModel && { model: formData.providerModel }),
      ...(formData.providerSecretName && { secretRef: { name: formData.providerSecretName } }),
    };

    // Tool Registry
    if (formData.toolRegistryName) {
      (yaml.spec as Record<string, unknown>).toolRegistryRef = {
        name: formData.toolRegistryName,
        ...(formData.toolRegistryNamespace && { namespace: formData.toolRegistryNamespace }),
      };
    }

    // Session
    if (formData.sessionType !== "memory" || formData.sessionTtl !== "24h") {
      (yaml.spec as Record<string, unknown>).session = {
        type: formData.sessionType,
        ttl: formData.sessionTtl,
      };
    }

    // Runtime
    const runtime: Record<string, unknown> = {};
    if (formData.replicas !== 1) {
      runtime.replicas = formData.replicas;
    }
    if (formData.cpuRequest !== "100m" || formData.memoryRequest !== "128Mi") {
      runtime.resources = {
        requests: {
          cpu: formData.cpuRequest,
          memory: formData.memoryRequest,
        },
        limits: {
          cpu: formData.cpuLimit,
          memory: formData.memoryLimit,
        },
      };
    }
    if (Object.keys(runtime).length > 0) {
      (yaml.spec as Record<string, unknown>).runtime = runtime;
    }

    return yaml;
  }, [formData, currentWorkspace]);

  const handleSubmit = async () => {
    if (!currentWorkspace) {
      setError("No workspace selected");
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      const agentSpec = generateYaml();
      await dataService.createAgent(currentWorkspace.name, agentSpec);
      setSuccess(true);
      queryClient.invalidateQueries({ queryKey: ["agents"] });

      // Close dialog after a short delay
      setTimeout(() => {
        onOpenChange(false);
        setStep(0);
        setFormData(INITIAL_FORM_DATA);
        setSuccess(false);
      }, 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create agent");
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    if (!isSubmitting) {
      onOpenChange(false);
      setStep(0);
      setFormData(INITIAL_FORM_DATA);
      setError(null);
      setSuccess(false);
    }
  };

  const renderStep = () => {
    switch (step) {
      case 0:
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Agent Name</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => updateField("name", e.target.value.toLowerCase().replaceAll(/[^a-z0-9-]/g, "-"))}
                placeholder="my-agent"
              />
              <p className="text-xs text-muted-foreground">
                Lowercase letters, numbers, and hyphens only
              </p>
            </div>
            <div className="space-y-2">
              <Label>Workspace</Label>
              <div className="flex h-10 items-center rounded-md border border-input bg-muted px-3 py-2 text-sm">
                {currentWorkspace?.displayName || currentWorkspace?.name || "No workspace selected"}
              </div>
              <p className="text-xs text-muted-foreground">
                Agent will be deployed to the {currentWorkspace?.namespace || "default"} namespace
              </p>
            </div>
          </div>
        );

      case 1:
        return (
          <div className="space-y-4">
            <Label>Agent Framework</Label>
            <RadioGroup
              value={formData.framework}
              onValueChange={(v) => updateField("framework", v as FrameworkType)}
              className="grid grid-cols-1 gap-2"
            >
              {FRAMEWORKS.map((fw) => (
                <label
                  key={fw.value}
                  htmlFor={fw.value}
                  className={cn(
                    "flex items-center space-x-3 rounded-lg border p-3 cursor-pointer transition-colors",
                    formData.framework === fw.value
                      ? "border-primary bg-primary/5"
                      : "hover:bg-muted/50"
                  )}
                >
                  <RadioGroupItem value={fw.value} id={fw.value} />
                  <div className="flex-1">
                    <span className="cursor-pointer font-medium">
                      {fw.label}
                    </span>
                    <p className="text-xs text-muted-foreground">{fw.description}</p>
                  </div>
                </label>
              ))}
            </RadioGroup>

            {formData.framework === "custom" && (
              <div className="space-y-2 pt-2">
                <Label htmlFor="customImage">Container Image</Label>
                <Input
                  id="customImage"
                  value={formData.customImage}
                  onChange={(e) => updateField("customImage", e.target.value)}
                  placeholder="myregistry/my-agent:v1.0"
                />
              </div>
            )}
          </div>
        );

      case 2:
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>PromptPack</Label>
              <p className="text-xs text-muted-foreground mb-2">
                Showing PromptPacks in {currentWorkspace?.namespace} namespace
              </p>
              <Select
                value={formData.promptPackName}
                onValueChange={(v) => updateField("promptPackName", v)}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select a PromptPack" />
                </SelectTrigger>
                <SelectContent>
                  {promptPacks?.map((pack) => (
                    <SelectItem key={pack.metadata.uid} value={pack.metadata.name}>
                      <div className="flex items-center gap-2">
                        <span>{pack.metadata.name}</span>
                        <Badge variant="outline" className="text-xs">
                          {pack.status?.phase || "Unknown"}
                        </Badge>
                      </div>
                    </SelectItem>
                  ))}
                  {(!promptPacks || promptPacks.length === 0) && (
                    <SelectItem value="__no_promptpacks__" disabled>
                      No PromptPacks available
                    </SelectItem>
                  )}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Release Track</Label>
              <RadioGroup
                value={formData.promptPackTrack}
                onValueChange={(v) => updateField("promptPackTrack", v)}
                className="flex gap-4"
              >
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="stable" id="track-stable" />
                  <Label htmlFor="track-stable" className="cursor-pointer">Stable</Label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="canary" id="track-canary" />
                  <Label htmlFor="track-canary" className="cursor-pointer">Canary</Label>
                </div>
              </RadioGroup>
            </div>
          </div>
        );

      case 3:
        return (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>LLM Provider</Label>
              <Select
                value={formData.providerType}
                onValueChange={(v) => {
                  updateField("providerType", v as ProviderType);
                  updateField("providerModel", "");
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PROVIDERS.map((p) => (
                    <SelectItem key={p.value} value={p.value}>
                      {p.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Model</Label>
              <Select
                value={formData.providerModel}
                onValueChange={(v) => updateField("providerModel", v)}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select a model" />
                </SelectTrigger>
                <SelectContent>
                  {PROVIDERS.find((p) => p.value === formData.providerType)?.models.map((model) => (
                    <SelectItem key={model} value={model}>
                      {model}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {formData.providerType !== "ollama" && (
              <div className="space-y-2">
                <Label htmlFor="secretName">API Key Secret Name</Label>
                <Input
                  id="secretName"
                  value={formData.providerSecretName}
                  onChange={(e) => updateField("providerSecretName", e.target.value)}
                  placeholder="llm-api-key"
                />
                <p className="text-xs text-muted-foreground">
                  Kubernetes Secret containing the API key
                </p>
              </div>
            )}
          </div>
        );

      case 4:
        return (
          <div className="space-y-6">
            <div className="space-y-2">
              <Label>Tool Registry (Optional)</Label>
              <p className="text-xs text-muted-foreground mb-2">
                Cross-namespace references are supported
              </p>
              <Select
                value={formData.toolRegistryName ? `${formData.toolRegistryNamespace || currentWorkspace?.namespace}/${formData.toolRegistryName}` : "__none__"}
                onValueChange={(v) => {
                  if (v === "__none__") {
                    updateField("toolRegistryName", "");
                    updateField("toolRegistryNamespace", "");
                  } else {
                    const [ns, name] = v.split("/");
                    updateField("toolRegistryName", name);
                    // Only set namespace if different from agent namespace
                    updateField("toolRegistryNamespace", ns === currentWorkspace?.namespace ? "" : ns);
                  }
                }}
              >
                <SelectTrigger>
                  <SelectValue placeholder="None" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">None</SelectItem>
                  {toolRegistries?.map((registry) => {
                    const ns = registry.metadata.namespace || "default";
                    const name = registry.metadata.name;
                    const isSameNamespace = ns === currentWorkspace?.namespace;
                    return (
                      <SelectItem key={registry.metadata.uid} value={`${ns}/${name}`}>
                        <div className="flex items-center gap-2">
                          <span>{name}</span>
                          {!isSameNamespace && (
                            <Badge variant="secondary" className="text-xs">
                              {ns}
                            </Badge>
                          )}
                          <Badge variant="outline" className="text-xs">
                            {registry.status?.discoveredToolsCount || 0} tools
                          </Badge>
                        </div>
                      </SelectItem>
                    );
                  })}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-4 border-t pt-4">
              <Label className="text-base">Session Storage</Label>
              <RadioGroup
                value={formData.sessionType}
                onValueChange={(v) => updateField("sessionType", v as SessionType)}
                className="flex gap-4"
              >
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="memory" id="session-memory" />
                  <Label htmlFor="session-memory" className="cursor-pointer">In-Memory</Label>
                </div>
                <div className="flex items-center space-x-2">
                  <RadioGroupItem value="redis" id="session-redis" />
                  <Label htmlFor="session-redis" className="cursor-pointer">Redis</Label>
                </div>
              </RadioGroup>

              <div className="space-y-2">
                <Label htmlFor="sessionTtl">Session TTL</Label>
                <Input
                  id="sessionTtl"
                  value={formData.sessionTtl}
                  onChange={(e) => updateField("sessionTtl", e.target.value)}
                  placeholder="24h"
                />
              </div>
            </div>
          </div>
        );

      case 5:
        return (
          <div className="space-y-6">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Facade Type</Label>
                <Select
                  value={formData.facadeType}
                  onValueChange={(v) => updateField("facadeType", v as FacadeType)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="websocket">WebSocket</SelectItem>
                    <SelectItem value="grpc">gRPC</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label htmlFor="facadePort">Port</Label>
                <Input
                  id="facadePort"
                  type="number"
                  value={formData.facadePort}
                  onChange={(e) => updateField("facadePort", Number.parseInt(e.target.value) || 8080)}
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="replicas">Replicas</Label>
              <Input
                id="replicas"
                type="number"
                min={0}
                value={formData.replicas}
                onChange={(e) => updateField("replicas", Number.parseInt(e.target.value) || 0)}
              />
            </div>

            <div className="space-y-4 border-t pt-4">
              <Label className="text-base">Resource Limits</Label>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="cpuRequest">CPU Request</Label>
                  <Input
                    id="cpuRequest"
                    value={formData.cpuRequest}
                    onChange={(e) => updateField("cpuRequest", e.target.value)}
                    placeholder="100m"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="cpuLimit">CPU Limit</Label>
                  <Input
                    id="cpuLimit"
                    value={formData.cpuLimit}
                    onChange={(e) => updateField("cpuLimit", e.target.value)}
                    placeholder="500m"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="memoryRequest">Memory Request</Label>
                  <Input
                    id="memoryRequest"
                    value={formData.memoryRequest}
                    onChange={(e) => updateField("memoryRequest", e.target.value)}
                    placeholder="128Mi"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="memoryLimit">Memory Limit</Label>
                  <Input
                    id="memoryLimit"
                    value={formData.memoryLimit}
                    onChange={(e) => updateField("memoryLimit", e.target.value)}
                    placeholder="512Mi"
                  />
                </div>
              </div>
            </div>
          </div>
        );

      case 6:
        return (
          <div className="space-y-4">
            {success ? (
              <div className="flex flex-col items-center justify-center py-8 text-center">
                <div className="rounded-full bg-green-500/10 p-3 mb-4">
                  <Check className="h-8 w-8 text-green-500" />
                </div>
                <h3 className="text-lg font-semibold">Agent Created!</h3>
                <p className="text-sm text-muted-foreground mt-1">
                  {formData.name} is being deployed to {currentWorkspace?.namespace}
                </p>
              </div>
            ) : (
              <>
                <div className="flex items-center justify-between">
                  <h3 className="font-medium">Review Configuration</h3>
                  <Badge variant="outline">
                    {currentWorkspace?.namespace}/{formData.name}
                  </Badge>
                </div>

                <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
                  <div className="text-muted-foreground">Framework</div>
                  <div>{FRAMEWORKS.find((f) => f.value === formData.framework)?.label}</div>

                  <div className="text-muted-foreground">PromptPack</div>
                  <div>{formData.promptPackName} ({formData.promptPackTrack})</div>

                  <div className="text-muted-foreground">Provider</div>
                  <div>
                    {PROVIDERS.find((p) => p.value === formData.providerType)?.label}
                    {formData.providerModel && ` / ${formData.providerModel}`}
                  </div>

                  <div className="text-muted-foreground">Facade</div>
                  <div>{formData.facadeType}:{formData.facadePort}</div>

                  <div className="text-muted-foreground">Replicas</div>
                  <div>{formData.replicas}</div>
                </div>

                <div className="border-t pt-4">
                  <h4 className="text-sm font-medium mb-2">YAML Preview</h4>
                  <YamlBlock
                    data={generateYaml()}
                    className="max-h-[200px] overflow-auto text-xs"
                  />
                </div>

                {error && (
                  <div className="flex items-center gap-2 p-3 rounded-lg bg-destructive/10 text-destructive text-sm">
                    <AlertCircle className="h-4 w-4 shrink-0" />
                    {error}
                  </div>
                )}
              </>
            )}
          </div>
        );

      default:
        return null;
    }
  };

  // Show message if deployments are disabled (read-only or no permission)
  if (isDisabled) {
    return (
      <Dialog open={open} onOpenChange={handleClose}>
        <DialogContent className="sm:max-w-[400px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Lock className="h-5 w-5 text-muted-foreground" />
              {isReadOnly ? "Read-Only Mode" : "Permission Denied"}
            </DialogTitle>
            <DialogDescription>
              {isReadOnly
                ? "Deployments are disabled in this dashboard."
                : "You don't have permission to deploy agents."}
            </DialogDescription>
          </DialogHeader>

          <div className="py-6 text-center">
            <div className="rounded-full bg-muted p-4 w-fit mx-auto mb-4">
              <Lock className="h-8 w-8 text-muted-foreground" />
            </div>
            <p className="text-sm text-muted-foreground max-w-xs mx-auto">
              {disabledMessage}
            </p>
          </div>

          <div className="flex justify-end pt-4 border-t">
            <Button variant="outline" onClick={handleClose}>
              Close
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Deploy New Agent</DialogTitle>
          <DialogDescription>
            Step {step + 1} of {STEPS.length}: {STEPS[step].description}
          </DialogDescription>
        </DialogHeader>

        {/* Progress */}
        <Progress value={((step + 1) / STEPS.length) * 100} className="h-1" />

        {/* Step indicators */}
        <div className="flex justify-between px-2 mb-2">
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
            </div>
          ))}
        </div>

        {/* Form content */}
        <div className="py-4 min-h-[280px]">{renderStep()}</div>

        {/* Navigation */}
        <div className="flex justify-between pt-4 border-t">
          <Button
            variant="outline"
            onClick={() => setStep((s) => Math.max(0, s - 1))}
            disabled={step === 0 || isSubmitting || success}
          >
            <ChevronLeft className="h-4 w-4 mr-1" />
            Back
          </Button>

          {step < STEPS.length - 1 ? (
            <Button
              onClick={() => setStep((s) => s + 1)}
              disabled={!canProceed()}
            >
              Next
              <ChevronRight className="h-4 w-4 ml-1" />
            </Button>
          ) : (
            <Button
              onClick={handleSubmit}
              disabled={isSubmitting || success}
            >
              {renderDeployButtonContent(isSubmitting, success)}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
