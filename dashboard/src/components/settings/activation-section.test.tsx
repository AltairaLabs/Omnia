/**
 * Tests for ActivationSection component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ActivationSection } from "./activation-section";
import { useLicense } from "@/hooks/use-license";
import type { License } from "@/types/license";

vi.mock("@/hooks/use-license");
vi.mock("swr", () => ({
  default: vi.fn(),
}));

import useSWR from "swr";

const mockUseLicense = vi.mocked(useLicense);
const mockUseSWR = vi.mocked(useSWR);

function createMockLicense(): License {
  return {
    id: "test-license",
    tier: "enterprise",
    customer: "Test Corp",
    features: {
      gitSource: true,
      ociSource: true,
      s3Source: true,
      loadTesting: true,
      dataGeneration: true,
      scheduling: true,
      distributedWorkers: true,
    },
    limits: {
      maxScenarios: 0,
      maxWorkerReplicas: 0,
    },
    issuedAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
  };
}

const mockActivations = [
  {
    fingerprint: "cluster-1",
    clusterName: "production-us-east",
    activatedAt: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
    lastSeen: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
  },
  {
    fingerprint: "cluster-2",
    clusterName: "staging-eu-west",
    activatedAt: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
    lastSeen: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
  },
];

describe("ActivationSection", () => {
  const mockMutate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = vi.fn();
  });

  it("should not render for non-enterprise license", () => {
    mockUseLicense.mockReturnValue({
      license: { ...createMockLicense(), tier: "open-core" },
      isLoading: false,
      error: undefined,
      canUseFeature: () => false,
      canUseSourceType: () => false,
      canUseJobType: () => false,
      canUseScheduling: () => false,
      canUseWorkerReplicas: () => false,
      canUseScenarioCount: () => false,
      isExpired: false,
      isEnterprise: false,
      refresh: vi.fn(),
    });

    mockUseSWR.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: undefined,
      mutate: mockMutate,
      isValidating: false,
    } as unknown as ReturnType<typeof useSWR>);

    const { container } = render(<ActivationSection />);
    expect(container).toBeEmptyDOMElement();
  });

  it("should render for enterprise license", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense(),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    mockUseSWR.mockReturnValue({
      data: { activations: mockActivations },
      isLoading: false,
      error: undefined,
      mutate: mockMutate,
      isValidating: false,
    } as unknown as ReturnType<typeof useSWR>);

    render(<ActivationSection />);
    expect(screen.getByText("Cluster Activations")).toBeInTheDocument();
  });

  it("should show empty state when no activations", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense(),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    mockUseSWR.mockReturnValue({
      data: { activations: [] },
      isLoading: false,
      error: undefined,
      mutate: mockMutate,
      isValidating: false,
    } as unknown as ReturnType<typeof useSWR>);

    render(<ActivationSection />);
    expect(screen.getByText("No clusters activated yet")).toBeInTheDocument();
  });

  it("should display activation list with cluster names", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense(),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    mockUseSWR.mockReturnValue({
      data: { activations: mockActivations },
      isLoading: false,
      error: undefined,
      mutate: mockMutate,
      isValidating: false,
    } as unknown as ReturnType<typeof useSWR>);

    render(<ActivationSection />);

    expect(screen.getByText("production-us-east")).toBeInTheDocument();
    expect(screen.getByText("staging-eu-west")).toBeInTheDocument();
  });

  it("should show relative time for last seen", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense(),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    mockUseSWR.mockReturnValue({
      data: { activations: mockActivations },
      isLoading: false,
      error: undefined,
      mutate: mockMutate,
      isValidating: false,
    } as unknown as ReturnType<typeof useSWR>);

    render(<ActivationSection />);

    // Should show relative time like "5 minutes ago" or "1 hour ago"
    expect(screen.getByText(/minutes? ago/)).toBeInTheDocument();
    expect(screen.getByText(/hour(s)? ago/)).toBeInTheDocument();
  });

  it("should have refresh button that triggers mutate", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense(),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    mockUseSWR.mockReturnValue({
      data: { activations: mockActivations },
      isLoading: false,
      error: undefined,
      mutate: mockMutate,
      isValidating: false,
    } as unknown as ReturnType<typeof useSWR>);

    render(<ActivationSection />);

    const refreshButton = screen.getByRole("button", { name: /Refresh/i });
    fireEvent.click(refreshButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should show deactivate confirmation dialog", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense(),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    mockUseSWR.mockReturnValue({
      data: { activations: mockActivations },
      isLoading: false,
      error: undefined,
      mutate: mockMutate,
      isValidating: false,
    } as unknown as ReturnType<typeof useSWR>);

    render(<ActivationSection />);

    // Find and click the delete button for first activation
    const deleteButtons = screen.getAllByRole("button", { name: "" });
    const trashButton = deleteButtons.find(btn =>
      btn.querySelector('svg.lucide-trash-2')
    );
    if (trashButton) {
      fireEvent.click(trashButton);
    }

    expect(screen.getByText("Deactivate Cluster")).toBeInTheDocument();
    // Dialog text should mention the cluster name
    expect(screen.getByText(/free up a license slot/i)).toBeInTheDocument();
  });

  it("should call deactivate API when confirmed", async () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense(),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: false,
      isEnterprise: true,
      refresh: vi.fn(),
    });

    mockUseSWR.mockReturnValue({
      data: { activations: mockActivations },
      isLoading: false,
      error: undefined,
      mutate: mockMutate,
      isValidating: false,
    } as unknown as ReturnType<typeof useSWR>);

    (global.fetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ success: true }),
    });

    render(<ActivationSection />);

    // Click trash button to open dialog
    const deleteButtons = screen.getAllByRole("button", { name: "" });
    const trashButton = deleteButtons.find(btn =>
      btn.querySelector('svg.lucide-trash-2')
    );
    if (trashButton) {
      fireEvent.click(trashButton);
    }

    // Click the deactivate button in dialog
    const deactivateButton = screen.getByRole("button", { name: /^Deactivate$/i });
    fireEvent.click(deactivateButton);

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        "/api/license/activations/cluster-1",
        expect.objectContaining({ method: "DELETE" })
      );
    });
  });
});
