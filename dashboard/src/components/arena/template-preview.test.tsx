/**
 * Tests for template-preview component
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { TemplatePreview } from "./template-preview";
import type { RenderedFile } from "@/types/arena-template";

// Mock clipboard API
const mockClipboard = {
  writeText: vi.fn().mockResolvedValue(undefined),
};

describe("TemplatePreview", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    Object.defineProperty(navigator, "clipboard", {
      value: mockClipboard,
      writable: true,
      configurable: true,
    });
    mockClipboard.writeText.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders empty state when no files", () => {
    render(<TemplatePreview files={[]} />);
    expect(screen.getByText("No files to preview")).toBeInTheDocument();
  });

  it("renders file list", () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "name: test" },
      { path: "prompts/main.prompt.yaml", content: "prompt: hello" },
    ];
    render(<TemplatePreview files={files} />);

    // Files may appear in both list and preview header, so check for at least one
    expect(screen.getAllByText("config.yaml").length).toBeGreaterThan(0);
    expect(screen.getAllByText("main.prompt.yaml").length).toBeGreaterThan(0);
  });

  it("shows file count in header", () => {
    const files: RenderedFile[] = [
      { path: "file1.yaml", content: "content1" },
      { path: "file2.yaml", content: "content2" },
      { path: "file3.yaml", content: "content3" },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByText("Files (3)")).toBeInTheDocument();
  });

  it("selects first file by default", () => {
    const files: RenderedFile[] = [
      { path: "first.yaml", content: "first content" },
      { path: "second.yaml", content: "second content" },
    ];
    render(<TemplatePreview files={files} />);

    // First file should be selected and its content visible
    expect(screen.getByText("first content")).toBeInTheDocument();
  });

  it("switches to selected file when clicked", () => {
    const files: RenderedFile[] = [
      { path: "first.yaml", content: "first content" },
      { path: "second.yaml", content: "second content" },
    ];
    render(<TemplatePreview files={files} />);

    // Click on second file
    fireEvent.click(screen.getByText("second.yaml"));

    // Should show second file content
    expect(screen.getByText("second content")).toBeInTheDocument();
  });

  it("shows file path in code preview header", () => {
    const files: RenderedFile[] = [
      { path: "config/settings.yaml", content: "enabled: true" },
    ];
    render(<TemplatePreview files={files} />);

    // Should show full path in header
    expect(screen.getByText("config/settings.yaml")).toBeInTheDocument();
  });

  it("detects yaml language from extension", () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "name: test" },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByText("yaml")).toBeInTheDocument();
  });

  it("detects yml language from extension", () => {
    const files: RenderedFile[] = [
      { path: "config.yml", content: "name: test" },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByText("yaml")).toBeInTheDocument();
  });

  it("detects json language from extension", () => {
    const files: RenderedFile[] = [
      { path: "package.json", content: '{"name": "test"}' },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByText("json")).toBeInTheDocument();
  });

  it("detects markdown language from extension", () => {
    const files: RenderedFile[] = [
      { path: "README.md", content: "# Title" },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByText("markdown")).toBeInTheDocument();
  });

  it("defaults to text for unknown extensions", () => {
    const files: RenderedFile[] = [
      { path: "file.xyz", content: "some content" },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByText("text")).toBeInTheDocument();
  });

  it("shows line count", () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "line1\nline2\nline3" },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByText("3 lines")).toBeInTheDocument();
  });

  it("shows singular line text for single line", () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "single line" },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByText("1 line")).toBeInTheDocument();
  });

  it("copies content to clipboard when copy button clicked", async () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "name: test" },
    ];
    render(<TemplatePreview files={files} />);

    const copyButton = screen.getByRole("button", { name: /copy/i });
    fireEvent.click(copyButton);

    expect(mockClipboard.writeText).toHaveBeenCalledWith("name: test");
  });

  it("shows copied state after copying", async () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "name: test" },
    ];
    render(<TemplatePreview files={files} />);

    const copyButton = screen.getByRole("button", { name: /copy/i });

    // Click and wait for the async clipboard operation to complete
    await act(async () => {
      fireEvent.click(copyButton);
      // Allow the clipboard promise to resolve
      await Promise.resolve();
    });

    // Now check for "Copied" text
    expect(screen.getByText("Copied")).toBeInTheDocument();
  });

  it("has a copy button", () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "name: test" },
    ];
    render(<TemplatePreview files={files} />);

    expect(screen.getByRole("button", { name: /copy/i })).toBeInTheDocument();
  });

  it("displays file content in code block", () => {
    const content = "key: value\nanother: setting";
    const files: RenderedFile[] = [
      { path: "config.yaml", content },
    ];
    const { container } = render(<TemplatePreview files={files} />);

    // Content is rendered inside a <code> block
    const codeElement = container.querySelector("code");
    expect(codeElement).toBeInTheDocument();
    expect(codeElement?.textContent).toContain("key: value");
  });

  it("applies selected styling to active file", () => {
    const files: RenderedFile[] = [
      { path: "first.yaml", content: "first" },
      { path: "second.yaml", content: "second" },
    ];
    const { container } = render(<TemplatePreview files={files} />);

    // First file button should have bg-accent class
    const firstButton = container.querySelector("button.bg-accent");
    expect(firstButton).toBeInTheDocument();
  });

  it("applies custom className", () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "content" },
    ];
    const { container } = render(
      <TemplatePreview files={files} className="custom-class" />
    );

    expect(container.querySelector(".custom-class")).toBeInTheDocument();
  });

  it("shows icon for yaml files", () => {
    const files: RenderedFile[] = [
      { path: "config.yaml", content: "content" },
    ];
    const { container } = render(<TemplatePreview files={files} />);

    // FileCode icon (or any svg) should be present for yaml files
    const svgs = container.querySelectorAll("svg");
    expect(svgs.length).toBeGreaterThan(0);
  });

  it("extracts filename from path for display", () => {
    const files: RenderedFile[] = [
      { path: "deep/nested/path/file.yaml", content: "content" },
    ];
    render(<TemplatePreview files={files} />);

    // Should show just the filename in the list
    expect(screen.getByText("file.yaml")).toBeInTheDocument();
    // But full path in the preview header
    expect(screen.getByText("deep/nested/path/file.yaml")).toBeInTheDocument();
  });
});
