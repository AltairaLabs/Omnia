/**
 * Tests for LicenseSection component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { LicenseSection } from "./license-section";
import { useLicense } from "@/hooks/use-license";
import type { License } from "@/types/license";

vi.mock("@/hooks/use-license");

const mockUseLicense = vi.mocked(useLicense);
const mockRefresh = vi.fn();

function createMockLicense(overrides: Partial<License> = {}): License {
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
    issuedAt: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
    expiresAt: new Date(Date.now() + 335 * 24 * 60 * 60 * 1000).toISOString(),
    ...overrides,
  };
}

function mockEnterpriseLicense(overrides: Partial<License> = {}) {
  mockUseLicense.mockReturnValue({
    license: createMockLicense(overrides),
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
    refresh: mockRefresh,
  });
}

function mockOpenCoreLicense() {
  mockUseLicense.mockReturnValue({
    license: createMockLicense({
      tier: "open-core",
      customer: "Open Core User",
      features: {
        gitSource: false,
        ociSource: false,
        s3Source: false,
        loadTesting: false,
        dataGeneration: false,
        scheduling: false,
        distributedWorkers: false,
      },
      limits: {
        maxScenarios: 10,
        maxWorkerReplicas: 1,
      },
    }),
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
    refresh: mockRefresh,
  });
}

describe("LicenseSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render enterprise license with customer name", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    expect(screen.getByText(/Test Corp/)).toBeInTheDocument();
    expect(screen.getByText("License")).toBeInTheDocument();
  });

  it("should show Active badge for valid enterprise license", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("should show Expired badge for expired license", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense({
        expiresAt: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
      }),
      isLoading: false,
      error: undefined,
      canUseFeature: () => true,
      canUseSourceType: () => true,
      canUseJobType: () => true,
      canUseScheduling: () => true,
      canUseWorkerReplicas: () => true,
      canUseScenarioCount: () => true,
      isExpired: true,
      isEnterprise: true,
      refresh: mockRefresh,
    });

    render(<LicenseSection />);
    expect(screen.getByText("Expired")).toBeInTheDocument();
  });

  it("should show warning badge when expiring within 30 days", () => {
    mockUseLicense.mockReturnValue({
      license: createMockLicense({
        expiresAt: new Date(Date.now() + 15 * 24 * 60 * 60 * 1000).toISOString(),
      }),
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
      refresh: mockRefresh,
    });

    render(<LicenseSection />);
    expect(screen.getByText(/Expires in \d+ days?/)).toBeInTheDocument();
  });

  it("should show open core description for open-core license", () => {
    mockOpenCoreLicense();
    render(<LicenseSection />);

    expect(screen.getByText(/Open Core/)).toBeInTheDocument();
    expect(screen.getByText(/Free for evaluation/)).toBeInTheDocument();
  });

  it("should show enabled features with checkmarks", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    expect(screen.getByText("Git Sources")).toBeInTheDocument();
    expect(screen.getByText("S3 Sources")).toBeInTheDocument();
    expect(screen.getByText("Job Scheduling")).toBeInTheDocument();
  });

  it("should show Unlimited for zero limits", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    const unlimitedElements = screen.getAllByText("Unlimited");
    expect(unlimitedElements.length).toBeGreaterThanOrEqual(2);
  });

  it("should show actual limits for non-zero values", () => {
    mockOpenCoreLicense();
    render(<LicenseSection />);

    expect(screen.getByText("10")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
  });

  it("should show issued and expiry dates for enterprise license", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    expect(screen.getByText("Issued")).toBeInTheDocument();
    expect(screen.getByText("Expires")).toBeInTheDocument();
  });

  it("should render upload license button", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    expect(screen.getByRole("button", { name: /Upload License/i })).toBeInTheDocument();
  });

  it("should open upload dialog when clicking upload button", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    const uploadButton = screen.getByRole("button", { name: /Upload License/i });
    fireEvent.click(uploadButton);

    // Dialog should be open - check for dialog description text which is unique
    expect(screen.getByText(/enterprise license file/i)).toBeInTheDocument();
    expect(screen.getByText(/Drag and drop/i)).toBeInTheDocument();
  });

  it("should handle file selection via input", async () => {
    mockEnterpriseLicense();
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    });

    render(<LicenseSection />);

    // Open dialog
    const uploadButton = screen.getByRole("button", { name: /Upload License/i });
    fireEvent.click(uploadButton);

    // Find the file input
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    expect(fileInput).toBeTruthy();

    // Create a mock file
    const file = new File(["test content"], "test.pem", { type: "application/x-pem-file" });

    // Trigger file selection
    fireEvent.change(fileInput, { target: { files: [file] } });

    // Wait for upload to complete
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        "/api/license",
        expect.objectContaining({ method: "POST" })
      );
    });

    expect(mockRefresh).toHaveBeenCalled();
  });

  it("should handle drag and drop file upload", async () => {
    mockEnterpriseLicense();
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    });

    render(<LicenseSection />);

    // Open dialog
    const uploadButton = screen.getByRole("button", { name: /Upload License/i });
    fireEvent.click(uploadButton);

    // Find the drop zone
    const dropZone = screen.getByRole("region", { name: /Drop zone/i });
    expect(dropZone).toBeTruthy();

    // Create a mock file
    const file = new File(["test content"], "test.pem", { type: "application/x-pem-file" });

    // Simulate drag over
    fireEvent.dragOver(dropZone, {
      dataTransfer: { files: [file] },
    });

    // Simulate drop
    fireEvent.drop(dropZone, {
      dataTransfer: { files: [file] },
    });

    // Wait for upload to complete
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        "/api/license",
        expect.objectContaining({ method: "POST" })
      );
    });

    expect(mockRefresh).toHaveBeenCalled();
  });

  it("should handle upload failure gracefully", async () => {
    mockEnterpriseLicense();
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
    });

    render(<LicenseSection />);

    // Open dialog
    const uploadButton = screen.getByRole("button", { name: /Upload License/i });
    fireEvent.click(uploadButton);

    // Find the file input
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;

    // Create a mock file
    const file = new File(["test content"], "test.pem", { type: "application/x-pem-file" });

    // Trigger file selection
    fireEvent.change(fileInput, { target: { files: [file] } });

    // Wait for fetch to be called
    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalled();
    });

    // Refresh should not be called on failure
    expect(mockRefresh).not.toHaveBeenCalled();
  });

  it("should activate drag state on drag over", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    // Open dialog
    const uploadButton = screen.getByRole("button", { name: /Upload License/i });
    fireEvent.click(uploadButton);

    // Find the drop zone
    const dropZone = screen.getByRole("region", { name: /Drop zone/i });

    // Simulate drag over
    fireEvent.dragOver(dropZone);

    // The drop zone should have active styling (border-primary class)
    expect(dropZone.className).toContain("border-primary");
  });

  it("should deactivate drag state on drag leave", () => {
    mockEnterpriseLicense();
    render(<LicenseSection />);

    // Open dialog
    const uploadButton = screen.getByRole("button", { name: /Upload License/i });
    fireEvent.click(uploadButton);

    // Find the drop zone
    const dropZone = screen.getByRole("region", { name: /Drop zone/i });

    // Simulate drag over then leave
    fireEvent.dragOver(dropZone);
    fireEvent.dragLeave(dropZone);

    // The drop zone should not have active styling
    expect(dropZone.className).not.toContain("border-primary");
  });
});
