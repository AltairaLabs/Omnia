/**
 * Tests for Arena ConfigDialog component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfigDialog } from "./config-dialog";
import type { ArenaConfig, ArenaSource } from "@/types/arena";

// Mock hooks
const mockCreateConfig = vi.fn();
const mockUpdateConfig = vi.fn();

vi.mock("@/hooks/use-arena-configs", () => ({
  useArenaConfigMutations: vi.fn(() => ({
    createConfig: mockCreateConfig,
    updateConfig: mockUpdateConfig,
    loading: false,
    error: null,
  })),
}));

vi.mock("@/hooks/use-arena-source-content", () => ({
  useArenaSourceContent: vi.fn(() => ({
    tree: [],
    fileCount: 0,
    directoryCount: 0,
    loading: false,
    error: null,
    refetch: vi.fn(),
  })),
}));

const mockSources: ArenaSource[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaSource",
    metadata: { name: "git-source", namespace: "default" },
    spec: { type: "git", interval: "5m", git: { url: "https://github.com/org/repo.git" } },
    status: { phase: "Ready" },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaSource",
    metadata: { name: "oci-source", namespace: "default" },
    spec: { type: "oci", interval: "5m", oci: { url: "oci://ghcr.io/org/pkg" } },
    status: { phase: "Ready" },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaSource",
    metadata: { name: "pending-source", namespace: "default" },
    spec: { type: "git", interval: "5m", git: { url: "https://github.com/org/pending.git" } },
    status: { phase: "Pending" },
  },
];

const mockConfig: ArenaConfig = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "ArenaConfig",
  metadata: { name: "test-config", namespace: "default" },
  spec: {
    sourceRef: { name: "git-source" },
    arenaFile: "arena.yaml",
    scenarios: {
      include: ["scenarios/**/*.yaml"],
      exclude: ["scenarios/*-wip.yaml"],
    },
    defaults: {
      temperature: 0.7,
      concurrency: 10,
      timeout: "30s",
    },
  },
  status: { phase: "Ready", scenarioCount: 5 },
};

