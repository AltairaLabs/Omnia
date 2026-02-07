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
  useProviders: vi.fn(() => ({
    data: [],
    isLoading: false,
    error: null,
  })),
  useDevSession: vi.fn(() => ({
    session: null,
    isLoading: false,
    error: null,
    isReady: false,
    endpoint: null,
    createSession: vi.fn(),
    deleteSession: vi.fn(),
    sendHeartbeat: vi.fn(),
    refresh: vi.fn(),
  })),
}));

vi.mock("@/hooks/use-provider-binding-status", () => ({
  useProviderBindingStatus: vi.fn(() => new Map()),
}));

vi.mock("@/hooks/use-providers", () => ({
  useProviders: vi.fn(() => ({
    data: [],
    isLoading: false,
    error: null,
  })),
}));

vi.mock("@/hooks/use-toast", () => ({
  useToast: () => ({
    toast: vi.fn(),
  }),
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace" },
  }),
}));

vi.mock("@/stores/results-panel-store", () => ({
  useResultsPanelStore: vi.fn((selector) => {
    const state = {
      isOpen: false,
      activeTab: "problems",
      height: 30,
      currentJobName: null,
      problemsCount: 0,
      open: vi.fn(),
      close: vi.fn(),
      toggle: vi.fn(),
      setActiveTab: vi.fn(),
      setHeight: vi.fn(),
      setCurrentJob: vi.fn(),
      setProblemsCount: vi.fn(),
      openJobLogs: vi.fn(),
      openJobResults: vi.fn(),
    };
    return selector(state);
  }),
  useResultsPanelActiveTab: vi.fn(() => "problems"),
}));

vi.mock("@/lib/config", () => ({
  getRuntimeConfig: vi.fn(() =>
    Promise.resolve({
      enterpriseEnabled: false,
      demoMode: false,
      readOnlyMode: false,
      readOnlyMessage: "",
      wsProxyUrl: "",
      grafanaUrl: "",
      lokiEnabled: false,
      tempoEnabled: false,
      hideEnterprise: false,
    })
  ),
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
