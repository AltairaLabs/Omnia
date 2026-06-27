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
import { FieldError } from "@/components/ui/field-error";
import { cn } from "@/lib/utils";
import { usePromptPacks, useToolRegistries, useProviders } from "@/hooks/resources";
import { useReadOnly } from "@/hooks/core";
import { usePermissions, Permission } from "@/hooks/auth";
import { useDataService } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import { useFieldValidation, type FieldInput } from "@/hooks/use-field-validation";
import { crdConstraints } from "@/types/generated/crd-constraints";
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
type FacadeType = "websocket" | "grpc" | "rest";
type ContextStoreType = "memory" | "redis";
type AgentMode = "agent" | "function";

/** Default placeholder shown in the schema editors when mode=function
 * is selected. Gives authors a starting shape they can edit; it is
 * never silently submitted as-is because canProceed() rejects empty
 * `properties` to nudge the user to actually define their contract. */
const DEFAULT_INPUT_SCHEMA = `{
  "type": "object",
  "properties": {},
  "required": []
}`;
const DEFAULT_OUTPUT_SCHEMA = `{
  "type": "object",
  "properties": {},
  "required": []
}`;

interface WizardFormData {
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
  promptPackTrack: string;
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

/** isValidJsonObject returns true iff the string parses to a JSON
 * object (not an array, not a scalar). Schemas are objects; rejecting
 * arrays/scalars saves a debugging session against CEL admission. */
export function isValidJsonObject(raw: string): boolean {
  try {
    const parsed = JSON.parse(raw);
    return (
      parsed !== null &&
      typeof parsed === "object" &&
      !Array.isArray(parsed)
    );
  } catch {
    return false;
  }
}

const INITIAL_FORM_DATA: WizardFormData = {
  name: "",
  mode: "agent",
  inputSchemaJson: DEFAULT_INPUT_SCHEMA,
  outputSchemaJson: DEFAULT_OUTPUT_SCHEMA,
  framework: "promptkit",
  frameworkVersion: "",
  customImage: "",
  promptPackName: "",
  promptPackTrack: "stable",
  providerRefName: "",
  toolRegistryName: "",
  toolRegistryNamespace: "",
  contextType: "memory",
  contextTtl: "24h",
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
  { title: "Options", description: "Tools & context" },
  { title: "Runtime", description: "Resources & scaling" },
  { title: "Review", description: "Deploy agent" },
];

const FRAMEWORKS: { value: FrameworkType; label: string; description: string }[] = [
  { value: "promptkit", label: "PromptKit", description: "AltairaLabs' native framework" },
  { value: "langchain", label: "LangChain", description: "Popular Python framework" },
  { value: "autogen", label: "AutoGen", description: "Microsoft's agent framework" },
  { value: "custom", label: "Custom", description: "Your own container image" },
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

/** composeAgentYaml turns the wizard's form-data into the AgentRuntime
 * resource passed to dataService.createAgent. Pure function — no React
 * state, no hooks — so the YAML composer can be unit-tested directly.
 * Exported for tests; production code calls it via DeployWizard. */
export function composeAgentYaml(
  formData: WizardFormData,
  namespace: string | undefined,
): Record<string, unknown> {
  // Function mode serves HTTP (POST /functions/{name}); the CEL gate
  // requires facade.type 'rest'. Pin it rather than trusting facadeType.
  const facadeType: FacadeType =
    formData.mode === "function" ? "rest" : formData.facadeType;
  const yaml: Record<string, unknown> = {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: formData.name,
      namespace: namespace || "default",
    },
    spec: {
      promptPackRef: {
        name: formData.promptPackName,
        track: formData.promptPackTrack,
      },
      facade: {
        type: facadeType,
        port: formData.facadePort,
      },
    },
  };
  const spec = yaml.spec as Record<string, unknown>;

  // Function-mode fields. Only emitted when mode === "function" so
  // existing agent-mode YAMLs are byte-identical to the pre-PR output.
  if (formData.mode === "function") {
    spec.mode = "function";
    spec.inputSchema = JSON.parse(formData.inputSchemaJson);
    spec.outputSchema = JSON.parse(formData.outputSchemaJson);
  }

  if (formData.framework !== "promptkit" || formData.customImage) {
    spec.framework = {
      type: formData.framework,
      ...(formData.frameworkVersion && { version: formData.frameworkVersion }),
      ...(formData.customImage && { image: formData.customImage }),
    };
  }

  if (formData.providerRefName) {
    spec.providers = [
      { name: "default", providerRef: { name: formData.providerRefName } },
    ];
  }

  if (formData.toolRegistryName) {
    spec.toolRegistryRef = {
      name: formData.toolRegistryName,
      ...(formData.toolRegistryNamespace && {
        namespace: formData.toolRegistryNamespace,
      }),
    };
  }

  if (formData.contextType !== "memory" || formData.contextTtl !== "24h") {
    spec.context = { type: formData.contextType, ttl: formData.contextTtl };
  }

  const runtime: Record<string, unknown> = {};
  if (formData.replicas !== 1) {
    runtime.replicas = formData.replicas;
  }
  if (formData.cpuRequest !== "100m" || formData.memoryRequest !== "128Mi") {
    runtime.resources = {
      requests: { cpu: formData.cpuRequest, memory: formData.memoryRequest },
      limits: { cpu: formData.cpuLimit, memory: formData.memoryLimit },
    };
  }
  if (Object.keys(runtime).length > 0) {
    spec.runtime = runtime;
  }

  return yaml;
}

interface BasicInfoStepProps {
  formData: WizardFormData;
  currentWorkspace: { name?: string; namespace?: string; displayName?: string } | null;
  updateField: <K extends keyof WizardFormData>(field: K, value: WizardFormData[K]) => void;
  errors?: Record<string, string>;
  validate?: (path: string, value: unknown) => void;
}

/** BasicInfoStep is step 0 of the wizard. Pulled out so renderStep
 * stays below SonarCloud's cognitive-complexity cap with the new
 * mode toggle + schema editor branches. */
export function BasicInfoStep({
  formData,
  currentWorkspace,
  updateField,
  errors = {},
  validate = () => {},
}: Readonly<BasicInfoStepProps>) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="name">Agent Name</Label>
        <Input
          id="name"
          value={formData.name}
          aria-invalid={!!errors["metadata.name"]}
          aria-describedby={errors["metadata.name"] ? "agent-name-error" : undefined}
          onChange={(e) => {
            const v = e.target.value.toLowerCase().replaceAll(/[^a-z0-9-]/g, "-");
            updateField("name", v);
            validate("metadata.name", v);
          }}
          placeholder="my-agent"
        />
        <p className="text-xs text-muted-foreground">
          Lowercase letters, numbers, and hyphens only
        </p>
        <FieldError id="agent-name-error" message={errors["metadata.name"]} />
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
      <div className="space-y-2">
        <Label>Mode</Label>
        <RadioGroup
          value={formData.mode}
          onValueChange={(v) => updateField("mode", v as AgentMode)}
          className="grid grid-cols-1 gap-2"
        >
          <ModeRadioOption
            id="mode-agent"
            value="agent"
            label="Agent"
            description="Long-lived conversational runtime over WebSocket."
            selected={formData.mode === "agent"}
          />
          <ModeRadioOption
            id="mode-function"
            value="function"
            label="Function"
            description="One-shot HTTP invocation with structured input + output schemas."
            selected={formData.mode === "function"}
          />
        </RadioGroup>
      </div>
      {formData.mode === "function" && (
        <FunctionSchemaEditors
          inputSchemaJson={formData.inputSchemaJson}
          outputSchemaJson={formData.outputSchemaJson}
          onChangeInputSchema={(v) => updateField("inputSchemaJson", v)}
          onChangeOutputSchema={(v) => updateField("outputSchemaJson", v)}
        />
      )}
    </div>
  );
}