describe("ConfigDialog", () => {
  const mockOnOpenChange = vi.fn();
  const mockOnSuccess = vi.fn();
  const mockOnClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateConfig.mockReset();
    mockUpdateConfig.mockReset();
  });

  describe("Create mode", () => {
    it("renders create dialog with empty fields", () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      expect(screen.getByRole("heading", { name: "Create Config" })).toBeInTheDocument();
      expect(screen.getByLabelText("Name")).toHaveValue("");
    });

    it("only shows ready sources in dropdown", async () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Click on the source select
      const sourceSelect = screen.getByRole("combobox", { name: /source/i });
      await userEvent.click(sourceSelect);

      // Should show ready sources
      expect(screen.getByText("git-source")).toBeInTheDocument();
      expect(screen.getByText("oci-source")).toBeInTheDocument();

      // Should not show pending source
      expect(screen.queryByText("pending-source")).not.toBeInTheDocument();
    });

    it("shows validation error when name is empty", async () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Select a source first
      const sourceSelect = screen.getByRole("combobox", { name: /source/i });
      await userEvent.click(sourceSelect);
      await userEvent.click(screen.getByText("git-source"));

      // Click Create Config without entering a name
      const createButton = screen.getByRole("button", { name: "Create Config" });
      await userEvent.click(createButton);

      expect(screen.getByText("Name is required")).toBeInTheDocument();
    });

    it("shows validation error when source is not selected", async () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Enter a name but don't select source
      const nameInput = screen.getByLabelText("Name");
      await userEvent.type(nameInput, "new-config");

      const createButton = screen.getByRole("button", { name: "Create Config" });
      await userEvent.click(createButton);

      expect(screen.getByText("Source is required")).toBeInTheDocument();
    });

    it("shows validation error for invalid temperature", async () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Enter name and select source
      const nameInput = screen.getByLabelText("Name");
      await userEvent.type(nameInput, "new-config");

      const sourceSelect = screen.getByRole("combobox", { name: /source/i });
      await userEvent.click(sourceSelect);
      await userEvent.click(screen.getByText("git-source"));

      // Enter invalid temperature
      const temperatureInput = screen.getByLabelText("Temperature");
      await userEvent.type(temperatureInput, "5");

      const createButton = screen.getByRole("button", { name: "Create Config" });
      await userEvent.click(createButton);

      expect(screen.getByText("Temperature must be a number between 0 and 2")).toBeInTheDocument();
    });

    it("creates config successfully with valid data", async () => {
      mockCreateConfig.mockResolvedValueOnce(mockConfig);

      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Enter name
      const nameInput = screen.getByLabelText("Name");
      await userEvent.type(nameInput, "new-config");

      // Select source
      const sourceSelect = screen.getByRole("combobox", { name: /source/i });
      await userEvent.click(sourceSelect);
      await userEvent.click(screen.getByText("git-source"));

      // Click Create Config
      const createButton = screen.getByRole("button", { name: "Create Config" });
      await userEvent.click(createButton);

      await waitFor(() => {
        expect(mockCreateConfig).toHaveBeenCalledWith("new-config", expect.objectContaining({
          sourceRef: { name: "git-source" },
        }));
      });

      expect(mockOnSuccess).toHaveBeenCalled();
    });

    it("creates config with all optional fields", async () => {
      mockCreateConfig.mockResolvedValueOnce(mockConfig);

      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Enter name
      const nameInput = screen.getByLabelText("Name");
      await userEvent.type(nameInput, "new-config");

      // Select source
      const sourceSelect = screen.getByRole("combobox", { name: /source/i });
      await userEvent.click(sourceSelect);
      await userEvent.click(screen.getByText("git-source"));

      // Enter arena file
      const arenaFileInput = screen.getByLabelText(/Arena File Name/);
      await userEvent.type(arenaFileInput, "custom-arena.yaml");

      // Enter temperature
      const temperatureInput = screen.getByLabelText("Temperature");
      await userEvent.type(temperatureInput, "0.8");

      // Enter concurrency
      const concurrencyInput = screen.getByLabelText("Concurrency");
      await userEvent.type(concurrencyInput, "5");

      // Enter timeout
      const timeoutInput = screen.getByLabelText("Timeout");
      await userEvent.type(timeoutInput, "60s");

      // Click Create Config
      const createButton = screen.getByRole("button", { name: "Create Config" });
      await userEvent.click(createButton);

      await waitFor(() => {
        expect(mockCreateConfig).toHaveBeenCalledWith("new-config", expect.objectContaining({
          sourceRef: { name: "git-source" },
          arenaFile: "custom-arena.yaml",
          defaults: expect.objectContaining({
            temperature: 0.8,
            concurrency: 5,
            timeout: "60s",
          }),
        }));
      });
    });
  });

  describe("Edit mode", () => {
    it("renders edit dialog with pre-filled fields", () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={mockConfig}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      expect(screen.getByText("Edit Config")).toBeInTheDocument();
      expect(screen.getByLabelText("Name")).toHaveValue("test-config");
      expect(screen.getByLabelText("Name")).toBeDisabled();
      expect(screen.getByLabelText(/Arena File Name/)).toHaveValue("arena.yaml");
    });

    it("updates config successfully", async () => {
      mockUpdateConfig.mockResolvedValueOnce({
        ...mockConfig,
        spec: { ...mockConfig.spec, defaults: { temperature: 0.5 } },
      });

      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={mockConfig}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Change temperature
      const temperatureInput = screen.getByLabelText("Temperature");
      await userEvent.clear(temperatureInput);
      await userEvent.type(temperatureInput, "0.5");

      // Click Save Changes
      const saveButton = screen.getByRole("button", { name: "Save Changes" });
      await userEvent.click(saveButton);

      await waitFor(() => {
        expect(mockUpdateConfig).toHaveBeenCalledWith("test-config", expect.objectContaining({
          sourceRef: { name: "git-source" },
        }));
      });

      expect(mockOnSuccess).toHaveBeenCalled();
    });
  });

  describe("Error handling", () => {
    it("displays API error message on create failure", async () => {
      mockCreateConfig.mockRejectedValueOnce(new Error("Config already exists"));

      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Enter name and select source
      const nameInput = screen.getByLabelText("Name");
      await userEvent.type(nameInput, "new-config");

      const sourceSelect = screen.getByRole("combobox", { name: /source/i });
      await userEvent.click(sourceSelect);
      await userEvent.click(screen.getByText("git-source"));

      // Click Create Config
      const createButton = screen.getByRole("button", { name: "Create Config" });
      await userEvent.click(createButton);

      await waitFor(() => {
        expect(screen.getByText("Config already exists")).toBeInTheDocument();
      });
    });
  });

  describe("Cancel functionality", () => {
    it("closes dialog when Cancel is clicked", async () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={mockSources}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      const cancelButton = screen.getByRole("button", { name: "Cancel" });
      await userEvent.click(cancelButton);

      expect(mockOnClose).toHaveBeenCalled();
      expect(mockOnOpenChange).toHaveBeenCalledWith(false);
    });
  });

  describe("No sources available", () => {
    it("shows message when no ready sources are available", async () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={[mockSources[2]]} // Only pending source
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Click on the source select
      const sourceSelect = screen.getByRole("combobox", { name: /source/i });
      await userEvent.click(sourceSelect);

      // Should show the no ready sources message
      expect(screen.getByText("No ready sources available")).toBeInTheDocument();
    });

    it("shows message when sources array is empty", async () => {
      render(
        <ConfigDialog
          open={true}
          onOpenChange={mockOnOpenChange}
          config={null}
          sources={[]}
          onSuccess={mockOnSuccess}
          onClose={mockOnClose}
        />
      );

      // Click on the source select
      const sourceSelect = screen.getByRole("combobox", { name: /source/i });
      await userEvent.click(sourceSelect);

      // Should show the no ready sources message
      expect(screen.getByText("No ready sources available")).toBeInTheDocument();
    });
  });
});
