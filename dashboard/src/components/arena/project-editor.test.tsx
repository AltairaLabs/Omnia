import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { ProjectEditor } from "./project-editor";

// Mock all dependencies
vi.mock("@/stores", () => ({
  useProjectEditorStore: vi.fn((selector) => {
    const state = {
      currentProject: null,
      fileTree: [],
      activeFilePath: null,
      openFiles: [],
      projectLoading: false,
      projectError: null,
      setCurrentProject: vi.fn(),
      setFileTree: vi.fn(),
      openFile: vi.fn(),
      updateFileContent: vi.fn(),
      markFileSaved: vi.fn(),
      setProjectLoading: vi.fn(),
    };
    return selector(state);
  }),
  useActiveFile: vi.fn(() => null),
  useHasUnsavedChanges: vi.fn(() => false),
}));

vi.mock("@/hooks", () => ({
  useArenaProjects: vi.fn(() => ({
    projects: [],
    loading: false,
    error: null,
    refetch: vi.fn(),
  })),
  useArenaProject: vi.fn(() => ({
    project: null,
    loading: false,
    error: null,
    refetch: vi.fn(),
  })),
  useArenaProjectMutations: vi.fn(() => ({
    createProject: vi.fn(),
    deleteProject: vi.fn(),
  })),
  useArenaProjectFiles: vi.fn(() => ({
    getFileContent: vi.fn(),
    updateFileContent: vi.fn(),
    createFile: vi.fn(),
    deleteFile: vi.fn(),
    refreshFileTree: vi.fn(),
  })),
}));

vi.mock("@/hooks/use-toast", () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

describe("ProjectEditor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render the project editor component", () => {
    render(<ProjectEditor />);

    // Should render without errors
    expect(document.body).toBeInTheDocument();
  });

  it("should show empty state when no project is selected", () => {
    render(<ProjectEditor />);

    expect(screen.getByText(/select a project/i)).toBeInTheDocument();
  });

  it("should apply custom className", () => {
    const { container } = render(<ProjectEditor className="custom-class" />);

    expect(container.firstChild).toHaveClass("custom-class");
  });

  it("should show Create a new project link", () => {
    render(<ProjectEditor />);

    expect(screen.getByText(/create a new project/i)).toBeInTheDocument();
  });

  it("should render with initialProjectId", () => {
    const { container } = render(<ProjectEditor initialProjectId="test-project" />);

    expect(container).toBeInTheDocument();
  });
});
