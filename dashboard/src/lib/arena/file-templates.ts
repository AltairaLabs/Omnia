/**
 * Arena file templates for creating new typed files.
 * Each file type has a template with starter content.
 */

export type ArenaFileKind = "prompt" | "provider" | "scenario" | "tool" | "persona";

export interface ArenaFileTypeConfig {
  kind: ArenaFileKind;
  label: string;
  extension: string;
  template: string;
}

/**
 * Configuration for each Arena file type.
 */
export const ARENA_FILE_TYPES: ArenaFileTypeConfig[] = [
  {
    kind: "prompt",
    label: "Prompt",
    extension: ".prompt.yaml",
    template: `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: PromptConfig

metadata:
  name: {{name}}

spec:
  task_type: general
  version: "1.0.0"
  description: ""
  system_template: |
    You are a helpful assistant.
`,
  },
  {
    kind: "provider",
    label: "Provider",
    extension: ".provider.yaml",
    template: `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider

metadata:
  name: {{name}}

spec:
  id: {{name}}
  type: openai
  model: gpt-4o-mini
  defaults:
    temperature: 0.7
    max_tokens: 1000
`,
  },
  {
    kind: "scenario",
    label: "Scenario",
    extension: ".scenario.yaml",
    template: `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Scenario

metadata:
  name: {{name}}

spec:
  description: ""
  inputs: {}
  expected_output: ""
`,
  },
  {
    kind: "tool",
    label: "Tool",
    extension: ".tool.yaml",
    template: `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Tool

metadata:
  name: {{name}}

spec:
  description: ""
  parameters:
    type: object
    properties: {}
    required: []
`,
  },
  {
    kind: "persona",
    label: "Persona",
    extension: ".persona.yaml",
    template: `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Persona

metadata:
  name: {{name}}

spec:
  description: ""
  traits: []
  voice: ""
`,
  },
];

/**
 * Get the file type configuration for a given kind.
 */
export function getFileTypeConfig(kind: ArenaFileKind): ArenaFileTypeConfig | undefined {
  return ARENA_FILE_TYPES.find((t) => t.kind === kind);
}

/**
 * Generate a unique base name for a file type.
 * Returns something like "new-prompt", "new-provider", etc.
 */
export function generateUniqueBaseName(kind: ArenaFileKind): string {
  return `new-${kind}`;
}

/**
 * Generate the full filename with the appropriate extension.
 * @param baseName - The base name without extension (e.g., "my-prompt")
 * @param kind - The Arena file kind
 * @returns The full filename (e.g., "my-prompt.prompt.yaml")
 */
export function generateFileName(baseName: string, kind: ArenaFileKind): string {
  const config = getFileTypeConfig(kind);
  if (!config) {
    return `${baseName}.yaml`;
  }
  return `${baseName}${config.extension}`;
}

/**
 * Generate the file content for a given file type.
 * Replaces {{name}} placeholders with the actual name.
 * @param name - The name to use in the template (typically the baseName)
 * @param kind - The Arena file kind
 * @returns The generated file content
 */
export function generateFileContent(name: string, kind: ArenaFileKind): string {
  const config = getFileTypeConfig(kind);
  if (!config) {
    return "";
  }
  return config.template.replaceAll("{{name}}", name);
}
