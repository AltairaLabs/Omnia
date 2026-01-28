import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DeployButton } from "./deploy-button";

// Mock deployment hook
const mockDeploy = vi.fn();
const mockRefetch = vi.fn();
let mockStatus: unknown = null;
let mockLoading = false;
let mockDeploying = false;

vi.mock("@/hooks/use-project-deployment", () => ({
  useProjectDeployment: () => ({
    status: mockStatus,
    loading: mockLoading,
    deploying: mockDeploying,
    deploy: mockDeploy,
    refetch: mockRefetch,
  }),
}));

// Mock toast
const mockToast = vi.fn();
vi.mock("@/hooks/use-toast", () => ({
  useToast: () => ({ toast: mockToast }),
}));

describe("DeployButton", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStatus = null;
    mockLoading = false;
    mockDeploying = false;
  });

  it("should render deploy button", () => {
    render(<DeployButton projectId="test-project" />);

    expect(screen.getByRole("button")).toBeInTheDocument();
    expect(screen.getByText("Deploy")).toBeInTheDocument();
  });

  it("should disable button when projectId is undefined", () => {
    render(<DeployButton projectId={undefined} />);

    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
  });

  it("should disable button when disabled prop is true", () => {
    render(<DeployButton projectId="test-project" disabled />);

    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
  });

  it("should disable button while deploying", () => {
    mockDeploying = true;
    render(<DeployButton projectId="test-project" />);

    const button = screen.getByRole("button");
    expect(button).toBeDisabled();
  });

  it("should show dropdown menu when clicked", async () => {
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    const button = screen.getByRole("button");
    await user.click(button);

    expect(screen.getByText("Quick Deploy")).toBeInTheDocument();
    expect(screen.getByText("Deploy with Options...")).toBeInTheDocument();
    expect(screen.getByText("Refresh Status")).toBeInTheDocument();
  });

  it("should call deploy when Quick Deploy is clicked", async () => {
    mockDeploy.mockResolvedValueOnce({
      isNew: true,
      source: { metadata: { name: "test-source" } },
    });
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Quick Deploy"));

    expect(mockDeploy).toHaveBeenCalled();
  });

  it("should show toast on successful deploy", async () => {
    mockDeploy.mockResolvedValueOnce({
      isNew: true,
      source: { metadata: { name: "test-source" } },
    });
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Quick Deploy"));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Deployed",
      })
    );
  });

  it("should show toast on deploy error", async () => {
    mockDeploy.mockRejectedValueOnce(new Error("Deploy failed"));
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Quick Deploy"));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Deploy Failed",
        variant: "destructive",
      })
    );
  });

  it("should apply custom className", () => {
    const { container } = render(
      <DeployButton projectId="test-project" className="custom-class" />
    );

    // The className is applied to the button inside the dropdown trigger
    const button = container.querySelector("button");
    expect(button).toHaveClass("custom-class");
  });

  it("should show redeployed message when isNew is false", async () => {
    mockDeploy.mockResolvedValueOnce({
      isNew: false,
      source: { metadata: { name: "existing-source" } },
    });
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Quick Deploy"));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Redeployed",
      })
    );
  });

  it("should call refetch when Refresh Status is clicked", async () => {
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Refresh Status"));

    expect(mockRefetch).toHaveBeenCalled();
  });

  it("should show deploy status indicator when deployed and Ready", () => {
    mockStatus = {
      deployed: true,
      source: {
        metadata: { name: "test-source" },
        spec: { interval: "5m" },
        status: { phase: "Ready" },
      },
    };
    render(<DeployButton projectId="test-project" />);

    // Check for the green check circle (deployed and ready)
    const button = screen.getByRole("button");
    expect(button.querySelector(".text-green-500")).toBeInTheDocument();
  });

  it("should show deploy status indicator when deployed and Failed", () => {
    mockStatus = {
      deployed: true,
      source: {
        metadata: { name: "test-source" },
        spec: { interval: "5m" },
        status: { phase: "Failed" },
      },
    };
    render(<DeployButton projectId="test-project" />);

    // Check for the red x circle (failed)
    const button = screen.getByRole("button");
    expect(button.querySelector(".text-red-500")).toBeInTheDocument();
  });

  it("should show deploy status indicator when deployed with unknown phase", () => {
    mockStatus = {
      deployed: true,
      source: {
        metadata: { name: "test-source" },
        spec: { interval: "5m" },
        status: { phase: "Pending" },
      },
    };
    render(<DeployButton projectId="test-project" />);

    // Check for the yellow clock (pending)
    const button = screen.getByRole("button");
    expect(button.querySelector(".text-yellow-500")).toBeInTheDocument();
  });

  it("should not show status indicator when loading", () => {
    mockLoading = true;
    mockStatus = {
      deployed: true,
      source: {
        metadata: { name: "test-source" },
        spec: { interval: "5m" },
        status: { phase: "Ready" },
      },
    };
    render(<DeployButton projectId="test-project" />);

    // Should not show any status icons
    const button = screen.getByRole("button");
    expect(button.querySelector(".text-green-500")).not.toBeInTheDocument();
    expect(button.querySelector(".text-red-500")).not.toBeInTheDocument();
    expect(button.querySelector(".text-yellow-500")).not.toBeInTheDocument();
  });

  it("should not show status indicator when not deployed", () => {
    mockStatus = {
      deployed: false,
    };
    render(<DeployButton projectId="test-project" />);

    const button = screen.getByRole("button");
    expect(button.querySelector(".text-green-500")).not.toBeInTheDocument();
  });

  it("should open advanced deploy dialog", async () => {
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Deploy with Options..."));

    expect(screen.getByText("Deploy Project")).toBeInTheDocument();
    expect(screen.getByLabelText("Source Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Sync Interval")).toBeInTheDocument();
  });

  it("should submit advanced deploy form", async () => {
    mockDeploy.mockResolvedValueOnce({
      isNew: true,
      source: { metadata: { name: "custom-source" } },
    });
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Deploy with Options..."));

    // Fill in the form
    const nameInput = screen.getByLabelText("Source Name");
    const intervalInput = screen.getByLabelText("Sync Interval");
    await user.clear(nameInput);
    await user.type(nameInput, "custom-source");
    await user.clear(intervalInput);
    await user.type(intervalInput, "10m");

    // Submit
    await user.click(screen.getByRole("button", { name: /deploy/i }));

    expect(mockDeploy).toHaveBeenCalledWith({
      name: "custom-source",
      syncInterval: "10m",
    });
  });

  it("should show error toast on advanced deploy failure", async () => {
    mockDeploy.mockRejectedValueOnce(new Error("Advanced deploy failed"));
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Deploy with Options..."));

    // Submit without changing defaults
    await user.click(screen.getByRole("button", { name: /deploy/i }));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Deploy Failed",
        variant: "destructive",
      })
    );
  });

  it("should close dialog and show toast on successful advanced deploy", async () => {
    mockDeploy.mockResolvedValueOnce({
      isNew: false,
      source: { metadata: { name: "updated-source" } },
    });
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Deploy with Options..."));

    // Submit
    await user.click(screen.getByRole("button", { name: /deploy/i }));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Redeployed",
      })
    );
    // Dialog should close
    expect(screen.queryByText("Deploy Project")).not.toBeInTheDocument();
  });

  it("should show currently deployed info in advanced dialog", async () => {
    mockStatus = {
      deployed: true,
      source: {
        metadata: { name: "existing-source" },
        spec: { interval: "2m" },
        status: { phase: "Ready" },
      },
      configMap: { fileCount: 5 },
    };
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Deploy with Options..."));

    expect(screen.getByText("Currently Deployed")).toBeInTheDocument();
    expect(screen.getByText(/Source: existing-source/)).toBeInTheDocument();
    expect(screen.getByText(/Phase: Ready/)).toBeInTheDocument();
    expect(screen.getByText(/Files: 5/)).toBeInTheDocument();
  });

  it("should close dialog when Cancel is clicked", async () => {
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Deploy with Options..."));

    expect(screen.getByText("Deploy Project")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByText("Deploy Project")).not.toBeInTheDocument();
  });

  it("should show Redeploy button text when already deployed", async () => {
    mockStatus = {
      deployed: true,
      source: {
        metadata: { name: "existing-source" },
        spec: { interval: "5m" },
        status: { phase: "Ready" },
      },
    };
    const user = userEvent.setup();
    render(<DeployButton projectId="test-project" />);

    await user.click(screen.getByRole("button"));
    await user.click(screen.getByText("Deploy with Options..."));

    expect(screen.getByRole("button", { name: "Redeploy" })).toBeInTheDocument();
  });

  it("should not call deploy when projectId is undefined (quick deploy)", () => {
    // When projectId is undefined, the button is disabled and deploy cannot be called
    render(<DeployButton projectId={undefined} />);

    // Deploy should not have been called since button is disabled
    expect(mockDeploy).not.toHaveBeenCalled();
  });
});
