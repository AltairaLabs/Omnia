import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { FileTree } from "./file-tree";
import type { FileTreeNode } from "@/types/arena-project";

// Mock the toast hook
vi.mock("@/hooks/use-toast", () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

// Mock the providers hook used by import dialogs
vi.mock("@/hooks/use-providers", () => ({
  useProviders: () => ({
    data: [],
    loading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

// Mock the tool registries hook used by import dialogs
vi.mock("@/hooks/use-tool-registries", () => ({
  useToolRegistries: () => ({
    data: [],
    loading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

describe("FileTree", () => {
  const defaultProps = {
    tree: [] as FileTreeNode[],
    loading: false,
    error: null,
    selectedPath: undefined,
    onSelectFile: vi.fn(),
    onCreateFile: vi.fn(),
    onDeleteFile: vi.fn(),
  };

  const mockTree: FileTreeNode[] = [
    {
      name: "config.arena.yaml",
      path: "config.arena.yaml",
      isDirectory: false,
      type: "arena",
    },
    {
      name: "prompts",
      path: "prompts",
      isDirectory: true,
      children: [
        {
          name: "system.prompt.yaml",
          path: "prompts/system.prompt.yaml",
          isDirectory: false,
          type: "prompt",
        },
      ],
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should show loading state", () => {
    render(<FileTree {...defaultProps} loading={true} />);

    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it("should show error state", () => {
    render(<FileTree {...defaultProps} error="Failed to load files" />);

    expect(screen.getByText(/failed to load files/i)).toBeInTheDocument();
  });

  it("should show empty state when no files", () => {
    render(<FileTree {...defaultProps} tree={[]} />);

    expect(screen.getByText(/no files in project/i)).toBeInTheDocument();
  });

  it("should render file tree", () => {
    render(<FileTree {...defaultProps} tree={mockTree} />);

    expect(screen.getByText("config.arena.yaml")).toBeInTheDocument();
    expect(screen.getByText("prompts")).toBeInTheDocument();
  });

  it("should expand directory on click", () => {
    render(<FileTree {...defaultProps} tree={mockTree} />);

    // Children should not be visible initially
    expect(screen.queryByText("system.prompt.yaml")).not.toBeInTheDocument();

    // Click to expand
    fireEvent.click(screen.getByText("prompts"));

    // Children should now be visible
    expect(screen.getByText("system.prompt.yaml")).toBeInTheDocument();
  });

  it("should collapse directory on second click", () => {
    render(<FileTree {...defaultProps} tree={mockTree} />);

    // Expand
    fireEvent.click(screen.getByText("prompts"));
    expect(screen.getByText("system.prompt.yaml")).toBeInTheDocument();

    // Collapse
    fireEvent.click(screen.getByText("prompts"));
    expect(screen.queryByText("system.prompt.yaml")).not.toBeInTheDocument();
  });

  it("should call onSelectFile when file is clicked", () => {
    const onSelectFile = vi.fn();
    render(<FileTree {...defaultProps} tree={mockTree} onSelectFile={onSelectFile} />);

    fireEvent.click(screen.getByText("config.arena.yaml"));

    expect(onSelectFile).toHaveBeenCalledWith("config.arena.yaml", "config.arena.yaml");
  });

  it("should call onSelectFile for nested file", () => {
    const onSelectFile = vi.fn();
    render(<FileTree {...defaultProps} tree={mockTree} onSelectFile={onSelectFile} />);

    // Expand prompts
    fireEvent.click(screen.getByText("prompts"));

    // Click nested file
    fireEvent.click(screen.getByText("system.prompt.yaml"));

    expect(onSelectFile).toHaveBeenCalledWith("prompts/system.prompt.yaml", "system.prompt.yaml");
  });

  it("should apply custom className", () => {
    const { container } = render(
      <FileTree {...defaultProps} tree={mockTree} className="custom-class" />
    );

    expect(container.firstChild).toHaveClass("custom-class");
  });
});
