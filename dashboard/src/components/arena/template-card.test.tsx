/**
 * Tests for template-card component
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TemplateCard } from "./template-card";
import type { TemplateMetadata } from "@/types/arena-template";

// Test data factory
function createMockTemplate(overrides: Partial<TemplateMetadata> = {}): TemplateMetadata {
  return {
    name: "basic-chatbot",
    displayName: "Basic Chatbot",
    description: "A simple chatbot template for getting started",
    version: "1.0.0",
    category: "chatbot",
    tags: ["mock", "beginner", "starter"],
    variables: [
      { name: "projectName", type: "string", required: true },
      { name: "provider", type: "enum", options: ["mock", "openai"] },
    ],
    path: "templates/basic-chatbot",
    ...overrides,
  };
}

describe("TemplateCard", () => {
  it("renders template name", () => {
    const template = createMockTemplate();
    render(<TemplateCard template={template} />);
    expect(screen.getByText("Basic Chatbot")).toBeInTheDocument();
  });

  it("falls back to name when displayName is not set", () => {
    const template = createMockTemplate({ displayName: undefined });
    render(<TemplateCard template={template} />);
    expect(screen.getByText("basic-chatbot")).toBeInTheDocument();
  });

  it("renders version badge when version is set", () => {
    const template = createMockTemplate({ version: "2.1.0" });
    render(<TemplateCard template={template} />);
    expect(screen.getByText("v2.1.0")).toBeInTheDocument();
  });

  it("does not render version badge when version is not set", () => {
    const template = createMockTemplate({ version: undefined });
    render(<TemplateCard template={template} />);
    expect(screen.queryByText(/^v\d/)).not.toBeInTheDocument();
  });

  it("renders category badge when category is set", () => {
    const template = createMockTemplate({ category: "agent" });
    render(<TemplateCard template={template} />);
    expect(screen.getByText("agent")).toBeInTheDocument();
  });

  it("does not render category badge when category is not set", () => {
    const template = createMockTemplate({ category: undefined });
    render(<TemplateCard template={template} />);
    expect(screen.queryByText("chatbot")).not.toBeInTheDocument();
    expect(screen.queryByText("agent")).not.toBeInTheDocument();
  });

  it("renders description when set", () => {
    const template = createMockTemplate({ description: "Test description" });
    render(<TemplateCard template={template} />);
    expect(screen.getByText("Test description")).toBeInTheDocument();
  });

  it("renders tags when provided", () => {
    const template = createMockTemplate({ tags: ["tag1", "tag2"] });
    render(<TemplateCard template={template} />);
    expect(screen.getByText("tag1")).toBeInTheDocument();
    expect(screen.getByText("tag2")).toBeInTheDocument();
  });

  it("limits displayed tags to 4", () => {
    const template = createMockTemplate({ tags: ["t1", "t2", "t3", "t4", "t5", "t6"] });
    render(<TemplateCard template={template} />);
    expect(screen.getByText("t1")).toBeInTheDocument();
    expect(screen.getByText("t4")).toBeInTheDocument();
    expect(screen.getByText("+2")).toBeInTheDocument();
    expect(screen.queryByText("t5")).not.toBeInTheDocument();
  });

  it("renders variable count when variables exist", () => {
    const template = createMockTemplate({
      variables: [
        { name: "v1", type: "string" },
        { name: "v2", type: "number" },
        { name: "v3", type: "boolean" },
      ],
    });
    render(<TemplateCard template={template} />);
    expect(screen.getByText("3 variables")).toBeInTheDocument();
  });

  it("renders singular variable text for one variable", () => {
    const template = createMockTemplate({
      variables: [{ name: "v1", type: "string" }],
    });
    render(<TemplateCard template={template} />);
    expect(screen.getByText("1 variable")).toBeInTheDocument();
  });

  it("does not render variable count when no variables", () => {
    const template = createMockTemplate({ variables: undefined });
    render(<TemplateCard template={template} />);
    expect(screen.queryByText(/variable/)).not.toBeInTheDocument();
  });

  it("renders source name when provided", () => {
    const template = createMockTemplate();
    render(<TemplateCard template={template} sourceName="community" />);
    expect(screen.getByText("community")).toBeInTheDocument();
  });

  it("calls onSelect when card is clicked", () => {
    const template = createMockTemplate();
    const onSelect = vi.fn();
    render(<TemplateCard template={template} onSelect={onSelect} />);

    fireEvent.click(screen.getByText("Basic Chatbot"));
    expect(onSelect).toHaveBeenCalledTimes(1);
  });

  it("renders Use Template button when onUse is provided", () => {
    const template = createMockTemplate();
    const onUse = vi.fn();
    render(<TemplateCard template={template} onUse={onUse} />);
    expect(screen.getByRole("button", { name: /use template/i })).toBeInTheDocument();
  });

  it("calls onUse when Use Template button is clicked", () => {
    const template = createMockTemplate();
    const onUse = vi.fn();
    const onSelect = vi.fn();
    render(<TemplateCard template={template} onSelect={onSelect} onUse={onUse} />);

    fireEvent.click(screen.getByRole("button", { name: /use template/i }));
    expect(onUse).toHaveBeenCalledTimes(1);
    // Should not propagate to onSelect
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("does not render Use Template button when onUse is not provided", () => {
    const template = createMockTemplate();
    render(<TemplateCard template={template} />);
    expect(screen.queryByRole("button", { name: /use template/i })).not.toBeInTheDocument();
  });

  it("applies selected styling when selected", () => {
    const template = createMockTemplate();
    const { container } = render(<TemplateCard template={template} selected />);
    const card = container.querySelector("[class*='ring-2']");
    expect(card).not.toBeNull();
  });

  it("applies custom className", () => {
    const template = createMockTemplate();
    const { container } = render(<TemplateCard template={template} className="custom-class" />);
    const card = container.querySelector(".custom-class");
    expect(card).not.toBeNull();
  });

  it("applies different colors for different categories", () => {
    const categories = ["chatbot", "agent", "assistant", "evaluation", "workflow"];

    for (const category of categories) {
      const template = createMockTemplate({ category });
      const { unmount } = render(<TemplateCard template={template} />);

      // The category badge should have some color class
      const badge = screen.getByText(category);
      expect(badge).toBeInTheDocument();

      unmount();
    }
  });

  it("handles undefined tags gracefully", () => {
    const template = createMockTemplate({ tags: undefined });
    render(<TemplateCard template={template} />);
    // Should not crash and not render any tag badges
    expect(screen.queryByText("mock")).not.toBeInTheDocument();
  });

  it("handles empty tags array", () => {
    const template = createMockTemplate({ tags: [] });
    render(<TemplateCard template={template} />);
    expect(screen.queryByText(/\+\d/)).not.toBeInTheDocument();
  });
});
