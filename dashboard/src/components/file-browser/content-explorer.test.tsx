/**
 * Tests for ContentExplorer — focuses on behavior unique to the shared
 * component (notably the markdown Preview/Source toggle). Tree navigation
 * and file loading are also covered by source-explorer.test.tsx via its
 * thin wrapper.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ContentExplorer, parseFrontmatter } from "./content-explorer";
import type { ArenaSourceContentNode } from "@/types/arena";

vi.mock("@/components/ui/resizable", () => ({
  ResizablePanelGroup: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="resizable-group">{children}</div>
  ),
  ResizablePanel: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="resizable-panel">{children}</div>
  ),
  ResizableHandle: () => <div data-testid="resizable-handle" />,
}));

vi.mock("@/components/arena", () => ({
  YamlEditor: ({ value, language }: { value: string; language: string }) => (
    <div data-testid="yaml-editor" data-language={language}>
      {value}
    </div>
  ),
  YamlEditorEmptyState: () => <div data-testid="yaml-editor-empty" />,
}));

vi.mock("@/components/ui/markdown", () => ({
  Markdown: ({ content }: { content: string }) => (
    <div data-testid="markdown">{content}</div>
  ),
}));

const tree: ArenaSourceContentNode[] = [
  { name: "README.md", path: "README.md", isDirectory: false, size: 32 },
  { name: "config.yaml", path: "config.yaml", isDirectory: false, size: 32 },
];

function makeLoadFile(payload: Record<string, string>) {
  return vi.fn(async (path: string) => ({ content: payload[path] ?? "" }));
}

describe("parseFrontmatter", () => {
  it("returns null frontmatter for content without a leading fence", () => {
    expect(parseFrontmatter("# Hello\nworld")).toEqual({
      frontmatter: null,
      body: "# Hello\nworld",
    });
  });

  it("returns null when the closing fence is missing", () => {
    expect(parseFrontmatter("---\nname: foo\nstill no close")).toEqual({
      frontmatter: null,
      body: "---\nname: foo\nstill no close",
    });
  });

  it("parses simple key/value pairs and strips quotes", () => {
    const { frontmatter, body } = parseFrontmatter(
      `---\nname: foo\ntitle: "Quoted title"\n---\nbody`
    );
    expect(frontmatter).toEqual({ name: "foo", title: "Quoted title" });
    expect(body).toBe("body");
  });

  it("joins indented continuation lines onto the previous key", () => {
    const { frontmatter } = parseFrontmatter(
      `---\ndescription: line one\n  line two\n  line three\n---\n`
    );
    expect(frontmatter).toEqual({ description: "line one line two line three" });
  });
});

describe("ContentExplorer markdown handling", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders Preview by default for a markdown file", async () => {
    const loadFile = makeLoadFile({ "README.md": "# Hello world" });
    render(
      <ContentExplorer
        tree={tree}
        fileCount={2}
        loading={false}
        error={null}
        loadFile={loadFile}
      />
    );

    fireEvent.click(screen.getByText("README.md"));

    await waitFor(() => {
      expect(screen.getByTestId("markdown")).toHaveTextContent("# Hello world");
    });
    expect(screen.queryByTestId("yaml-editor")).not.toBeInTheDocument();
  });

  it("toggles to Source view and back to Preview", async () => {
    const loadFile = makeLoadFile({ "README.md": "# Hello" });
    render(
      <ContentExplorer
        tree={tree}
        fileCount={2}
        loading={false}
        error={null}
        loadFile={loadFile}
      />
    );

    fireEvent.click(screen.getByText("README.md"));
    await waitFor(() => expect(screen.getByTestId("markdown")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Source" }));
    expect(screen.getByTestId("yaml-editor")).toHaveAttribute("data-language", "markdown");

    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(screen.getByTestId("markdown")).toBeInTheDocument();
  });

  it("does not show the Preview/Source toggle for non-markdown files", async () => {
    const loadFile = makeLoadFile({ "config.yaml": "key: value" });
    render(
      <ContentExplorer
        tree={tree}
        fileCount={2}
        loading={false}
        error={null}
        loadFile={loadFile}
      />
    );

    fireEvent.click(screen.getByText("config.yaml"));

    await waitFor(() => {
      expect(screen.getByTestId("yaml-editor")).toHaveAttribute("data-language", "yaml");
    });
    expect(screen.queryByRole("button", { name: "Preview" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Source" })).not.toBeInTheDocument();
  });

  it("resets to Preview when switching between markdown files", async () => {
    const multiTree: ArenaSourceContentNode[] = [
      { name: "a.md", path: "a.md", isDirectory: false, size: 1 },
      { name: "b.md", path: "b.md", isDirectory: false, size: 1 },
    ];
    const loadFile = makeLoadFile({ "a.md": "alpha", "b.md": "beta" });

    render(
      <ContentExplorer
        tree={multiTree}
        fileCount={2}
        loading={false}
        error={null}
        loadFile={loadFile}
      />
    );

    fireEvent.click(screen.getByText("a.md"));
    await waitFor(() => expect(screen.getByTestId("markdown")).toHaveTextContent("alpha"));

    fireEvent.click(screen.getByRole("button", { name: "Source" }));
    expect(screen.getByTestId("yaml-editor")).toBeInTheDocument();

    fireEvent.click(screen.getByText("b.md"));
    await waitFor(() => expect(screen.getByTestId("markdown")).toHaveTextContent("beta"));
  });

  it("uses consumer-supplied file icon override when provided", () => {
    const getFileIcon = vi.fn((name: string) =>
      name === "README.md" ? <span data-testid="custom-icon" /> : undefined
    );
    render(
      <ContentExplorer
        tree={tree}
        fileCount={2}
        loading={false}
        error={null}
        loadFile={vi.fn()}
        getFileIcon={getFileIcon}
      />
    );

    expect(screen.getByTestId("custom-icon")).toBeInTheDocument();
    expect(getFileIcon).toHaveBeenCalledWith("README.md");
    expect(getFileIcon).toHaveBeenCalledWith("config.yaml");
  });

  it("shows custom labels in empty / loading states", () => {
    const { rerender } = render(
      <ContentExplorer
        tree={[]}
        fileCount={0}
        loading={true}
        error={null}
        loadFile={vi.fn()}
        labels={{ loading: "Reticulating splines..." }}
      />
    );
    expect(screen.getByText("Reticulating splines...")).toBeInTheDocument();

    rerender(
      <ContentExplorer
        tree={[]}
        fileCount={0}
        loading={false}
        error={null}
        loadFile={vi.fn()}
        labels={{ emptyTitle: "Nothing here", emptyDescription: "Try syncing." }}
      />
    );
    expect(screen.getByText("Nothing here")).toBeInTheDocument();
    expect(screen.getByText("Try syncing.")).toBeInTheDocument();
  });

  it("renders frontmatter as a metadata card and strips it from the body", async () => {
    const skill = `---
name: algorithmic-art
description: Creating algorithmic art using p5.js
license: Complete terms in LICENSE.txt
---

Body content goes here.`;
    const loadFile = makeLoadFile({ "README.md": skill });
    render(
      <ContentExplorer
        tree={tree}
        fileCount={2}
        loading={false}
        error={null}
        loadFile={loadFile}
      />
    );

    fireEvent.click(screen.getByText("README.md"));

    await waitFor(() => {
      expect(screen.getByText("algorithmic-art")).toBeInTheDocument();
    });
    expect(screen.getByText("Creating algorithmic art using p5.js")).toBeInTheDocument();
    expect(screen.getByText("Complete terms in LICENSE.txt")).toBeInTheDocument();

    // Body shows without the leading frontmatter fence text.
    const body = screen.getByTestId("markdown");
    expect(body.textContent).toContain("Body content goes here.");
    expect(body.textContent).not.toContain("name: algorithmic-art");
  });

  it("does not render a frontmatter card when there is no frontmatter", async () => {
    const loadFile = makeLoadFile({ "README.md": "# Just a heading" });
    render(
      <ContentExplorer
        tree={tree}
        fileCount={2}
        loading={false}
        error={null}
        loadFile={loadFile}
      />
    );

    fireEvent.click(screen.getByText("README.md"));

    await waitFor(() => {
      expect(screen.getByTestId("markdown")).toHaveTextContent("# Just a heading");
    });
    // No <dt>/<dd> from a metadata card.
    expect(screen.queryByRole("definition")).not.toBeInTheDocument();
  });

  it("auto-selects defaultFile and expands ancestor folders", async () => {
    const nested: ArenaSourceContentNode[] = [
      {
        name: "docs",
        path: "docs",
        isDirectory: true,
        children: [
          { name: "intro.md", path: "docs/intro.md", isDirectory: false, size: 1 },
        ],
      },
    ];
    const loadFile = makeLoadFile({ "docs/intro.md": "# Intro" });

    render(
      <ContentExplorer
        tree={nested}
        fileCount={1}
        loading={false}
        error={null}
        loadFile={loadFile}
        defaultFile="docs/intro.md"
      />
    );

    await waitFor(() => {
      expect(loadFile).toHaveBeenCalledWith("docs/intro.md");
      expect(screen.getByTestId("markdown")).toHaveTextContent("# Intro");
    });
  });
});
