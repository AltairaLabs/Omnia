/**
 * Conversion functions for importing external CRD resources into Arena YAML format.
 */

import type { Provider } from "@/types/generated/provider";
import type { DiscoveredTool } from "@/types/tool-registry";

/**
 * Annotation prefix for Omnia reconciliation annotations.
 * These annotations allow the ArenaJob runtime to reconcile imported
 * Arena resources back to their original cluster CRDs.
 */
const ANNOTATION_PREFIX = "omnia.altairalabs.ai";

/**
 * Convert a Provider CRD to Arena Provider YAML format.
 * Maps the omnia.altairalabs.ai/v1alpha1 Provider to promptkit.altairalabs.ai/v1alpha1 Provider.
 *
 * Adds reconciliation annotations:
 * - omnia.altairalabs.ai/provider-name: original provider name
 * - omnia.altairalabs.ai/provider-namespace: original provider namespace
 */
export function convertProviderToArena(provider: Provider): string {
  const namespace = provider.metadata.namespace || "default";
  const name = provider.metadata.name;
  const id = `${namespace}-${name}`;

  // Build defaults section
  const defaults: Record<string, unknown> = {};
  if (provider.spec.defaults?.temperature !== undefined) {
    // Convert string temperature to number
    const temp = Number.parseFloat(provider.spec.defaults.temperature);
    if (!Number.isNaN(temp)) {
      defaults.temperature = temp;
    }
  }
  if (provider.spec.defaults?.maxTokens !== undefined) {
    defaults.max_tokens = provider.spec.defaults.maxTokens;
  }
  if (provider.spec.defaults?.topP !== undefined) {
    const topP = Number.parseFloat(provider.spec.defaults.topP);
    if (!Number.isNaN(topP)) {
      defaults.top_p = topP;
    }
  }
  if (provider.spec.defaults?.contextWindow !== undefined) {
    defaults.context_window = provider.spec.defaults.contextWindow;
  }
  if (provider.spec.defaults?.truncationStrategy !== undefined) {
    defaults.truncation_strategy = provider.spec.defaults.truncationStrategy;
  }

  // Build the YAML lines with reconciliation annotations
  const lines: string[] = [
    "apiVersion: promptkit.altairalabs.ai/v1alpha1",
    "kind: Provider",
    "",
    "metadata:",
    `  name: ${id}`,
    "  annotations:",
    `    ${ANNOTATION_PREFIX}/provider-name: ${name}`,
    `    ${ANNOTATION_PREFIX}/provider-namespace: ${namespace}`,
    "",
    "spec:",
    `  id: ${id}`,
    `  type: ${provider.spec.type}`,
  ];

  if (provider.spec.model) {
    lines.push(`  model: ${provider.spec.model}`);
  }

  if (provider.spec.baseURL) {
    lines.push(`  base_url: ${provider.spec.baseURL}`);
  }

  if (Object.keys(defaults).length > 0) {
    lines.push("  defaults:");
    for (const [key, value] of Object.entries(defaults)) {
      lines.push(`    ${key}: ${value}`);
    }
  }

  return lines.join("\n") + "\n";
}

/**
 * Generate a filename for an imported provider.
 * Format: {namespace}-{name}.provider.yaml
 */
export function generateProviderFilename(provider: Provider): string {
  const namespace = provider.metadata.namespace || "default";
  const name = provider.metadata.name;
  return `${namespace}-${name}.provider.yaml`;
}

/**
 * Format a single property for YAML output.
 */
function formatSchemaProperty(propName: string, propValue: unknown): string[] {
  const lines: string[] = [`      ${propName}:`];

  if (!propValue || typeof propValue !== "object") {
    return lines;
  }

  const prop = propValue as Record<string, unknown>;

  if (prop.type) {
    lines.push(`        type: ${String(prop.type)}`);
  }
  if (prop.description) {
    lines.push(`        description: ${quoteYamlString(String(prop.description))}`);
  }
  if (prop.enum && Array.isArray(prop.enum)) {
    const enumValues = prop.enum.map((e) => quoteYamlString(String(e))).join(", ");
    lines.push(`        enum: [${enumValues}]`);
  }
  if (prop.default !== undefined) {
    lines.push(`        default: ${formatYamlValue(prop.default)}`);
  }

  return lines;
}

