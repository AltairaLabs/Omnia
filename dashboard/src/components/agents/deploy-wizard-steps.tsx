"use client";

/**
 * deploy-wizard-steps.tsx — step sub-components for DeployWizard.
 *
 * Steps 1-5 (Framework, PromptPack, Provider, Options, Runtime) live here so
 * that deploy-wizard.tsx stays within the file-length guardrail.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { FieldError } from "@/components/ui/field-error";
import { cn } from "@/lib/utils";
import type { PromptPackTrack } from "@/types/agent-runtime";

// ---------------------------------------------------------------------------
// Shared types — kept in sync with deploy-wizard.tsx
// ---------------------------------------------------------------------------

export type FrameworkType = "promptkit" | "langchain" | "autogen" | "custom";
export type FacadeType = "websocket" | "grpc" | "rest";
export type ContextStoreType = "memory" | "redis";
export type AgentMode = "agent" | "function";

export interface WizardFormData {
  // Step 1: Basic Info
  name: string;
  mode: AgentMode;
  inputSchemaJson: string;
  outputSchemaJson: string;
  // Step 2: Framework
  framework: FrameworkType;
  frameworkVersion: string;
  customImage: string;
  // Step 3: PromptPack
  promptPackName: string;
  promptPackTrack: PromptPackTrack;
  // Step 4: Provider
  providerRefName: string;
  // Step 5: Tools & Context
  toolRegistryName: string;
  toolRegistryNamespace: string;
  contextType: ContextStoreType;
  contextTtl: string;
  // Step 6: Runtime
  replicas: number;
  cpuRequest: string;
  cpuLimit: string;
  memoryRequest: string;
  memoryLimit: string;
  facadeType: FacadeType;
  facadePort: number;
}

// Release tracks offered by the PromptPack step. Typed as PromptPackTrack so a
// value spec.promptPackRef.track does not accept fails typecheck rather than
// the API server (#1882 — the radio used to emit "canary", which is not in the
// CRD's stable|prerelease enum, so every Canary deploy was rejected).
const PROMPT_PACK_TRACKS: { value: PromptPackTrack; label: string }[] = [
  { value: "stable", label: "Stable" },
  { value: "prerelease", label: "Prerelease" },
];

const FRAMEWORKS: { value: FrameworkType; label: string; description: string }[] = [
  { value: "promptkit", label: "PromptKit", description: "AltairaLabs' native framework" },
  { value: "langchain", label: "LangChain", description: "Popular Python framework" },
  { value: "autogen", label: "AutoGen", description: "Microsoft's agent framework" },
  { value: "custom", label: "Custom", description: "Your own container image" },
];

// ---------------------------------------------------------------------------
// FrameworkStep — wizard step 1
// ---------------------------------------------------------------------------

interface FrameworkStepProps {
  formData: WizardFormData;
  updateField: <K extends keyof WizardFormData>(field: K, value: WizardFormData[K]) => void;
}

/**
 * Only `promptkit` has a built-in default runtime image (operator
 * `--framework-image=promptkit=...` / chart `framework.image`). Every other
 * framework type — including the built-in `langchain`/`autogen` choices, not
 * just `custom` — blocks with `FrameworkImageUnavailable` unless an image is
 * supplied here or configured cluster-wide via `framework.images.<type>`.
 */
function imageHelpText(framework: FrameworkType): string {
  if (framework === "custom") {
    return "Required — your container image implementing the omnia.runtime.v1 contract.";
  }
  return "Required — no built-in image is provided for this framework. Pin an explicit tag; avoid \"latest\".";
}

