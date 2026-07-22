/**
 * Tests for deploy-wizard-steps.tsx — the step sub-components rendered by
 * DeployWizard. Focused primarily on FrameworkStep (finding #3, custom
 * runtime wave 1 follow-up): only `promptkit` has a built-in default
 * runtime image, so the Container Image input — and the canProceed gate in
 * deploy-wizard.tsx — must require an image for ANY non-promptkit
 * framework, not just `custom`.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import {
  FrameworkStep,
  PromptPackStep,
  ProviderStep,
  OptionsStep,
  RuntimeStep,
  type WizardFormData,
} from "./deploy-wizard-steps";
import { crdConstraints } from "@/types/generated/crd-constraints";

const baseForm: WizardFormData = {
  name: "test-agent",
  mode: "agent",
  inputSchemaJson: "{}",
  outputSchemaJson: "{}",
  framework: "promptkit",
  frameworkVersion: "",
  customImage: "",
  promptPackName: "",
  promptPackTrack: "stable",
  providerRefName: "",
  toolRegistryName: "",
  toolRegistryNamespace: "",
  contextType: "memory",
  contextTtl: "24h",
  replicas: 1,
  cpuRequest: "100m",
  cpuLimit: "500m",
  memoryRequest: "128Mi",
  memoryLimit: "512Mi",
  facadeType: "websocket",
  facadePort: 8080,
};

const workspace = { name: "ws", namespace: "ns-a", displayName: "Workspace A" };

describe("FrameworkStep", () => {
  it("does NOT render the Container Image input for promptkit", () => {
    render(<FrameworkStep formData={baseForm} updateField={vi.fn()} />);
    expect(screen.queryByLabelText("Container Image")).not.toBeInTheDocument();
  });

  it("renders the Container Image input for langchain", () => {
    render(
      <FrameworkStep formData={{ ...baseForm, framework: "langchain" }} updateField={vi.fn()} />,
    );
    expect(screen.getByLabelText("Container Image")).toBeInTheDocument();
    expect(
      screen.getByText(/no built-in image is provided for this framework/i),
    ).toBeInTheDocument();
  });

  it("renders the Container Image input for langchain", () => {
    render(
      <FrameworkStep formData={{ ...baseForm, framework: "langchain" }} updateField={vi.fn()} />,
    );
    expect(screen.getByLabelText("Container Image")).toBeInTheDocument();
  });

  it("renders the Container Image input for custom with contract-specific help text", () => {
    render(
      <FrameworkStep formData={{ ...baseForm, framework: "custom" }} updateField={vi.fn()} />,
    );
    expect(screen.getByLabelText("Container Image")).toBeInTheDocument();
    expect(screen.getByText(/omnia\.runtime\.v1 contract/i)).toBeInTheDocument();
  });

  it("propagates image input edits through updateField", () => {
    const updateField = vi.fn();
    render(
      <FrameworkStep formData={{ ...baseForm, framework: "langchain" }} updateField={updateField} />,
    );
    fireEvent.change(screen.getByLabelText("Container Image"), {
      target: { value: "ghcr.io/acme/langchain-runtime:v1.0" },
    });
    expect(updateField).toHaveBeenCalledWith("customImage", "ghcr.io/acme/langchain-runtime:v1.0");
  });

  it("switches the selected framework via updateField", () => {
    const updateField = vi.fn();
    render(<FrameworkStep formData={baseForm} updateField={updateField} />);
    fireEvent.click(screen.getByRole("radio", { name: /LangChain/i }));
    expect(updateField).toHaveBeenCalledWith("framework", "langchain");
  });

  it("clears a stale customImage and version when switching back to promptkit", () => {
    const updateField = vi.fn();
    render(
      <FrameworkStep
        formData={{ ...baseForm, framework: "langchain", customImage: "ghcr.io/acme/lc:v1" }}
        updateField={updateField}
      />,
    );
    fireEvent.click(screen.getByRole("radio", { name: /PromptKit/i }));
    expect(updateField).toHaveBeenCalledWith("framework", "promptkit");
    expect(updateField).toHaveBeenCalledWith("customImage", "");
    expect(updateField).toHaveBeenCalledWith("frameworkVersion", "");
  });

  it("does not clear customImage when switching between non-promptkit frameworks", () => {
    const updateField = vi.fn();
    render(
      <FrameworkStep
        formData={{ ...baseForm, framework: "langchain", customImage: "ghcr.io/acme/lc:v1" }}
        updateField={updateField}
      />,
    );
    fireEvent.click(screen.getByRole("radio", { name: /Custom/i }));
    expect(updateField).toHaveBeenCalledWith("framework", "custom");
    expect(updateField).not.toHaveBeenCalledWith("customImage", "");
  });
});

describe("PromptPackStep", () => {
  it("renders the empty-state item when no PromptPacks are available", () => {
    render(
      <PromptPackStep
        formData={baseForm}
        currentWorkspace={workspace}
        promptPacks={[]}
        updateField={vi.fn()}
      />,
    );
    expect(screen.getByText(/Showing PromptPacks in ns-a namespace/)).toBeInTheDocument();
  });

  it("lists available PromptPacks", () => {
    render(
      <PromptPackStep
        formData={baseForm}
        currentWorkspace={workspace}
        promptPacks={[{ metadata: { uid: "1", name: "support-pack" }, status: { phase: "Ready" } }]}
        updateField={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("combobox"));
    expect(screen.getByText("support-pack")).toBeInTheDocument();
  });

  // #1882: the radio offered "canary", which spec.promptPackRef.track rejects
  // (enum is stable|prerelease), so every Canary deploy failed at the API
  // server. Assert against the CRD's own enum — crd-constraints.ts is
  // regenerated from the CRD OpenAPI schema by `make generate-dashboard-types`,
  // so this test tracks the schema rather than restating it.
  it("only offers Release Track values the AgentRuntime CRD accepts", () => {
    const allowed = crdConstraints.AgentRuntime["spec.promptPackRef.track"].enum;
    render(
      <PromptPackStep
        formData={baseForm}
        currentWorkspace={workspace}
        promptPacks={[]}
        updateField={vi.fn()}
      />,
    );
    const offered = screen.getAllByRole("radio").map((r) => r.getAttribute("value"));
    expect(offered.length).toBeGreaterThan(0);
    for (const value of offered) {
      expect(allowed).toContain(value);
    }
  });

  it("switches the release track via updateField", () => {
    const updateField = vi.fn();
    render(
      <PromptPackStep
        formData={baseForm}
        currentWorkspace={workspace}
        promptPacks={[]}
        updateField={updateField}
      />,
    );
    fireEvent.click(screen.getByLabelText("Prerelease"));
    expect(updateField).toHaveBeenCalledWith("promptPackTrack", "prerelease");
  });
});

describe("ProviderStep", () => {
  it("shows the warning when no Providers are configured", () => {
    render(
      <ProviderStep
        formData={baseForm}
        currentWorkspace={workspace}
        providers={[]}
        updateField={vi.fn()}
      />,
    );
    expect(screen.getByText(/No Providers configured/)).toBeInTheDocument();
  });

  it("lists available Providers", () => {
    render(
      <ProviderStep
        formData={baseForm}
        currentWorkspace={workspace}
        providers={[{ metadata: { uid: "1", name: "claude" }, spec: { type: "claude", model: "sonnet" } }]}
        updateField={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("combobox"));
    expect(screen.getByRole("option", { name: /claude/i })).toBeInTheDocument();
  });
});

describe("OptionsStep", () => {
  it("updates context TTL via updateField", () => {
    const updateField = vi.fn();
    render(
      <OptionsStep
        formData={baseForm}
        currentWorkspace={workspace}
        toolRegistries={[]}
        updateField={updateField}
      />,
    );
    fireEvent.change(screen.getByLabelText("Context TTL"), { target: { value: "48h" } });
    expect(updateField).toHaveBeenCalledWith("contextTtl", "48h");
  });

  it("clears the tool registry selection", () => {
    const updateField = vi.fn();
    render(
      <OptionsStep
        formData={{ ...baseForm, toolRegistryName: "support-tools", toolRegistryNamespace: "" }}
        currentWorkspace={workspace}
        toolRegistries={[]}
        updateField={updateField}
      />,
    );
    fireEvent.click(screen.getByRole("combobox"));
    fireEvent.click(screen.getByText("None"));
    expect(updateField).toHaveBeenCalledWith("toolRegistryName", "");
    expect(updateField).toHaveBeenCalledWith("toolRegistryNamespace", "");
  });

  it("selects a cross-namespace tool registry", () => {
    const updateField = vi.fn();
    render(
      <OptionsStep
        formData={baseForm}
        currentWorkspace={workspace}
        toolRegistries={[
          { metadata: { uid: "1", name: "shared-tools", namespace: "ns-b" }, status: { discoveredToolsCount: 3 } },
        ]}
        updateField={updateField}
      />,
    );
    fireEvent.click(screen.getByRole("combobox"));
    fireEvent.click(screen.getByText("shared-tools"));
    expect(updateField).toHaveBeenCalledWith("toolRegistryName", "shared-tools");
    expect(updateField).toHaveBeenCalledWith("toolRegistryNamespace", "ns-b");
  });
});

describe("RuntimeStep", () => {
  it("renders the REST-only facade note in function mode", () => {
    render(
      <RuntimeStep
        formData={{ ...baseForm, mode: "function" }}
        updateField={vi.fn()}
        errors={{}}
        validate={vi.fn()}
      />,
    );
    expect(screen.getByText(/Function mode uses the REST \(HTTP\) facade\./)).toBeInTheDocument();
  });

  it("validates the facade port on change", () => {
    const validate = vi.fn();
    render(
      <RuntimeStep formData={baseForm} updateField={vi.fn()} errors={{}} validate={validate} />,
    );
    fireEvent.change(screen.getByLabelText("Port"), { target: { value: "9090" } });
    expect(validate).toHaveBeenCalledWith("spec.facade.port", 9090);
  });

  it("updates replicas and resource fields", () => {
    const updateField = vi.fn();
    render(
      <RuntimeStep formData={baseForm} updateField={updateField} errors={{}} validate={vi.fn()} />,
    );
    fireEvent.change(screen.getByLabelText("Replicas"), { target: { value: "3" } });
    expect(updateField).toHaveBeenCalledWith("replicas", 3);
    fireEvent.change(screen.getByLabelText("CPU Request"), { target: { value: "250m" } });
    expect(updateField).toHaveBeenCalledWith("cpuRequest", "250m");
  });
});
