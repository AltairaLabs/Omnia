/**
 * Tests for template-browser component
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { TemplateBrowser } from "./template-browser";
import type { ArenaTemplateSource, TemplateMetadata } from "@/types/arena-template";

type TemplateWithSource = TemplateMetadata & { sourceName: string };

// Test data factory for sources
function createMockSource(overrides: Partial<ArenaTemplateSource> = {}): ArenaTemplateSource {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaTemplateSource",
    metadata: { name: "test-source", namespace: "test-ns" },
    spec: { type: "git", git: { url: "https://github.com/test/repo" } },
    status: {
      phase: "Ready",
      templateCount: 2,
    },
    ...overrides,
  };
}

// Test data factory for templates
function createMockTemplates(): TemplateWithSource[] {
  return [
    {
      name: "basic-chatbot",
      displayName: "Basic Chatbot",
      description: "A simple chatbot",
      category: "chatbot",
      tags: ["mock", "beginner"],
      path: "templates/basic-chatbot",
      sourceName: "test-source",
    },
    {
      name: "advanced-agent",
      displayName: "Advanced Agent",
      description: "Complex agent template",
      category: "agent",
      tags: ["openai", "advanced"],
      path: "templates/advanced-agent",
      sourceName: "test-source",
    },
  ];
}

describe("TemplateBrowser", () => {
  it("renders templates count", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} />);
    expect(screen.getByText(/2 templates available/)).toBeInTheDocument();
  });

  it("displays templates", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} />);
    expect(screen.getByText("Basic Chatbot")).toBeInTheDocument();
    expect(screen.getByText("Advanced Agent")).toBeInTheDocument();
  });

  it("shows 0 templates when templates array is empty", () => {
    const sources = [
      createMockSource({
        status: { phase: "Error" },
      }),
    ];
    render(<TemplateBrowser templates={[]} sources={sources} />);
    expect(screen.getByText("0 templates available")).toBeInTheDocument();
  });

  it("filters templates by search query", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} />);

    const searchInput = screen.getByPlaceholderText("Search templates...");
    fireEvent.change(searchInput, { target: { value: "chatbot" } });

    expect(screen.getByText("Basic Chatbot")).toBeInTheDocument();
    expect(screen.queryByText("Advanced Agent")).not.toBeInTheDocument();
  });

  it("renders category tabs from template data", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} />);

    // Verify category tabs are rendered based on template categories
    const tabs = screen.getAllByRole("tab");
    const tabTexts = tabs.map(tab => tab.textContent?.toLowerCase());

    // Should have "All", "chatbot", and "agent" tabs
    expect(tabTexts).toContain("all");
    expect(tabTexts.some(t => t?.includes("chatbot"))).toBe(true);
    expect(tabTexts.some(t => t?.includes("agent"))).toBe(true);
  });

  it("filters templates by tags", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} />);

    // Click on mock tag in the filter section (getAllByText to handle duplicates)
    const mockTags = screen.getAllByText("mock");
    // First one should be in the filter section
    fireEvent.click(mockTags[0]);

    expect(screen.getByText("Basic Chatbot")).toBeInTheDocument();
    expect(screen.queryByText("Advanced Agent")).not.toBeInTheDocument();
  });

  it("toggles tag selection", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} />);

    // Get the first mock tag (filter section)
    const mockTags = screen.getAllByText("mock");
    const filterTag = mockTags[0];

    // First click selects
    fireEvent.click(filterTag);
    expect(screen.getByText(/1 template/)).toBeInTheDocument();

    // Second click deselects
    fireEvent.click(filterTag);
    expect(screen.getByText(/2 templates/)).toBeInTheDocument();
  });

  it("shows error state", () => {
    const error = new Error("Failed to load");
    render(<TemplateBrowser templates={[]} sources={[]} error={error} />);
    // The error message should contain the error text
    expect(screen.getByText(/Failed to load/)).toBeInTheDocument();
  });

  it("shows retry button in error state when onRefetch is provided", () => {
    const error = new Error("Failed to load");
    const onRefetch = vi.fn();
    render(<TemplateBrowser templates={[]} sources={[]} error={error} onRefetch={onRefetch} />);

    const retryButton = screen.getByRole("button", { name: /retry/i });
    fireEvent.click(retryButton);
    expect(onRefetch).toHaveBeenCalled();
  });

  it("shows loading skeletons when loading", () => {
    render(<TemplateBrowser templates={[]} sources={[]} loading />);
    // Should show skeleton elements
    const skeletons = document.querySelectorAll(".h-48");
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("shows empty state when no templates", () => {
    render(<TemplateBrowser templates={[]} sources={[]} />);
    expect(screen.getByText("No templates found")).toBeInTheDocument();
    expect(screen.getByText("Add a template source to get started")).toBeInTheDocument();
  });

  it("shows different empty state message when filtered", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} />);

    const searchInput = screen.getByPlaceholderText("Search templates...");
    fireEvent.change(searchInput, { target: { value: "nonexistent" } });

    expect(screen.getByText("No templates found")).toBeInTheDocument();
    expect(screen.getByText("Try adjusting your search or filters")).toBeInTheDocument();
  });

  it("calls onRefetch when refresh button is clicked", () => {
    const onRefetch = vi.fn();
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} onRefetch={onRefetch} />);

    const refreshButton = screen.getByRole("button", { name: /refresh/i });
    fireEvent.click(refreshButton);
    expect(onRefetch).toHaveBeenCalled();
  });

  it("calls onSelectTemplate when template card is clicked", () => {
    const onSelectTemplate = vi.fn();
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} onSelectTemplate={onSelectTemplate} />);

    fireEvent.click(screen.getByText("Basic Chatbot"));
    expect(onSelectTemplate).toHaveBeenCalledTimes(1);
    expect(onSelectTemplate).toHaveBeenCalledWith(
      expect.objectContaining({ name: "basic-chatbot" }),
      "test-source"
    );
  });

  it("toggles view mode between grid and list", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    const { container } = render(<TemplateBrowser templates={templates} sources={sources} />);

    // Initial grid mode
    expect(container.querySelector(".grid")).toBeInTheDocument();

    // Click list view button
    const listButton = screen.getAllByRole("button")[1]; // Second button is list view
    fireEvent.click(listButton);

    // Should now be in list mode
    expect(container.querySelector(".space-y-2")).toBeInTheDocument();
  });

  it("shows syncing status when sources are pending", () => {
    const sources = [
      createMockSource({
        status: { phase: "Fetching" },
      }),
    ];
    render(<TemplateBrowser templates={[]} sources={sources} />);
    expect(screen.getByText(/1 source syncing/)).toBeInTheDocument();
  });

  it("shows category tabs when templates have categories", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} />);

    expect(screen.getByRole("tab", { name: /all/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /chatbot/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /agent/i })).toBeInTheDocument();
  });

  it("limits displayed tags to 10", () => {
    // Use letter-based tags to ensure predictable sort order (alphabetical)
    const manyTags = ["alpha", "beta", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet", "kilo", "lima", "mike", "november", "oscar"];
    const sources = [
      createMockSource({
        status: { phase: "Ready" },
      }),
    ];
    const templates: TemplateWithSource[] = [
      {
        name: "many-tags-template",
        tags: manyTags,
        path: "templates/many",
        sourceName: "test-source",
      },
    ];
    render(<TemplateBrowser templates={templates} sources={sources} />);

    // Should show first 10 tags (alphabetically sorted) and a "+5 more" badge in filter section
    // Tags may appear in both filter section and card, so use getAllByText
    expect(screen.getAllByText("alpha").length).toBeGreaterThan(0);
    // After alpha, sorted tags are: beta, charlie, delta, echo, foxtrot, golf, hotel, india, juliet
    // kilo, lima, mike, november, oscar would not be shown in the filter section
    expect(screen.getByText("+5 more")).toBeInTheDocument();
  });

  it("aggregates templates from multiple sources", () => {
    const sources = [
      createMockSource({ metadata: { name: "source-1", namespace: "ns" } }),
      createMockSource({ metadata: { name: "source-2", namespace: "ns" } }),
    ];
    const templates: TemplateWithSource[] = [
      { name: "t1", displayName: "Template 1", path: "p1", sourceName: "source-1" },
      { name: "t2", displayName: "Template 2", path: "p2", sourceName: "source-2" },
    ];
    render(<TemplateBrowser templates={templates} sources={sources} />);

    expect(screen.getByText("Template 1")).toBeInTheDocument();
    expect(screen.getByText("Template 2")).toBeInTheDocument();
    expect(screen.getByText("2 templates available")).toBeInTheDocument();
  });

  it("applies custom className", () => {
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    const { container } = render(
      <TemplateBrowser templates={templates} sources={sources} className="custom-class" />
    );
    expect(container.querySelector(".custom-class")).toBeInTheDocument();
  });

  it("disables refresh button when loading", () => {
    const onRefetch = vi.fn();
    const sources = [createMockSource()];
    const templates = createMockTemplates();
    render(<TemplateBrowser templates={templates} sources={sources} onRefetch={onRefetch} loading />);

    const refreshButton = screen.getByRole("button", { name: /refresh/i });
    expect(refreshButton).toBeDisabled();
  });
});
