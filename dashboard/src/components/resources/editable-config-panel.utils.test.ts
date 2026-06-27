import { describe, it, expect } from "vitest";
import * as yaml from "js-yaml";
import {
  sanitizeResourceForEditor,
  buildUpdateBodyFromYaml,
  isEditableYaml,
} from "./editable-config-panel.utils";

const RESOURCE = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "ToolRegistry",
  metadata: {
    name: "gh",
    namespace: "ws-ns",
    labels: { "omnia.altairalabs.ai/workspace": "ws1" },
    annotations: { "kubectl.kubernetes.io/last-applied-configuration": "{...}", note: "keep" },
    resourceVersion: "42",
    uid: "abc",
    creationTimestamp: "2026-01-01T00:00:00Z",
    managedFields: [{ manager: "x" }],
  },
  spec: { handlers: [{ name: "h", type: "http" }] },
  status: { phase: "Ready" },
};

describe("sanitizeResourceForEditor", () => {
  it("strips status and server-managed metadata but keeps name/namespace/labels/spec", () => {
    const text = sanitizeResourceForEditor(RESOURCE);
    const parsed = yaml.load(text) as Record<string, unknown> & {
      metadata: Record<string, unknown>;
      spec: { handlers: { name: string }[] };
    };
    expect(parsed.status).toBeUndefined();
    expect(parsed.metadata.resourceVersion).toBeUndefined();
    expect(parsed.metadata.uid).toBeUndefined();
    expect(parsed.metadata.managedFields).toBeUndefined();
    expect(parsed.metadata.creationTimestamp).toBeUndefined();
    expect(parsed.metadata.name).toBe("gh");
    expect(parsed.metadata.namespace).toBe("ws-ns");
    expect(parsed.spec.handlers[0].name).toBe("h");
  });

  it("drops the last-applied-configuration annotation but keeps user annotations", () => {
    const parsed = yaml.load(sanitizeResourceForEditor(RESOURCE)) as {
      metadata: { annotations: Record<string, string> };
    };
    expect(
      parsed.metadata.annotations["kubectl.kubernetes.io/last-applied-configuration"]
    ).toBeUndefined();
    expect(parsed.metadata.annotations.note).toBe("keep");
  });
});

describe("buildUpdateBodyFromYaml", () => {
  it("parses spec + labels/annotations and attaches the resourceVersion", () => {
    const text = sanitizeResourceForEditor(RESOURCE);
    const body = buildUpdateBodyFromYaml(text, "42");
    expect(body.spec).toEqual({ handlers: [{ name: "h", type: "http" }] });
    expect(body.metadata.resourceVersion).toBe("42");
    expect(body.metadata.annotations?.note).toBe("keep");
  });

  it("omits resourceVersion when none is provided", () => {
    const body = buildUpdateBodyFromYaml("spec:\n  handlers: []\n", undefined);
    expect(body.metadata.resourceVersion).toBeUndefined();
    expect(body.spec).toEqual({ handlers: [] });
  });

  it("throws on YAML that is not an object", () => {
    expect(() => buildUpdateBodyFromYaml("- just\n- a\n- list\n")).toThrow();
  });
});

describe("isEditableYaml", () => {
  it("returns true for valid object YAML", () => {
    expect(isEditableYaml("spec:\n  handlers: []\n")).toBe(true);
  });
  it("returns false for malformed YAML", () => {
    expect(isEditableYaml("spec:\n  - : :\n    bad")).toBe(false);
  });
  it("returns false for non-object YAML", () => {
    expect(isEditableYaml("42")).toBe(false);
  });
});
