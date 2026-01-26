/**
 * Arena Template System types - matches ee/api/v1alpha1/arenatemplatesource_types.go
 *
 * The template system enables users to:
 * - Browse templates from Git/OCI sources
 * - Fill in variables via a wizard
 * - Create new projects from templates
 */

import { ObjectMeta, Condition } from "./common";
import { GitSourceSpec, OCISourceSpec, ConfigMapSourceSpec, Artifact } from "./arena";

// =============================================================================
// Template Variable Types
// =============================================================================

/** Variable types supported in templates */
export type TemplateVariableType = "string" | "number" | "boolean" | "enum";

/** Variable value type - can be string, number, or boolean */
export type TemplateVariableValue = string | number | boolean;

/** Template variable definition */
export interface TemplateVariable {
  /** Variable name used in templates */
  name: string;
  /** Variable type */
  type: TemplateVariableType;
  /** Description of the variable */
  description?: string;
  /** Whether the variable is required */
  required?: boolean;
  /** Default value (as string) */
  default?: string;
  /** Regex pattern for string validation */
  pattern?: string;
  /** Allowed values for enum type */
  options?: string[];
  /** Minimum value for number type (as string) */
  min?: string;
  /** Maximum value for number type (as string) */
  max?: string;
}

/** Template file specification */
export interface TemplateFileSpec {
  /** Path to the file or directory */
  path: string;
  /** Whether to apply Go template rendering */
  render?: boolean;
}

// =============================================================================
// Template Metadata
// =============================================================================

/** Template metadata from template.yaml */
export interface TemplateMetadata {
  /** Unique template name */
  name: string;
  /** Semantic version */
  version?: string;
  /** Human-readable display name */
  displayName?: string;
  /** Template description */
  description?: string;
  /** Category (e.g., chatbot, agent, assistant) */
  category?: string;
  /** Searchable tags */
  tags?: string[];
  /** Configurable variables */
  variables?: TemplateVariable[];
  /** Files to include and how to process them */
  files?: TemplateFileSpec[];
  /** Path within the source */
  path: string;
}

// =============================================================================
// ArenaTemplateSource CRD
// =============================================================================

/** Source type for template sources */
export type ArenaTemplateSourceType = "git" | "oci" | "configmap";

/** Phase of the template source */
export type ArenaTemplateSourcePhase =
  | "Pending"
  | "Ready"
  | "Fetching"
  | "Scanning"
  | "Error";

/** ArenaTemplateSource specification */
export interface ArenaTemplateSourceSpec {
  /** Source type */
  type: ArenaTemplateSourceType;
  /** Git source configuration */
  git?: GitSourceSpec;
  /** OCI registry configuration */
  oci?: OCISourceSpec;
  /** ConfigMap configuration */
  configMap?: ConfigMapSourceSpec;
  /** Sync interval (e.g., "1h", "30m") */
  syncInterval?: string;
  /** Suspend sync operations */
  suspend?: boolean;
  /** Timeout for fetch operations */
  timeout?: string;
  /** Path to templates within source */
  templatesPath?: string;
}

/** ArenaTemplateSource status */
export interface ArenaTemplateSourceStatus {
  /** Current phase */
  phase?: ArenaTemplateSourcePhase;
  /** Standard conditions */
  conditions?: Condition[];
  /** Observed generation */
  observedGeneration?: number;
  /** Number of templates discovered */
  templateCount?: number;
  /** Discovered template metadata */
  templates?: TemplateMetadata[];
  /** Last fetch timestamp */
  lastFetchTime?: string;
  /** Next scheduled fetch */
  nextFetchTime?: string;
  /** Current head version */
  headVersion?: string;
  /** Artifact info */
  artifact?: Artifact;
  /** Status message */
  message?: string;
}

/** ArenaTemplateSource resource */
export interface ArenaTemplateSource {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "ArenaTemplateSource";
  metadata: ObjectMeta;
  spec: ArenaTemplateSourceSpec;
  status?: ArenaTemplateSourceStatus;
}

// =============================================================================
// Template Rendering
// =============================================================================

/** Input for template rendering */
export interface TemplateRenderInput {
  /** Variable values keyed by variable name */
  variables: Record<string, TemplateVariableValue>;
  /** Project name for the rendered output */
  projectName: string;
  /** Optional project description */
  projectDescription?: string;
  /** Optional project tags */
  projectTags?: string[];
}

/** Single rendered file */
export interface RenderedFile {
  /** File path */
  path: string;
  /** File content */
  content: string;
}

/** Output from template rendering */
export interface TemplateRenderOutput {
  /** Rendered files */
  files: RenderedFile[];
  /** Project ID if created */
  projectId?: string;
  /** Any warnings during rendering */
  warnings?: string[];
}

/** Validation error for a variable */
export interface VariableValidationError {
  /** Variable name */
  variable: string;
  /** Error message */
  message: string;
}

// =============================================================================
// API Response Types
// =============================================================================

/** List response for template sources */
export interface TemplateSourceListResponse {
  sources: ArenaTemplateSource[];
}

/** List response for templates within a source */
export interface TemplateListResponse {
  templates: TemplateMetadata[];
  sourcePhase: ArenaTemplateSourcePhase;
}

/** Template detail response */
export interface TemplateDetailResponse {
  template: TemplateMetadata;
  sourceName: string;
  sourcePhase: ArenaTemplateSourcePhase;
}

