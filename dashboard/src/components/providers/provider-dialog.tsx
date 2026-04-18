"use client";

import { useState } from "react";
import { useProviderMutations } from "@/hooks/resources";
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { AlertCircle, Loader2, ChevronDown } from "lucide-react";
import type { Provider, ProviderSpec } from "@/types/generated/provider";

// --- Types ---

type CredentialSource = "secret" | "envVar" | "filePath";

interface FormState {
  name: string;
  providerType: ProviderSpec["type"];
  model: string;
  baseURL: string;
  capabilities: string[];
  // Credential
  credentialSource: CredentialSource;
  credentialSecretName: string;
  credentialSecretKey: string;
  credentialEnvVar: string;
  credentialFilePath: string;
  // Defaults
  temperature: string;
  topP: string;
  maxTokens: string;
  contextWindow: string;
  // Pricing
  inputCostPer1K: string;
  outputCostPer1K: string;
  cachedCostPer1K: string;
  // Platform (hyperscaler hosting)
  platformType: "" | "bedrock" | "vertex" | "azure";
  platformRegion: string;
  platformProject: string;
  platformEndpoint: string;
  // Platform auth
  authType: "" | "workloadIdentity" | "accessKey" | "serviceAccount" | "servicePrincipal";
  authRoleArn: string;
  authServiceAccountEmail: string;
  authSecretName: string;
  authSecretKey: string;
}

// --- Constants ---

// NOTE: Platform-hosted providers (claude on bedrock, openai on azure, gemini
// on vertex) are not yet supported in this dialog. Author a Provider CR
// manifest directly to configure spec.platform / spec.auth. See issue #909.
const PROVIDER_TYPES: { value: ProviderSpec["type"]; label: string }[] = [
  { value: "claude", label: "Claude (Anthropic)" },
  { value: "openai", label: "OpenAI" },
  { value: "gemini", label: "Gemini (Google)" },
  { value: "vllm", label: "vLLM" },
  { value: "voyageai", label: "Voyage AI" },
  { value: "ollama", label: "Ollama (Local)" },
  { value: "mock", label: "Mock (Testing)" },
];

const LOCAL_TYPES: Set<ProviderSpec["type"]> = new Set(["ollama", "mock", "vllm"]);

const PLATFORM_ELIGIBLE_TYPES: Set<ProviderSpec["type"]> = new Set([
  "claude",
  "openai",
  "gemini",
]);

// eslint-disable-next-line @typescript-eslint/no-unused-vars -- value consumers land in Task 3 UI
const PLATFORM_TYPES = ["bedrock", "vertex", "azure"] as const;
type PlatformType = (typeof PLATFORM_TYPES)[number];

// Auth methods allowed per platform. Mirrors the CRD's CEL auth matrix.
const AUTH_BY_PLATFORM: Record<PlatformType, readonly string[]> = {
  bedrock: ["workloadIdentity", "accessKey"],
  vertex: ["workloadIdentity", "serviceAccount"],
  azure: ["workloadIdentity", "servicePrincipal"],
};

// Routing supported today by the PromptKit runtime. All other 6 combos
// save correctly but surface a non-blocking warning in the UI. Tracked
// upstream in PromptKit#1009.
const CANONICAL_ROUTED: ReadonlySet<string> = new Set([
  "claude/bedrock",
  "openai/azure",
  "gemini/vertex",
]);

// eslint-disable-next-line @typescript-eslint/no-unused-vars -- consumed by Task 4 warning banner
function isCanonicalRouted(
  providerType: ProviderSpec["type"],
  platformType: string,
): boolean {
  return CANONICAL_ROUTED.has(`${providerType}/${platformType}`);
}

function supportsPlatform(type: ProviderSpec["type"]): boolean {
  return PLATFORM_ELIGIBLE_TYPES.has(type);
}

const ALL_CAPABILITIES = [
  "text",
  "streaming",
  "vision",
  "tools",
  "json",
  "audio",
  "video",
  "documents",
  "duplex",
] as const;

// --- Helpers ---

function isLocal(type: ProviderSpec["type"]): boolean {
  return LOCAL_TYPES.has(type);
}

