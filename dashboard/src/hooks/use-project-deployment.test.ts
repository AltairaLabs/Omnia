import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace", namespace: "test-ns" },
  }),
}));

// Import after mocks
import {
  useProjectDeploymentStatus,
  useProjectDeploymentMutations,
  useProjectDeployment,
} from "./use-project-deployment";

describe("use-project-deployment", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe("useProjectDeploymentStatus", () => {
    it("should set loading false when projectId is undefined", async () => {
      const { result } = renderHook(() =>
        useProjectDeploymentStatus(undefined)
      );

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.status).toBeNull();
    });

    it("should fetch deployment status when projectId is provided", async () => {
      const mockStatus = { deployed: false };
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockStatus),
      } as Response);

      const { result } = renderHook(() =>
        useProjectDeploymentStatus("test-project")
      );

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(global.fetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project/deployment"
      );
    });

    it("should return status data when deployed", async () => {
      const mockStatus = {
        deployed: true,
        source: { metadata: { name: "test-source" } },
      };
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockStatus),
      } as Response);

      const { result } = renderHook(() =>
        useProjectDeploymentStatus("test-project")
      );

      await waitFor(() => {
        expect(result.current.status?.deployed).toBe(true);
      });
    });

    it("should handle fetch errors", async () => {
      vi.mocked(global.fetch).mockRejectedValueOnce(new Error("Network error"));

      const { result } = renderHook(() =>
        useProjectDeploymentStatus("test-project")
      );

      await waitFor(() => {
        expect(result.current.error).toBeTruthy();
      });

      expect(result.current.status).toBeNull();
    });
  });

  describe("useProjectDeploymentMutations", () => {
    it("should have deploy function", () => {
      const { result } = renderHook(() => useProjectDeploymentMutations());

      expect(typeof result.current.deploy).toBe("function");
    });

    it("should call fetch when deploy is called", async () => {
      const mockResponse = {
        source: { metadata: { name: "test-source" } },
        configMap: { name: "test-config", namespace: "test-ns" },
        isNew: true,
      };
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response);

      const { result } = renderHook(() => useProjectDeploymentMutations());

      await act(async () => {
        await result.current.deploy("test-project");
      });

      expect(global.fetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project/deploy",
        expect.objectContaining({
          method: "POST",
        })
      );
    });

    it("should handle deploy errors", async () => {
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve("Deploy failed"),
      } as Response);

      const { result } = renderHook(() => useProjectDeploymentMutations());

      await expect(
        act(() => result.current.deploy("test-project"))
      ).rejects.toThrow("Deploy failed");
    });
  });

  describe("useProjectDeployment", () => {
    it("should return combined status and mutations", async () => {
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ deployed: false }),
      } as Response);

      const { result } = renderHook(() =>
        useProjectDeployment("test-project")
      );

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.status?.deployed).toBe(false);
      expect(typeof result.current.deploy).toBe("function");
      expect(typeof result.current.refetch).toBe("function");
    });
  });
});
