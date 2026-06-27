import * as yaml from "js-yaml";

const LAST_APPLIED_ANNOTATION = "kubectl.kubernetes.io/last-applied-configuration";

export interface EditableResource {
  apiVersion?: string;
  kind?: string;
  metadata?: {
    name?: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    resourceVersion?: string;
  } & Record<string, unknown>;
  spec?: unknown;
  status?: unknown;
}

export interface UpdateResourceBody {
  metadata: {
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    resourceVersion?: string;
  };
  spec: unknown;
}

function cleanAnnotations(
  annotations?: Record<string, string>
): Record<string, string> | undefined {
  if (!annotations) return undefined;
  const entries = Object.entries(annotations).filter(([k]) => k !== LAST_APPLIED_ANNOTATION);
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

/** Build the editable YAML view: keep apiVersion/kind/editable metadata/spec; drop the rest. */
export function sanitizeResourceForEditor(resource: EditableResource): string {
  const md = resource.metadata ?? {};
  const metadata: Record<string, unknown> = {};
  if (md.name) metadata.name = md.name;
  if (md.namespace) metadata.namespace = md.namespace;
  if (md.labels) metadata.labels = md.labels;
  const annotations = cleanAnnotations(md.annotations);
  if (annotations) metadata.annotations = annotations;

  const view = {
    apiVersion: resource.apiVersion,
    kind: resource.kind,
    metadata,
    spec: resource.spec,
  };
  return yaml.dump(view, { noRefs: true, sortKeys: false });
}

/** Parse edited YAML into a PUT body, attaching the held resourceVersion for concurrency. */
export function buildUpdateBodyFromYaml(
  yamlText: string,
  resourceVersion?: string
): UpdateResourceBody {
  const parsed = yaml.load(yamlText);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("YAML must describe a resource object");
  }
  const obj = parsed as EditableResource;
  const md = obj.metadata ?? {};
  const metadata: UpdateResourceBody["metadata"] = {
    labels: md.labels,
    annotations: md.annotations,
  };
  if (resourceVersion) {
    metadata.resourceVersion = resourceVersion;
  }
  return { metadata, spec: obj.spec };
}

/** True when the text parses to a YAML object (the save-enable gate). */
export function isEditableYaml(yamlText: string): boolean {
  try {
    const parsed = yaml.load(yamlText);
    return !!parsed && typeof parsed === "object" && !Array.isArray(parsed);
  } catch {
    return false;
  }
}