interface ModeRadioOptionProps {
  id: string;
  value: AgentMode;
  label: string;
  description: string;
  selected: boolean;
}

function ModeRadioOption({ id, value, label, description, selected }: Readonly<ModeRadioOptionProps>) {
  return (
    <label
      htmlFor={id}
      className={cn(
        "flex items-center space-x-3 rounded-lg border p-3 cursor-pointer transition-colors",
        selected ? "border-primary bg-primary/5" : "hover:bg-muted/50",
      )}
    >
      <RadioGroupItem value={value} id={id} />
      <div className="flex-1">
        <span className="cursor-pointer font-medium">{label}</span>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
    </label>
  );
}

/** FunctionSchemaEditors renders the input + output JSON-Schema
 * editors shown when the wizard's mode is "function". Extracted from
 * renderStep so the parent's switch stays under SonarCloud's
 * cognitive-complexity cap. */
interface FunctionSchemaEditorsProps {
  inputSchemaJson: string;
  outputSchemaJson: string;
  onChangeInputSchema: (v: string) => void;
  onChangeOutputSchema: (v: string) => void;
}

export function FunctionSchemaEditors({
  inputSchemaJson,
  outputSchemaJson,
  onChangeInputSchema,
  onChangeOutputSchema,
}: Readonly<FunctionSchemaEditorsProps>) {
  const inputValid = isValidJsonObject(inputSchemaJson);
  const outputValid = isValidJsonObject(outputSchemaJson);
  return (
    <div className="space-y-4 border-t pt-4" data-testid="function-schemas">
      <SchemaEditor
        id="inputSchemaJson"
        label="Input schema (JSON Schema)"
        value={inputSchemaJson}
        valid={inputValid}
        errorTestId="input-schema-error"
        errorMessage="Input schema must be a valid JSON object."
        onChange={onChangeInputSchema}
      />
      <SchemaEditor
        id="outputSchemaJson"
        label="Output schema (JSON Schema)"
        value={outputSchemaJson}
        valid={outputValid}
        errorTestId="output-schema-error"
        errorMessage="Output schema must be a valid JSON object."
        onChange={onChangeOutputSchema}
      />
    </div>
  );
}

interface SchemaEditorProps {
  id: string;
  label: string;
  value: string;
  valid: boolean;
  errorTestId: string;
  errorMessage: string;
  onChange: (v: string) => void;
}

