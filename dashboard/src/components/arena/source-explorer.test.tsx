/**
 * Tests for SourceExplorer component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { SourceExplorer } from "./source-explorer";
import type { ArenaSourceContentNode } from "@/types/arena";

// Mock workspace context
const mockWorkspace = { name: "default", namespace: "omnia-system", permissions: { write: true } };
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({ currentWorkspace: mockWorkspace })),
}));

// Mock content hook
const mockContentResult: {
  tree: ArenaSourceContentNode[];
  fileCount: number;
  directoryCount: number;
  loading: boolean;
  error: Error | null;
  refetch: ReturnType<typeof vi.fn>;
} = {
  tree: [],
  fileCount: 0,
  directoryCount: 0,
  loading: false,
  error: null,
  refetch: vi.fn(),
};
vi.mock("@/hooks/use-arena-source-content", () => ({
  useArenaSourceContent: vi.fn(() => mockContentResult),
}));

// Mock ResizablePanel components (they require browser APIs)
vi.mock("@/components/ui/resizable", () => ({
  ResizablePanelGroup: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="resizable-group">{children}</div>
  ),
  ResizablePanel: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="resizable-panel">{children}</div>
  ),
  ResizableHandle: () => <div data-testid="resizable-handle" />,
}));

// Mock YamlEditor
vi.mock("./yaml-editor", () => ({
  YamlEditor: ({ value, readOnly }: { value: string; readOnly: boolean }) => (
    <div data-testid="yaml-editor" data-readonly={readOnly}>
      {value}
    </div>
  ),
  YamlEditorEmptyState: () => (
    <div data-testid="yaml-editor-empty">No file selected</div>
  ),
}));

const sampleTree: ArenaSourceContentNode[] = [
  {
    name: "prompts",
    path: "prompts",
    isDirectory: true,
    children: [
      { name: "main.yaml", path: "prompts/main.yaml", isDirectory: false, size: 256 },
    ],
  },
  { name: "config.arena.yaml", path: "config.arena.yaml", isDirectory: false, size: 128 },
  { name: "README.md", path: "README.md", isDirectory: false, size: 512 },
];

describe("SourceExplorer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockContentResult.tree = [];
    mockContentResult.fileCount = 0;
    mockContentResult.loading = false;
    mockContentResult.error = null;
  });

  it("shows loading state", () => {
    mockContentResult.loading = true;
    render(<SourceExplorer sourceName="test-source" />);
    expect(screen.getByText("Loading source content...")).toBeInTheDocument();
  });

  it("shows error state", () => {
    mockContentResult.error = new Error("Connection failed");
    render(<SourceExplorer sourceName="test-source" />);
    expect(screen.getByText("Unable to load content")).toBeInTheDocument();
    expect(screen.getByText("Connection failed")).toBeInTheDocument();
  });

  it("shows empty state when no content", () => {
    render(<SourceExplorer sourceName="test-source" />);
    expect(screen.getByText("No content available")).toBeInTheDocument();
  });

  it("renders file tree with files and directories", () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;
    render(<SourceExplorer sourceName="test-source" />);

    expect(screen.getByText("prompts")).toBeInTheDocument();
    expect(screen.getByText("config.arena.yaml")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
    expect(screen.getByText("(3)")).toBeInTheDocument();
  });

  it("shows empty editor state when no file selected", () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;
    render(<SourceExplorer sourceName="test-source" />);
    expect(screen.getByTestId("yaml-editor-empty")).toBeInTheDocument();
  });

  it("expands directory on click", () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;
    render(<SourceExplorer sourceName="test-source" />);

    // Directory children should not be visible initially
    expect(screen.queryByText("main.yaml")).not.toBeInTheDocument();

    // Click the directory
    fireEvent.click(screen.getByText("prompts"));

    // Children should now be visible
    expect(screen.getByText("main.yaml")).toBeInTheDocument();
  });

  it("fetches and displays file content on file click", async () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;

    const mockFileContent = "name: test-config\nversion: 1.0";
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ path: "config.arena.yaml", content: mockFileContent, size: 128 }),
    });

    render(<SourceExplorer sourceName="test-source" />);

    fireEvent.click(screen.getByText("config.arena.yaml"));

    await waitFor(() => {
      expect(global.fetch).toHaveBeenCalledWith(
        "/api/workspaces/default/arena/sources/test-source/file?path=config.arena.yaml"
      );
    });

    await waitFor(() => {
      const editor = screen.getByTestId("yaml-editor");
      expect(editor).toHaveAttribute("data-readonly", "true");
      expect(editor.textContent).toContain("test-config");
      expect(editor.textContent).toContain("version: 1.0");
    });
  });

  it("shows file path bar when a file is selected", async () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ path: "README.md", content: "# Hello", size: 7 }),
    });

    render(<SourceExplorer sourceName="test-source" />);
    fireEvent.click(screen.getByText("README.md"));

    await waitFor(() => {
      // The path bar shows the full path in a monospace font element
      const pathBar = screen.getByText("README.md", { selector: ".font-mono" });
      expect(pathBar).toBeInTheDocument();
    });
  });

  it("shows error when file fetch fails", async () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;

    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      statusText: "Not Found",
      json: () => Promise.resolve({ error: "File not found" }),
    });

    render(<SourceExplorer sourceName="test-source" />);
    fireEvent.click(screen.getByText("config.arena.yaml"));

    await waitFor(() => {
      expect(screen.getByText("File not found")).toBeInTheDocument();
    });
  });

  it("handles keyboard navigation with Enter key", () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ path: "config.arena.yaml", content: "test", size: 4 }),
    });

    render(<SourceExplorer sourceName="test-source" />);

    const fileButton = screen.getByText("config.arena.yaml").closest("[role='button']")!;
    fireEvent.keyDown(fileButton, { key: "Enter" });

    expect(global.fetch).toHaveBeenCalled();
  });

  it("handles keyboard navigation with Space key on directory", () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;
    render(<SourceExplorer sourceName="test-source" />);

    const dirButton = screen.getByText("prompts").closest("[role='button']")!;
    fireEvent.keyDown(dirButton, { key: " " });

    expect(screen.getByText("main.yaml")).toBeInTheDocument();
  });

  it("renders correct icons for different file types", () => {
    const treeWithTypes: ArenaSourceContentNode[] = [
      { name: "main.go", path: "main.go", isDirectory: false, size: 100 },
      { name: "notes.md", path: "notes.md", isDirectory: false, size: 50 },
      { name: "data.txt", path: "data.txt", isDirectory: false, size: 30 },
    ];
    mockContentResult.tree = treeWithTypes;
    mockContentResult.fileCount = 3;
    render(<SourceExplorer sourceName="test-source" />);

    expect(screen.getByText("main.go")).toBeInTheDocument();
    expect(screen.getByText("notes.md")).toBeInTheDocument();
    expect(screen.getByText("data.txt")).toBeInTheDocument();
  });

  it("detects language for various file extensions", async () => {
    const fileTypes: ArenaSourceContentNode[] = [
      { name: "main.go", path: "main.go", isDirectory: false, size: 10 },
      { name: "app.ts", path: "app.ts", isDirectory: false, size: 10 },
      { name: "script.js", path: "script.js", isDirectory: false, size: 10 },
      { name: "run.sh", path: "run.sh", isDirectory: false, size: 10 },
      { name: "tool.py", path: "tool.py", isDirectory: false, size: 10 },
      { name: "Makefile", path: "Makefile", isDirectory: false, size: 10 },
    ];
    mockContentResult.tree = fileTypes;
    mockContentResult.fileCount = 6;

    // Click each file to exercise getLanguageForFile branches
    for (const file of fileTypes) {
      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ path: file.path, content: "content", size: 7 }),
      });

      const { unmount } = render(<SourceExplorer sourceName="test-source" />);
      fireEvent.click(screen.getByText(file.name));

      await waitFor(() => {
        expect(screen.getByTestId("yaml-editor")).toBeInTheDocument();
      });

      unmount();
    }
  });

  it("expands directory via chevron button", () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;
    render(<SourceExplorer sourceName="test-source" />);

    // Click the chevron button specifically (not the row)
    const dirRow = screen.getByText("prompts").closest("[role='button']")!;
    const chevronButton = dirRow.querySelector("button")!;
    fireEvent.click(chevronButton);

    expect(screen.getByText("main.yaml")).toBeInTheDocument();
  });

  it("collapses directory on second click", () => {
    mockContentResult.tree = sampleTree;
    mockContentResult.fileCount = 3;
    render(<SourceExplorer sourceName="test-source" />);

    // Expand
    fireEvent.click(screen.getByText("prompts"));
    expect(screen.getByText("main.yaml")).toBeInTheDocument();

    // Collapse
    fireEvent.click(screen.getByText("prompts"));
    expect(screen.queryByText("main.yaml")).not.toBeInTheDocument();
  });
});
