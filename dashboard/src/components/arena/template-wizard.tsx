"use client";

import { useState, useCallback, useMemo } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Progress } from "@/components/ui/progress";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { TemplatePreview } from "./template-preview";
import {
  ChevronLeft,
  ChevronRight,
  Rocket,
  Loader2,
  Check,
  AlertCircle,
  FileCode,
  Settings,
  Eye,
} from "lucide-react";
import {
  validateVariables,
  getDefaultVariableValues,
  type TemplateMetadata,
  type TemplateVariable,
  type TemplateRenderInput,
  type RenderedFile,
  type VariableValidationError,
} from "@/types/arena-template";

// =============================================================================
// Types
// =============================================================================

type VariableValue = string | number | boolean;

const STEPS = [
  { title: "Variables", description: "Configure template variables", icon: Settings },
  { title: "Preview", description: "Review rendered output", icon: Eye },
  { title: "Create", description: "Create project", icon: Rocket },
];

interface WizardFormState {
  variables: Record<string, VariableValue>;
  projectName: string;
  projectDescription: string;
  projectTags: string[];
}

export interface TemplateWizardProps {
  readonly template: TemplateMetadata;
  readonly sourceName: string;
  readonly loading?: boolean;
  /** Available providers for provider bindings */
  readonly providers?: ProviderOption[];
  readonly onPreview?: (input: Omit<TemplateRenderInput, "projectName">) => Promise<{ files: RenderedFile[]; errors?: VariableValidationError[] }>;
  readonly onSubmit: (input: TemplateRenderInput) => Promise<{ projectId?: string }>;
  readonly onSuccess?: (projectId: string) => void;
  readonly onClose?: () => void;
  readonly className?: string;
}

// =============================================================================
// Helpers
// =============================================================================

function getInitialFormState(template: TemplateMetadata): WizardFormState {
  // Check if there's a project-bound variable with a default value to use as project name
  const projectNameVar = template.variables?.find(
    (v) => v.binding?.kind === "project" && v.binding?.field === "name" && v.binding?.autoPopulate
  );
  const initialProjectName = projectNameVar?.default || "";

  return {
    variables: getDefaultVariableValues(template.variables || []),
    projectName: initialProjectName,
    projectDescription: template.description || "",
    projectTags: [],
  };
}

interface StepValidationResult {
  valid: boolean;
  message?: string;
  errorMap?: Record<string, string>;
}

function validateProjectName(projectName: string): StepValidationResult | null {
  if (!projectName.trim()) {
    return { valid: false, message: "Project name is required" };
  }
  if (!/^[a-z][a-z0-9-]*$/.test(projectName)) {
    return {
      valid: false,
      message: "Project name must start with a letter and contain only lowercase letters, numbers, and hyphens",
    };
  }
  return null;
}

function validateStep0(
  template: TemplateMetadata,
  form: WizardFormState,
  visibleVariables: TemplateVariable[]
): StepValidationResult {
  const errors = validateVariables(template.variables || [], form.variables);
  const errorMap: Record<string, string> = {};
  for (const err of errors) {
    errorMap[err.variable] = err.message;
  }

  const projectNameError = validateProjectName(form.projectName);
  if (projectNameError) {
    return projectNameError;
  }

  // Check required provider-bound variables have a selection
  for (const v of visibleVariables) {
    if (v.binding?.kind === "provider" && v.required) {
      const value = form.variables[v.name];
      if (!value || value === "") {
        errorMap[v.name] = `${v.name} is required - please select a provider`;
      }
    }
  }

  if (errors.length > 0 || Object.keys(errorMap).length > 0) {
    return { valid: false, message: "Please fix variable errors", errorMap };
  }
  return { valid: true, errorMap };
}

// =============================================================================
// Variable Input Component
// =============================================================================

export interface ProviderOption {
  name: string;
  model: string;
  displayName?: string;
}

