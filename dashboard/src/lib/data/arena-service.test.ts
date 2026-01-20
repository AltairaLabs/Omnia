/**
 * Tests for ArenaService API client.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { ArenaService } from "./arena-service";
import type { ArenaSource, ArenaConfig, ArenaJob, ArenaStats } from "@/types/arena";

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("ArenaService", () => {
  let service: ArenaService;

  beforeEach(() => {
    service = new ArenaService();
    mockFetch.mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  // ============================================================
  // ArenaSources
  // ============================================================

  describe("getArenaSources", () => {
    it("should fetch arena sources for a workspace", async () => {
      const mockSources: ArenaSource[] = [
        {
          apiVersion: "omnia.altairalabs.ai/v1alpha1",
          kind: "ArenaSource",
          metadata: { name: "test-source", namespace: "test-ws" },
          spec: { type: "configmap", configMapRef: { name: "test-cm" } },
        },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockSources),
      });

      const result = await service.getArenaSources("test-ws");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/sources");
      expect(result).toEqual(mockSources);
    });

    it("should return empty array on 401/403/404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 401 });
      expect(await service.getArenaSources("test-ws")).toEqual([]);

      mockFetch.mockResolvedValueOnce({ ok: false, status: 403 });
      expect(await service.getArenaSources("test-ws")).toEqual([]);

      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });
      expect(await service.getArenaSources("test-ws")).toEqual([]);
    });

    it("should throw on other errors", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Internal Server Error" });

      await expect(service.getArenaSources("test-ws")).rejects.toThrow(
        "Failed to fetch arena sources: Internal Server Error"
      );
    });

    it("should encode workspace name in URL", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve([]) });

      await service.getArenaSources("test workspace");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test%20workspace/arena/sources");
    });
  });

  describe("getArenaSource", () => {
    it("should fetch a single arena source", async () => {
      const mockSource: ArenaSource = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaSource",
        metadata: { name: "test-source", namespace: "test-ws" },
        spec: { type: "configmap", configMapRef: { name: "test-cm" } },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockSource),
      });

      const result = await service.getArenaSource("test-ws", "test-source");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/sources/test-source");
      expect(result).toEqual(mockSource);
    });

    it("should return undefined on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });

      const result = await service.getArenaSource("test-ws", "not-found");

      expect(result).toBeUndefined();
    });

    it("should throw on other errors", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Server Error" });

      await expect(service.getArenaSource("test-ws", "test")).rejects.toThrow(
        "Failed to fetch arena source: Server Error"
      );
    });
  });

  describe("createArenaSource", () => {
    it("should create an arena source", async () => {
      const mockSource: ArenaSource = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaSource",
        metadata: { name: "new-source", namespace: "test-ws" },
        spec: { type: "configmap", configMapRef: { name: "test-cm" } },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockSource),
      });

      const result = await service.createArenaSource("test-ws", "new-source", {
        type: "configmap",
        configMapRef: { name: "test-cm" },
      });

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/sources", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          metadata: { name: "new-source" },
          spec: { type: "configmap", configMapRef: { name: "test-cm" } },
        }),
      });
      expect(result).toEqual(mockSource);
    });

    it("should throw with error text on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve("Validation failed"),
      });

      await expect(
        service.createArenaSource("test-ws", "new-source", { type: "configmap" })
      ).rejects.toThrow("Validation failed");
    });

    it("should throw default message when no error text", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve(""),
      });

      await expect(
        service.createArenaSource("test-ws", "new-source", { type: "configmap" })
      ).rejects.toThrow("Failed to create arena source");
    });
  });

  describe("updateArenaSource", () => {
    it("should update an arena source", async () => {
      const mockSource: ArenaSource = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaSource",
        metadata: { name: "test-source", namespace: "test-ws" },
        spec: { type: "configmap", configMapRef: { name: "updated-cm" } },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockSource),
      });

      const result = await service.updateArenaSource("test-ws", "test-source", {
        type: "configmap",
        configMapRef: { name: "updated-cm" },
      });

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/sources/test-source", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ spec: { type: "configmap", configMapRef: { name: "updated-cm" } } }),
      });
      expect(result).toEqual(mockSource);
    });

    it("should throw on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve("Not found"),
      });

      await expect(
        service.updateArenaSource("test-ws", "test", { type: "configmap" })
      ).rejects.toThrow("Not found");
    });
  });

  describe("deleteArenaSource", () => {
    it("should delete an arena source", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.deleteArenaSource("test-ws", "test-source");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/sources/test-source", {
        method: "DELETE",
      });
    });

    it("should throw on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve("Cannot delete"),
      });

      await expect(service.deleteArenaSource("test-ws", "test")).rejects.toThrow("Cannot delete");
    });
  });

  describe("syncArenaSource", () => {
    it("should trigger sync for an arena source", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.syncArenaSource("test-ws", "test-source");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-ws/arena/sources/test-source/sync",
        { method: "POST" }
      );
    });

    it("should throw on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve("Sync failed"),
      });

      await expect(service.syncArenaSource("test-ws", "test")).rejects.toThrow("Sync failed");
    });
  });

  // ============================================================
  // ArenaConfigs
  // ============================================================

  describe("getArenaConfigs", () => {
    it("should fetch arena configs for a workspace", async () => {
      const mockConfigs: ArenaConfig[] = [
        {
          apiVersion: "omnia.altairalabs.ai/v1alpha1",
          kind: "ArenaConfig",
          metadata: { name: "test-config", namespace: "test-ws" },
          spec: { sourceRef: { name: "test-source" } },
        },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockConfigs),
      });

      const result = await service.getArenaConfigs("test-ws");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/configs");
      expect(result).toEqual(mockConfigs);
    });

    it("should return empty array on auth/not-found errors", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 401 });
      expect(await service.getArenaConfigs("test-ws")).toEqual([]);

      mockFetch.mockResolvedValueOnce({ ok: false, status: 403 });
      expect(await service.getArenaConfigs("test-ws")).toEqual([]);

      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });
      expect(await service.getArenaConfigs("test-ws")).toEqual([]);
    });

    it("should throw on other errors", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Error" });

      await expect(service.getArenaConfigs("test-ws")).rejects.toThrow(
        "Failed to fetch arena configs: Error"
      );
    });
  });

  describe("getArenaConfig", () => {
    it("should fetch a single arena config", async () => {
      const mockConfig: ArenaConfig = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaConfig",
        metadata: { name: "test-config", namespace: "test-ws" },
        spec: { sourceRef: { name: "test-source" } },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockConfig),
      });

      const result = await service.getArenaConfig("test-ws", "test-config");

      expect(result).toEqual(mockConfig);
    });

    it("should return undefined on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });

      expect(await service.getArenaConfig("test-ws", "not-found")).toBeUndefined();
    });
  });

  describe("getArenaConfigScenarios", () => {
    it("should fetch scenarios for a config", async () => {
      const mockScenarios = [
        { name: "scenario1", path: "scenarios/s1.yaml" },
        { name: "scenario2", path: "scenarios/s2.yaml" },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockScenarios),
      });

      const result = await service.getArenaConfigScenarios("test-ws", "test-config");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-ws/arena/configs/test-config/scenarios"
      );
      expect(result).toEqual(mockScenarios);
    });

    it("should return empty array on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });

      expect(await service.getArenaConfigScenarios("test-ws", "not-found")).toEqual([]);
    });
  });

  describe("createArenaConfig", () => {
    it("should create an arena config", async () => {
      const mockConfig: ArenaConfig = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaConfig",
        metadata: { name: "new-config", namespace: "test-ws" },
        spec: { sourceRef: { name: "test-source" } },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockConfig),
      });

      const result = await service.createArenaConfig("test-ws", "new-config", {
        sourceRef: { name: "test-source" },
      });

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/configs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          metadata: { name: "new-config" },
          spec: { sourceRef: { name: "test-source" } },
        }),
      });
      expect(result).toEqual(mockConfig);
    });
  });

  describe("updateArenaConfig", () => {
    it("should update an arena config", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({}),
      });

      await service.updateArenaConfig("test-ws", "test-config", {
        sourceRef: { name: "updated-source" },
      });

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/configs/test-config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ spec: { sourceRef: { name: "updated-source" } } }),
      });
    });
  });

  describe("deleteArenaConfig", () => {
    it("should delete an arena config", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.deleteArenaConfig("test-ws", "test-config");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/configs/test-config", {
        method: "DELETE",
      });
    });
  });

  // ============================================================
  // ArenaJobs
  // ============================================================

  describe("getArenaJobs", () => {
    it("should fetch arena jobs for a workspace", async () => {
      const mockJobs: ArenaJob[] = [
        {
          apiVersion: "omnia.altairalabs.ai/v1alpha1",
          kind: "ArenaJob",
          metadata: { name: "test-job", namespace: "test-ws" },
          spec: { configRef: { name: "test-config" }, type: "evaluation" },
        },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockJobs),
      });

      const result = await service.getArenaJobs("test-ws");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/jobs");
      expect(result).toEqual(mockJobs);
    });

    it("should add query parameters when options provided", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve([]),
      });

      await service.getArenaJobs("test-ws", {
        type: "evaluation",
        phase: "Running",
        configRef: "my-config",
        limit: 10,
        sort: "recent",
      });

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-ws/arena/jobs?type=evaluation&phase=Running&configRef=my-config&limit=10&sort=recent"
      );
    });

    it("should not add empty query string when no options", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve([]),
      });

      await service.getArenaJobs("test-ws", {});

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/jobs");
    });

    it("should return empty array on auth/not-found errors", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 401 });
      expect(await service.getArenaJobs("test-ws")).toEqual([]);

      mockFetch.mockResolvedValueOnce({ ok: false, status: 403 });
      expect(await service.getArenaJobs("test-ws")).toEqual([]);

      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });
      expect(await service.getArenaJobs("test-ws")).toEqual([]);
    });
  });

  describe("getArenaJob", () => {
    it("should fetch a single arena job", async () => {
      const mockJob: ArenaJob = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaJob",
        metadata: { name: "test-job", namespace: "test-ws" },
        spec: { configRef: { name: "test-config" }, type: "evaluation" },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockJob),
      });

      const result = await service.getArenaJob("test-ws", "test-job");

      expect(result).toEqual(mockJob);
    });

    it("should return undefined on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });

      expect(await service.getArenaJob("test-ws", "not-found")).toBeUndefined();
    });
  });

  describe("getArenaJobResults", () => {
    it("should fetch job results", async () => {
      const mockResults = {
        jobName: "test-job",
        completedAt: "2024-01-15T10:00:00Z",
        summary: { total: 10, passed: 8, failed: 2, errors: 0, skipped: 0, passRate: 0.8 },
        results: [],
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResults),
      });

      const result = await service.getArenaJobResults("test-ws", "test-job");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-ws/arena/jobs/test-job/results"
      );
      expect(result).toEqual(mockResults);
    });

    it("should return undefined on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });

      expect(await service.getArenaJobResults("test-ws", "not-found")).toBeUndefined();
    });
  });

  describe("getArenaJobMetrics", () => {
    it("should fetch job metrics", async () => {
      const mockMetrics = {
        currentRps: 100,
        latencyP50: 50,
        latencyP95: 100,
        latencyP99: 150,
        errorRate: 0.01,
        activeWorkers: 4,
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockMetrics),
      });

      const result = await service.getArenaJobMetrics("test-ws", "test-job");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-ws/arena/jobs/test-job/metrics"
      );
      expect(result).toEqual(mockMetrics);
    });

    it("should return undefined on 404", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });

      expect(await service.getArenaJobMetrics("test-ws", "not-found")).toBeUndefined();
    });
  });

  describe("createArenaJob", () => {
    it("should create an arena job", async () => {
      const mockJob: ArenaJob = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaJob",
        metadata: { name: "new-job", namespace: "test-ws" },
        spec: { configRef: { name: "test-config" }, type: "evaluation" },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockJob),
      });

      const result = await service.createArenaJob("test-ws", "new-job", {
        configRef: { name: "test-config" },
        type: "evaluation",
      });

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/jobs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          metadata: { name: "new-job" },
          spec: { configRef: { name: "test-config" }, type: "evaluation" },
        }),
      });
      expect(result).toEqual(mockJob);
    });
  });

  describe("cancelArenaJob", () => {
    it("should cancel an arena job", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.cancelArenaJob("test-ws", "test-job");

      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-ws/arena/jobs/test-job/cancel",
        { method: "POST" }
      );
    });

    it("should throw on failure", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve("Job already completed"),
      });

      await expect(service.cancelArenaJob("test-ws", "test-job")).rejects.toThrow(
        "Job already completed"
      );
    });
  });

  describe("deleteArenaJob", () => {
    it("should delete an arena job", async () => {
      mockFetch.mockResolvedValueOnce({ ok: true });

      await service.deleteArenaJob("test-ws", "test-job");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/jobs/test-job", {
        method: "DELETE",
      });
    });
  });

  // ============================================================
  // Stats
  // ============================================================

  describe("getArenaStats", () => {
    it("should fetch arena stats for a workspace", async () => {
      const mockStats: ArenaStats = {
        sources: { total: 5, ready: 4, failed: 1, active: 4 },
        configs: { total: 3, ready: 3, scenarios: 20 },
        jobs: { total: 10, running: 1, queued: 0, completed: 8, failed: 1, successRate: 0.89 },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockStats),
      });

      const result = await service.getArenaStats("test-ws");

      expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-ws/arena/stats");
      expect(result).toEqual(mockStats);
    });

    it("should return default stats on auth/not-found errors", async () => {
      const defaultStats = {
        sources: { total: 0, ready: 0, failed: 0, active: 0 },
        configs: { total: 0, ready: 0, scenarios: 0 },
        jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
      };

      mockFetch.mockResolvedValueOnce({ ok: false, status: 401 });
      expect(await service.getArenaStats("test-ws")).toEqual(defaultStats);

      mockFetch.mockResolvedValueOnce({ ok: false, status: 403 });
      expect(await service.getArenaStats("test-ws")).toEqual(defaultStats);

      mockFetch.mockResolvedValueOnce({ ok: false, status: 404 });
      expect(await service.getArenaStats("test-ws")).toEqual(defaultStats);
    });

    it("should throw on other errors", async () => {
      mockFetch.mockResolvedValueOnce({ ok: false, status: 500, statusText: "Server Error" });

      await expect(service.getArenaStats("test-ws")).rejects.toThrow(
        "Failed to fetch arena stats: Server Error"
      );
    });
  });

  // ============================================================
  // Service properties
  // ============================================================

  describe("service properties", () => {
    it("should have correct name", () => {
      expect(service.name).toBe("ArenaService");
    });
  });
});
