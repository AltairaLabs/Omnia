"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { FieldError } from "@/components/ui/field-error";
import { SecretKeySelect } from "./secret-key-select";
import { AUTH_BY_PLATFORM, type FormState, type ValidateProps } from "./provider-dialog";

// --- Platform ---

export function PlatformFields({
  form,
  updateForm,
  namespace,
}: Readonly<{
  form: FormState;
  updateForm: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
  namespace?: string;
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
            <SecretKeySelect
              idPrefix="auth"
              namespace={namespace}
              secretName={form.authSecretName}
              secretKey={form.authSecretKey}
              onSecretNameChange={(v) => updateForm("authSecretName", v)}
              onSecretKeyChange={(v) => updateForm("authSecretKey", v)}
            />
          )}
        </>
      )}
    </div>
  );
}

// --- TTS ---

export function TTSFields({
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

// --- STT ---

export function STTFields({
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

// --- Embedding ---

export function EmbeddingFields({
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
