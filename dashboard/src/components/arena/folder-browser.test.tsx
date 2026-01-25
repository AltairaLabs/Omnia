/**
 * Tests for FolderBrowser component.
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { FolderBrowser } from "./folder-browser";
import type { ArenaSourceContentNode } from "@/types/arena";

const mockTree: ArenaSourceContentNode[] = [
  {
    name: "scenarios",
    path: "scenarios",
    isDirectory: true,
    children: [
      { name: "test.yaml", path: "scenarios/test.yaml", isDirectory: false, size: 512 },
      { name: "basic.yaml", path: "scenarios/basic.yaml", isDirectory: false, size: 256 },
    ],
  },
  { name: "config.arena.yaml", path: "config.arena.yaml", isDirectory: false, size: 1024 },
  { name: "README.md", path: "README.md", isDirectory: false, size: 2048 },
];

describe("FolderBrowser", () => {
  it("renders loading state", () => {
    render(
      <FolderBrowser
        tree={[]}
        loading={true}
        onSelectFolder={vi.fn()}
      />
    );

    expect(screen.getByText("Loading content...")).toBeInTheDocument();
  });

  it("renders error state", () => {
    render(
      <FolderBrowser
        tree={[]}
        error="Failed to load content"
        onSelectFolder={vi.fn()}
      />
    );

    expect(screen.getByText("Failed to load content")).toBeInTheDocument();
  });

  it("renders empty state when no content", () => {
    render(
      <FolderBrowser
        tree={[]}
        onSelectFolder={vi.fn()}
      />
    );

    expect(screen.getByText("No content available. The source may need to be synced.")).toBeInTheDocument();
  });

  it("renders tree with folders and files", () => {
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={vi.fn()}
      />
    );

    // Should show root option
    expect(screen.getByText("/ (root)")).toBeInTheDocument();

    // Should show directories and files
    expect(screen.getByText("scenarios")).toBeInTheDocument();
    expect(screen.getByText("config.arena.yaml")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
  });

  it("calls onSelectFolder when root is clicked", () => {
    const mockOnSelect = vi.fn();
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={mockOnSelect}
      />
    );

    fireEvent.click(screen.getByText("/ (root)"));
    expect(mockOnSelect).toHaveBeenCalledWith("");
  });

  it("calls onSelectFolder when a folder is clicked", () => {
    const mockOnSelect = vi.fn();
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={mockOnSelect}
      />
    );

    fireEvent.click(screen.getByText("scenarios"));
    expect(mockOnSelect).toHaveBeenCalledWith("scenarios");
  });

  it("expands folder when chevron is clicked", () => {
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={vi.fn()}
      />
    );

    // Children should not be visible initially
    expect(screen.queryByText("test.yaml")).not.toBeInTheDocument();

    // Click the expand button (chevron) - find it by role
    const scenariosRow = screen.getByText("scenarios").closest("div[role='button']");
    const expandButton = scenariosRow?.querySelector("button");
    if (expandButton) {
      fireEvent.click(expandButton);
    }

    // Children should now be visible
    expect(screen.getByText("test.yaml")).toBeInTheDocument();
    expect(screen.getByText("basic.yaml")).toBeInTheDocument();
  });

  it("calls onSelectFile when a file is clicked", () => {
    const mockOnSelectFile = vi.fn();
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={vi.fn()}
        onSelectFile={mockOnSelectFile}
      />
    );

    fireEvent.click(screen.getByText("config.arena.yaml"));
    expect(mockOnSelectFile).toHaveBeenCalledWith(
      "config.arena.yaml",
      "",
      "config.arena.yaml"
    );
  });

  it("calls onSelectFile with correct folder path for nested files", () => {
    const mockOnSelectFile = vi.fn();
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={vi.fn()}
        onSelectFile={mockOnSelectFile}
      />
    );

    // Expand the scenarios folder first
    const scenariosRow = screen.getByText("scenarios").closest("div[role='button']");
    const expandButton = scenariosRow?.querySelector("button");
    if (expandButton) {
      fireEvent.click(expandButton);
    }

    // Click on a nested file
    fireEvent.click(screen.getByText("test.yaml"));
    expect(mockOnSelectFile).toHaveBeenCalledWith(
      "scenarios/test.yaml",
      "scenarios",
      "test.yaml"
    );
  });

  it("highlights selected folder", () => {
    render(
      <FolderBrowser
        tree={mockTree}
        selectedPath="scenarios"
        onSelectFolder={vi.fn()}
      />
    );

    // Use getAllByText since "scenarios" appears in tree and footer
    const scenariosElements = screen.getAllByText("scenarios");
    const scenariosRow = scenariosElements[0].closest("div[role='button']");
    expect(scenariosRow).toHaveClass("bg-primary/10");
  });

  it("highlights root when selectedPath is empty", () => {
    render(
      <FolderBrowser
        tree={mockTree}
        selectedPath=""
        onSelectFolder={vi.fn()}
      />
    );

    const rootRow = screen.getByText("/ (root)").closest("div[role='button']");
    expect(rootRow).toHaveClass("bg-primary/10");
  });

  it("shows selected path in footer", () => {
    render(
      <FolderBrowser
        tree={mockTree}
        selectedPath="scenarios"
        onSelectFolder={vi.fn()}
      />
    );

    // "scenarios" appears both in tree and footer code element
    const scenariosElements = screen.getAllByText("scenarios");
    expect(scenariosElements.length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("Selected:")).toBeInTheDocument();
  });

  it("shows / for root in footer when selectedPath is empty", () => {
    render(
      <FolderBrowser
        tree={mockTree}
        selectedPath=""
        onSelectFolder={vi.fn()}
      />
    );

    const selectedCode = screen.getAllByRole("button").find(el =>
      el.textContent?.includes("/ (root)")
    );
    expect(selectedCode).toBeInTheDocument();
  });

  it("files are not clickable without onSelectFile", () => {
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={vi.fn()}
      />
    );

    const fileElement = screen.getByText("config.arena.yaml").closest("div");
    expect(fileElement).not.toHaveAttribute("role", "button");
  });

  it("handles keyboard navigation for folders", () => {
    const mockOnSelect = vi.fn();
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={mockOnSelect}
      />
    );

    const scenariosRow = screen.getByText("scenarios").closest("div[role='button']");
    if (scenariosRow) {
      fireEvent.keyDown(scenariosRow, { key: "Enter" });
      expect(mockOnSelect).toHaveBeenCalledWith("scenarios");
    }
  });

  it("handles keyboard navigation for files", () => {
    const mockOnSelectFile = vi.fn();
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={vi.fn()}
        onSelectFile={mockOnSelectFile}
      />
    );

    const fileRow = screen.getByText("config.arena.yaml").closest("div[role='button']");
    if (fileRow) {
      fireEvent.keyDown(fileRow, { key: " " });
      expect(mockOnSelectFile).toHaveBeenCalled();
    }
  });

  it("applies custom className", () => {
    const { container } = render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={vi.fn()}
        className="custom-class"
      />
    );

    expect(container.firstChild).toHaveClass("custom-class");
  });

  it("respects maxHeight prop", () => {
    render(
      <FolderBrowser
        tree={mockTree}
        onSelectFolder={vi.fn()}
        maxHeight="300px"
      />
    );

    const scrollContainer = document.querySelector(".overflow-y-auto");
    expect(scrollContainer).toHaveStyle({ maxHeight: "300px" });
  });
});
