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
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  isValidJsonObject,
  BasicInfoStep,
  FunctionSchemaEditors,
  composeAgentYaml,
  DeployWizard,
} from "./deploy-wizard";

// ---------------------------------------------------------------------------
// Module-level mocks — must be at the top level so vitest hoists them before
// imports. Each mock is kept minimal: only what DeployWizard actually reads.
// ---------------------------------------------------------------------------

vi.mock("@/hooks/core", () => ({
  useReadOnly: vi.fn(() => ({ isReadOnly: false, message: "" })),
}));

vi.mock("@/hooks/auth", () => ({
  usePermissions: vi.fn(() => ({ can: () => true })),
  Permission: { AGENTS_DEPLOY: "agents:deploy" },
}));

vi.mock("@/hooks/resources", () => ({
  usePromptPacks: vi.fn(() => ({ data: [] })),
  useToolRegistries: vi.fn(() => ({ data: [] })),
  useProviders: vi.fn(() => ({ data: [] })),
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({
    currentWorkspace: { name: "ws", namespace: "ns-a", displayName: "Workspace A" },
  })),
}));

vi.mock("@/lib/data", () => ({
  useDataService: vi.fn(() => ({
    createAgent: vi.fn().mockResolvedValue({}),
  })),
}));

function renderWizard() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <DeployWizard open={true} onOpenChange={vi.fn()} />
    </QueryClientProvider>,
  );
}

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
    promptPackTrack: "stable" as const,
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