export function FrameworkStep({ formData, updateField }: Readonly<FrameworkStepProps>) {
  const requiresImage = formData.framework !== "promptkit";
  return (
    <div className="space-y-4">
      <Label>Agent Framework</Label>
      <RadioGroup
        value={formData.framework}
        onValueChange={(v) => {
          const next = v as FrameworkType;
          updateField("framework", next);
          // promptkit hides the image input and uses its built-in image, so
          // clear any stale image/version left from a previous selection —
          // otherwise it leaks into the generated YAML.
          if (next === "promptkit") {
            updateField("customImage", "");
            updateField("frameworkVersion", "");
          }
        }}
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
              <span className="cursor-pointer font-medium">{fw.label}</span>
              <p className="text-xs text-muted-foreground">{fw.description}</p>
            </div>
          </label>
        ))}
      </RadioGroup>

      {requiresImage && (
        <div className="space-y-2 pt-2">
          <Label htmlFor="customImage">Container Image</Label>
          <Input
            id="customImage"
            value={formData.customImage}
            onChange={(e) => updateField("customImage", e.target.value)}
            placeholder="myregistry/my-agent:v1.0"
          />
          <p className="text-xs text-muted-foreground">{imageHelpText(formData.framework)}</p>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// PromptPackStep — wizard step 2
// ---------------------------------------------------------------------------

interface PromptPackItem {
  metadata: { uid?: string; name: string };
  status?: { phase?: string };
}

interface PromptPackStepProps {
  formData: WizardFormData;
  currentWorkspace: { name?: string; namespace?: string; displayName?: string } | null;
  promptPacks: PromptPackItem[] | undefined;
  updateField: <K extends keyof WizardFormData>(field: K, value: WizardFormData[K]) => void;
}

export function PromptPackStep({
  formData,
  currentWorkspace,
  promptPacks,
  updateField,
}: Readonly<PromptPackStepProps>) {
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
                    {pack.status?.phase ?? "Unknown"}
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
          onValueChange={(v) => updateField("promptPackTrack", v as PromptPackTrack)}
          className="flex gap-4"
        >
          {PROMPT_PACK_TRACKS.map((track) => (
            <div key={track.value} className="flex items-center space-x-2">
              <RadioGroupItem value={track.value} id={`track-${track.value}`} />
              <Label htmlFor={`track-${track.value}`} className="cursor-pointer">
                {track.label}
              </Label>
            </div>
          ))}
        </RadioGroup>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ProviderStep — wizard step 3
// ---------------------------------------------------------------------------

interface ProviderItem {
  metadata: { uid?: string; name: string };
  spec: { type: string; model?: string };
}

interface ProviderStepProps {
  formData: WizardFormData;
  currentWorkspace: { name?: string; namespace?: string; displayName?: string } | null;
  providers: ProviderItem[] | undefined;
  updateField: <K extends keyof WizardFormData>(field: K, value: WizardFormData[K]) => void;
}

export function ProviderStep({
  formData,
  currentWorkspace,
  providers,
  updateField,
}: Readonly<ProviderStepProps>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>LLM Provider</Label>
        <p className="text-xs text-muted-foreground mb-2">
          Select a configured Provider from {currentWorkspace?.namespace} namespace
        </p>
        <Select
          value={formData.providerRefName}
          onValueChange={(v) => updateField("providerRefName", v)}
        >
          <SelectTrigger>
            <SelectValue placeholder="Select a Provider" />
          </SelectTrigger>
          <SelectContent>
            {providers?.map((provider) => (
              <SelectItem key={provider.metadata.uid} value={provider.metadata.name}>
                <div className="flex items-center gap-2">
                  <span>{provider.metadata.name}</span>
                  <Badge variant="outline" className="text-xs">
                    {provider.spec.type}
                  </Badge>
                  {provider.spec.model && (
                    <Badge variant="secondary" className="text-xs">
                      {provider.spec.model}
                    </Badge>
                  )}
                </div>
              </SelectItem>
            ))}
            {(!providers || providers.length === 0) && (
              <SelectItem value="__no_providers__" disabled>
                No Providers available
              </SelectItem>
            )}
          </SelectContent>
        </Select>
        {(!providers || providers.length === 0) && (
          <p className="text-xs text-warning">
            No Providers configured. Create a Provider resource first.
          </p>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// OptionsStep — wizard step 4
// ---------------------------------------------------------------------------

interface ToolRegistryItem {
  metadata: { uid?: string; name: string; namespace?: string };
  status?: { discoveredToolsCount?: number };
}

interface OptionsStepProps {
  formData: WizardFormData;
  currentWorkspace: { name?: string; namespace?: string; displayName?: string } | null;
  toolRegistries: ToolRegistryItem[] | undefined;
  updateField: <K extends keyof WizardFormData>(field: K, value: WizardFormData[K]) => void;
}

export function OptionsStep({
  formData,
  currentWorkspace,
  toolRegistries,
  updateField,
}: Readonly<OptionsStepProps>) {
  const toolSelectValue = formData.toolRegistryName
    ? `${formData.toolRegistryNamespace || currentWorkspace?.namespace}/${formData.toolRegistryName}`
    : "__none__";

  const handleToolRegistryChange = (v: string) => {
    if (v === "__none__") {
      updateField("toolRegistryName", "");
      updateField("toolRegistryNamespace", "");
    } else {
      const [ns, name] = v.split("/");
      updateField("toolRegistryName", name);
      updateField("toolRegistryNamespace", ns === currentWorkspace?.namespace ? "" : ns);
    }
  };

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <Label>Tool Registry (Optional)</Label>
        <p className="text-xs text-muted-foreground mb-2">
          Cross-namespace references are supported
        </p>
        <Select value={toolSelectValue} onValueChange={handleToolRegistryChange}>
          <SelectTrigger>
            <SelectValue placeholder="None" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none__">None</SelectItem>
            {toolRegistries?.map((registry) => {
              const ns = registry.metadata.namespace ?? "default";
              const name = registry.metadata.name;
              const isSameNamespace = ns === currentWorkspace?.namespace;
              return (
                <SelectItem key={registry.metadata.uid} value={`${ns}/${name}`}>
                  <div className="flex items-center gap-2">
                    <span>{name}</span>
                    {!isSameNamespace && (
                      <Badge variant="secondary" className="text-xs">{ns}</Badge>
                    )}
                    <Badge variant="outline" className="text-xs">
                      {registry.status?.discoveredToolsCount ?? 0} tools
                    </Badge>
                  </div>
                </SelectItem>
              );
            })}
          </SelectContent>
        </Select>
      </div>

      <div className="space-y-4 border-t pt-4">
        <Label className="text-base">Context Store</Label>
        <RadioGroup
          value={formData.contextType}
          onValueChange={(v) => updateField("contextType", v as ContextStoreType)}
          className="flex gap-4"
        >
          <div className="flex items-center space-x-2">
            <RadioGroupItem value="memory" id="context-memory" />
            <Label htmlFor="context-memory" className="cursor-pointer">In-Memory</Label>
          </div>
          <div className="flex items-center space-x-2">
            <RadioGroupItem value="redis" id="context-redis" />
            <Label htmlFor="context-redis" className="cursor-pointer">Redis</Label>
          </div>
        </RadioGroup>

        <div className="space-y-2">
          <Label htmlFor="contextTtl">Context TTL</Label>
          <Input
            id="contextTtl"
            value={formData.contextTtl}
            onChange={(e) => updateField("contextTtl", e.target.value)}
            placeholder="24h"
          />
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// RuntimeStep — wizard step 5
// ---------------------------------------------------------------------------

interface RuntimeStepProps {
  formData: WizardFormData;
  updateField: <K extends keyof WizardFormData>(field: K, value: WizardFormData[K]) => void;
  errors: Record<string, string>;
  validate: (path: string, value: unknown) => void;
}

export function RuntimeStep({ formData, updateField, errors, validate }: Readonly<RuntimeStepProps>) {
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label>Facade Type</Label>
          <Select
            value={formData.mode === "function" ? "rest" : formData.facadeType}
            onValueChange={(v) => {
              updateField("facadeType", v as FacadeType);
              validate("spec.facade.type", v);
            }}
            disabled={formData.mode === "function"}
          >
            <SelectTrigger
              aria-invalid={!!errors["spec.facade.type"]}
              aria-describedby={errors["spec.facade.type"] ? "facade-type-error" : undefined}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {formData.mode === "function" ? (
                <SelectItem value="rest">REST (HTTP)</SelectItem>
              ) : (
                <>
                  <SelectItem value="websocket">WebSocket</SelectItem>
                  <SelectItem value="grpc">gRPC</SelectItem>
                </>
              )}
            </SelectContent>
          </Select>
          <FieldError id="facade-type-error" message={errors["spec.facade.type"]} />
          {formData.mode === "function" && (
            <p className="text-xs text-muted-foreground">
              Function mode uses the REST (HTTP) facade.
            </p>
          )}
        </div>

        <div className="space-y-2">
          <Label htmlFor="facadePort">Port</Label>
          <Input
            id="facadePort"
            type="number"
            value={formData.facadePort}
            aria-invalid={!!errors["spec.facade.port"]}
            aria-describedby={errors["spec.facade.port"] ? "facade-port-error" : undefined}
            onChange={(e) => {
              const v = Number.parseInt(e.target.value) || 8080;
              updateField("facadePort", v);
              validate("spec.facade.port", v);
            }}
          />
          <FieldError id="facade-port-error" message={errors["spec.facade.port"]} />
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
}
