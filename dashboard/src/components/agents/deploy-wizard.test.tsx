/**
 * Tests for the Functions-mode additions to DeployWizard (#1103 PR 6
 * follow-up). Focuses on the new isValidJsonObject helper and the
 * BasicInfoStep + FunctionSchemaEditors components. Full-wizard
 * integration (mode toggle → YAML composer → createAgent call) is
 * covered separately by deploy-wizard-yaml.test.tsx.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import {
  isValidJsonObject,
  BasicInfoStep,
  FunctionSchemaEditors,
  composeAgentYaml,
} from "./deploy-wizard";

describe("isValidJsonObject", () => {
  it("accepts a JSON object", () => {
    expect(isValidJsonObject('{"a":1}')).toBe(true);
  });

  it("accepts an empty object", () => {
    expect(isValidJsonObject("{}")).toBe(true);
  });

  it("rejects malformed JSON", () => {
    expect(isValidJsonObject("{not json")).toBe(false);
  });

  it("rejects JSON arrays (schemas are objects)", () => {
    expect(isValidJsonObject("[1,2,3]")).toBe(false);
  });

  it("rejects JSON scalars", () => {
    expect(isValidJsonObject('"a string"')).toBe(false);
    expect(isValidJsonObject("42")).toBe(false);
    expect(isValidJsonObject("true")).toBe(false);
  });

  it("rejects JSON null", () => {
    expect(isValidJsonObject("null")).toBe(false);
  });

  it("rejects the empty string", () => {
    expect(isValidJsonObject("")).toBe(false);
  });
});

describe("FunctionSchemaEditors", () => {
  it("renders both editors with the supplied values", () => {
    render(
      <FunctionSchemaEditors
        inputSchemaJson='{"type":"object"}'
        outputSchemaJson='{"type":"object","required":["a"]}'
        onChangeInputSchema={vi.fn()}
        onChangeOutputSchema={vi.fn()}
      />,
    );
    const input = screen.getByLabelText("Input schema (JSON Schema)") as HTMLTextAreaElement;
    const output = screen.getByLabelText("Output schema (JSON Schema)") as HTMLTextAreaElement;
    expect(input.value).toBe('{"type":"object"}');
    expect(output.value).toBe('{"type":"object","required":["a"]}');
  });

  it("shows the error label when the input schema is invalid JSON", () => {
    render(
      <FunctionSchemaEditors
        inputSchemaJson="{ broken"
        outputSchemaJson='{"type":"object"}'
        onChangeInputSchema={vi.fn()}
        onChangeOutputSchema={vi.fn()}
      />,
    );
    expect(screen.getByTestId("input-schema-error")).toBeInTheDocument();
    expect(screen.queryByTestId("output-schema-error")).not.toBeInTheDocument();
  });

  it("shows the error label when the output schema is invalid JSON", () => {
    render(
      <FunctionSchemaEditors
        inputSchemaJson='{"type":"object"}'
        outputSchemaJson="not-an-object"
        onChangeInputSchema={vi.fn()}
        onChangeOutputSchema={vi.fn()}
      />,
    );
    expect(screen.getByTestId("output-schema-error")).toBeInTheDocument();
  });

  it("propagates textarea edits through the change handlers", () => {
    const onInput = vi.fn();
    const onOutput = vi.fn();
    render(
      <FunctionSchemaEditors
        inputSchemaJson="{}"
        outputSchemaJson="{}"
        onChangeInputSchema={onInput}
        onChangeOutputSchema={onOutput}
      />,
    );
    fireEvent.change(screen.getByLabelText("Input schema (JSON Schema)"), {
      target: { value: '{"new":"input"}' },
    });
    fireEvent.change(screen.getByLabelText("Output schema (JSON Schema)"), {
      target: { value: '{"new":"output"}' },
    });
    expect(onInput).toHaveBeenCalledWith('{"new":"input"}');
    expect(onOutput).toHaveBeenCalledWith('{"new":"output"}');
  });
});

describe("BasicInfoStep", () => {
  const baseForm = {
    name: "test-fn",
    mode: "agent" as const,
    inputSchemaJson: "{}",
    outputSchemaJson: "{}",
    framework: "promptkit" as const,
    frameworkVersion: "",
    customImage: "",
    promptPackName: "",
    promptPackTrack: "stable",
    providerRefName: "",
    toolRegistryName: "",
    toolRegistryNamespace: "",
    contextType: "memory" as const,
    contextTtl: "24h",
    replicas: 1,
    cpuRequest: "100m",
    cpuLimit: "500m",
    memoryRequest: "128Mi",
    memoryLimit: "512Mi",
    facadeType: "websocket" as const,
    facadePort: 8080,
  };

  it("does NOT render the schema editors when mode is 'agent'", () => {
    render(
      <BasicInfoStep
        formData={baseForm}
        currentWorkspace={{ name: "ws", namespace: "ns-a" }}
        updateField={vi.fn()}
      />,
    );
    expect(screen.queryByTestId("function-schemas")).not.toBeInTheDocument();
  });

  it("renders the schema editors when mode is 'function'", () => {
    render(
      <BasicInfoStep
        formData={{ ...baseForm, mode: "function" }}
        currentWorkspace={{ name: "ws", namespace: "ns-a" }}
        updateField={vi.fn()}
      />,
    );
    expect(screen.getByTestId("function-schemas")).toBeInTheDocument();
  });

  it("calls updateField('mode', ...) when the toggle is clicked", () => {
    const updateField = vi.fn();
    render(
      <BasicInfoStep
        formData={baseForm}
        currentWorkspace={{ name: "ws", namespace: "ns-a" }}
        updateField={updateField}
      />,
    );
    fireEvent.click(screen.getByLabelText(/Function/));
    expect(updateField).toHaveBeenCalledWith("mode", "function");
  });

  it("falls back to 'default' namespace in the description when none set", () => {
    render(
      <BasicInfoStep
        formData={baseForm}
        currentWorkspace={{ name: "ws" }}
        updateField={vi.fn()}
      />,
    );
    expect(screen.getByText(/default namespace/)).toBeInTheDocument();
  });
});

describe("composeAgentYaml", () => {
  const baseForm = {
    name: "my-fn",
    mode: "agent" as const,
    inputSchemaJson: "{}",
    outputSchemaJson: "{}",
    framework: "promptkit" as const,
    frameworkVersion: "",
    customImage: "",
    promptPackName: "my-pack",
    promptPackTrack: "stable",
    providerRefName: "",
    toolRegistryName: "",
    toolRegistryNamespace: "",
    contextType: "memory" as const,
    contextTtl: "24h",
    replicas: 1,
    cpuRequest: "100m",
    cpuLimit: "500m",
    memoryRequest: "128Mi",
    memoryLimit: "512Mi",
    facadeType: "websocket" as const,
    facadePort: 8080,
  };

  it("emits a minimal agent-mode AgentRuntime when nothing is customised", () => {
    const yaml = composeAgentYaml(baseForm, "ns-a");
    expect(yaml).toMatchObject({
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "AgentRuntime",
      metadata: { name: "my-fn", namespace: "ns-a" },
      spec: {
        promptPackRef: { name: "my-pack", track: "stable" },
        facade: { type: "websocket", port: 8080 },
      },
    });
    // Function-mode fields must be absent — pre-PR YAMLs byte-identical.
    const spec = (yaml as { spec: Record<string, unknown> }).spec;
    expect(spec).not.toHaveProperty("mode");
    expect(spec).not.toHaveProperty("inputSchema");
    expect(spec).not.toHaveProperty("outputSchema");
  });

  it("emits mode + parsed schemas when mode is 'function'", () => {
    const yaml = composeAgentYaml(
      {
        ...baseForm,
        mode: "function",
        inputSchemaJson: '{"type":"object","required":["q"]}',
        outputSchemaJson: '{"type":"object","required":["a"]}',
      },
      "ns-a",
    );
    const spec = (yaml as { spec: Record<string, unknown> }).spec;
    expect(spec.mode).toBe("function");
    expect(spec.inputSchema).toEqual({ type: "object", required: ["q"] });
    expect(spec.outputSchema).toEqual({ type: "object", required: ["a"] });
  });

  it("pins facade.type to 'rest' in function mode regardless of facadeType", () => {
    const yaml = composeAgentYaml(
      { ...baseForm, mode: "function", facadeType: "websocket" },
      "ns-a",
    );
    const spec = (yaml as { spec: { facade: { type: string } } }).spec;
    expect(spec.facade.type).toBe("rest");
  });

  it("falls back to 'default' namespace when none supplied", () => {
    const yaml = composeAgentYaml(baseForm, undefined);
    expect((yaml as { metadata: { namespace: string } }).metadata.namespace).toBe("default");
  });

  it("includes provider ref when set", () => {
    const yaml = composeAgentYaml({ ...baseForm, providerRefName: "openai-1" }, "ns-a");
    const spec = (yaml as { spec: { providers?: unknown[] } }).spec;
    expect(spec.providers).toEqual([
      { name: "default", providerRef: { name: "openai-1" } },
    ]);
  });

  it("omits the runtime block when defaults are unchanged", () => {
    const yaml = composeAgentYaml(baseForm, "ns-a");
    expect((yaml as { spec: Record<string, unknown> }).spec).not.toHaveProperty("runtime");
  });

  it("includes runtime.replicas when replicas != 1", () => {
    const yaml = composeAgentYaml({ ...baseForm, replicas: 3 }, "ns-a");
    const spec = (yaml as { spec: { runtime?: { replicas?: number } } }).spec;
    expect(spec.runtime?.replicas).toBe(3);
  });
});
