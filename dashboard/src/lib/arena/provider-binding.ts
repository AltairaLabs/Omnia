/**
 * Utilities for extracting and inserting provider binding annotations
 * in Arena provider YAML files.
 *
 * Binding annotations link an Arena provider YAML to a cluster Provider CRD:
 * - omnia.altairalabs.ai/provider-name
 * - omnia.altairalabs.ai/provider-namespace
 */

import * as yaml from "js-yaml";

const ANNOTATION_PREFIX = "omnia.altairalabs.ai";
const PROVIDER_NAME_KEY = `${ANNOTATION_PREFIX}/provider-name`;
const PROVIDER_NAMESPACE_KEY = `${ANNOTATION_PREFIX}/provider-namespace`;

export interface BindingAnnotations {
  providerName: string;
  providerNamespace: string;
}

/**
 * Extract provider binding annotations from a YAML string.
 * Returns null if the YAML is invalid or has no binding annotations.
 */
export function extractBindingAnnotations(content: string): BindingAnnotations | null {
  try {
    const doc = yaml.load(content) as Record<string, unknown> | null;
    if (!doc || typeof doc !== "object") return null;

    const metadata = doc.metadata as Record<string, unknown> | undefined;
    if (!metadata || typeof metadata !== "object") return null;

    const annotations = metadata.annotations as Record<string, string> | undefined;
    if (!annotations || typeof annotations !== "object") return null;

    const providerName = annotations[PROVIDER_NAME_KEY];
    const providerNamespace = annotations[PROVIDER_NAMESPACE_KEY];

    if (!providerName || typeof providerName !== "string") return null;

    return {
      providerName,
      providerNamespace: providerNamespace || "default",
    };
  } catch {
    return null;
  }
}

/**
 * Insert or update binding annotations in a provider YAML string.
 * Preserves the rest of the YAML content.
 */
export function insertBindingAnnotations(
  content: string,
  providerName: string,
  providerNamespace: string
): string {
  try {
    const doc = yaml.load(content) as Record<string, unknown> | null;
    if (!doc || typeof doc !== "object") return content;

    // Ensure metadata exists
    if (!doc.metadata || typeof doc.metadata !== "object") {
      doc.metadata = {};
    }
    const metadata = doc.metadata as Record<string, unknown>;

    // Ensure annotations exists
    if (!metadata.annotations || typeof metadata.annotations !== "object") {
      metadata.annotations = {};
    }
    const annotations = metadata.annotations as Record<string, string>;

    // Set the binding annotations
    annotations[PROVIDER_NAME_KEY] = providerName;
    annotations[PROVIDER_NAMESPACE_KEY] = providerNamespace;

    return yaml.dump(doc, { lineWidth: -1, noRefs: true });
  } catch {
    return content;
  }
}
