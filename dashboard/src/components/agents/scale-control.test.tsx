import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ScaleControl } from "./scale-control";

// Mock hooks
const mockUseReadOnly = vi.fn(() => ({ isReadOnly: false as boolean, message: "" }));
const mockUsePermissions = vi.fn(() => ({ can: () => true as boolean }));

vi.mock("@/hooks", () => ({
  useReadOnly: () => mockUseReadOnly(),
  usePermissions: () => mockUsePermissions(),
  Permission: { AGENTS_SCALE: "agents.scale" },
}));

describe("ScaleControl", () => {
  const mockOnScale = vi.fn().mockResolvedValue(undefined);

  beforeEach(() => {
    vi.clearAllMocks();
    mockUseReadOnly.mockReturnValue({ isReadOnly: false, message: "" });
    mockUsePermissions.mockReturnValue({ can: () => true });
  });

  it("renders replica count", () => {
    render(
      <ScaleControl
        currentReplicas={2}
        desiredReplicas={2}
        onScale={mockOnScale}
      />
    );

    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("renders scale buttons when not autoscaling", () => {
    render(
      <ScaleControl
        currentReplicas={1}
        desiredReplicas={1}
        onScale={mockOnScale}
      />
    );

    expect(screen.getByText("Scale Down")).toBeInTheDocument();
    expect(screen.getByText("Scale Up")).toBeInTheDocument();
  });

  it("hides scale buttons when autoscaling is enabled", () => {
    render(
      <ScaleControl
        currentReplicas={2}
        desiredReplicas={2}
        autoscalingEnabled={true}
        autoscalingType="hpa"
        onScale={mockOnScale}
      />
    );

    expect(screen.queryByText("Scale Down")).not.toBeInTheDocument();
    expect(screen.queryByText("Scale Up")).not.toBeInTheDocument();
    expect(screen.getByText(/Scaling is managed by/)).toBeInTheDocument();
  });

  it("shows autoscaling indicator when enabled", () => {
    render(
      <ScaleControl
        currentReplicas={2}
        desiredReplicas={2}
        autoscalingEnabled={true}
        autoscalingType="hpa"
        minReplicas={1}
        maxReplicas={10}
        onScale={mockOnScale}
      />
    );

    expect(screen.getByText("HPA")).toBeInTheDocument();
  });

  it("shows KEDA indicator for keda autoscaling", () => {
    render(
      <ScaleControl
        currentReplicas={2}
        desiredReplicas={2}
        autoscalingEnabled={true}
        autoscalingType="keda"
        minReplicas={0}
        maxReplicas={5}
        onScale={mockOnScale}
      />
    );

    expect(screen.getByText("KEDA")).toBeInTheDocument();
  });

  it("renders compact view with autoscaling", () => {
    render(
      <ScaleControl
        currentReplicas={3}
        desiredReplicas={3}
        autoscalingEnabled={true}
        autoscalingType="hpa"
        onScale={mockOnScale}
        compact
      />
    );

    // In compact mode, HPA text is inside tooltip, not directly visible
    // The replica count is split across elements, so check for both parts
    expect(screen.getByText(/3\//)).toBeInTheDocument();
    // Scale buttons should not be visible when autoscaling is enabled
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("renders compact view without autoscaling", () => {
    render(
      <ScaleControl
        currentReplicas={1}
        desiredReplicas={1}
        onScale={mockOnScale}
        compact
      />
    );

    // In compact mode, buttons are icons only
    // The replica count is split across elements, so check for both parts
    expect(screen.getByText(/1\//)).toBeInTheDocument();
  });

  it("respects min and max replicas", () => {
    render(
      <ScaleControl
        currentReplicas={1}
        desiredReplicas={1}
        minReplicas={1}
        maxReplicas={3}
        onScale={mockOnScale}
      />
    );

    const scaleDownButton = screen.getByText("Scale Down").closest("button");
    expect(scaleDownButton).toBeDisabled();
  });

  it("calls onScale when scale up is clicked", async () => {
    const user = userEvent.setup();
    render(
      <ScaleControl
        currentReplicas={1}
        desiredReplicas={1}
        minReplicas={0}
        maxReplicas={10}
        onScale={mockOnScale}
      />
    );

    await user.click(screen.getByText("Scale Up"));

    await waitFor(() => {
      expect(mockOnScale).toHaveBeenCalledWith(2);
    });
  });

  it("shows confirmation dialog when scaling to zero", async () => {
    const user = userEvent.setup();
    render(
      <ScaleControl
        currentReplicas={1}
        desiredReplicas={1}
        minReplicas={0}
        maxReplicas={10}
        onScale={mockOnScale}
      />
    );

    await user.click(screen.getByText("Scale Down"));

    // Should show confirmation dialog
    await waitFor(() => {
      expect(screen.getByText("Scale to Zero?")).toBeInTheDocument();
    });
  });

  it("disables scale buttons when read-only", () => {
    mockUseReadOnly.mockReturnValue({ isReadOnly: true, message: "Read only mode" });

    render(
      <ScaleControl
        currentReplicas={2}
        desiredReplicas={2}
        onScale={mockOnScale}
      />
    );

    const scaleDownButton = screen.getByText("Scale Down").closest("button");
    const scaleUpButton = screen.getByText("Scale Up").closest("button");
    expect(scaleDownButton).toBeDisabled();
    expect(scaleUpButton).toBeDisabled();
  });

  it("disables scale buttons when no permission", () => {
    mockUsePermissions.mockReturnValue({ can: () => false });

    render(
      <ScaleControl
        currentReplicas={2}
        desiredReplicas={2}
        onScale={mockOnScale}
      />
    );

    const scaleDownButton = screen.getByText("Scale Down").closest("button");
    const scaleUpButton = screen.getByText("Scale Up").closest("button");
    expect(scaleDownButton).toBeDisabled();
    expect(scaleUpButton).toBeDisabled();
  });

  it("shows loading state during scaling", async () => {
    // Make onScale return a pending promise
    let resolveScale: () => void;
    const slowScale = new Promise<void>((resolve) => {
      resolveScale = resolve;
    });
    const slowMockOnScale = vi.fn().mockReturnValue(slowScale);

    const user = userEvent.setup();
    render(
      <ScaleControl
        currentReplicas={2}
        desiredReplicas={2}
        minReplicas={0}
        maxReplicas={10}
        onScale={slowMockOnScale}
      />
    );

    await user.click(screen.getByText("Scale Up"));

    // Check that we're in loading state - buttons should be disabled
    await waitFor(() => {
      const scaleUpButton = screen.getByText("Scale Up").closest("button");
      expect(scaleUpButton).toBeDisabled();
    });

    // Resolve the promise
    resolveScale!();
  });

  it("cancels scale to zero from confirmation dialog", async () => {
    const user = userEvent.setup();
    render(
      <ScaleControl
        currentReplicas={1}
        desiredReplicas={1}
        minReplicas={0}
        maxReplicas={10}
        onScale={mockOnScale}
      />
    );

    // Click scale down to trigger confirmation dialog
    await user.click(screen.getByText("Scale Down"));

    // Wait for dialog
    await waitFor(() => {
      expect(screen.getByText("Scale to Zero?")).toBeInTheDocument();
    });

    // Click cancel
    await user.click(screen.getByText("Cancel"));

    // onScale should not have been called
    expect(mockOnScale).not.toHaveBeenCalled();
  });

  it("confirms scale to zero from confirmation dialog", async () => {
    const user = userEvent.setup();
    render(
      <ScaleControl
        currentReplicas={1}
        desiredReplicas={1}
        minReplicas={0}
        maxReplicas={10}
        onScale={mockOnScale}
      />
    );

    // Click scale down to trigger confirmation dialog
    await user.click(screen.getByText("Scale Down"));

    // Wait for dialog
    await waitFor(() => {
      expect(screen.getByText("Scale to Zero?")).toBeInTheDocument();
    });

    // Click confirm - button text is "Scale to Zero"
    await user.click(screen.getByRole("button", { name: "Scale to Zero" }));

    // onScale should have been called with 0
    await waitFor(() => {
      expect(mockOnScale).toHaveBeenCalledWith(0);
    });
  });

  it("displays different replica counts for current vs desired", () => {
    render(
      <ScaleControl
        currentReplicas={1}
        desiredReplicas={3}
        onScale={mockOnScale}
      />
    );

    // Should show current/desired in the display
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText(/3/)).toBeInTheDocument();
  });
});
