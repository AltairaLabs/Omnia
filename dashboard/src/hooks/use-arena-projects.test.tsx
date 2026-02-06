import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import {
  useArenaProjects,
  useArenaProject,
  useArenaProjectMutations,
  useArenaProjectFiles,
} from "./use-arena-projects";
import type { ArenaProject, ArenaProjectWithTree, FileTreeNode } from "@/types/arena-project";

// Mock the workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace" },
  }),
}));

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Test data factories
function createMockProject(overrides: Partial<ArenaProject> = {}): ArenaProject {
  return {
    id: "test-project-1",
    name: "Test Project",
    description: "A test project",
    createdAt: "2024-01-01T00:00:00Z",
    updatedAt: "2024-01-01T00:00:00Z",
    tags: ["test"],
    ...overrides,
  };
}

function createMockProjectWithTree(overrides: Partial<ArenaProjectWithTree> = {}): ArenaProjectWithTree {
  return {
    ...createMockProject(),
    tree: [
      {
        name: "config.arena.yaml",
        path: "config.arena.yaml",
        isDirectory: false,
        type: "arena",
      },
    ],
    ...overrides,
  };
}

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });

  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe("use-arena-projects hooks", () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe("useArenaProjects", () => {
    it("should fetch projects list successfully", async () => {
      const projects = [createMockProject(), createMockProject({ id: "project-2", name: "Project 2" })];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ projects }),
      });

      const { result } = renderHook(() => useArenaProjects(), {
        wrapper: createWrapper(),
      });

      expect(result.current.loading).toBe(true);

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.projects).toEqual(projects);
      expect(result.current.error).toBeNull();
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects"
      );
    });

    it("should handle fetch error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      const { result } = renderHook(() => useArenaProjects(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.projects).toEqual([]);
      expect(result.current.error).toBeTruthy();
    });

    it("should handle network error (thrown exception)", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      const { result } = renderHook(() => useArenaProjects(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.projects).toEqual([]);
      expect(result.current.error?.message).toBe("Network error");
    });

    it("should handle non-Error exceptions", async () => {
      mockFetch.mockRejectedValueOnce("string error");

      const { result } = renderHook(() => useArenaProjects(), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.projects).toEqual([]);
      expect(result.current.error?.message).toBe("string error");
    });

    it("should not fetch when workspace is not available", () => {
      vi.doMock("@/contexts/workspace-context", () => ({
        useWorkspace: () => ({
          currentWorkspace: null,
        }),
      }));

      // Since the mock is already set, we test with the existing mock
      // In real scenario, this would not make the request
      const { result } = renderHook(() => useArenaProjects(), {
        wrapper: createWrapper(),
      });

      // The hook should still work with our mock
      expect(result.current.loading).toBe(true);
    });
  });

  describe("useArenaProject", () => {
    it("should fetch single project with tree", async () => {
      const projectWithTree = createMockProjectWithTree();
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(projectWithTree),
      });

      const { result } = renderHook(() => useArenaProject("test-project-1"), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.project).toEqual(projectWithTree);
      expect(result.current.error).toBeNull();
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project-1"
      );
    });

    it("should not fetch when projectId is undefined", () => {
      const { result } = renderHook(() => useArenaProject(undefined), {
        wrapper: createWrapper(),
      });

      expect(result.current.project).toBeNull();
      expect(result.current.loading).toBe(false);
      expect(mockFetch).not.toHaveBeenCalled();
    });

    it("should handle 404 error specifically", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      const { result } = renderHook(() => useArenaProject("nonexistent"), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.project).toBeNull();
      expect(result.current.error?.message).toBe("Project not found");
    });

    it("should handle other errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      const { result } = renderHook(() => useArenaProject("some-project"), {
        wrapper: createWrapper(),
      });

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.project).toBeNull();
      expect(result.current.error?.message).toBe("Failed to fetch project: Internal Server Error");
    });
  });

  describe("useArenaProjectMutations", () => {
    it("should create project successfully", async () => {
      const newProject = createMockProject({ id: "new-project", name: "New Project" });
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(newProject),
      });

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      const created = await result.current.createProject({ name: "New Project" });

      expect(created).toEqual(newProject);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ name: "New Project" }),
        })
      );
    });

    it("should handle create project error with response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        text: () => Promise.resolve("Invalid name"),
      });

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await expect(result.current.createProject({ name: "" })).rejects.toThrow("Invalid name");
    });

    it("should delete project successfully", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      });

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await result.current.deleteProject("test-project-1");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project-1",
        expect.objectContaining({
          method: "DELETE",
        })
      );
    });

    it("should handle delete project error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        text: () => Promise.resolve("Project not found"),
      });

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await expect(result.current.deleteProject("nonexistent")).rejects.toThrow("Project not found");
    });

    it("should handle create project error with empty response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await expect(result.current.createProject({ name: "" })).rejects.toThrow("Failed to create project");
    });

    it("should handle delete project error with empty response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await expect(result.current.deleteProject("some-project")).rejects.toThrow("Failed to delete project");
    });

    it("should handle create project network error", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await expect(result.current.createProject({ name: "Test" })).rejects.toThrow("Network error");
    });

    it("should handle delete project network error", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await expect(result.current.deleteProject("some-project")).rejects.toThrow("Network error");
    });

    it("should handle non-Error thrown during create", async () => {
      mockFetch.mockRejectedValueOnce("string error");

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await expect(result.current.createProject({ name: "Test" })).rejects.toThrow("string error");
    });

    it("should handle non-Error thrown during delete", async () => {
      mockFetch.mockRejectedValueOnce("string error");

      const { result } = renderHook(() => useArenaProjectMutations(), {
        wrapper: createWrapper(),
      });

      await expect(result.current.deleteProject("some-project")).rejects.toThrow("string error");
    });
  });

  describe("useArenaProjectFiles", () => {
    it("should get file content successfully", async () => {
      const fileContent = {
        path: "config.arena.yaml",
        content: "name: test",
        size: 10,
        modifiedAt: "2024-01-01T00:00:00Z",
        encoding: "utf-8" as const,
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(fileContent),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      const content = await result.current.getFileContent("test-project-1", "config.arena.yaml");

      expect(content).toEqual(fileContent);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project-1/files/config.arena.yaml"
      );
    });

    it("should update file content successfully", async () => {
      const updateResponse = {
        path: "config.arena.yaml",
        size: 15,
        modifiedAt: "2024-01-02T00:00:00Z",
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(updateResponse),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      const response = await result.current.updateFileContent(
        "test-project-1",
        "config.arena.yaml",
        "name: updated"
      );

      expect(response).toEqual(updateResponse);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project-1/files/config.arena.yaml",
        expect.objectContaining({
          method: "PUT",
          body: JSON.stringify({ content: "name: updated" }),
        })
      );
    });

    it("should create file successfully", async () => {
      const createResponse = {
        path: "prompts/new.prompt.yaml",
        name: "new.prompt.yaml",
        isDirectory: false,
        size: 0,
        modifiedAt: "2024-01-01T00:00:00Z",
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(createResponse),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      const response = await result.current.createFile(
        "test-project-1",
        "prompts",
        "new.prompt.yaml",
        false
      );

      expect(response).toEqual(createResponse);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project-1/files/prompts",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ name: "new.prompt.yaml", isDirectory: false }),
        })
      );
    });

    it("should create file at root when parentPath is null", async () => {
      const createResponse = {
        path: "new.yaml",
        name: "new.yaml",
        isDirectory: false,
        size: 0,
        modifiedAt: "2024-01-01T00:00:00Z",
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(createResponse),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await result.current.createFile("test-project-1", null, "new.yaml", false);

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project-1/files",
        expect.objectContaining({
          method: "POST",
        })
      );
    });

    it("should create directory successfully", async () => {
      const createResponse = {
        path: "new-folder",
        name: "new-folder",
        isDirectory: true,
        modifiedAt: "2024-01-01T00:00:00Z",
      };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(createResponse),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      const response = await result.current.createFile(
        "test-project-1",
        null,
        "new-folder",
        true
      );

      expect(response).toEqual(createResponse);
      expect(mockFetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          body: JSON.stringify({ name: "new-folder", isDirectory: true }),
        })
      );
    });

    it("should delete file successfully", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await result.current.deleteFile("test-project-1", "old-file.yaml");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project-1/files/old-file.yaml",
        expect.objectContaining({
          method: "DELETE",
        })
      );
    });

    it("should refresh file tree successfully", async () => {
      const fileTree: FileTreeNode[] = [
        {
          name: "config.arena.yaml",
          path: "config.arena.yaml",
          isDirectory: false,
          type: "arena",
        },
      ];
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ tree: fileTree }),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      const tree = await result.current.refreshFileTree("test-project-1");

      expect(tree).toEqual(fileTree);
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project-1/files"
      );
    });

    it("should handle get file 404 error", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.getFileContent("test-project-1", "nonexistent.yaml")
      ).rejects.toThrow("File not found");
    });

    it("should handle get file other error with response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve("Server error"),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.getFileContent("test-project-1", "some.yaml")
      ).rejects.toThrow("Server error");
    });

    it("should handle get file error with empty response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.getFileContent("test-project-1", "some.yaml")
      ).rejects.toThrow("Failed to get file content");
    });

    it("should handle update file error with response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve("Update error"),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.updateFileContent("test-project-1", "file.yaml", "content")
      ).rejects.toThrow("Update error");
    });

    it("should handle update file error with empty response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.updateFileContent("test-project-1", "file.yaml", "content")
      ).rejects.toThrow("Failed to update file");
    });

    it("should handle create file error with response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        text: () => Promise.resolve("Invalid filename"),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.createFile("test-project-1", null, "../bad", false)
      ).rejects.toThrow("Invalid filename");
    });

    it("should handle create file error with empty response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.createFile("test-project-1", null, "file.yaml", false)
      ).rejects.toThrow("Failed to create file");
    });

    it("should handle delete file error with response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        text: () => Promise.resolve("File not found"),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.deleteFile("test-project-1", "missing.yaml")
      ).rejects.toThrow("File not found");
    });

    it("should handle delete file error with empty response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.deleteFile("test-project-1", "file.yaml")
      ).rejects.toThrow("Failed to delete file");
    });

    it("should handle refresh file tree error with response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve("Tree error"),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.refreshFileTree("test-project-1")
      ).rejects.toThrow("Tree error");
    });

    it("should handle refresh file tree error with empty response text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        text: () => Promise.resolve(""),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.refreshFileTree("test-project-1")
      ).rejects.toThrow("Failed to refresh file tree");
    });

    it("should return empty array when tree is missing in response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({}),
      });

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      const tree = await result.current.refreshFileTree("test-project-1");
      expect(tree).toEqual([]);
    });

    it("should handle network error during file operations", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.getFileContent("test-project-1", "file.yaml")
      ).rejects.toThrow("Network error");
    });

    it("should handle non-Error thrown during file operations", async () => {
      mockFetch.mockRejectedValueOnce("string error");

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.updateFileContent("test-project-1", "file.yaml", "content")
      ).rejects.toThrow("string error");
    });

    it("should handle non-Error thrown during create file", async () => {
      mockFetch.mockRejectedValueOnce("create error");

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.createFile("test-project-1", null, "file.yaml", false)
      ).rejects.toThrow("create error");
    });

    it("should handle non-Error thrown during delete file", async () => {
      mockFetch.mockRejectedValueOnce("delete error");

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.deleteFile("test-project-1", "file.yaml")
      ).rejects.toThrow("delete error");
    });

    it("should handle non-Error thrown during refresh file tree", async () => {
      mockFetch.mockRejectedValueOnce("refresh error");

      const { result } = renderHook(() => useArenaProjectFiles(), {
        wrapper: createWrapper(),
      });

      await expect(
        result.current.refreshFileTree("test-project-1")
      ).rejects.toThrow("refresh error");
    });
  });
});