function getInitialFormState(provider?: Provider | null): FormState {
  if (provider) {
    const spec = provider.spec;
    const credential = spec.credential;
    let credentialSource: CredentialSource = "secret";
    if (credential?.envVar) credentialSource = "envVar";
    else if (credential?.filePath) credentialSource = "filePath";

    const platform = spec.platform;
    const auth = spec.auth;
    return {
      name: provider.metadata?.name || "",
      providerType: spec.type,
      model: spec.model || "",
      baseURL: spec.baseURL || "",
      capabilities: spec.capabilities || [],
      credentialSource,
      credentialSecretName: credential?.secretRef?.name || spec.secretRef?.name || "",
      credentialSecretKey: credential?.secretRef?.key || spec.secretRef?.key || "",
      credentialEnvVar: credential?.envVar || "",
      credentialFilePath: credential?.filePath || "",
      temperature: spec.defaults?.temperature || "",
      topP: spec.defaults?.topP || "",
      maxTokens: spec.defaults?.maxTokens?.toString() || "",
      contextWindow: spec.defaults?.contextWindow?.toString() || "",
      inputCostPer1K: spec.pricing?.inputCostPer1K || "",
      outputCostPer1K: spec.pricing?.outputCostPer1K || "",
      cachedCostPer1K: spec.pricing?.cachedCostPer1K || "",
      platformType: (platform?.type ?? "") as FormState["platformType"],
      platformRegion: platform?.region ?? "",
      platformProject: platform?.project ?? "",
      platformEndpoint: platform?.endpoint ?? "",
      authType: (auth?.type ?? "") as FormState["authType"],
      authRoleArn: auth?.roleArn ?? "",
      authServiceAccountEmail: auth?.serviceAccountEmail ?? "",
      authSecretName: auth?.credentialsSecretRef?.name ?? "",
      authSecretKey: auth?.credentialsSecretRef?.key ?? "",
    };
  }

  return {
    name: "",
    providerType: "claude",
    model: "",
    baseURL: "",
    capabilities: [],
    credentialSource: "secret",
    credentialSecretName: "",
    credentialSecretKey: "",
    credentialEnvVar: "",
    credentialFilePath: "",
    temperature: "",
    topP: "",
    maxTokens: "",
    contextWindow: "",
    inputCostPer1K: "",
    outputCostPer1K: "",
    cachedCostPer1K: "",
    platformType: "",
    platformRegion: "",
    platformProject: "",
    platformEndpoint: "",
    authType: "",
    authRoleArn: "",
    authServiceAccountEmail: "",
    authSecretName: "",
    authSecretKey: "",
  };
}

function validateName(name: string): string | null {
  if (!name.trim()) return "Name is required";
  if (!/^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$/.test(name)) {
    return "Name must be a valid DNS subdomain (lowercase alphanumeric, hyphens, dots)";
  }
  return null;
}

function validateCredentialFields(form: FormState): string | null {
  if (form.credentialSource === "secret" && !form.credentialSecretName.trim()) {
    return "Secret name is required";
  }
  if (form.credentialSource === "envVar" && !form.credentialEnvVar.trim()) {
    return "Environment variable name is required";
  }
  if (form.credentialSource === "filePath" && !form.credentialFilePath.trim()) {
    return "File path is required";
  }
  return null;
}

function validatePlatformFields(form: FormState): string | null {
  if (!form.platformType) return null;

  if (
    (form.platformType === "bedrock" || form.platformType === "vertex") &&
    !form.platformRegion.trim()
  ) {
    return "Region is required for bedrock and vertex";
  }
  if (form.platformType === "vertex" && !form.platformProject.trim()) {
    return "Project is required for vertex";
  }
  if (form.platformType === "azure" && !form.platformEndpoint.trim()) {
    return "Endpoint is required for azure";
  }

  const allowed = AUTH_BY_PLATFORM[form.platformType];
  if (!form.authType) return "Auth type is required when a platform is configured";
  if (!allowed.includes(form.authType)) {
    return `Auth type ${form.authType} is not valid for platform ${form.platformType}`;
  }

  if (form.authType !== "workloadIdentity" && !form.authSecretName.trim()) {
    return "Credentials secret name is required for static auth";
  }

  return null;
}

function validateForm(form: FormState): string | null {
  const nameError = validateName(form.name);
  if (nameError) return nameError;

  if (form.platformType) {
    return validatePlatformFields(form);
  }

  if (!isLocal(form.providerType)) {
    return validateCredentialFields(form);
  }

  return null;
}

function buildCredential(form: FormState): ProviderSpec["credential"] | undefined {
  if (form.credentialSource === "secret" && form.credentialSecretName) {
    return {
      secretRef: {
        name: form.credentialSecretName,
        ...(form.credentialSecretKey ? { key: form.credentialSecretKey } : {}),
      },
    };
  }
  if (form.credentialSource === "envVar" && form.credentialEnvVar) {
    return { envVar: form.credentialEnvVar };
  }
  if (form.credentialSource === "filePath" && form.credentialFilePath) {
    return { filePath: form.credentialFilePath };
  }
  return undefined;
}

