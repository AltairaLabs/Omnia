"use client";

import { useState } from "react";
import { useProviderMutations } from "@/hooks/resources";
import { useWorkspace } from "@/contexts/workspace-context";
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
import { AlertCircle, Loader2 } from "lucide-react";
import type { Provider, ProviderSpec } from "@/types/generated/provider";
import { useFieldValidation } from "@/hooks/use-field-validation";
import { FieldError } from "@/components/ui/field-error";
import { crdConstraints } from "@/types/generated/crd-constraints";
import { SecretKeySelect } from "./secret-key-select";
import { AddCredentialSecretDialog } from "@/components/credentials/add-credential-secret-dialog";
import {
  CapabilitiesFields,
  DefaultsFields,
  HeadersFields,
  PricingFields,
} from "./provider-dialog-fields";
import {
  EmbeddingFields,
  PlatformFields,
  STTFields,
  TTSFields,
} from "./provider-model-fields";

// --- Types ---

type CredentialSource = "secret" | "envVar" | "filePath";
type ProviderRole = NonNullable<ProviderSpec["role"]>;

export interface FormState {
  name: string;
  role: ProviderRole;
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
  // Platform (hyperscaler hosting; llm role only)
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
  // TTS role config
  ttsVoice: string;
  ttsFormat: "" | "pcm" | "mp3" | "opus" | "wav" | "flac";
  ttsSampleRate: string;
  // STT role config
  sttLanguage: string;
  sttSampleRate: string;
  // Embedding role config
  embeddingDimensions: string;
  embeddingDistance: "" | "cosine" | "l2" | "dot";
  // Custom HTTP headers (gateway providers like OpenRouter, tenant routing, etc.)
  headerEntries: Array<{ id: string; key: string; value: string }>;
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
  { value: "cartesia", label: "Cartesia" },
  { value: "elevenlabs", label: "ElevenLabs" },
  { value: "imagen", label: "Imagen (Google)" },
  { value: "huggingface", label: "HuggingFace" },
  { value: "ollama", label: "Ollama (Local)" },
  { value: "mock", label: "Mock (Testing)" },
];

const LOCAL_TYPES: Set<ProviderSpec["type"]> = new Set(["ollama", "mock", "vllm"]);

const PLATFORM_ELIGIBLE_TYPES: Set<ProviderSpec["type"]> = new Set([
  "claude",
  "openai",
  "gemini",
]);

const ROLE_OPTIONS: { value: ProviderRole; label: string; description: string }[] = [
  { value: "llm", label: "LLM (chat / completion)", description: "Standard LLM chat and completion." },
  { value: "embedding", label: "Embedding", description: "Vector embeddings for retrieval." },
  { value: "tts", label: "Text-to-Speech", description: "Synthesize audio from text." },
  { value: "stt", label: "Speech-to-Text", description: "Transcribe audio to text." },
  { value: "image", label: "Image generation", description: "Generate images from prompts." },
  { value: "inference", label: "Inference", description: "Generic classify/inference providers (e.g. HuggingFace)." },
];

// Mirrors the CRD CEL matrix (api/v1alpha1/provider_types.go). Keep in sync
// when PromptKit adds vendor support for additional roles.
const VENDORS_BY_ROLE: Record<ProviderRole, readonly ProviderSpec["type"][]> = {
  llm: ["claude", "openai", "gemini", "ollama", "mock", "vllm"],
  embedding: ["openai", "voyageai", "gemini", "ollama"],
  tts: ["openai", "cartesia", "elevenlabs"],
  stt: ["openai"],
  image: ["imagen"],
  inference: ["huggingface"],
};

function vendorAllowedForRole(role: ProviderRole, type: ProviderSpec["type"]): boolean {
  return VENDORS_BY_ROLE[role].includes(type);
}

function firstVendorForRole(role: ProviderRole): ProviderSpec["type"] {
  return VENDORS_BY_ROLE[role][0];
}

