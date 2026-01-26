import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ProjectToolbar, NewProjectButton } from "./project-toolbar";
import type { ArenaProject } from "@/types/arena-project";

describe("ProjectToolbar", () => {
  const mockProject: ArenaProject = {
    id: "project-1",
    name: "Test Project",
    description: "A test project",
    createdAt: "2024-01-01T00:00:00Z",
    updatedAt: "2024-01-01T00:00:00Z",
  };

  const mockProjects: ArenaProject[] = [
    mockProject,
    {
      id: "project-2",
      name: "Another Project",
      createdAt: "2024-01-02T00:00:00Z",
      updatedAt: "2024-01-02T00:00:00Z",
    },
  ];

  const defaultProps = {
    projects: mockProjects,
    currentProject: mockProject,
    hasUnsavedChanges: false,
    saving: false,
    loading: false,
    onProjectSelect: vi.fn(),
    onSave: vi.fn(),
    onNewProject: vi.fn(),
    onRefresh: vi.fn(),
    onDeleteProject: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render toolbar", () => {
    render(<ProjectToolbar {...defaultProps} />);

    // Toolbar contains the project selector and buttons
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });

  it("should show project selector with current project", () => {
    render(<ProjectToolbar {...defaultProps} />);

    expect(screen.getByText("Test Project")).toBeInTheDocument();
  });

  it("should show project options when selector is clicked", async () => {
    const user = userEvent.setup();
    render(<ProjectToolbar {...defaultProps} />);

    const selector = screen.getByRole("combobox");
    await user.click(selector);

    await waitFor(() => {
      expect(screen.getByText("Another Project")).toBeInTheDocument();
    });
  });

  it("should call onProjectSelect when project is selected", async () => {
    const onProjectSelect = vi.fn();
    const user = userEvent.setup();
    render(<ProjectToolbar {...defaultProps} onProjectSelect={onProjectSelect} />);

    const selector = screen.getByRole("combobox");
    await user.click(selector);

    const option = await screen.findByText("Another Project");
    await user.click(option);

    expect(onProjectSelect).toHaveBeenCalledWith("project-2");
  });

  it("should show save button with text", () => {
    render(<ProjectToolbar {...defaultProps} />);

    expect(screen.getByText("Save")).toBeInTheDocument();
  });

  it("should disable save button when no unsaved changes", () => {
    render(<ProjectToolbar {...defaultProps} hasUnsavedChanges={false} />);

    const saveButton = screen.getByText("Save").closest("button");
    expect(saveButton).toBeDisabled();
  });

  it("should enable save button when there are unsaved changes", () => {
    render(<ProjectToolbar {...defaultProps} hasUnsavedChanges={true} />);

    const saveButton = screen.getByText("Save").closest("button");
    expect(saveButton).not.toBeDisabled();
  });

  it("should call onSave when save button is clicked", async () => {
    const onSave = vi.fn();
    const user = userEvent.setup();
    render(<ProjectToolbar {...defaultProps} hasUnsavedChanges={true} onSave={onSave} />);

    const saveButton = screen.getByText("Save").closest("button");
    await user.click(saveButton!);

    expect(onSave).toHaveBeenCalledTimes(1);
  });

  it("should disable save button while saving", () => {
    render(<ProjectToolbar {...defaultProps} saving={true} hasUnsavedChanges={true} />);

    const saveButton = screen.getByText("Save").closest("button");
    expect(saveButton).toBeDisabled();
  });

  it("should show new project button with title", () => {
    render(<ProjectToolbar {...defaultProps} />);

    expect(screen.getByTitle("New Project")).toBeInTheDocument();
  });

  it("should call onNewProject when new button is clicked", async () => {
    const onNewProject = vi.fn();
    const user = userEvent.setup();
    render(<ProjectToolbar {...defaultProps} onNewProject={onNewProject} />);

    const newButton = screen.getByTitle("New Project");
    await user.click(newButton);

    expect(onNewProject).toHaveBeenCalledTimes(1);
  });

  it("should show refresh button with title", () => {
    render(<ProjectToolbar {...defaultProps} />);

    expect(screen.getByTitle("Refresh")).toBeInTheDocument();
  });

  it("should call onRefresh when refresh button is clicked", async () => {
    const onRefresh = vi.fn();
    const user = userEvent.setup();
    render(<ProjectToolbar {...defaultProps} onRefresh={onRefresh} />);

    const refreshButton = screen.getByTitle("Refresh");
    await user.click(refreshButton);

    expect(onRefresh).toHaveBeenCalledTimes(1);
  });

  it("should disable refresh button while loading", () => {
    render(<ProjectToolbar {...defaultProps} loading={true} />);

    const refreshButton = screen.getByTitle("Refresh");
    expect(refreshButton).toBeDisabled();
  });

  it("should show delete button when onDeleteProject is provided and project is selected", () => {
    render(<ProjectToolbar {...defaultProps} />);

    expect(screen.getByTitle("Delete Project")).toBeInTheDocument();
  });

  it("should not show delete button when onDeleteProject is undefined", () => {
    render(<ProjectToolbar {...defaultProps} onDeleteProject={undefined} />);

    expect(screen.queryByTitle("Delete Project")).not.toBeInTheDocument();
  });

  it("should not show delete button when no project is selected", () => {
    render(<ProjectToolbar {...defaultProps} currentProject={null} />);

    expect(screen.queryByTitle("Delete Project")).not.toBeInTheDocument();
  });

  it("should call onDeleteProject when delete button is clicked", async () => {
    const onDeleteProject = vi.fn();
    const user = userEvent.setup();
    render(<ProjectToolbar {...defaultProps} onDeleteProject={onDeleteProject} />);

    const deleteButton = screen.getByTitle("Delete Project");
    await user.click(deleteButton);

    expect(onDeleteProject).toHaveBeenCalledTimes(1);
  });

  it("should show placeholder when no project is selected", () => {
    render(<ProjectToolbar {...defaultProps} currentProject={null} />);

    expect(screen.getByText(/select a project/i)).toBeInTheDocument();
  });

  it("should show unsaved changes indicator when there are changes", () => {
    render(<ProjectToolbar {...defaultProps} hasUnsavedChanges={true} />);

    expect(screen.getByText(/unsaved changes/i)).toBeInTheDocument();
  });

  it("should apply custom className", () => {
    const { container } = render(<ProjectToolbar {...defaultProps} className="custom-class" />);

    expect(container.firstChild).toHaveClass("custom-class");
  });
});

describe("NewProjectButton", () => {
  it("should render button with text", () => {
    render(<NewProjectButton onClick={vi.fn()} />);

    expect(screen.getByText("New Project")).toBeInTheDocument();
  });

  it("should call onClick when clicked", async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(<NewProjectButton onClick={onClick} />);

    const button = screen.getByText("New Project");
    await user.click(button);

    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("should be disabled when disabled prop is true", () => {
    render(<NewProjectButton onClick={vi.fn()} disabled={true} />);

    const button = screen.getByText("New Project").closest("button");
    expect(button).toBeDisabled();
  });

  it("should be enabled when disabled prop is false", () => {
    render(<NewProjectButton onClick={vi.fn()} disabled={false} />);

    const button = screen.getByText("New Project").closest("button");
    expect(button).not.toBeDisabled();
  });
});
