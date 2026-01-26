import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { EditorTabs, EditorTabsEmptyState } from "./editor-tabs";
import { useProjectEditorStore } from "@/stores/project-editor-store";

describe("EditorTabs", () => {
  beforeEach(() => {
    // Reset store state
    act(() => {
      useProjectEditorStore.getState().closeAllFiles();
    });
  });

  it("should render nothing when no files are open", () => {
    render(<EditorTabs />);

    // Should not have any tab buttons
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("should render tabs for open files", () => {
    act(() => {
      useProjectEditorStore.getState().openFile("file1.yaml", "file1.yaml", "content1");
      useProjectEditorStore.getState().openFile("file2.yaml", "file2.yaml", "content2");
    });

    render(<EditorTabs />);

    expect(screen.getByText("file1.yaml")).toBeInTheDocument();
    expect(screen.getByText("file2.yaml")).toBeInTheDocument();
  });

  it("should show close buttons for tabs", () => {
    act(() => {
      useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
    });

    render(<EditorTabs />);

    // Should have a close button (with aria-label "Close test.yaml")
    expect(screen.getByRole("button", { name: /close test\.yaml/i })).toBeInTheDocument();
  });

  it("should show dirty indicator for modified files", () => {
    act(() => {
      useProjectEditorStore.getState().openFile("test.yaml", "test.yaml", "content");
      useProjectEditorStore.getState().updateFileContent("test.yaml", "modified content");
    });

    render(<EditorTabs />);

    // Should have the tab with the file name
    const tab = screen.getByRole("tab");
    expect(tab).toBeInTheDocument();
    expect(screen.getByText("test.yaml")).toBeInTheDocument();
  });
});

describe("EditorTabsEmptyState", () => {
  it("should render empty state message", () => {
    render(<EditorTabsEmptyState />);

    expect(screen.getByText(/no files open/i)).toBeInTheDocument();
  });

  it("should provide helpful instruction", () => {
    render(<EditorTabsEmptyState />);

    expect(screen.getByText(/select a file from the tree/i)).toBeInTheDocument();
  });
});