/**
 * Convert an input schema to YAML parameter lines.
 */
function formatInputSchema(inputSchema: unknown): string[] {
  const lines: string[] = ["  parameters:"];

  if (!inputSchema || typeof inputSchema !== "object") {
    lines.push("    type: object", "    properties: {}", "    required: []");
    return lines;
  }

  const schema = inputSchema as Record<string, unknown>;
  lines.push(`    type: ${String(schema.type || "object")}`);

  if (schema.properties && typeof schema.properties === "object") {
    lines.push("    properties:");
    const properties = schema.properties as Record<string, unknown>;
    for (const [propName, propValue] of Object.entries(properties)) {
      lines.push(...formatSchemaProperty(propName, propValue));
    }
  }

  if (schema.required && Array.isArray(schema.required)) {
    const required = schema.required.map((r) => quoteYamlString(String(r))).join(", ");
    lines.push(`    required: [${required}]`);
  } else {
    lines.push("    required: []");
  }

  return lines;
}

export interface ToolConversionOptions {
  registryName: string;
  registryNamespace: string;
}

/**
 * Convert a discovered tool from a ToolRegistry to Arena Tool YAML format.
 *
 * Adds reconciliation annotations:
 * - omnia.altairalabs.ai/registry-name: original tool registry name
 * - omnia.altairalabs.ai/registry-namespace: original tool registry namespace
 * - omnia.altairalabs.ai/tool-name: original tool name within the registry
 */
export function convertToolToArena(tool: DiscoveredTool, options: ToolConversionOptions): string {
  const { registryName, registryNamespace } = options;
  const id = `${registryName}-${tool.name}`;

  const lines: string[] = [
    "apiVersion: promptkit.altairalabs.ai/v1alpha1",
    "kind: Tool",
    "",
    "metadata:",
    `  name: ${id}`,
    "  annotations:",
    `    ${ANNOTATION_PREFIX}/registry-name: ${registryName}`,
    `    ${ANNOTATION_PREFIX}/registry-namespace: ${registryNamespace}`,
    `    ${ANNOTATION_PREFIX}/tool-name: ${tool.name}`,
    "",
    "spec:",
    `  description: ${quoteYamlString(tool.description || "")}`,
    ...formatInputSchema(tool.inputSchema),
  ];

  return lines.join("\n") + "\n";
}

/**
 * Generate a filename for an imported tool.
 * Format: {registryName}-{toolName}.tool.yaml
 */
export function generateToolFilename(tool: DiscoveredTool, registryName: string): string {
  // Sanitize tool name for use in filename - replace invalid characters with dashes
  const sanitizedToolName = tool.name
    .split("")
    .map((char) => (/[a-zA-Z0-9-_]/.test(char) ? char : "-"))
    .join("");
  return `${registryName}-${sanitizedToolName}.tool.yaml`;
}

/**
 * Quote a string for YAML if needed.
 */
function quoteYamlString(str: string): string {
  // If the string contains special characters, quote it
  if (
    str.includes(":") ||
    str.includes("#") ||
    str.includes("'") ||
    str.includes('"') ||
    str.includes("\n") ||
    str.startsWith(" ") ||
    str.endsWith(" ")
  ) {
    // Use double quotes and escape internal double quotes
    const escaped = str.replaceAll('"', '\\"').replaceAll("\n", "\\n");
    return `"${escaped}"`;
  }
  return str;
}

/**
 * Format a value for YAML output.
 */
function formatYamlValue(value: unknown): string {
  if (typeof value === "string") {
    return quoteYamlString(value);
  }
  if (typeof value === "boolean" || typeof value === "number") {
    return String(value);
  }
  if (value === null) {
    return "null";
  }
  if (Array.isArray(value)) {
    return `[${value.map((v) => formatYamlValue(v)).join(", ")}]`;
  }
  // For objects, just convert to JSON-ish representation
  return JSON.stringify(value);
}
