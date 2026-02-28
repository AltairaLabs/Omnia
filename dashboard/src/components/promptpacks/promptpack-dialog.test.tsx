import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PromptPackDialog } from "./promptpack-dialog";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace", namespace: "test-ns" },
    workspaces: [{ name: "test-workspace", namespace: "test-ns" }],
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

// Mock mutations hook
const mockCreatePromptPack = vi.fn();

vi.mock("@/hooks/use-promptpack-mutations", () => ({
  usePromptPackMutations: () => ({
    createPromptPack: mockCreatePromptPack,
    loading: false,
    error: null,
  }),
}));

describe("PromptPackDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreatePromptPack.mockResolvedValue({
      metadata: { name: "test-pack" },
      spec: {},
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders dialog when open", () => {
    render(
      <PromptPackDialog open={true} onOpenChange={vi.fn()} />
    );

    expect(
      screen.getByRole("heading", { name: "Create PromptPack" })
    ).toBeInTheDocument();
  });

  it("does not render dialog when closed", () => {
    render(
      <PromptPackDialog open={false} onOpenChange={vi.fn()} />
    );

    expect(
      screen.queryByText("Create PromptPack")
    ).not.toBeInTheDocument();
  });

  it("closes dialog on cancel", () => {
    const onOpenChange = vi.fn();
    render(
      <PromptPackDialog open={true} onOpenChange={onOpenChange} />
    );

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("validates required name field", async () => {
    vi.useRealTimers();
    render(
      <PromptPackDialog open={true} onOpenChange={vi.fn()} />
    );

    fireEvent.click(
      screen.getByRole("button", { name: /create promptpack/i })
    );

    await waitFor(() => {
      expect(screen.getByText("Name is required")).toBeInTheDocument();
    });
    expect(mockCreatePromptPack).not.toHaveBeenCalled();
  });

  it("validates DNS name format", async () => {
    vi.useRealTimers();
    const user = userEvent.setup();
    render(
      <PromptPackDialog open={true} onOpenChange={vi.fn()} />
    );

    await user.type(screen.getByLabelText("Name"), "Invalid Name!");

    fireEvent.click(
      screen.getByRole("button", { name: /create promptpack/i })
    );

    await waitFor(() => {
      expect(
        screen.getByText(/must be a valid DNS subdomain/i)
      ).toBeInTheDocument();
    });
  });

  it("validates version format", async () => {
    vi.useRealTimers();
    const user = userEvent.setup();
    render(
      <PromptPackDialog open={true} onOpenChange={vi.fn()} />
    );

    await user.type(screen.getByLabelText("Name"), "my-pack");
    await user.type(screen.getByLabelText("ConfigMap Reference"), "my-configmap");
    await user.type(screen.getByLabelText("Version"), "invalid");

    fireEvent.click(
      screen.getByRole("button", { name: /create promptpack/i })
    );

    await waitFor(() => {
      expect(
        screen.getByText(/version must be valid semver/i)
      ).toBeInTheDocument();
    });
  });

  it("creates PromptPack with immediate rollout", async () => {
    vi.useRealTimers();
    const user = userEvent.setup();
    const onSuccess = vi.fn();
    const onOpenChange = vi.fn();

    render(
      <PromptPackDialog
        open={true}
        onOpenChange={onOpenChange}
        onSuccess={onSuccess}
      />
    );

    await user.type(screen.getByLabelText("Name"), "my-pack");
    await user.type(screen.getByLabelText("ConfigMap Reference"), "my-configmap");
    await user.type(screen.getByLabelText("Version"), "1.0.0");

    fireEvent.click(
      screen.getByRole("button", { name: /create promptpack/i })
    );

    await waitFor(() => {
      expect(mockCreatePromptPack).toHaveBeenCalledWith(
        "my-pack",
        expect.objectContaining({
          source: {
            type: "configmap",
            configMapRef: { name: "my-configmap" },
          },
          version: "1.0.0",
          rollout: { type: "immediate" },
        })
      );
    });

    expect(onSuccess).toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("creates PromptPack with canary rollout", async () => {
    vi.useRealTimers();
    const user = userEvent.setup();

    render(
      <PromptPackDialog open={true} onOpenChange={vi.fn()} />
    );

    await user.type(screen.getByLabelText("Name"), "canary-pack");
    await user.type(screen.getByLabelText("ConfigMap Reference"), "my-config");
    await user.type(screen.getByLabelText("Version"), "2.0.0");

    // Switch to canary
    fireEvent.click(screen.getByLabelText("Canary"));

    fireEvent.click(
      screen.getByRole("button", { name: /create promptpack/i })
    );

    await waitFor(() => {
      expect(mockCreatePromptPack).toHaveBeenCalledWith(
        "canary-pack",
        expect.objectContaining({
          version: "2.0.0",
          rollout: {
            type: "canary",
            canary: {
              weight: 10,
              stepWeight: 10,
              interval: "5m",
            },
          },
        })
      );
    });
  });

  it("shows canary config fields only when canary is selected", () => {
    render(
      <PromptPackDialog open={true} onOpenChange={vi.fn()} />
    );

    // Canary config should not be visible by default (immediate)
    expect(screen.queryByText("Canary Configuration")).not.toBeInTheDocument();

    // Switch to canary
    fireEvent.click(screen.getByLabelText("Canary"));

    expect(screen.getByText("Canary Configuration")).toBeInTheDocument();
  });

  it("shows error on mutation failure", async () => {
    vi.useRealTimers();
    const user = userEvent.setup();
    mockCreatePromptPack.mockRejectedValue(new Error("API error: conflict"));

    render(
      <PromptPackDialog open={true} onOpenChange={vi.fn()} />
    );

    await user.type(screen.getByLabelText("Name"), "my-pack");
    await user.type(screen.getByLabelText("ConfigMap Reference"), "my-config");
    await user.type(screen.getByLabelText("Version"), "1.0.0");

    fireEvent.click(
      screen.getByRole("button", { name: /create promptpack/i })
    );

    await waitFor(() => {
      expect(screen.getByText("API error: conflict")).toBeInTheDocument();
    });
  });

  it("strips v prefix from version", async () => {
    vi.useRealTimers();
    const user = userEvent.setup();

    render(
      <PromptPackDialog open={true} onOpenChange={vi.fn()} />
    );

    await user.type(screen.getByLabelText("Name"), "my-pack");
    await user.type(screen.getByLabelText("ConfigMap Reference"), "my-config");
    await user.type(screen.getByLabelText("Version"), "v1.2.3");

    fireEvent.click(
      screen.getByRole("button", { name: /create promptpack/i })
    );

    await waitFor(() => {
      expect(mockCreatePromptPack).toHaveBeenCalledWith(
        "my-pack",
        expect.objectContaining({
          version: "1.2.3",
        })
      );
    });
  });
});
