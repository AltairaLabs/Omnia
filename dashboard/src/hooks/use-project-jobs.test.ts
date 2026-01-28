import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";

// Mock workspace context first
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace", namespace: "test-ns" },
  }),
}));

// Import after mocks
import {
  useProjectJobs,
  useProjectRunMutations,
  useProjectJobsWithRun,
} from "./use-project-jobs";

describe("use-project-jobs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  describe("useProjectJobs", () => {
    it("should set loading false when projectId is undefined", async () => {
      const { result } = renderHook(() => useProjectJobs(undefined));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.jobs).toEqual([]);
    });

    it("should fetch jobs when projectId is provided", async () => {
      const mockJobs = {
        jobs: [{ metadata: { name: "job-1" } }],
        deployed: true,
      };
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockJobs),
      } as Response);

      const { result } = renderHook(() => useProjectJobs("test-project"));

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(global.fetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project/jobs"
      );
    });

    it("should return jobs data", async () => {
      const mockJobs = {
        jobs: [{ metadata: { name: "job-1" } }],
        deployed: true,
      };
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockJobs),
      } as Response);

      const { result } = renderHook(() => useProjectJobs("test-project"));

      await waitFor(() => {
        expect(result.current.jobs).toHaveLength(1);
      });
    });

    it("should handle fetch errors", async () => {
      vi.mocked(global.fetch).mockRejectedValueOnce(new Error("Network error"));

      const { result } = renderHook(() => useProjectJobs("test-project"));

      await waitFor(() => {
        expect(result.current.error).toBeTruthy();
      });

      expect(result.current.jobs).toEqual([]);
    });
  });

  describe("useProjectRunMutations", () => {
    it("should have run function", () => {
      const { result } = renderHook(() => useProjectRunMutations());

      expect(typeof result.current.run).toBe("function");
    });

    it("should call fetch when run is called", async () => {
      const mockResponse = {
        job: { metadata: { name: "job-1" } },
        source: { metadata: { name: "source-1" } },
      };
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      } as Response);

      const { result } = renderHook(() => useProjectRunMutations());

      await act(async () => {
        await result.current.run("test-project", { type: "evaluation" });
      });

      expect(global.fetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/projects/test-project/run",
        expect.objectContaining({
          method: "POST",
        })
      );
    });

    it("should handle run errors", async () => {
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: false,
        json: () => Promise.resolve({ message: "Run failed" }),
      } as Response);

      const { result } = renderHook(() => useProjectRunMutations());

      await expect(
        act(() => result.current.run("test-project", { type: "evaluation" }))
      ).rejects.toThrow("Run failed");
    });
  });

  describe("useProjectJobsWithRun", () => {
    it("should return combined jobs and run function", async () => {
      vi.mocked(global.fetch).mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ jobs: [], deployed: false }),
      } as Response);

      const { result } = renderHook(() =>
        useProjectJobsWithRun("test-project")
      );

      await waitFor(() => {
        expect(result.current.loading).toBe(false);
      });

      expect(result.current.jobs).toEqual([]);
      expect(typeof result.current.run).toBe("function");
      expect(typeof result.current.refetch).toBe("function");
    });
  });
});