// Slice of FormState that only llm-role Providers may carry. Cleared when
// the user switches off the llm role.
const PLATFORM_FIELDS_BLANK = {
  platformType: "" as FormState["platformType"],
  platformRegion: "",
  platformProject: "",
  platformEndpoint: "",
  authType: "" as FormState["authType"],
  authRoleArn: "",
  authServiceAccountEmail: "",
  authSecretName: "",
  authSecretKey: "",
};

const TTS_FIELDS_BLANK = {
  ttsVoice: "",
  ttsFormat: "" as FormState["ttsFormat"],
  ttsSampleRate: "",
};

const STT_FIELDS_BLANK = {
  sttLanguage: "",
  sttSampleRate: "",
};

const EMBEDDING_FIELDS_BLANK = {
  embeddingDimensions: "",
  embeddingDistance: "" as FormState["embeddingDistance"],
};

function preservePlatformFields(prev: FormState): Pick<FormState,
  | "platformType" | "platformRegion" | "platformProject" | "platformEndpoint"
  | "authType" | "authRoleArn" | "authServiceAccountEmail"
  | "authSecretName" | "authSecretKey"
> {
  return {
    platformType: prev.platformType,
    platformRegion: prev.platformRegion,
    platformProject: prev.platformProject,
    platformEndpoint: prev.platformEndpoint,
    authType: prev.authType,
    authRoleArn: prev.authRoleArn,
    authServiceAccountEmail: prev.authServiceAccountEmail,
    authSecretName: prev.authSecretName,
    authSecretKey: prev.authSecretKey,
  };
}

function preserveTTSFields(prev: FormState) {
  return {
    ttsVoice: prev.ttsVoice,
    ttsFormat: prev.ttsFormat,
    ttsSampleRate: prev.ttsSampleRate,
  };
}

function preserveSTTFields(prev: FormState) {
  return {
    sttLanguage: prev.sttLanguage,
    sttSampleRate: prev.sttSampleRate,
  };
}

function preserveEmbeddingFields(prev: FormState) {
  return {
    embeddingDimensions: prev.embeddingDimensions,
    embeddingDistance: prev.embeddingDistance,
  };
}

// applyRoleChange snaps the form to a valid state for the new role: vendor
// gets reset if not allowed, and role-specific blocks not belonging to the
// new role are wiped (the CRD CEL gate enforces "at most one of
// tts/stt/embedding"). Extracted so handleRoleChange stays under the sonarjs
// cognitive-complexity threshold (15).
function applyRoleChange(prev: FormState, role: ProviderRole): FormState {
  const providerType = vendorAllowedForRole(role, prev.providerType)
    ? prev.providerType
    : firstVendorForRole(role);

  return {
    ...prev,
    role,
    providerType,
    ...(role === "llm" ? preservePlatformFields(prev) : PLATFORM_FIELDS_BLANK),
    ...(role === "tts" ? preserveTTSFields(prev) : TTS_FIELDS_BLANK),
    ...(role === "stt" ? preserveSTTFields(prev) : STT_FIELDS_BLANK),
    ...(role === "embedding" ? preserveEmbeddingFields(prev) : EMBEDDING_FIELDS_BLANK),
  };
}

export type PlatformType = "bedrock" | "vertex" | "azure";

// Auth methods allowed per platform. Mirrors the CRD's CEL auth matrix.
export const AUTH_BY_PLATFORM: Record<PlatformType, readonly string[]> = {
  bedrock: ["workloadIdentity", "accessKey"],
  vertex: ["workloadIdentity", "serviceAccount"],
  azure: ["workloadIdentity", "servicePrincipal"],
};

function supportsPlatform(type: ProviderSpec["type"]): boolean {
  return PLATFORM_ELIGIBLE_TYPES.has(type);
}

// --- Helpers ---

function isLocal(type: ProviderSpec["type"]): boolean {
  return LOCAL_TYPES.has(type);
}