interface VariableInputProps {
  readonly variable: TemplateVariable;
  readonly value: string | number | boolean | undefined;
  readonly onChange: (value: string | number | boolean) => void;
  readonly error?: string;
  /** Available providers for provider binding */
  readonly providers?: ProviderOption[];
}

function VariableInput({ variable, value, onChange, error, providers }: VariableInputProps) {
  const id = `var-${variable.name}`;
  const binding = variable.binding;

  // For provider bindings, render a provider selector
  if (binding?.kind === "provider" && providers && providers.length > 0) {
    return (
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <Label htmlFor={id} className="text-sm font-medium">
            {variable.name}
            {variable.required && <span className="text-destructive ml-1">*</span>}
          </Label>
          <Badge variant="outline" className="text-xs">Provider</Badge>
        </div>

        {variable.description && (
          <p className="text-xs text-muted-foreground">{variable.description}</p>
        )}

        <Select
          value={String(value || "")}
          onValueChange={(v) => onChange(v)}
        >
          <SelectTrigger id={id}>
            <SelectValue placeholder="Select a provider model" />
          </SelectTrigger>
          <SelectContent>
            {providers.map((provider) => (
              <SelectItem key={`${provider.name}-${provider.model}`} value={provider.model}>
                {provider.displayName || provider.name} ({provider.model})
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {error && (
          <p className="text-xs text-destructive">{error}</p>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label htmlFor={id} className="text-sm font-medium">
          {variable.name}
          {variable.required && <span className="text-destructive ml-1">*</span>}
        </Label>
        {variable.type !== "boolean" && variable.default && (
          <span className="text-xs text-muted-foreground">
            Default: {variable.default}
          </span>
        )}
      </div>

      {variable.description && (
        <p className="text-xs text-muted-foreground">{variable.description}</p>
      )}

      {variable.type === "boolean" ? (
        <Switch
          id={id}
          checked={value === true || value === "true"}
          onCheckedChange={(checked) => onChange(checked)}
        />
      ) : variable.type === "enum" && variable.options ? (
        <Select
          value={String(value || "")}
          onValueChange={(v) => onChange(v)}
        >
          <SelectTrigger id={id}>
            <SelectValue placeholder="Select an option" />
          </SelectTrigger>
          <SelectContent>
            {variable.options.map((option) => (
              <SelectItem key={option} value={option}>
                {option}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : variable.type === "number" ? (
        <Input
          id={id}
          type="number"
          value={value === undefined ? "" : String(value)}
          onChange={(e) => {
            const num = parseFloat(e.target.value);
            onChange(isNaN(num) ? e.target.value : num);
          }}
          min={variable.min}
          max={variable.max}
          placeholder={variable.default || "Enter a number"}
        />
      ) : (
        <Input
          id={id}
          type="text"
          value={String(value || "")}
          onChange={(e) => onChange(e.target.value)}
          placeholder={variable.default || `Enter ${variable.name}`}
        />
      )}

      {error && (
        <p className="text-xs text-destructive">{error}</p>
      )}
    </div>
  );
}

// =============================================================================
// Main Wizard Component
// =============================================================================

export function TemplateWizard({
  template,
  sourceName,
  loading,
  providers = [],
  onPreview,
  onSubmit,
  onSuccess,
  onClose,
  className,
}: TemplateWizardProps) {
  const [currentStep, setCurrentStep] = useState(0);
  const [form, setForm] = useState<WizardFormState>(() =>
    getInitialFormState(template)
  );
  const [previewFiles, setPreviewFiles] = useState<RenderedFile[]>([]);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [validationErrors, setValidationErrors] = useState<
    Record<string, string>
  >({});

  // Filter out auto-populated project variables (they're synced with projectName)
  const visibleVariables = useMemo(() => {
    if (!template.variables) return [];
    return template.variables.filter(
      (v) => !(v.binding?.kind === "project" && v.binding?.autoPopulate)
    );
  }, [template.variables]);

  const hasVariables = visibleVariables.length > 0;
  const displayName = template.displayName || template.name;

  // Validate current step
  const stepValidation = useMemo((): StepValidationResult => {
    if (currentStep === 0) {
      return validateStep0(template, form, visibleVariables);
    }
    if (currentStep === 1) {
      // Preview step - always valid after preview is loaded
      return { valid: previewFiles.length > 0 };
    }
    return { valid: true };
  }, [currentStep, form, template, visibleVariables, previewFiles]);

  const progress = ((currentStep + 1) / STEPS.length) * 100;

  const handleVariableChange = useCallback(
    (name: string, value: string | number | boolean) => {
      setForm((prev) => ({
        ...prev,
        variables: { ...prev.variables, [name]: value },
      }));
      setValidationErrors((prev) => {
        const newErrors = { ...prev };
        delete newErrors[name];
        return newErrors;
      });
    },
    []
  );

  // eslint-disable-next-line sonarjs/cognitive-complexity
  const handleNext = useCallback(async () => {
    setError(null);

    if (currentStep === 0) {
      // Validate before moving to preview
      if (!stepValidation.valid) {
        if (stepValidation.errorMap) {
          setValidationErrors(stepValidation.errorMap);
        }
        setError(stepValidation.message || "Please fix errors");
        return;
      }

      // Fetch preview
      if (onPreview) {
        setPreviewLoading(true);
        try {
          const result = await onPreview({
            variables: {
              ...form.variables,
              projectName: form.projectName,
            },
          });
          setPreviewFiles(result.files);
          if (result.errors && result.errors.length > 0) {
            const errorMap: Record<string, string> = {};
            for (const err of result.errors) {
              errorMap[err.variable] = err.message;
            }
            setValidationErrors(errorMap);
          }
        } catch (err) {
          setError(err instanceof Error ? err.message : "Failed to generate preview");
          return;
        } finally {
          setPreviewLoading(false);
        }
      }
    }

    setCurrentStep((prev) => Math.min(prev + 1, STEPS.length - 1));
  }, [currentStep, form, onPreview, stepValidation]);

  const handleBack = useCallback(() => {
    setCurrentStep((prev) => Math.max(prev - 1, 0));
    setError(null);
  }, []);

  const handleSubmit = useCallback(async () => {
    setError(null);
    setSubmitLoading(true);

    try {
      const input: TemplateRenderInput = {
        variables: {
          ...form.variables,
          projectName: form.projectName,
        },
        projectName: form.projectName,
        projectDescription: form.projectDescription || undefined,
        projectTags: form.projectTags.length > 0 ? form.projectTags : undefined,
      };

      const result = await onSubmit(input);
      if (result.projectId) {
        onSuccess?.(result.projectId);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create project");
    } finally {
      setSubmitLoading(false);
    }
  }, [form, onSubmit, onSuccess]);

  return (
    <div className={cn("space-y-4", className)}>
      {/* Header */}
      <div className="flex-shrink-0">
        <div className="flex items-center gap-2 mb-2">
          <Badge variant="outline">{sourceName}</Badge>
          {template.version && (
            <Badge variant="secondary">v{template.version}</Badge>
          )}
        </div>
        <h2 className="text-xl font-semibold">{displayName}</h2>
        {template.description && (
          <p className="text-sm text-muted-foreground mt-1">
            {template.description}
          </p>
        )}
      </div>

      {/* Progress */}
      <div className="space-y-2 flex-shrink-0">
        <div className="flex items-center justify-between text-sm">
          <span className="font-medium">{STEPS[currentStep].title}</span>
          <span className="text-muted-foreground">
            Step {currentStep + 1} of {STEPS.length}
          </span>
        </div>
        <Progress value={progress} className="h-2" />
        <div className="flex justify-between">
          {STEPS.map((step, i) => {
            const Icon = step.icon;
            return (
              <div
                key={step.title}
                className={cn(
                  "flex items-center gap-1 text-xs",
                  i <= currentStep ? "text-primary" : "text-muted-foreground"
                )}
              >
                {i < currentStep ? (
                  <Check className="h-3 w-3" />
                ) : (
                  <Icon className="h-3 w-3" />
                )}
                <span className="hidden sm:inline">{step.title}</span>
              </div>
            );
          })}
        </div>
      </div>

      {/* Error */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {/* Step content */}
      <div className="flex-1 overflow-y-auto min-h-0">
        {currentStep === 0 && (
          <div className="space-y-6">
            {/* Project name (always required) */}
            <div className="space-y-2">
              <Label htmlFor="projectName" className="text-sm font-medium">
                Project Name <span className="text-destructive">*</span>
              </Label>
              <Input
                id="projectName"
                value={form.projectName}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, projectName: e.target.value }))
                }
                placeholder="my-project"
              />
              <p className="text-xs text-muted-foreground">
                Lowercase letters, numbers, and hyphens only
              </p>
            </div>

            {/* Project description */}
            <div className="space-y-2">
              <Label htmlFor="projectDescription" className="text-sm font-medium">
                Description
              </Label>
              <Textarea
                id="projectDescription"
                value={form.projectDescription}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, projectDescription: e.target.value }))
                }
                placeholder="Describe your project..."
                rows={2}
              />
            </div>

            {/* Template variables */}
            {hasVariables && (
              <div className="space-y-4">
                <div className="flex items-center gap-2">
                  <FileCode className="h-4 w-4" />
                  <h3 className="font-medium">Template Variables</h3>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  {visibleVariables.map((variable) => {
                    const isProviderBinding = variable.binding?.kind === "provider";
                    return (
                      <VariableInput
                        key={variable.name}
                        variable={variable}
                        value={form.variables[variable.name]}
                        onChange={(value) => handleVariableChange(variable.name, value)}
                        error={validationErrors[variable.name]}
                        providers={isProviderBinding ? providers : undefined}
                      />
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}

        {currentStep === 1 && (
          <div className="space-y-4">
            <h3 className="font-medium flex items-center gap-2">
              <Eye className="h-4 w-4" />
              Preview
            </h3>
            {previewLoading ? (
              <div className="flex items-center justify-center py-12">
                <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
              </div>
            ) : (
              <TemplatePreview files={previewFiles} />
            )}
          </div>
        )}

        {currentStep === 2 && (
          <div className="space-y-4">
            <h3 className="font-medium flex items-center gap-2">
              <Rocket className="h-4 w-4" />
              Ready to Create
            </h3>
            <div className="rounded-lg border p-4 space-y-3">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Project Name</span>
                <span className="font-medium">{form.projectName}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Template</span>
                <span className="font-medium">{displayName}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Source</span>
                <span className="font-medium">{sourceName}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Files</span>
                <span className="font-medium">{previewFiles.length} files</span>
              </div>
            </div>
            <p className="text-sm text-muted-foreground">
              Click Create to generate your project from the template.
            </p>
          </div>
        )}
      </div>

      {/* Navigation */}
      <div className="flex items-center justify-between pt-4 border-t flex-shrink-0">
        <Button
          variant="outline"
          onClick={currentStep === 0 ? onClose : handleBack}
          disabled={loading || submitLoading}
        >
          <ChevronLeft className="h-4 w-4 mr-1" />
          {currentStep === 0 ? "Cancel" : "Back"}
        </Button>

        {currentStep < STEPS.length - 1 ? (
          <Button
            onClick={handleNext}
            disabled={loading || previewLoading || !stepValidation.valid}
          >
            {previewLoading ? (
              <Loader2 className="h-4 w-4 mr-1 animate-spin" />
            ) : (
              <ChevronRight className="h-4 w-4 mr-1" />
            )}
            Next
          </Button>
        ) : (
          <Button
            onClick={handleSubmit}
            disabled={loading || submitLoading}
          >
            {submitLoading ? (
              <Loader2 className="h-4 w-4 mr-1 animate-spin" />
            ) : (
              <Rocket className="h-4 w-4 mr-1" />
            )}
            Create Project
          </Button>
        )}
      </div>
    </div>
  );
}
