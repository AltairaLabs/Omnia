/**
 * Tests for ActivationSection component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ActivationSection } from "./activation-section";
import { useLicense } from "@/hooks/use-license";
import type { License } from "@/types/license";

vi.mock("@/hooks/use-license");

const mockUseLicense = vi.mocked(useLicense);
const mockFetchGlobal = vi.fn();

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

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return Wrapper;
}

function setupEnterpriseLicense() {
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
}

describe("ActivationSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = mockFetchGlobal;
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

    const { container } = render(<ActivationSection />, { wrapper: createWrapper() });
    expect(container).toBeEmptyDOMElement();
  });

  it("should render for enterprise license", async () => {
    setupEnterpriseLicense();

    mockFetchGlobal.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ activations: mockActivations }),
    });

    render(<ActivationSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Cluster Activations")).toBeInTheDocument();
    });
  });

  it("should show empty state when no activations", async () => {
    setupEnterpriseLicense();

    mockFetchGlobal.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ activations: [] }),
    });

    render(<ActivationSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("No clusters activated yet")).toBeInTheDocument();
    });
  });

  it("should display activation list with cluster names", async () => {
    setupEnterpriseLicense();

    mockFetchGlobal.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ activations: mockActivations }),
    });

    render(<ActivationSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("production-us-east")).toBeInTheDocument();
    });
    expect(screen.getByText("staging-eu-west")).toBeInTheDocument();
  });

  it("should show relative time for last seen", async () => {
    setupEnterpriseLicense();

    mockFetchGlobal.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ activations: mockActivations }),
    });

    render(<ActivationSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      // Should show relative time like "5 minutes ago" or "1 hour ago"
      expect(screen.getByText(/minutes? ago/)).toBeInTheDocument();
    });
    expect(screen.getByText(/hour(s)? ago/)).toBeInTheDocument();
  });

  it("should have refresh button", async () => {
    setupEnterpriseLicense();

    mockFetchGlobal.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ activations: mockActivations }),
    });

    render(<ActivationSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Refresh/i })).toBeInTheDocument();
    });
  });

  it("should show deactivate confirmation dialog", async () => {
    setupEnterpriseLicense();

    mockFetchGlobal.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ activations: mockActivations }),
    });

    render(<ActivationSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("production-us-east")).toBeInTheDocument();
    });

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
    setupEnterpriseLicense();

    mockFetchGlobal.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ activations: mockActivations }),
    });

    render(<ActivationSection />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("production-us-east")).toBeInTheDocument();
    });

    // Mock the delete API call
    mockFetchGlobal.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ success: true }),
    });

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
      expect(mockFetchGlobal).toHaveBeenCalledWith(
        "/api/license/activations/cluster-1",
        expect.objectContaining({ method: "DELETE" })
      );
    });
  });
});