// Monotonic counter for stable React keys on added header rows; reset only
// on full page reload, which is fine because the dialog itself remounts via
// `formResetKey` whenever it opens.
let nextHeaderEntryId = 0;
export function makeHeaderEntryId(): string {
  nextHeaderEntryId += 1;
  return `h-${nextHeaderEntryId}`;
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
    // Pre-role Providers omit spec.role; treat as llm for back-compat.
    const role: ProviderRole = spec.role ?? "llm";
    return {
      name: provider.metadata?.name || "",
      role,
      providerType: spec.type,
      model: spec.model || "",
      baseURL: spec.baseURL || "",
      capabilities: spec.capabilities || [],
      credentialSource,
      credentialSecretName: credential?.secretRef?.name || "",
      credentialSecretKey: credential?.secretRef?.key || "",
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
      ttsVoice: spec.tts?.voice ?? "",
      ttsFormat: spec.tts?.format ?? "",
      ttsSampleRate: spec.tts?.sampleRate?.toString() ?? "",
      sttLanguage: spec.stt?.language ?? "",
      sttSampleRate: spec.stt?.sampleRate?.toString() ?? "",
      embeddingDimensions: spec.embedding?.dimensions?.toString() ?? "",
      embeddingDistance: spec.embedding?.distance ?? "",
      headerEntries: Object.entries(spec.headers ?? {}).map(([key, value]) => ({
        id: makeHeaderEntryId(),
        key,
        value,
      })),
    };
  }

  return {
    name: "",
    role: "llm",
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
    ttsVoice: "",
    ttsFormat: "",
    ttsSampleRate: "",
    sttLanguage: "",
    sttSampleRate: "",
    embeddingDimensions: "",
    embeddingDistance: "",
    headerEntries: [],
  };
}

const ENV_VAR_NAME_RE = /^[A-Za-z_]\w*$/;
function envVarError(value: string): string | null {
  if (!value) return null;
  return ENV_VAR_NAME_RE.test(value)
    ? null
    : "Enter a variable NAME (e.g. ANTHROPIC_API_KEY), not a key=value or a secret value.";
}

/**
 * Checks that the active credential source has a non-empty value. Returns an
 * error message or null. Credential is required when the provider is not local
 * and (for llm role) no hosting platform is configured.
 */
function validateActiveCredential(form: FormState): string | null {
  const credentialRequired =
    !isLocal(form.providerType) && (form.role !== "llm" || !form.platformType);
  if (!credentialRequired) return null;

  if (form.credentialSource === "secret" && !form.credentialSecretName.trim()) {
    return "Secret name is required";
  }
  if (form.credentialSource === "envVar") {
    if (!form.credentialEnvVar.trim()) {
      return "Environment variable name is required";
    }
    const envErr = envVarError(form.credentialEnvVar);
    if (envErr) return envErr;
  }
  if (form.credentialSource === "filePath" && !form.credentialFilePath.trim()) {
    return "File path is required";
  }
  return null;
}

/**
 * Cross-field validation rules that cannot be expressed as single-field CRD
 * constraints. Returns an error string for the Alert banner, or null.
 *
 * - Role/vendor compatibility is enforced here because VENDORS_BY_ROLE is UI
 *   state, not a single-field constraint.
 * - Active credential source field is required when credential section is shown
 *   (conditional on source, so it cannot be expressed in the static map).
 * - Platform requirements (region, project, endpoint) are cross-field because
 *   each sub-field is only required conditionally on platformType.
 * - Auth secret is required conditionally on authType (not workloadIdentity).
 */