describe("BasicInfoStep with validation", () => {
  const baseForm = {
    name: "test-fn",
    mode: "agent" as const,
    inputSchemaJson: "{}",
    outputSchemaJson: "{}",
    framework: "promptkit" as const,
    frameworkVersion: "",
    customImage: "",
    promptPackName: "",
    promptPackTrack: "stable" as const,
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

  it("shows a FieldError when errors contains metadata.name", () => {
    render(
      <BasicInfoStep
        formData={baseForm}
        currentWorkspace={{ name: "ws", namespace: "ns-a" }}
        updateField={vi.fn()}
        errors={{ "metadata.name": "Use lowercase letters, numbers, and hyphens; must start and end with a letter or number." }}
        validate={vi.fn()}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert).toBeInTheDocument();
    expect(alert).toHaveTextContent(/must start and end with a letter or number/i);
  });

  it("calls validate with metadata.name on name input change", () => {
    const validate = vi.fn();
    render(
      <BasicInfoStep
        formData={baseForm}
        currentWorkspace={{ name: "ws", namespace: "ns-a" }}
        updateField={vi.fn()}
        errors={{}}
        validate={validate}
      />,
    );
    fireEvent.change(screen.getByLabelText(/Agent Name/i), { target: { value: "my-agent" } });
    expect(validate).toHaveBeenCalledWith("metadata.name", "my-agent");
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
    promptPackTrack: "stable" as const,
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
        facades: [{ type: "websocket", port: 8080 }],
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

  it("pins the facade type to 'rest' in function mode regardless of facadeType", () => {
    const yaml = composeAgentYaml(
      { ...baseForm, mode: "function", facadeType: "websocket" },
      "ns-a",
    );
    const spec = (yaml as { spec: { facades: { type: string }[] } }).spec;
    expect(spec.facades[0].type).toBe("rest");
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

  it("omits spec.framework entirely for promptkit with no customisation", () => {
    const yaml = composeAgentYaml(baseForm, "ns-a");
    expect((yaml as { spec: Record<string, unknown> }).spec).not.toHaveProperty("framework");
  });

  it("emits framework type and image for a non-promptkit framework", () => {
    const yaml = composeAgentYaml(
      { ...baseForm, framework: "langchain", customImage: "ghcr.io/acme/lc:v1" },
      "ns-a",
    );
    expect((yaml as { spec: { framework: unknown } }).spec.framework).toEqual({
      type: "langchain",
      image: "ghcr.io/acme/lc:v1",
    });
  });

  it("trims surrounding whitespace from the framework image", () => {
    const yaml = composeAgentYaml(
      { ...baseForm, framework: "custom", customImage: "  ghcr.io/acme/x:v1  " },
      "ns-a",
    );
    const framework = (yaml as { spec: { framework: { image: string } } }).spec.framework;
    expect(framework.image).toBe("ghcr.io/acme/x:v1");
  });

  it("does not emit a framework image for a non-promptkit framework when it is whitespace-only", () => {
    const yaml = composeAgentYaml(
      { ...baseForm, framework: "langchain", customImage: "   " },
      "ns-a",
    );
    const framework = (yaml as { spec: { framework: Record<string, unknown> } }).spec.framework;
    expect(framework).toEqual({ type: "langchain" });
    expect(framework).not.toHaveProperty("image");
  });

  it("never emits a stale customImage under promptkit", () => {
    // A customImage can linger in form state after switching back to promptkit.
    // It must not leak into the promptkit AgentRuntime as spec.framework.image.
    const yaml = composeAgentYaml(
      { ...baseForm, framework: "promptkit", customImage: "ghcr.io/acme/langchain:v1" },
      "ns-a",
    );
    expect((yaml as { spec: Record<string, unknown> }).spec).not.toHaveProperty("framework");
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

// ---------------------------------------------------------------------------
// I-1: Full-wizard integration — validate → error → Next disabled
// ---------------------------------------------------------------------------
describe("DeployWizard integration", () => {
  it("shows inline error and disables Next when an invalid name is typed", async () => {
    renderWizard();

    const nameInput = screen.getByLabelText(/Agent Name/i);
    // The wizard auto-formats on input (toLowerCase + replaceAll); type a
    // value that is invalid even after auto-format (a leading hyphen triggers
    // the CRD constraint that names must start with a letter or number).
    fireEvent.change(nameInput, { target: { value: "-bad" } });

    // (a) Inline error must be visible — role="alert" is the FieldError element
    const errorAlert = await screen.findByRole("alert");
    expect(errorAlert).toBeInTheDocument();
    expect(errorAlert).toHaveTextContent(/lowercase letters/i);

    // (b) Next button must be disabled
    const nextBtn = screen.getByRole("button", { name: /next/i });
    expect(nextBtn).toBeDisabled();
  });

  // Only `promptkit` has a built-in default runtime image (custom-runtime
  // wave 1). Selecting langchain/autogen in the wizard must block Next until
  // an image is supplied — same as `custom` — or the wizard silently submits
  // an AgentRuntime that is permanently unschedulable with no way to fix it
  // from the UI.
  it("blocks Next on the Framework step for langchain until an image is supplied", async () => {
    renderWizard();

    fireEvent.change(screen.getByLabelText(/Agent Name/i), { target: { value: "my-agent" } });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Now on the Framework step.
    await screen.findByText("Agent Framework");
    fireEvent.click(screen.getByRole("radio", { name: /LangChain/i }));

    const nextBtn = screen.getByRole("button", { name: /next/i });
    expect(nextBtn).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Container Image"), {
      target: { value: "ghcr.io/acme/langchain-runtime:v1.0" },
    });
    expect(nextBtn).not.toBeDisabled();
  });

  // Whitespace-only input is not a real image reference — it would be emitted as
  // spec.framework.image and deploy unschedulable. The gate must trim.
  it("keeps Next disabled when the Container Image is whitespace-only", async () => {
    renderWizard();

    fireEvent.change(screen.getByLabelText(/Agent Name/i), { target: { value: "my-agent" } });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await screen.findByText("Agent Framework");
    fireEvent.click(screen.getByRole("radio", { name: /LangChain/i }));

    const nextBtn = screen.getByRole("button", { name: /next/i });
    fireEvent.change(screen.getByLabelText("Container Image"), { target: { value: "   " } });
    expect(nextBtn).toBeDisabled();
  });

  it("does not require an image when promptkit is selected", async () => {
    renderWizard();

    fireEvent.change(screen.getByLabelText(/Agent Name/i), { target: { value: "my-agent" } });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await screen.findByText("Agent Framework");
    expect(screen.queryByLabelText("Container Image")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /next/i })).not.toBeDisabled();
  });
});
