/**
 * Minimal coverage for SkillSourceDialog. Issue #829.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SkillSourceDialog } from "./skill-source-dialog";

const mockFetch = vi.fn();
global.fetch = mockFetch;

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({
    currentWorkspace: {
      name: "test-workspace",
      permissions: {
        read: true,
        write: true,
        delete: true,
        manageMembers: false,
      },
    },
  })),
}));

function renderDialog(props: Partial<Parameters<typeof SkillSourceDialog>[0]> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <SkillSourceDialog
        open={true}
        onOpenChange={vi.fn()}
        onSuccess={vi.fn()}
        {...props}
      />
    </QueryClientProvider>
  );
}

describe("SkillSourceDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the create form in empty state", () => {
    renderDialog();
    expect(screen.getByText("Create SkillSource")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toHaveValue("");
  });

  it("renders the edit form pre-populated from an existing source", () => {
    renderDialog({
      source: {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "SkillSource",
        metadata: { name: "anthropic-skills" },
        spec: {
          type: "git",
          git: { url: "https://example.com/skills.git" },
          interval: "2h",
        },
      },
    });
    expect(screen.getByText("Edit SkillSource")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toHaveValue("anthropic-skills");
    expect(screen.getByLabelText("Repository URL")).toHaveValue(
      "https://example.com/skills.git"
    );
  });

  it("rejects empty name", async () => {
    renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(screen.getByText("Name is required")).toBeInTheDocument();
    });
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("rejects invalid DNS name", async () => {
    renderDialog();
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "Has Spaces" },
    });
    fireEvent.change(screen.getByLabelText("ConfigMap name"), {
      target: { value: "skills" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(screen.getByText(/must be lowercase alphanumeric/i)).toBeInTheDocument();
    });
  });

  it("submits a configmap spec successfully", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        metadata: { name: "skills-cm" },
        spec: { type: "configmap", interval: "1h", configMap: { name: "cm" } },
      }),
    });
    const onSuccess = vi.fn();
    renderDialog({ onSuccess });

    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "skills-cm" },
    });
    fireEvent.change(screen.getByLabelText("ConfigMap name"), {
      target: { value: "cm" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/skills",
        expect.objectContaining({ method: "POST" })
      );
    });
    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled();
    });
  });

  it("rejects git type without a URL", async () => {
    renderDialog({
      source: {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "SkillSource",
        metadata: { name: "skills-git" },
        spec: { type: "git", git: { url: "" }, interval: "1h" },
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(screen.getByText(/Git repository URL is required/i)).toBeInTheDocument();
    });
  });

  it("rejects oci type without a URL", async () => {
    renderDialog({
      source: {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "SkillSource",
        metadata: { name: "skills-oci" },
        spec: { type: "oci", oci: { url: "" }, interval: "1h" },
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(screen.getByText(/OCI registry URL is required/i)).toBeInTheDocument();
    });
  });

  it("rejects configmap type without a ConfigMap name", async () => {
    renderDialog();
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "skills-cm" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(screen.getByText(/ConfigMap name is required/i)).toBeInTheDocument();
    });
  });

  it("PUTs to the item endpoint when editing an existing source", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        metadata: { name: "skills-cm" },
        spec: { type: "configmap", interval: "2h", configMap: { name: "cm" } },
      }),
    });
    const onSuccess = vi.fn();
    renderDialog({
      onSuccess,
      source: {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "SkillSource",
        metadata: { name: "skills-cm" },
        spec: {
          type: "configmap",
          interval: "1h",
          configMap: { name: "cm" },
        },
      },
    });
    fireEvent.change(screen.getByLabelText("Reconcile interval"), {
      target: { value: "2h" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/skills/skills-cm",
        expect.objectContaining({ method: "PUT" })
      );
    });
  });

  it("surfaces server errors on submit", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      json: async () => ({ error: "Forbidden" }),
    });
    renderDialog();
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "skills-cm" },
    });
    fireEvent.change(screen.getByLabelText("ConfigMap name"), {
      target: { value: "cm" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => {
      expect(screen.getByText("Forbidden")).toBeInTheDocument();
    });
  });

  it("submits a git spec with branch and path when editing", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        metadata: { name: "skills-git" },
        spec: { type: "git", interval: "1h", git: { url: "u", path: "p" } },
      }),
    });
    renderDialog({
      source: {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "SkillSource",
        metadata: { name: "skills-git" },
        spec: {
          type: "git",
          git: {
            url: "https://example.com/skills.git",
            ref: { branch: "main" },
          },
          interval: "1h",
        },
      },
    });
    fireEvent.change(screen.getByLabelText("Branch"), {
      target: { value: "release" },
    });
    fireEvent.change(screen.getByLabelText("Path"), {
      target: { value: "skills/" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/skills/skills-git",
        expect.objectContaining({ method: "PUT" })
      );
    });
    const put = mockFetch.mock.calls[0][1] as { body: string };
    const body = JSON.parse(put.body);
    expect(body.spec.git.ref.branch).toBe("release");
    expect(body.spec.git.path).toBe("skills/");
  });

  it("renders OCI fields and submits with insecure toggle + filter", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({
        metadata: { name: "skills-oci" },
        spec: { type: "oci", interval: "1h", oci: { url: "u" } },
      }),
    });
    renderDialog({
      source: {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "SkillSource",
        metadata: { name: "skills-oci" },
        spec: {
          type: "oci",
          oci: { url: "oci://ghcr.io/org/skills" },
          interval: "1h",
        },
      },
    });
    // Toggle insecure.
    const insecureSwitch = screen.getByLabelText(/insecure/i);
    fireEvent.click(insecureSwitch);

    // Fill filter inputs.
    fireEvent.change(
      screen.getByPlaceholderText("Include: billing/*, ops/*"),
      { target: { value: "billing/*" } }
    );

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalled();
    });
    const put = mockFetch.mock.calls[0][1] as { body: string };
    const body = JSON.parse(put.body);
    expect(body.spec.oci.insecure).toBe(true);
    expect(body.spec.filter.include).toEqual(["billing/*"]);
  });

  it("propagates suspend switch to the submitted spec", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => ({}),
    });
    renderDialog();
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "skills-cm" },
    });
    fireEvent.change(screen.getByLabelText("ConfigMap name"), {
      target: { value: "cm" },
    });
    fireEvent.click(screen.getByLabelText(/Suspend reconciliation/i));
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(mockFetch).toHaveBeenCalled());
    const post = mockFetch.mock.calls[0][1] as { body: string };
    const body = JSON.parse(post.body);
    expect(body.spec.suspend).toBe(true);
  });

  it("pre-populates filter inputs with comma-joined arrays", () => {
    renderDialog({
      source: {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "SkillSource",
        metadata: { name: "skills-filtered" },
        spec: {
          type: "configmap",
          configMap: { name: "cm" },
          interval: "1h",
          filter: {
            include: ["billing/*", "ops/*"],
            exclude: ["**/draft/**"],
            names: ["refund-processing"],
          },
        },
      },
    });
    expect(screen.getByDisplayValue("billing/*, ops/*")).toBeInTheDocument();
    expect(screen.getByDisplayValue("**/draft/**")).toBeInTheDocument();
    expect(screen.getByDisplayValue("refund-processing")).toBeInTheDocument();
  });
});