/** Template render preview response */
export interface TemplatePreviewResponse {
  files: RenderedFile[];
  errors?: VariableValidationError[];
}

// =============================================================================
// UI State Types
// =============================================================================

/** Template wizard step */
export type TemplateWizardStep =
  | "source-select"
  | "template-select"
  | "variables"
  | "preview"
  | "create";

/** Template wizard state */
export interface TemplateWizardState {
  /** Current step */
  step: TemplateWizardStep;
  /** Selected source name */
  selectedSource?: string;
  /** Selected template name */
  selectedTemplate?: string;
  /** Variable values */
  variables: Record<string, TemplateVariableValue>;
  /** Project name */
  projectName: string;
  /** Project description */
  projectDescription?: string;
  /** Project tags */
  projectTags: string[];
  /** Rendered preview files */
  previewFiles?: RenderedFile[];
  /** Validation errors */
  validationErrors: VariableValidationError[];
  /** Loading state */
  loading: boolean;
  /** Error message */
  error?: string;
}

/** Template filter options */
export interface TemplateFilterOptions {
  /** Filter by category */
  category?: string;
  /** Filter by tags */
  tags?: string[];
  /** Search query */
  search?: string;
}

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Get display name for a template (falls back to name)
 */
export function getTemplateDisplayName(template: TemplateMetadata): string {
  return template.displayName || template.name;
}

/**
 * Get default values for template variables
 */
export function getDefaultVariableValues(
  variables: TemplateVariable[]
): Record<string, TemplateVariableValue> {
  const values: Record<string, TemplateVariableValue> = {};

  for (const v of variables) {
    if (v.default !== undefined) {
      switch (v.type) {
        case "number":
          values[v.name] = parseFloat(v.default) || 0;
          break;
        case "boolean":
          values[v.name] = v.default === "true";
          break;
        default:
          values[v.name] = v.default;
      }
    }
  }

  return values;
}

/**
 * Validate variable values against their definitions
 */
// eslint-disable-next-line sonarjs/cognitive-complexity
export function validateVariables(
  variables: TemplateVariable[],
  values: Record<string, TemplateVariableValue>
): VariableValidationError[] {
  const errors: VariableValidationError[] = [];

  for (const v of variables) {
    const value = values[v.name];

    // Check required
    if (v.required && (value === undefined || value === "")) {
      errors.push({
        variable: v.name,
        message: `${v.name} is required`,
      });
      continue;
    }

    if (value === undefined || value === "") {
      continue;
    }

    // Type-specific validation
    switch (v.type) {
      case "string":
        if (v.pattern) {
          const regex = new RegExp(v.pattern);
          if (!regex.test(String(value))) {
            errors.push({
              variable: v.name,
              message: `${v.name} does not match pattern: ${v.pattern}`,
            });
          }
        }
        break;

      case "number":
        const numValue = typeof value === "number" ? value : parseFloat(String(value));
        if (isNaN(numValue)) {
          errors.push({
            variable: v.name,
            message: `${v.name} must be a number`,
          });
        } else {
          if (v.min !== undefined && v.min !== "") {
            const minVal = parseFloat(v.min);
            if (!isNaN(minVal) && numValue < minVal) {
              errors.push({
                variable: v.name,
                message: `${v.name} must be >= ${v.min}`,
              });
            }
          }
          if (v.max !== undefined && v.max !== "") {
            const maxVal = parseFloat(v.max);
            if (!isNaN(maxVal) && numValue > maxVal) {
              errors.push({
                variable: v.name,
                message: `${v.name} must be <= ${v.max}`,
              });
            }
          }
        }
        break;

      case "enum":
        if (v.options && !v.options.includes(String(value))) {
          errors.push({
            variable: v.name,
            message: `${v.name} must be one of: ${v.options.join(", ")}`,
          });
        }
        break;
    }
  }

  return errors;
}

/**
 * Get unique categories from templates
 */
export function getTemplateCategories(templates: TemplateMetadata[]): string[] {
  const categories = new Set<string>();
  for (const t of templates) {
    if (t.category) {
      categories.add(t.category);
    }
  }
  return Array.from(categories).sort();
}

/**
 * Get unique tags from templates
 */
export function getTemplateTags(templates: TemplateMetadata[]): string[] {
  const tags = new Set<string>();
  for (const t of templates) {
    for (const tag of t.tags || []) {
      tags.add(tag);
    }
  }
  return Array.from(tags).sort();
}

/**
 * Filter templates by options
 */
export function filterTemplates(
  templates: TemplateMetadata[],
  options: TemplateFilterOptions
): TemplateMetadata[] {
  let filtered = templates;

  if (options.category) {
    filtered = filtered.filter(
      (t) => t.category?.toLowerCase() === options.category?.toLowerCase()
    );
  }

  if (options.tags && options.tags.length > 0) {
    const tagSet = new Set(options.tags.map((t) => t.toLowerCase()));
    filtered = filtered.filter((t) =>
      (t.tags || []).some((tag) => tagSet.has(tag.toLowerCase()))
    );
  }

  if (options.search) {
    const query = options.search.toLowerCase();
    filtered = filtered.filter(
      (t) =>
        t.name.toLowerCase().includes(query) ||
        t.displayName?.toLowerCase().includes(query) ||
        t.description?.toLowerCase().includes(query)
    );
  }

  return filtered;
}