function validateCrossFields(form: FormState): string | null {
  if (!vendorAllowedForRole(form.role, form.providerType)) {
    return `Vendor "${form.providerType}" is not supported for role "${form.role}"`;
  }
  // Platform is llm-only.
  if (form.role !== "llm" && form.platformType) {
    return "Hosting platform is only valid when role is llm";
  }

  const credentialError = validateActiveCredential(form);
  if (credentialError) return credentialError;

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
  if (!form.authType) return "Auth type is required when a platform is configured";

  const allowed = AUTH_BY_PLATFORM[form.platformType];
  if (!allowed.includes(form.authType)) {
    return `Auth type ${form.authType} is not valid for platform ${form.platformType}`;
  }
  if (form.authType !== "workloadIdentity" && !form.authSecretName.trim()) {
    return "Credentials secret name is required for static auth";
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

function buildHeaders(form: FormState): ProviderSpec["headers"] | undefined {
  const headers: Record<string, string> = {};
  for (const { key, value } of form.headerEntries) {
    const k = key.trim();
    if (k) headers[k] = value;
  }
  return Object.keys(headers).length > 0 ? headers : undefined;
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

function buildTTSConfig(form: FormState): NonNullable<ProviderSpec["tts"]> | undefined {
  if (form.role !== "tts") return undefined;
  const tts: NonNullable<ProviderSpec["tts"]> = {};
  if (form.ttsVoice) tts.voice = form.ttsVoice;
  if (form.ttsFormat) tts.format = form.ttsFormat;
  if (form.ttsSampleRate) {
    const n = Number.parseInt(form.ttsSampleRate, 10);
    if (!Number.isNaN(n)) tts.sampleRate = n;
  }
  // Role block must exist when role=tts (CEL gate); even an empty object is
  // accepted because every field is optional.
  return tts;
}

function buildSTTConfig(form: FormState): NonNullable<ProviderSpec["stt"]> | undefined {
  if (form.role !== "stt") return undefined;
  const stt: NonNullable<ProviderSpec["stt"]> = {};
  if (form.sttLanguage) stt.language = form.sttLanguage;
  if (form.sttSampleRate) {
    const n = Number.parseInt(form.sttSampleRate, 10);
    if (!Number.isNaN(n)) stt.sampleRate = n;
  }
  return stt;
}

function buildEmbeddingConfig(form: FormState): NonNullable<ProviderSpec["embedding"]> | undefined {
  if (form.role !== "embedding") return undefined;
  const emb: NonNullable<ProviderSpec["embedding"]> = {};
  if (form.embeddingDimensions) {
    const n = Number.parseInt(form.embeddingDimensions, 10);
    if (!Number.isNaN(n)) emb.dimensions = n;
  }
  if (form.embeddingDistance) emb.distance = form.embeddingDistance;
  return emb;
}

function buildSpec(form: FormState): ProviderSpec {
  const spec: ProviderSpec = {
    type: form.providerType,
    role: form.role,
  };

  if (form.model) spec.model = form.model;
  if (form.baseURL) spec.baseURL = form.baseURL;
  if (form.capabilities.length > 0) {
    spec.capabilities = form.capabilities as ProviderSpec["capabilities"];
  }

  // Platform/auth are llm-only; the wizard hides those fields for other
  // roles but be defensive anyway.
  if (form.role === "llm") {
    const platformPart = buildPlatformAndAuth(form);
    if (platformPart.platform) {
      spec.platform = platformPart.platform;
      spec.auth = platformPart.auth;
      // Direct-API credential is meaningless when platform is set; omit.
    } else if (!isLocal(form.providerType)) {
      spec.credential = buildCredential(form);
    }
  } else if (!isLocal(form.providerType)) {
    spec.credential = buildCredential(form);
  }

  const tts = buildTTSConfig(form);
  if (tts) spec.tts = tts;
  const stt = buildSTTConfig(form);
  if (stt) spec.stt = stt;
  const embedding = buildEmbeddingConfig(form);
  if (embedding) spec.embedding = embedding;

  spec.defaults = buildDefaults(form);
  spec.pricing = buildPricing(form);

  const headers = buildHeaders(form);
  if (headers) spec.headers = headers;

  return spec;
}

// --- Sub-components ---

export interface ValidateProps {
  validate: (path: string, value: unknown) => void;
  errors: Record<string, string>;
}

function CredentialFields({
  form,
  updateForm,
  validate,
  errors,
  namespace,
  onAddSecret,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
  namespace?: string;
  onAddSecret?: () => void;
} & ValidateProps>) {
  const envErr = envVarError(form.credentialEnvVar);
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
        <SecretKeySelect
          idPrefix="cred"
          namespace={namespace}
          secretName={form.credentialSecretName}
          secretKey={form.credentialSecretKey}
          onSecretNameChange={(v) => updateForm("credentialSecretName", v)}
          onSecretKeyChange={(v) => updateForm("credentialSecretKey", v)}
          onAddSecret={onAddSecret}
        />
      )}

      {form.credentialSource === "envVar" && (
        <div className="space-y-2">
          <Label htmlFor="cred-env-var">Environment variable name</Label>
          <Input
            id="cred-env-var"
            placeholder="ANTHROPIC_API_KEY"
            value={form.credentialEnvVar}
            onChange={(e) => updateForm("credentialEnvVar", e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            The name of an env var already present in the runtime — not a key=value or the secret itself.
          </p>
          {envErr && (
            <p className="text-xs text-destructive">{envErr}</p>
          )}
        </div>
      )}

      {form.credentialSource === "filePath" && (
        <div className="space-y-2">
          <Label htmlFor="cred-file-path">File Path</Label>
          <Input
            id="cred-file-path"
            placeholder="/var/run/secrets/api-key"
            aria-invalid={!!errors["spec.credential.filePath"]}
            aria-describedby={errors["spec.credential.filePath"] ? "cred-file-path-error" : undefined}
            value={form.credentialFilePath}
            onChange={(e) => {
              updateForm("credentialFilePath", e.target.value);
              validate("spec.credential.filePath", e.target.value);
            }}
          />
          <FieldError id="cred-file-path-error" message={errors["spec.credential.filePath"]} />
        </div>
      )}
    </div>
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

function buildValidateAllFields(
  formState: FormState,
  showCredential: boolean,
): Array<{ path: string; value: unknown }> {
  const fields: Array<{ path: string; value: unknown }> = [
    { path: "metadata.name", value: formState.name },
  ];

  if (showCredential && formState.credentialSource === "filePath") {
    // filePath keeps inline CRD validation. Secret (SecretKeySelect dropdown) and
    // envVar NAME format are validated on submit via validateActiveCredential
    // (cross-field), not the inline CRD validate path — see merge policy.
    fields.push({ path: "spec.credential.filePath", value: formState.credentialFilePath });
  }

  if (formState.role === "embedding") {
    fields.push({
      path: "spec.embedding.dimensions",
      value: formState.embeddingDimensions ? Number(formState.embeddingDimensions) : null,
    });
  }
  if (formState.role === "stt") {
    fields.push({ path: "spec.stt.language", value: formState.sttLanguage });
    fields.push({
      path: "spec.stt.sampleRate",
      value: formState.sttSampleRate ? Number(formState.sttSampleRate) : null,
    });
  }
  if (formState.role === "tts") {
    fields.push({
      path: "spec.tts.sampleRate",
      value: formState.ttsSampleRate ? Number(formState.ttsSampleRate) : null,
    });
  }

  return fields;
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
  const { currentWorkspace } = useWorkspace();
  const [formState, setFormState] = useState<FormState>(() => getInitialFormState(provider));
  const [error, setError] = useState<string | null>(null);
  const [showAddSecret, setShowAddSecret] = useState(false);

  const namespace = currentWorkspace?.namespace;

  const { errors, validate, validateAll, hasErrors } = useFieldValidation(crdConstraints.Provider);

  const updateForm = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setFormState((prev) => ({ ...prev, [key]: value }));
  };

  const handleProviderTypeChange = (type: ProviderSpec["type"]) => {
    setFormState((prev) => {
      const keepPlatform = supportsPlatform(type) && prev.role === "llm";
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

  const handleRoleChange = (role: ProviderRole) => {
    setFormState((prev) => applyRoleChange(prev, role));
  };

  const isLLMRole = formState.role === "llm";
  const showCredential =
    !isLocal(formState.providerType) && (isLLMRole ? !formState.platformType : true);
  const showPlatform = isLLMRole && supportsPlatform(formState.providerType);

  const handleSubmit = async () => {
    try {
      setError(null);

      const fields = buildValidateAllFields(formState, showCredential);
      if (!validateAll(fields)) return;

      // Cross-field checks not expressible as single-field CRD constraints.
      const crossFieldError = validateCrossFields(formState);
      if (crossFieldError) {
        setError(crossFieldError);
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

  // Narrow the vendor list to those the CRD CEL matrix accepts for this role
  // so the user can't pick an invalid (role, vendor) pair from the UI.
  const allowedVendors = VENDORS_BY_ROLE[formState.role];
  const vendorOptions = PROVIDER_TYPES.filter((t) => allowedVendors.includes(t.value));

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
              aria-invalid={!!errors["metadata.name"]}
              aria-describedby={errors["metadata.name"] ? "provider-name-error" : undefined}
              value={formState.name}
              onChange={(e) => {
                updateForm("name", e.target.value);
                validate("metadata.name", e.target.value);
              }}
              disabled={isEditing}
            />
            <FieldError id="provider-name-error" message={errors["metadata.name"]} />
          </div>

          {/* Role — pick first so the vendor list can narrow. Disabled in
              edit mode because changing role would invalidate references from
              AgentRuntime resources. */}
          <div className="space-y-2">
            <Label htmlFor="provider-role">Role</Label>
            <Select
              value={formState.role}
              onValueChange={(v) => handleRoleChange(v as ProviderRole)}
              disabled={isEditing}
            >
              <SelectTrigger id="provider-role">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ROLE_OPTIONS.map((r) => (
                  <SelectItem key={r.value} value={r.value}>
                    {r.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              {ROLE_OPTIONS.find((r) => r.value === formState.role)?.description}
            </p>
          </div>

          {/* Provider Type — narrowed by role. */}
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
                {vendorOptions.map((type) => (
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

          {showPlatform && <PlatformFields form={formState} updateForm={updateForm} namespace={namespace} />}

          {/* Role-specific config blocks (CEL-gated; at most one of tts/stt/embedding) */}
          {formState.role === "tts" && (
            <TTSFields form={formState} updateForm={updateForm} validate={validate} errors={errors} />
          )}
          {formState.role === "stt" && (
            <STTFields form={formState} updateForm={updateForm} validate={validate} errors={errors} />
          )}
          {formState.role === "embedding" && (
            <EmbeddingFields form={formState} updateForm={updateForm} validate={validate} errors={errors} />
          )}

          {/* Credential section */}
          {showCredential && (
            <div className="border rounded-lg p-4 space-y-4">
              <Label className="text-base font-semibold">Credentials</Label>
              <CredentialFields
                form={formState}
                updateForm={updateForm}
                validate={validate}
                errors={errors}
                namespace={namespace}
                onAddSecret={() => setShowAddSecret(true)}
              />
              <a
                href="https://omnia.altairalabs.ai/docs/how-to/manage-credentials"
                target="_blank"
                rel="noopener noreferrer"
                className="text-xs text-primary hover:underline"
              >
                How to add credentials
              </a>
            </div>
          )}

          {/* Capabilities */}
          <CapabilitiesFields form={formState} updateForm={updateForm} />

          {/* Defaults (collapsible) */}
          <DefaultsFields form={formState} updateForm={updateForm} />

          {/* Pricing (collapsible) */}
          <PricingFields form={formState} updateForm={updateForm} />

          {/* Headers (collapsible) */}
          <HeadersFields form={formState} updateForm={updateForm} />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Cancel
        </Button>
        <Button onClick={handleSubmit} disabled={hasErrors || loading}>
          {loading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
          {isEditing ? "Save Changes" : "Create Provider"}
        </Button>
      </DialogFooter>

      <AddCredentialSecretDialog
        open={showAddSecret}
        onOpenChange={setShowAddSecret}
        namespace={namespace}
        onCreated={(name) => {
          updateForm("credentialSecretName", name);
          setShowAddSecret(false);
        }}
      />
    </DialogContent>
  );
}
