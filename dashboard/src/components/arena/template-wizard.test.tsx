/**
 * Tests for template-wizard component
 */
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TemplateWizard } from "./template-wizard";
import type { TemplateMetadata } from "@/types/arena-template";

function createMockTemplate(overrides: Partial<TemplateMetadata> = {}): TemplateMetadata {
  return {
    name: "basic-chatbot",
    displayName: "Basic Chatbot",
    description: "A simple chatbot template for getting started",
    version: "1.0.0",
    category: "chatbot",
    tags: ["mock", "beginner"],
    variables: [
      { name: "provider", type: "enum", options: ["mock", "openai", "anthropic"], default: "mock" },
      { name: "temperature", type: "number", default: "0.7", min: "0", max: "2" },
      { name: "streaming", type: "boolean", default: "true" },
    ],
    path: "templates/basic-chatbot",
    ...overrides,
  };
}

describe("TemplateWizard", () => {
  const mockOnSubmit = vi.fn();
  const mockOnPreview = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockOnSubmit.mockResolvedValue({ projectId: "test-project-123" });
    mockOnPreview.mockResolvedValue({
      files: [{ path: "config.yaml", content: "name: test" }],
    });
  });

  it("renders template header information", () => {
    const template = createMockTemplate();
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    // Check that key header elements are present
    expect(screen.getAllByText("Basic Chatbot").length).toBeGreaterThan(0);
    // Description may appear in multiple places (header and textarea)
    expect(screen.getAllByText(/chatbot template/i).length).toBeGreaterThan(0);
  });

  it("shows source name badge", () => {
    const template = createMockTemplate();
    render(
      <TemplateWizard
        template={template}
        sourceName="community"
        onSubmit={mockOnSubmit}
      />
    );

    expect(screen.getAllByText("community").length).toBeGreaterThan(0);
  });

  it("shows version badge when version set", () => {
    const template = createMockTemplate({ version: "2.0.0" });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    expect(screen.getByText("v2.0.0")).toBeInTheDocument();
  });

  it("shows project name input on first step", () => {
    const template = createMockTemplate();
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    expect(screen.getByLabelText(/Project Name/)).toBeInTheDocument();
    expect(screen.getByPlaceholderText("my-project")).toBeInTheDocument();
  });

  it("shows template variables section", () => {
    const template = createMockTemplate();
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    expect(screen.getByText("Template Variables")).toBeInTheDocument();
    expect(screen.getByLabelText("provider")).toBeInTheDocument();
    expect(screen.getByLabelText("temperature")).toBeInTheDocument();
  });

  it("renders number variable as number input", () => {
    const template = createMockTemplate();
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    const tempInput = screen.getByLabelText("temperature");
    expect(tempInput).toHaveAttribute("type", "number");
  });

  it("renders boolean variable as switch", () => {
    const template = createMockTemplate();
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    const streamingSwitch = screen.getByRole("switch");
    expect(streamingSwitch).toBeInTheDocument();
  });

  it("disables next button when project name is empty", () => {
    const template = createMockTemplate({ variables: [] });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Next button should be disabled when validation fails
    const nextButton = screen.getByRole("button", { name: /next/i });
    expect(nextButton).toBeDisabled();
  });

  it("enables next button when project name is valid", () => {
    const template = createMockTemplate({ variables: [] });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Enter valid project name
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });

    // Next button should be enabled
    const nextButton = screen.getByRole("button", { name: /next/i });
    expect(nextButton).not.toBeDisabled();
  });

  it("calls onPreview when advancing to preview step", async () => {
    const template = createMockTemplate({ variables: [] });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Fill project name
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });

    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(mockOnPreview).toHaveBeenCalledWith(
        expect.objectContaining({
          variables: expect.objectContaining({ projectName: "my-project" }),
        })
      );
    });
  });

  it("advances to step 2 after preview", async () => {
    const template = createMockTemplate({ variables: [] });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Fill and advance
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByText("Step 2 of 3")).toBeInTheDocument();
    });
  });

  it("shows error when preview fails", async () => {
    const template = createMockTemplate({ variables: [] });
    mockOnPreview.mockRejectedValue(new Error("Preview failed"));

    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByText("Preview failed")).toBeInTheDocument();
    });
  });

  it("navigates back from step 2 to step 1", async () => {
    const template = createMockTemplate({ variables: [] });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Advance to preview
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByText("Step 2 of 3")).toBeInTheDocument();
    });

    // Go back
    fireEvent.click(screen.getByRole("button", { name: /back/i }));

    expect(screen.getByText("Step 1 of 3")).toBeInTheDocument();
  });

  it("calls onClose when cancel clicked on first step", () => {
    const template = createMockTemplate();
    const onClose = vi.fn();

    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onClose).toHaveBeenCalled();
  });

  it("navigates to step 3 (create) after step 2", async () => {
    const template = createMockTemplate({ variables: [] });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Navigate through wizard
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByText("Step 2 of 3")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(screen.getByText("Step 3 of 3")).toBeInTheDocument();
    });

    expect(screen.getByText("Ready to Create")).toBeInTheDocument();
  });

  it("calls onSubmit when create button clicked", async () => {
    const template = createMockTemplate({ variables: [] });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Navigate through wizard
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByText("Step 2 of 3")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByText("Step 3 of 3")).toBeInTheDocument());

    // Click create
    fireEvent.click(screen.getByRole("button", { name: /create project/i }));

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          projectName: "my-project",
        })
      );
    });
  });

  it("calls onSuccess with projectId after successful creation", async () => {
    const template = createMockTemplate({ variables: [] });
    const onSuccess = vi.fn();

    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
        onSuccess={onSuccess}
      />
    );

    // Navigate through wizard
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByText("Step 2 of 3")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByText("Step 3 of 3")).toBeInTheDocument());

    // Click create
    fireEvent.click(screen.getByRole("button", { name: /create project/i }));

    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalledWith("test-project-123");
    });
  });

  it("shows error when creation fails", async () => {
    const template = createMockTemplate({ variables: [] });
    mockOnSubmit.mockRejectedValue(new Error("Creation failed"));

    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Navigate through wizard
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByText("Step 2 of 3")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    await waitFor(() => expect(screen.getByText("Step 3 of 3")).toBeInTheDocument());

    // Click create
    fireEvent.click(screen.getByRole("button", { name: /create project/i }));

    await waitFor(() => {
      expect(screen.getByText("Creation failed")).toBeInTheDocument();
    });
  });

  it("applies default variable values", () => {
    const template = createMockTemplate();
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    // Temperature should have default value 0.7
    const tempInput = screen.getByLabelText("temperature");
    expect(tempInput).toHaveValue(0.7);
  });

  it("applies custom className", () => {
    const template = createMockTemplate();
    const { container } = render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        className="custom-class"
      />
    );

    expect(container.querySelector(".custom-class")).toBeInTheDocument();
  });

  it("disables cancel button when loading", () => {
    const template = createMockTemplate();
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        loading
      />
    );

    expect(screen.getByRole("button", { name: /cancel/i })).toBeDisabled();
  });

  it("shows variable description when present", () => {
    const template = createMockTemplate({
      variables: [
        { name: "myVar", type: "string", description: "This is a helpful description" },
      ],
    });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    expect(screen.getByText("This is a helpful description")).toBeInTheDocument();
  });

  it("renders enum variable as select", () => {
    const template = createMockTemplate({
      variables: [
        { name: "provider", type: "enum", options: ["mock", "openai", "anthropic"], default: "mock" },
      ],
    });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    // Should render a select/combobox for enum
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });

  it("shows required indicator for required variables", () => {
    const template = createMockTemplate({
      variables: [
        { name: "requiredVar", type: "string", required: true },
      ],
    });
    const { container } = render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    // Required variables show asterisk
    expect(container.querySelector(".text-destructive")).toBeInTheDocument();
  });

  it("handles preview with validation errors", async () => {
    const template = createMockTemplate({ variables: [] });
    mockOnPreview.mockResolvedValue({
      files: [{ path: "config.yaml", content: "name: test" }],
      errors: [{ variable: "testVar", message: "Invalid value" }],
    });

    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
        onPreview={mockOnPreview}
      />
    );

    // Fill project name and advance
    fireEvent.change(screen.getByLabelText(/Project Name/), {
      target: { value: "my-project" },
    });
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    await waitFor(() => {
      expect(mockOnPreview).toHaveBeenCalled();
    });
  });

  it("updates variable values when changed", () => {
    const template = createMockTemplate({
      variables: [
        { name: "myNumber", type: "number", default: "5" },
      ],
    });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    const input = screen.getByLabelText("myNumber");
    fireEvent.change(input, { target: { value: "10" } });
    expect(input).toHaveValue(10);
  });

  it("shows default value hint for non-boolean variables", () => {
    const template = createMockTemplate({
      variables: [
        { name: "myString", type: "string", default: "hello" },
      ],
    });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    expect(screen.getByText(/Default: hello/)).toBeInTheDocument();
  });

  it("toggles boolean variable with switch", () => {
    const template = createMockTemplate({
      variables: [
        { name: "enabled", type: "boolean", default: "true" },
      ],
    });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    const switchElement = screen.getByRole("switch");
    expect(switchElement).toHaveAttribute("data-state", "checked");
    fireEvent.click(switchElement);
    expect(switchElement).toHaveAttribute("data-state", "unchecked");
  });

  it("handles string variable without default", () => {
    const template = createMockTemplate({
      variables: [
        { name: "noDefault", type: "string" },
      ],
    });
    render(
      <TemplateWizard
        template={template}
        sourceName="test-source"
        onSubmit={mockOnSubmit}
      />
    );

    const input = screen.getByLabelText("noDefault");
    expect(input).toHaveValue("");
  });
});