function SchemaEditor({
  id,
  label,
  value,
  valid,
  errorTestId,
  errorMessage,
  onChange,
}: Readonly<SchemaEditorProps>) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <textarea
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={6}
        className={cn(
          "w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs",
          !valid && "border-destructive",
        )}
      />
      {!valid && (
        <p className="text-xs text-destructive" data-testid={errorTestId}>
          {errorMessage}
        </p>
      )}
    </div>
  );
}

interface RuntimeStepProps {
  formData: WizardFormData;
  updateField: <K extends keyof WizardFormData>(field: K, value: WizardFormData[K]) => void;
  errors: Record<string, string>;
  validate: (path: string, value: unknown) => void;
}

/** RuntimeStep is step 5 of the wizard. Extracted from renderStep to keep
 * renderStep below SonarCloud's cognitive-complexity cap. */
function RuntimeStep({ formData, updateField, errors, validate }: Readonly<RuntimeStepProps>) {
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

/** Fields validated at step 0 (Basic Info). */
function step0Fields(formData: WizardFormData): FieldInput[] {
  return [{ path: "metadata.name", value: formData.name }];
}

/** Fields validated at step 5 (Runtime). */
function step5Fields(formData: WizardFormData): FieldInput[] {
  return [
    { path: "spec.facade.port", value: formData.facadePort },
    { path: "spec.facade.type", value: formData.facadeType },
  ];
}

/** All constrained fields — used for final submit validation. */
function allConstrainedFields(formData: WizardFormData): FieldInput[] {
  return [...step0Fields(formData), ...step5Fields(formData)];
}

/**
 * Returns the constrained fields for the currently visible step.
 * Used to scope the Next-button disabled check to the active step only,
 * preventing stale errors from a different step from blocking navigation.
 */
function currentStepFields(step: number, formData: WizardFormData): FieldInput[] {
  if (step === 0) return step0Fields(formData);
  if (step === 5) return step5Fields(formData);
  return [];
}

export function DeployWizard({ open, onOpenChange }: Readonly<DeployWizardProps>) {
  const [step, setStep] = useState(0);
  const [formData, setFormData] = useState<WizardFormData>(INITIAL_FORM_DATA);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const { errors, validate, validateAll } = useFieldValidation(crdConstraints.AgentRuntime);

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
  // Get Providers from current workspace (only Ready ones)
  const { data: providers } = useProviders({ phase: "Ready" });

  const updateField = useCallback(<K extends keyof WizardFormData>(
    field: K,
    value: WizardFormData[K]
  ) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  }, []);

  const handleNext = useCallback(() => {
    let valid = true;
    if (step === 0) valid = validateAll(step0Fields(formData));
    if (step === 5) valid = validateAll(step5Fields(formData));
    if (valid) setStep((s) => s + 1);
  }, [step, formData, validateAll]);

  const canProceed = useCallback(() => {
    switch (step) {
      case 0: // Basic Info
        if (formData.name.length === 0 || !/^[a-z0-9-]+$/.test(formData.name)) {
          return false;
        }
        // Function-mode requires both schemas to be valid JSON objects.
        // CEL on the CRD enforces the same; failing here saves the
        // operator a round-trip to a 4xx admission rejection.
        if (formData.mode === "function") {
          return (
            isValidJsonObject(formData.inputSchemaJson) &&
            isValidJsonObject(formData.outputSchemaJson)
          );
        }
        return true;
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

  // Scope error-gating to only the fields on the current step so that
  // stale errors from a different step cannot block navigation.
  const currentStepHasErrors = currentStepFields(step, formData).some(
    (f) => !!errors[f.path],
  );

  const generateYaml = useCallback(
    () => composeAgentYaml(formData, currentWorkspace?.namespace),
    [formData, currentWorkspace],
  );

  const handleSubmit = async () => {
    if (!validateAll(allConstrainedFields(formData))) return;

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
          <BasicInfoStep
            formData={formData}
            currentWorkspace={currentWorkspace}
            updateField={updateField}
            errors={errors}
            validate={validate}
          />
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
                <p className="text-xs text-amber-600 dark:text-amber-400">
                  No Providers configured. Create a Provider resource first.
                </p>
              )}
            </div>
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

      case 5:
        return (
          <RuntimeStep
            formData={formData}
            updateField={updateField}
            errors={errors}
            validate={validate}
          />
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
                    {formData.providerRefName ? (
                      (() => {
                        const selectedProvider = providers?.find(p => p.metadata.name === formData.providerRefName);
                        const providerType = selectedProvider?.spec.type ?? "";
                        const providerModel = selectedProvider?.spec.model;
                        const modelSuffix = providerModel ? " / " + providerModel : "";
                        return selectedProvider
                          ? formData.providerRefName + " (" + providerType + modelSuffix + ")"
                          : formData.providerRefName;
                      })()
                    ) : (
                      <span className="text-muted-foreground italic">None selected</span>
                    )}
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
              onClick={handleNext}
              disabled={!canProceed() || currentStepHasErrors}
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