function buildDefaults(form: FormState): ProviderSpec["defaults"] | undefined {
  const defaults: NonNullable<ProviderSpec["defaults"]> = {};
  if (form.temperature) defaults.temperature = form.temperature;
  if (form.topP) defaults.topP = form.topP;
  if (form.maxTokens) defaults.maxTokens = Number.parseInt(form.maxTokens, 10);
  if (form.contextWindow) defaults.contextWindow = Number.parseInt(form.contextWindow, 10);
  return Object.keys(defaults).length > 0 ? defaults : undefined;
}

function buildPricing(form: FormState): ProviderSpec["pricing"] | undefined {
  const pricing: NonNullable<ProviderSpec["pricing"]> = {};
  if (form.inputCostPer1K) pricing.inputCostPer1K = form.inputCostPer1K;
  if (form.outputCostPer1K) pricing.outputCostPer1K = form.outputCostPer1K;
  if (form.cachedCostPer1K) pricing.cachedCostPer1K = form.cachedCostPer1K;
  return Object.keys(pricing).length > 0 ? pricing : undefined;
}

function buildPlatformAndAuth(
  form: FormState,
): Pick<ProviderSpec, "platform" | "auth"> {
  if (!form.platformType) return {};

  const platform: NonNullable<ProviderSpec["platform"]> = {
    type: form.platformType,
  };
  if (form.platformRegion) platform.region = form.platformRegion;
  if (form.platformProject) platform.project = form.platformProject;
  if (form.platformEndpoint) platform.endpoint = form.platformEndpoint;

  const auth: NonNullable<ProviderSpec["auth"]> = {
    type: form.authType as NonNullable<ProviderSpec["auth"]>["type"],
  };
  if (form.authRoleArn) auth.roleArn = form.authRoleArn;
  if (form.authServiceAccountEmail) {
    auth.serviceAccountEmail = form.authServiceAccountEmail;
  }
  if (form.authSecretName) {
    auth.credentialsSecretRef = {
      name: form.authSecretName,
      ...(form.authSecretKey ? { key: form.authSecretKey } : {}),
    };
  }

  return { platform, auth };
}

function buildSpec(form: FormState): ProviderSpec {
  const spec: ProviderSpec = {
    type: form.providerType,
  };

  if (form.model) spec.model = form.model;
  if (form.baseURL) spec.baseURL = form.baseURL;
  if (form.capabilities.length > 0) {
    spec.capabilities = form.capabilities as ProviderSpec["capabilities"];
  }

  const platformPart = buildPlatformAndAuth(form);
  if (platformPart.platform) {
    spec.platform = platformPart.platform;
    spec.auth = platformPart.auth;
    // Direct-API credential is meaningless when platform is set; omit.
  } else if (!isLocal(form.providerType)) {
    spec.credential = buildCredential(form);
  }

  spec.defaults = buildDefaults(form);
  spec.pricing = buildPricing(form);

  return spec;
}

// --- Sub-components ---

function CredentialFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  return (
    <div className="space-y-4">
      <Label>Credential Source</Label>
      <RadioGroup
        value={form.credentialSource}
        onValueChange={(v) => updateForm("credentialSource", v as CredentialSource)}
        className="flex gap-4"
      >
        <div className="flex items-center space-x-2">
          <RadioGroupItem value="secret" id="cred-secret" />
          <Label htmlFor="cred-secret" className="font-normal">Secret</Label>
        </div>
        <div className="flex items-center space-x-2">
          <RadioGroupItem value="envVar" id="cred-env" />
          <Label htmlFor="cred-env" className="font-normal">Env Variable</Label>
        </div>
        <div className="flex items-center space-x-2">
          <RadioGroupItem value="filePath" id="cred-file" />
          <Label htmlFor="cred-file" className="font-normal">File Path</Label>
        </div>
      </RadioGroup>

      {form.credentialSource === "secret" && (
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="cred-secret-name">Secret Name</Label>
            <Input
              id="cred-secret-name"
              placeholder="my-api-key"
              value={form.credentialSecretName}
              onChange={(e) => updateForm("credentialSecretName", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="cred-secret-key">Key (optional)</Label>
            <Input
              id="cred-secret-key"
              placeholder="ANTHROPIC_API_KEY"
              value={form.credentialSecretKey}
              onChange={(e) => updateForm("credentialSecretKey", e.target.value)}
            />
          </div>
        </div>
      )}

      {form.credentialSource === "envVar" && (
        <div className="space-y-2">
          <Label htmlFor="cred-env-var">Environment Variable</Label>
          <Input
            id="cred-env-var"
            placeholder="ANTHROPIC_API_KEY"
            value={form.credentialEnvVar}
            onChange={(e) => updateForm("credentialEnvVar", e.target.value)}
          />
        </div>
      )}

      {form.credentialSource === "filePath" && (
        <div className="space-y-2">
          <Label htmlFor="cred-file-path">File Path</Label>
          <Input
            id="cred-file-path"
            placeholder="/var/run/secrets/api-key"
            value={form.credentialFilePath}
            onChange={(e) => updateForm("credentialFilePath", e.target.value)}
          />
        </div>
      )}
    </div>
  );
}

function DefaultsFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const [open, setOpen] = useState(
    !!(form.temperature || form.topP || form.maxTokens || form.contextWindow)
  );

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-0 font-semibold">
          Defaults
          <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-4 pt-2">
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="temperature">Temperature</Label>
            <Input
              id="temperature"
              type="number"
              step="0.1"
              min="0"
              max="2"
              placeholder="0.7"
              value={form.temperature}
              onChange={(e) => updateForm("temperature", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="top-p">Top P</Label>
            <Input
              id="top-p"
              type="number"
              step="0.1"
              min="0"
              max="1"
              placeholder="0.9"
              value={form.topP}
              onChange={(e) => updateForm("topP", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="max-tokens">Max Tokens</Label>
            <Input
              id="max-tokens"
              type="number"
              min="1"
              placeholder="4096"
              value={form.maxTokens}
              onChange={(e) => updateForm("maxTokens", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="context-window">Context Window</Label>
            <Input
              id="context-window"
              type="number"
              min="1"
              placeholder="128000"
              value={form.contextWindow}
              onChange={(e) => updateForm("contextWindow", e.target.value)}
            />
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function PricingFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const [open, setOpen] = useState(
    !!(form.inputCostPer1K || form.outputCostPer1K || form.cachedCostPer1K)
  );

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-0 font-semibold">
          Pricing
          <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-4 pt-2">
        <div className="grid grid-cols-3 gap-4">
          <div className="space-y-2">
            <Label htmlFor="input-cost">Input / 1K tokens</Label>
            <Input
              id="input-cost"
              placeholder="0.003"
              value={form.inputCostPer1K}
              onChange={(e) => updateForm("inputCostPer1K", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="output-cost">Output / 1K tokens</Label>
            <Input
              id="output-cost"
              placeholder="0.015"
              value={form.outputCostPer1K}
              onChange={(e) => updateForm("outputCostPer1K", e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="cached-cost">Cached / 1K tokens</Label>
            <Input
              id="cached-cost"
              placeholder="0.0003"
              value={form.cachedCostPer1K}
              onChange={(e) => updateForm("cachedCostPer1K", e.target.value)}
            />
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function CapabilitiesFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const [open, setOpen] = useState(form.capabilities.length > 0);

  const toggleCapability = (cap: string) => {
    const current = form.capabilities;
    if (current.includes(cap)) {
      updateForm("capabilities", current.filter((c) => c !== cap));
    } else {
      updateForm("capabilities", [...current, cap]);
    }
  };

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-0 font-semibold">
          Capabilities
          <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-2">
        <div className="grid grid-cols-2 gap-2">
          {ALL_CAPABILITIES.map((cap) => (
            <div key={cap} className="flex items-center space-x-2">
              <Checkbox
                id={`cap-${cap}`}
                checked={form.capabilities.includes(cap)}
                onCheckedChange={() => toggleCapability(cap)}
              />
              <Label htmlFor={`cap-${cap}`} className="text-sm font-normal capitalize">
                {cap}
              </Label>
            </div>
          ))}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

// --- Main Dialog ---

interface ProviderDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider?: Provider | null;
  onSuccess?: () => void;
}

export function ProviderDialog({
  open,
  onOpenChange,
  provider,
  onSuccess,
}: Readonly<ProviderDialogProps>) {
  const { createProvider, updateProvider, loading } = useProviderMutations();
  const isEditing = !!provider;

  const formResetKey = `${provider?.metadata?.name ?? "new"}-${open}`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <ProviderDialogForm
        key={formResetKey}
        provider={provider}
        isEditing={isEditing}
        loading={loading}
        createProvider={createProvider}
        updateProvider={updateProvider}
        onSuccess={onSuccess}
        onOpenChange={onOpenChange}
      />
    </Dialog>
  );
}

interface ProviderDialogFormProps {
  provider?: Provider | null;
  isEditing: boolean;
  loading: boolean;
  createProvider: (name: string, spec: ProviderSpec) => Promise<Provider>;
  updateProvider: (name: string, spec: ProviderSpec) => Promise<Provider>;
  onSuccess?: () => void;
  onOpenChange: (open: boolean) => void;
}

function ProviderDialogForm({
  provider,
  isEditing,
  loading,
  createProvider,
  updateProvider,
  onSuccess,
  onOpenChange,
}: Readonly<ProviderDialogFormProps>) {
  const [formState, setFormState] = useState<FormState>(() => getInitialFormState(provider));
  const [error, setError] = useState<string | null>(null);

  const updateForm = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setFormState((prev) => ({ ...prev, [key]: value }));
  };

  const handleProviderTypeChange = (type: ProviderSpec["type"]) => {
    setFormState((prev) => {
      const keepPlatform = supportsPlatform(type);
      return {
        ...prev,
        providerType: type,
        // Reset credential fields
        credentialSource: "secret",
        credentialSecretName: "",
        credentialSecretKey: "",
        credentialEnvVar: "",
        credentialFilePath: "",
        // Clear platform/auth when switching to a non-eligible type
        platformType: keepPlatform ? prev.platformType : "",
        platformRegion: keepPlatform ? prev.platformRegion : "",
        platformProject: keepPlatform ? prev.platformProject : "",
        platformEndpoint: keepPlatform ? prev.platformEndpoint : "",
        authType: keepPlatform ? prev.authType : "",
        authRoleArn: keepPlatform ? prev.authRoleArn : "",
        authServiceAccountEmail: keepPlatform ? prev.authServiceAccountEmail : "",
        authSecretName: keepPlatform ? prev.authSecretName : "",
        authSecretKey: keepPlatform ? prev.authSecretKey : "",
      };
    });
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
        await updateProvider(formState.name, spec);
      } else {
        await createProvider(formState.name, spec);
      }

      onSuccess?.();
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save provider");
    }
  };

  const showCredential = !isLocal(formState.providerType);

  return (
    <DialogContent className="sm:max-w-[600px] max-h-[90vh] flex flex-col overflow-hidden">
      <DialogHeader>
        <DialogTitle>{isEditing ? "Edit Provider" : "Create Provider"}</DialogTitle>
        <DialogDescription>
          {isEditing
            ? "Update the configuration for this provider."
            : "Configure a new LLM provider for your workspace."}
        </DialogDescription>
      </DialogHeader>

      <div className="flex-1 min-h-0 overflow-y-auto -mx-6 px-6">
        <div className="space-y-6 py-4">
          {error && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {/* Name */}
          <div className="space-y-2">
            <Label htmlFor="provider-name">Name</Label>
            <Input
              id="provider-name"
              placeholder="my-provider"
              value={formState.name}
              onChange={(e) => updateForm("name", e.target.value)}
              disabled={isEditing}
            />
          </div>

          {/* Provider Type */}
          <div className="space-y-2">
            <Label htmlFor="provider-type">Provider Type</Label>
            <Select
              value={formState.providerType}
              onValueChange={handleProviderTypeChange}
              disabled={isEditing}
            >
              <SelectTrigger id="provider-type">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PROVIDER_TYPES.map((type) => (
                  <SelectItem key={type.value} value={type.value}>
                    {type.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Model */}
          <div className="space-y-2">
            <Label htmlFor="provider-model">Model</Label>
            <Input
              id="provider-model"
              placeholder="claude-sonnet-4-20250514"
              value={formState.model}
              onChange={(e) => updateForm("model", e.target.value)}
            />
          </div>

          {/* Base URL */}
          <div className="space-y-2">
            <Label htmlFor="provider-base-url">Base URL (optional)</Label>
            <Input
              id="provider-base-url"
              placeholder="https://api.example.com/v1"
              value={formState.baseURL}
              onChange={(e) => updateForm("baseURL", e.target.value)}
            />
          </div>

          {/* Credential section */}
          {showCredential && (
            <div className="border rounded-lg p-4 space-y-4">
              <Label className="text-base font-semibold">Credentials</Label>
              <CredentialFields form={formState} updateForm={updateForm} />
            </div>
          )}

          {/* Capabilities */}
          <CapabilitiesFields form={formState} updateForm={updateForm} />

          {/* Defaults (collapsible) */}
          <DefaultsFields form={formState} updateForm={updateForm} />

          {/* Pricing (collapsible) */}
          <PricingFields form={formState} updateForm={updateForm} />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={loading}>
          {loading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
          {isEditing ? "Save Changes" : "Create Provider"}
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}
