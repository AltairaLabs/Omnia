/**
 * Tests for provider binding annotation utilities.
 */

import { describe, it, expect } from "vitest";
import { extractBindingAnnotations, insertBindingAnnotations } from "./provider-binding";

describe("extractBindingAnnotations", () => {
  it("should extract binding annotations from valid YAML", () => {
    const yaml = `
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: default-my-provider
  annotations:
    omnia.altairalabs.ai/provider-name: my-provider
    omnia.altairalabs.ai/provider-namespace: default
spec:
  type: openai
`;
    const result = extractBindingAnnotations(yaml);
    expect(result).toEqual({
      providerName: "my-provider",
      providerNamespace: "default",
    });
  });

  it("should return null when no annotations are present", () => {
    const yaml = `
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: my-provider
spec:
  type: openai
`;
    const result = extractBindingAnnotations(yaml);
    expect(result).toBeNull();
  });

  it("should return null when annotations exist but no binding keys", () => {
    const yaml = `
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: my-provider
  annotations:
    some-other/annotation: value
spec:
  type: openai
`;
    const result = extractBindingAnnotations(yaml);
    expect(result).toBeNull();
  });

  it("should default namespace to 'default' when not provided", () => {
    const yaml = `
apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: my-provider
  annotations:
    omnia.altairalabs.ai/provider-name: my-provider
spec:
  type: openai
`;
    const result = extractBindingAnnotations(yaml);
    expect(result).toEqual({
      providerName: "my-provider",
      providerNamespace: "default",
    });
  });

  it("should return null for invalid YAML", () => {
    const result = extractBindingAnnotations("not: valid: yaml: {{{}}}");
    expect(result).toBeNull();
  });

  it("should return null for empty string", () => {
    const result = extractBindingAnnotations("");
    expect(result).toBeNull();
  });

  it("should return null for non-object YAML", () => {
    const result = extractBindingAnnotations("just a string");
    expect(result).toBeNull();
  });
});

describe("insertBindingAnnotations", () => {
  it("should insert annotations into YAML with existing metadata", () => {
    const yaml = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: my-provider
spec:
  type: openai
`;
    const result = insertBindingAnnotations(yaml, "my-provider", "default");
    expect(result).toContain("omnia.altairalabs.ai/provider-name: my-provider");
    expect(result).toContain("omnia.altairalabs.ai/provider-namespace: default");
  });

  it("should insert annotations into YAML with existing annotations", () => {
    const yaml = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: my-provider
  annotations:
    some-other/annotation: value
spec:
  type: openai
`;
    const result = insertBindingAnnotations(yaml, "my-provider", "production");
    expect(result).toContain("omnia.altairalabs.ai/provider-name: my-provider");
    expect(result).toContain("omnia.altairalabs.ai/provider-namespace: production");
    // Existing annotations should be preserved
    expect(result).toContain("some-other/annotation: value");
  });

  it("should update existing binding annotations", () => {
    const yaml = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: my-provider
  annotations:
    omnia.altairalabs.ai/provider-name: old-provider
    omnia.altairalabs.ai/provider-namespace: old-namespace
spec:
  type: openai
`;
    const result = insertBindingAnnotations(yaml, "new-provider", "new-namespace");
    expect(result).toContain("omnia.altairalabs.ai/provider-name: new-provider");
    expect(result).toContain("omnia.altairalabs.ai/provider-namespace: new-namespace");
    expect(result).not.toContain("old-provider");
    expect(result).not.toContain("old-namespace");
  });

  it("should create metadata and annotations when missing", () => {
    const yaml = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
spec:
  type: openai
`;
    const result = insertBindingAnnotations(yaml, "my-provider", "default");
    expect(result).toContain("omnia.altairalabs.ai/provider-name: my-provider");
    expect(result).toContain("omnia.altairalabs.ai/provider-namespace: default");
  });

  it("should return original content for invalid YAML", () => {
    const invalid = "not: valid: yaml: {{{}}}";
    const result = insertBindingAnnotations(invalid, "provider", "ns");
    expect(result).toBe(invalid);
  });

  it("should preserve spec fields after insertion", () => {
    const yaml = `apiVersion: promptkit.altairalabs.ai/v1alpha1
kind: Provider
metadata:
  name: my-provider
spec:
  type: openai
  model: gpt-4
`;
    const result = insertBindingAnnotations(yaml, "my-provider", "default");
    expect(result).toContain("type: openai");
    expect(result).toContain("model: gpt-4");
  });
});
