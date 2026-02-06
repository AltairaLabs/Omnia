/**
 * Tests for template-source-dialog component
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TemplateSourceDialog } from "./template-source-dialog";
import type { ArenaTemplateSource } from "@/types/arena-template";

// Mock the hook
vi.mock("@/hooks/use-template-sources", () => ({
  useTemplateSourceMutations: vi.fn(),
}));

import { useTemplateSourceMutations } from "@/hooks/use-template-sources";

function createMockSource(overrides: Partial<ArenaTemplateSource> = {}): ArenaTemplateSource {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaTemplateSource",
    metadata: { name: "test-source", namespace: "test-ns" },
    spec: {
      type: "git",
      git: { url: "https://github.com/test/repo", ref: { branch: "main" } },
      syncInterval: "1h",
      templatesPath: "templates/",
    },
    ...overrides,
  };
}

describe("TemplateSourceDialog", () => {
  const mockCreateSource = vi.fn();
  const mockUpdateSource = vi.fn();
  const mockDeleteSource = vi.fn();
  const mockSyncSource = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useTemplateSourceMutations).mockReturnValue({
      createSource: mockCreateSource,
      updateSource: mockUpdateSource,
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: false,
      error: null,
    });
  });

  it("renders create dialog title when no source provided", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );
    expect(screen.getByText("Add Template Source")).toBeInTheDocument();
    expect(screen.getByText("Add a new source for project templates.")).toBeInTheDocument();
  });

  it("renders edit dialog title when source provided", () => {
    const source = createMockSource();
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
        source={source}
      />
    );
    expect(screen.getByText("Edit Template Source")).toBeInTheDocument();
    expect(screen.getByText("Update the template source configuration.")).toBeInTheDocument();
  });

  it("populates form with source data when editing", () => {
    const source = createMockSource({
      metadata: { name: "my-source", namespace: "ns" },
      spec: {
        type: "git",
        git: { url: "https://github.com/org/repo", ref: { branch: "develop" } },
        syncInterval: "6h",
        templatesPath: "custom/path/",
      },
    });
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
        source={source}
      />
    );

    expect(screen.getByDisplayValue("my-source")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://github.com/org/repo")).toBeInTheDocument();
    expect(screen.getByDisplayValue("develop")).toBeInTheDocument();
    expect(screen.getByDisplayValue("custom/path/")).toBeInTheDocument();
  });

  it("disables name field when editing", () => {
    const source = createMockSource();
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
        source={source}
      />
    );

    const nameInput = screen.getByLabelText("Name");
    expect(nameInput).toBeDisabled();
  });

  it("validates required name field", async () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    const createButton = screen.getByRole("button", { name: "Create" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText("Name is required")).toBeInTheDocument();
    });
    expect(mockCreateSource).not.toHaveBeenCalled();
  });

  it("validates name format", async () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "Invalid Name!" } });

    const createButton = screen.getByRole("button", { name: "Create" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText(/must start with a letter/)).toBeInTheDocument();
    });
    expect(mockCreateSource).not.toHaveBeenCalled();
  });

  it("validates git URL when git type selected", async () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-source" } });

    const createButton = screen.getByRole("button", { name: "Create" });
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText("Git repository URL is required")).toBeInTheDocument();
    });
  });

  it("shows git-specific fields when git type selected", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    expect(screen.getByLabelText("Repository URL")).toBeInTheDocument();
    expect(screen.getByLabelText("Branch")).toBeInTheDocument();
  });

  it("shows git-specific fields by default", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    // Git is the default source type
    expect(screen.getByLabelText("Repository URL")).toBeInTheDocument();
    expect(screen.getByLabelText("Branch")).toBeInTheDocument();
  });

  it("has source type selector", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    // Check that source type label exists
    expect(screen.getByText("Source Type")).toBeInTheDocument();
  });

  it("calls createSource on valid form submission", async () => {
    mockCreateSource.mockResolvedValue({});
    const onSuccess = vi.fn();
    const onOpenChange = vi.fn();

    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={onOpenChange}
        onSuccess={onSuccess}
      />
    );

    // Fill form
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-source" } });
    fireEvent.change(screen.getByLabelText("Repository URL"), {
      target: { value: "https://github.com/test/repo" },
    });

    // Submit
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateSource).toHaveBeenCalledWith(
        "my-source",
        expect.objectContaining({
          type: "git",
          git: expect.objectContaining({ url: "https://github.com/test/repo" }),
        })
      );
    });
    expect(onSuccess).toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("calls updateSource when editing", async () => {
    mockUpdateSource.mockResolvedValue({});
    const source = createMockSource();
    const onSuccess = vi.fn();

    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
        source={source}
        onSuccess={onSuccess}
      />
    );

    // Change sync interval
    const syncSelect = screen.getByLabelText("Sync Interval");
    fireEvent.click(syncSelect);
    const option = await screen.findByText("6 hours");
    fireEvent.click(option);

    // Submit
    fireEvent.click(screen.getByRole("button", { name: "Update" }));

    await waitFor(() => {
      expect(mockUpdateSource).toHaveBeenCalledWith(
        "test-source",
        expect.objectContaining({
          syncInterval: "6h",
        })
      );
    });
    expect(onSuccess).toHaveBeenCalled();
  });

  it("shows error message on create failure", async () => {
    mockCreateSource.mockRejectedValue(new Error("Creation failed"));

    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    // Fill form
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-source" } });
    fireEvent.change(screen.getByLabelText("Repository URL"), {
      target: { value: "https://github.com/test/repo" },
    });

    // Submit
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("Creation failed")).toBeInTheDocument();
    });
  });

  it("calls onClose when cancel button clicked", () => {
    const onClose = vi.fn();
    const onOpenChange = vi.fn();

    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={onOpenChange}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("disables buttons when loading", () => {
    vi.mocked(useTemplateSourceMutations).mockReturnValue({
      createSource: mockCreateSource,
      updateSource: mockUpdateSource,
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: true,
      error: null,
    });

    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
  });

  it("shows loading spinner when loading", () => {
    vi.mocked(useTemplateSourceMutations).mockReturnValue({
      createSource: mockCreateSource,
      updateSource: mockUpdateSource,
      deleteSource: mockDeleteSource,
      syncSource: mockSyncSource,
      loading: true,
      error: null,
    });

    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    // Loading spinner should be visible (animate-spin class)
    expect(document.querySelector(".animate-spin")).toBeInTheDocument();
  });

  it("includes templates path in spec", async () => {
    mockCreateSource.mockResolvedValue({});

    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    // Fill form
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-source" } });
    fireEvent.change(screen.getByLabelText("Repository URL"), {
      target: { value: "https://github.com/test/repo" },
    });
    fireEvent.change(screen.getByLabelText("Templates Path"), {
      target: { value: "custom/templates/" },
    });

    // Submit
    fireEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateSource).toHaveBeenCalledWith(
        "my-source",
        expect.objectContaining({
          templatesPath: "custom/templates/",
        })
      );
    });
  });

  it("has templates path input", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    expect(screen.getByLabelText("Templates Path")).toBeInTheDocument();
    expect(screen.getByDisplayValue("templates/")).toBeInTheDocument();
  });

  it("has sync interval selector", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    expect(screen.getByLabelText("Sync Interval")).toBeInTheDocument();
  });

  it("allows changing branch field", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    const branchInput = screen.getByLabelText("Branch");
    fireEvent.change(branchInput, { target: { value: "develop" } });
    expect(branchInput).toHaveValue("develop");
  });

  it("allows changing repository URL field", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    const urlInput = screen.getByLabelText("Repository URL");
    fireEvent.change(urlInput, { target: { value: "https://github.com/new/repo" } });
    expect(urlInput).toHaveValue("https://github.com/new/repo");
  });

  it("allows changing templates path field", () => {
    render(
      <TemplateSourceDialog
        open={true}
        onOpenChange={vi.fn()}
      />
    );

    const pathInput = screen.getByLabelText("Templates Path");
    fireEvent.change(pathInput, { target: { value: "custom/path/" } });
    expect(pathInput).toHaveValue("custom/path/");
  });
});
