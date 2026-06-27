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
import { AlertCircle, Loader2, ChevronDown, Plus, Trash2 } from "lucide-react";
import type { Provider, ProviderSpec } from "@/types/generated/provider";
import { useFieldValidation } from "@/hooks/use-field-validation";
import { FieldError } from "@/components/ui/field-error";
import { crdConstraints } from "@/types/generated/crd-constraints";

// --- Types ---

type CredentialSource = "secret" | "envVar" | "filePath";
type ProviderRole = NonNullable<ProviderSpec["role"]>;

interface FormState {
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

type PlatformType = "bedrock" | "vertex" | "azure";

// Auth methods allowed per platform. Mirrors the CRD's CEL auth matrix.
const AUTH_BY_PLATFORM: Record<PlatformType, readonly string[]> = {
  bedrock: ["workloadIdentity", "accessKey"],
  vertex: ["workloadIdentity", "serviceAccount"],
  azure: ["workloadIdentity", "servicePrincipal"],
};

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

// Monotonic counter for stable React keys on added header rows; reset only
// on full page reload, which is fine because the dialog itself remounts via
// `formResetKey` whenever it opens.
let nextHeaderEntryId = 0;
function makeHeaderEntryId(): string {
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

/**
 * Cross-field validation rules that cannot be expressed as single-field CRD
 * constraints. Returns an error string for the Alert banner, or null.
 *
 * - Role/vendor compatibility is enforced here because VENDORS_BY_ROLE is UI
 *   state, not a single-field constraint.
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

interface ValidateProps {
  validate: (path: string, value: unknown) => void;
  errors: Record<string, string>;
}

function CredentialFields({
  form,
  updateForm,
  validate,
  errors,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
} & ValidateProps>) {
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
              aria-invalid={!!errors["spec.credential.secretRef.name"]}
              aria-describedby={errors["spec.credential.secretRef.name"] ? "cred-secret-name-error" : undefined}
              value={form.credentialSecretName}
              onChange={(e) => {
                updateForm("credentialSecretName", e.target.value);
                validate("spec.credential.secretRef.name", e.target.value);
              }}
            />
            <FieldError id="cred-secret-name-error" message={errors["spec.credential.secretRef.name"]} />
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
            aria-invalid={!!errors["spec.credential.envVar"]}
            aria-describedby={errors["spec.credential.envVar"] ? "cred-env-var-error" : undefined}
            value={form.credentialEnvVar}
            onChange={(e) => {
              updateForm("credentialEnvVar", e.target.value);
              validate("spec.credential.envVar", e.target.value);
            }}
          />
          <FieldError id="cred-env-var-error" message={errors["spec.credential.envVar"]} />
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

function HeadersFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const [open, setOpen] = useState(form.headerEntries.length > 0);

  const updateEntry = (index: number, field: "key" | "value", next: string) => {
    const entries = form.headerEntries.map((entry, i) =>
      i === index ? { ...entry, [field]: next } : entry,
    );
    updateForm("headerEntries", entries);
  };

  const addEntry = () => {
    updateForm("headerEntries", [
      ...form.headerEntries,
      { id: makeHeaderEntryId(), key: "", value: "" },
    ]);
  };

  const removeEntry = (index: number) => {
    updateForm(
      "headerEntries",
      form.headerEntries.filter((_, i) => i !== index),
    );
  };

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" className="w-full justify-between px-0 font-semibold">
          HTTP Headers
          <ChevronDown className={`h-4 w-4 transition-transform ${open ? "rotate-180" : ""}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-3 pt-2">
        <p className="text-sm text-muted-foreground">
          Custom HTTP headers sent on every provider request. Used by gateway providers
          (e.g., OpenRouter&rsquo;s <code>HTTP-Referer</code> / <code>X-Title</code>) or tenant
          routing. Collisions with built-in provider headers are rejected by PromptKit.
        </p>
        {form.headerEntries.map((entry, index) => (
          <div key={entry.id} className="flex gap-2 items-start">
            <Input
              aria-label={`Header ${index + 1} name`}
              placeholder="HTTP-Referer"
              value={entry.key}
              onChange={(e) => updateEntry(index, "key", e.target.value)}
              className="flex-1"
            />
            <Input
              aria-label={`Header ${index + 1} value`}
              placeholder="https://my-app.example.com"
              value={entry.value}
              onChange={(e) => updateEntry(index, "value", e.target.value)}
              className="flex-1"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`Remove header ${index + 1}`}
              onClick={() => removeEntry(index)}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={addEntry}>
          <Plus className="h-4 w-4 mr-1" />
          Add header
        </Button>
      </CollapsibleContent>
    </Collapsible>
  );
}

function PlatformFields({
  form,
  updateForm,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}>) {
  const authOptions =
    form.platformType ? AUTH_BY_PLATFORM[form.platformType] : [];

  // Radix Select disallows empty-string item values, so we use "none" as a
  // sentinel for "no platform" and translate at the boundary.
  const PLATFORM_NONE = "none";

  const onPlatformChange = (value: string) => {
    const next = (value === PLATFORM_NONE ? "" : value) as FormState["platformType"];
    updateForm("platformType", next);
    updateForm("platformRegion", "");
    updateForm("platformProject", "");
    updateForm("platformEndpoint", "");
    updateForm("authType", "");
    updateForm("authRoleArn", "");
    updateForm("authServiceAccountEmail", "");
    updateForm("authSecretName", "");
    updateForm("authSecretKey", "");
  };

  const onAuthTypeChange = (value: string) => {
    updateForm("authType", value as FormState["authType"]);
    updateForm("authRoleArn", "");
    updateForm("authServiceAccountEmail", "");
    updateForm("authSecretName", "");
    updateForm("authSecretKey", "");
  };

  return (
    <div className="border rounded-lg p-4 space-y-4">
      <Label className="text-base font-semibold">Hosting Platform (optional)</Label>

      <div className="space-y-2">
        <Label htmlFor="platform-type">Platform</Label>
        <Select
          value={form.platformType || PLATFORM_NONE}
          onValueChange={onPlatformChange}
        >
          <SelectTrigger id="platform-type">
            <SelectValue placeholder="None (direct API)" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={PLATFORM_NONE}>None (direct API)</SelectItem>
            <SelectItem value="bedrock">AWS Bedrock</SelectItem>
            <SelectItem value="vertex">GCP Vertex</SelectItem>
            <SelectItem value="azure">Azure AI Foundry</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {form.platformType && (
        <>
          {(form.platformType === "bedrock" || form.platformType === "vertex") && (
            <div className="space-y-2">
              <Label htmlFor="platform-region">Region</Label>
              <Input
                id="platform-region"
                placeholder={form.platformType === "bedrock" ? "us-east-1" : "us-central1"}
                value={form.platformRegion}
                onChange={(e) => updateForm("platformRegion", e.target.value)}
              />
            </div>
          )}

          {form.platformType === "vertex" && (
            <div className="space-y-2">
              <Label htmlFor="platform-project">Project</Label>
              <Input
                id="platform-project"
                placeholder="my-gcp-project"
                value={form.platformProject}
                onChange={(e) => updateForm("platformProject", e.target.value)}
              />
            </div>
          )}

          {form.platformType === "azure" && (
            <>
              <div className="space-y-2">
                <Label htmlFor="platform-endpoint">Endpoint</Label>
                <Input
                  id="platform-endpoint"
                  placeholder="https://my-resource.openai.azure.com"
                  value={form.platformEndpoint}
                  onChange={(e) => updateForm("platformEndpoint", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="platform-region">Region (optional)</Label>
                <Input
                  id="platform-region"
                  placeholder="eastus"
                  value={form.platformRegion}
                  onChange={(e) => updateForm("platformRegion", e.target.value)}
                />
              </div>
            </>
          )}

          <div className="space-y-2">
            <Label htmlFor="auth-type">Auth</Label>
            <Select value={form.authType} onValueChange={onAuthTypeChange}>
              <SelectTrigger id="auth-type">
                <SelectValue placeholder="Select auth method" />
              </SelectTrigger>
              <SelectContent>
                {authOptions.map((opt) => (
                  <SelectItem key={opt} value={opt}>
                    {opt}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {form.authType === "workloadIdentity" && form.platformType === "bedrock" && (
            <div className="space-y-2">
              <Label htmlFor="auth-role-arn">Role ARN (optional)</Label>
              <Input
                id="auth-role-arn"
                placeholder="arn:aws:iam::123456789012:role/omnia-bedrock"
                value={form.authRoleArn}
                onChange={(e) => updateForm("authRoleArn", e.target.value)}
              />
            </div>
          )}

          {form.authType === "workloadIdentity" && form.platformType === "vertex" && (
            <div className="space-y-2">
              <Label htmlFor="auth-service-account-email">Service Account Email (optional)</Label>
              <Input
                id="auth-service-account-email"
                placeholder="omnia-vertex@my-project.iam.gserviceaccount.com"
                value={form.authServiceAccountEmail}
                onChange={(e) => updateForm("authServiceAccountEmail", e.target.value)}
              />
            </div>
          )}

          {form.authType && form.authType !== "workloadIdentity" && (
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="auth-secret-name">Credentials Secret Name</Label>
                <Input
                  id="auth-secret-name"
                  placeholder="my-cloud-credentials"
                  value={form.authSecretName}
                  onChange={(e) => updateForm("authSecretName", e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="auth-secret-key">Key (optional)</Label>
                <Input
                  id="auth-secret-key"
                  placeholder=""
                  value={form.authSecretKey}
                  onChange={(e) => updateForm("authSecretKey", e.target.value)}
                />
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function TTSFields({
  form,
  updateForm,
  validate,
  errors,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
} & ValidateProps>) {
  const TTS_FORMAT_NONE = "none";
  return (
    <div className="border rounded-lg p-4 space-y-4">
      <Label className="text-base font-semibold">Text-to-Speech</Label>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="tts-voice">Voice</Label>
          <Input
            id="tts-voice"
            placeholder="alloy"
            value={form.ttsVoice}
            onChange={(e) => updateForm("ttsVoice", e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="tts-format">Format</Label>
          <Select
            value={form.ttsFormat || TTS_FORMAT_NONE}
            onValueChange={(v) =>
              updateForm("ttsFormat", (v === TTS_FORMAT_NONE ? "" : v) as FormState["ttsFormat"])
            }
          >
            <SelectTrigger id="tts-format">
              <SelectValue placeholder="Provider default" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={TTS_FORMAT_NONE}>Provider default</SelectItem>
              <SelectItem value="pcm">pcm</SelectItem>
              <SelectItem value="mp3">mp3</SelectItem>
              <SelectItem value="opus">opus</SelectItem>
              <SelectItem value="wav">wav</SelectItem>
              <SelectItem value="flac">flac</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="tts-sample-rate">Sample Rate (Hz)</Label>
          <Input
            id="tts-sample-rate"
            type="number"
            min="8000"
            placeholder="24000"
            aria-invalid={!!errors["spec.tts.sampleRate"]}
            aria-describedby={errors["spec.tts.sampleRate"] ? "tts-sample-rate-error" : undefined}
            value={form.ttsSampleRate}
            onChange={(e) => {
              updateForm("ttsSampleRate", e.target.value);
              validate("spec.tts.sampleRate", e.target.value ? Number(e.target.value) : null);
            }}
          />
          <FieldError id="tts-sample-rate-error" message={errors["spec.tts.sampleRate"]} />
        </div>
      </div>
    </div>
  );
}

function STTFields({
  form,
  updateForm,
  validate,
  errors,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
} & ValidateProps>) {
  return (
    <div className="border rounded-lg p-4 space-y-4">
      <Label className="text-base font-semibold">Speech-to-Text</Label>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="stt-language">Language (ISO-639-1)</Label>
          <Input
            id="stt-language"
            placeholder="en"
            aria-invalid={!!errors["spec.stt.language"]}
            aria-describedby={errors["spec.stt.language"] ? "stt-language-error" : undefined}
            value={form.sttLanguage}
            onChange={(e) => {
              updateForm("sttLanguage", e.target.value);
              validate("spec.stt.language", e.target.value);
            }}
          />
          <FieldError id="stt-language-error" message={errors["spec.stt.language"]} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="stt-sample-rate">Sample Rate (Hz)</Label>
          <Input
            id="stt-sample-rate"
            type="number"
            min="8000"
            placeholder="16000"
            aria-invalid={!!errors["spec.stt.sampleRate"]}
            aria-describedby={errors["spec.stt.sampleRate"] ? "stt-sample-rate-error" : undefined}
            value={form.sttSampleRate}
            onChange={(e) => {
              updateForm("sttSampleRate", e.target.value);
              validate("spec.stt.sampleRate", e.target.value ? Number(e.target.value) : null);
            }}
          />
          <FieldError id="stt-sample-rate-error" message={errors["spec.stt.sampleRate"]} />
        </div>
      </div>
    </div>
  );
}

function EmbeddingFields({
  form,
  updateForm,
  validate,
  errors,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
} & ValidateProps>) {
  const DISTANCE_NONE = "none";
  return (
    <div className="border rounded-lg p-4 space-y-4">
      <Label className="text-base font-semibold">Embedding</Label>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="embedding-dimensions">Dimensions</Label>
          <Input
            id="embedding-dimensions"
            type="number"
            min="1"
            placeholder="1536"
            aria-invalid={!!errors["spec.embedding.dimensions"]}
            aria-describedby={errors["spec.embedding.dimensions"] ? "embedding-dimensions-error" : undefined}
            value={form.embeddingDimensions}
            onChange={(e) => {
              updateForm("embeddingDimensions", e.target.value);
              validate("spec.embedding.dimensions", e.target.value ? Number(e.target.value) : null);
            }}
          />
          <FieldError id="embedding-dimensions-error" message={errors["spec.embedding.dimensions"]} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="embedding-distance">Distance metric</Label>
          <Select
            value={form.embeddingDistance || DISTANCE_NONE}
            onValueChange={(v) =>
              updateForm(
                "embeddingDistance",
                (v === DISTANCE_NONE ? "" : v) as FormState["embeddingDistance"],
              )
            }
          >
            <SelectTrigger id="embedding-distance">
              <SelectValue placeholder="Consumer chooses" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={DISTANCE_NONE}>Consumer chooses</SelectItem>
              <SelectItem value="cosine">cosine</SelectItem>
              <SelectItem value="l2">l2</SelectItem>
              <SelectItem value="dot">dot</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
    </div>
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

function buildValidateAllFields(
  formState: FormState,
  showCredential: boolean,
): Array<{ path: string; value: unknown }> {
  const fields: Array<{ path: string; value: unknown }> = [
    { path: "metadata.name", value: formState.name },
  ];

  if (showCredential) {
    if (formState.credentialSource === "secret") {
      fields.push({ path: "spec.credential.secretRef.name", value: formState.credentialSecretName });
    } else if (formState.credentialSource === "envVar") {
      fields.push({ path: "spec.credential.envVar", value: formState.credentialEnvVar });
    } else if (formState.credentialSource === "filePath") {
      fields.push({ path: "spec.credential.filePath", value: formState.credentialFilePath });
    }
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
  const [formState, setFormState] = useState<FormState>(() => getInitialFormState(provider));
  const [error, setError] = useState<string | null>(null);

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

          {showPlatform && <PlatformFields form={formState} updateForm={updateForm} />}

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
              <CredentialFields form={formState} updateForm={updateForm} validate={validate} errors={errors} />
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
    </DialogContent>
  );
}
