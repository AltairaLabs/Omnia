import type { OpenAPIToolPreviewItem } from "@/types/tool-registry";

export type ToolLintSeverity = "warning";

export interface ToolLint {
  id: "weak-description" | "undescribed-required";
  severity: ToolLintSeverity;
  message: string;
}

// Matches a description that is just an HTTP method + path, e.g. "GET /pets/{id}".
const METHOD_PATH_RE = /^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+\/\S*$/;

interface JSONSchemaLike {
  properties?: Record<string, { description?: string } | undefined>;
  required?: unknown;
}

function asSchema(schema: unknown): JSONSchemaLike {
  return schema && typeof schema === "object" ? (schema as JSONSchemaLike) : {};
}

function weakDescription(description: string): boolean {
  const d = description.trim();
  return d === "" || METHOD_PATH_RE.test(d);
}

function undescribedRequired(schema: JSONSchemaLike): string[] {
  const required = Array.isArray(schema.required)
    ? schema.required.filter((r): r is string => typeof r === "string")
    : [];
  const props = schema.properties ?? {};
  return required.filter((name) => !props[name]?.description?.trim());
}

export function computeToolLints(tool: OpenAPIToolPreviewItem): ToolLint[] {
  const lints: ToolLint[] = [];

  if (weakDescription(tool.description)) {
    lints.push({
      id: "weak-description",
      severity: "warning",
      message:
        "The model only sees the method and path as this tool's description — it may not understand when to use it.",
    });
  }

  const undescribed = undescribedRequired(asSchema(tool.inputSchema));
  if (undescribed.length > 0) {
    lints.push({
      id: "undescribed-required",
      severity: "warning",
      message: `Required field(s) have no description: ${undescribed.join(", ")}. The model must guess their meaning.`,
    });
  }

  return lints;
}
