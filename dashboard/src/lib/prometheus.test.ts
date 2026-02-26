import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  queryPrometheus,
  queryPrometheusRange,
  isPrometheusAvailable,
  matrixToTimeSeries,
  type PrometheusResponse,
  type PrometheusVectorResult,
  type PrometheusMatrixResult,
} from "./prometheus";

// Mock global fetch
const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

describe("prometheus utilities", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("queryPrometheus", () => {
    it("should make instant query request", async () => {
      const mockResponse: PrometheusResponse<PrometheusVectorResult> = {
        status: "success",
        data: {
          resultType: "vector",
          result: [
            {
              metric: { __name__: "up", job: "prometheus" },
              value: [1704067200, "1"],
            },
          ],
        },
      };

      mockFetch.mockResolvedValueOnce({
        json: () => Promise.resolve(mockResponse),
      });

      const result = await queryPrometheus("up");

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/prometheus/query?query=up")
      );
      expect(result).toEqual(mockResponse);
    });

    it("should include time parameter when provided", async () => {
      mockFetch.mockResolvedValueOnce({
        json: () => Promise.resolve({ status: "success", data: { result: [] } }),
      });

      await queryPrometheus("up", 1704067200);

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("time=1704067200")
      );
    });

    it("should encode query string properly", async () => {
      mockFetch.mockResolvedValueOnce({
        json: () => Promise.resolve({ status: "success", data: { result: [] } }),
      });

      await queryPrometheus('rate(requests_total[5m])');

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("query=rate")
      );
    });
  });

  describe("queryPrometheusRange", () => {
    it("should make range query request", async () => {
      const mockResponse: PrometheusResponse<PrometheusMatrixResult> = {
        status: "success",
        data: {
          resultType: "matrix",
          result: [
            {
              metric: { agent: "test" },
              values: [
                [1704067200, "10"],
                [1704070800, "20"],
              ],
            },
          ],
        },
      };

      mockFetch.mockResolvedValueOnce({
        json: () => Promise.resolve(mockResponse),
      });

      const result = await queryPrometheusRange(
        "requests_total",
        1704067200,
        1704153600,
        "1h"
      );

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("/api/prometheus/query_range")
      );
      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("step=1h")
      );
      expect(result).toEqual(mockResponse);
    });

    it("should handle Date objects for start and end", async () => {
      mockFetch.mockResolvedValueOnce({
        json: () => Promise.resolve({ status: "success", data: { result: [] } }),
      });

      const start = new Date("2024-01-01T00:00:00.000Z");
      const end = new Date("2024-01-02T00:00:00.000Z");

      await queryPrometheusRange("requests_total", start, end);

      const callUrl = mockFetch.mock.calls[0][0];
      expect(callUrl).toContain("start=");
      expect(callUrl).toContain("end=");
    });

    it("should use default step of 1h", async () => {
      mockFetch.mockResolvedValueOnce({
        json: () => Promise.resolve({ status: "success", data: { result: [] } }),
      });

      await queryPrometheusRange("requests_total", 1704067200, 1704153600);

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringContaining("step=1h")
      );
    });
  });

  describe("isPrometheusAvailable", () => {
    it("should return true when Prometheus is reachable", async () => {
      mockFetch.mockResolvedValueOnce({
        json: () =>
          Promise.resolve({
            status: "success",
            data: { result: [] },
          }),
      });

      const available = await isPrometheusAvailable();
      expect(available).toBe(true);
    });

    it("should return false when query fails", async () => {
      mockFetch.mockResolvedValueOnce({
        json: () =>
          Promise.resolve({
            status: "error",
            error: "connection refused",
          }),
      });

      const available = await isPrometheusAvailable();
      expect(available).toBe(false);
    });

    it("should return false when fetch throws", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      const available = await isPrometheusAvailable();
      expect(available).toBe(false);
    });
  });

  describe("matrixToTimeSeries", () => {
    it("should convert matrix result to time series", () => {
      const result: PrometheusResponse<PrometheusMatrixResult> = {
        status: "success",
        data: {
          resultType: "matrix",
          result: [
            {
              metric: { agent: "agent1" },
              values: [
                [1704067200, "10"],
                [1704070800, "20"],
              ],
            },
          ],
        },
      };

      const timeSeries = matrixToTimeSeries(result);

      expect(timeSeries).toHaveLength(2);
      expect(timeSeries[0].timestamp).toEqual(new Date(1704067200 * 1000));
      expect(timeSeries[0].values.agent1).toBe(10);
      expect(timeSeries[1].values.agent1).toBe(20);
    });

    it("should merge multiple series at same timestamp", () => {
      const result: PrometheusResponse<PrometheusMatrixResult> = {
        status: "success",
        data: {
          resultType: "matrix",
          result: [
            {
              metric: { agent: "agent1" },
              values: [[1704067200, "10"]],
            },
            {
              metric: { agent: "agent2" },
              values: [[1704067200, "20"]],
            },
          ],
        },
      };

      const timeSeries = matrixToTimeSeries(result);

      expect(timeSeries).toHaveLength(1);
      expect(timeSeries[0].values.agent1).toBe(10);
      expect(timeSeries[0].values.agent2).toBe(20);
    });

    it("should sort by timestamp", () => {
      const result: PrometheusResponse<PrometheusMatrixResult> = {
        status: "success",
        data: {
          resultType: "matrix",
          result: [
            {
              metric: { agent: "test" },
              values: [
                [1704070800, "20"],
                [1704067200, "10"],
                [1704074400, "30"],
              ],
            },
          ],
        },
      };

      const timeSeries = matrixToTimeSeries(result);

      expect(timeSeries[0].values.test).toBe(10);
      expect(timeSeries[1].values.test).toBe(20);
      expect(timeSeries[2].values.test).toBe(30);
    });

    it("should use provider or model as key when agent is missing", () => {
      const result: PrometheusResponse<PrometheusMatrixResult> = {
        status: "success",
        data: {
          resultType: "matrix",
          result: [
            {
              metric: { provider: "anthropic" },
              values: [[1704067200, "10"]],
            },
            {
              metric: { model: "gpt-4" },
              values: [[1704067200, "20"]],
            },
          ],
        },
      };

      const timeSeries = matrixToTimeSeries(result);

      expect(timeSeries[0].values.anthropic).toBe(10);
      expect(timeSeries[0].values["gpt-4"]).toBe(20);
    });

    it("should use 'value' as default key when no labels match", () => {
      const result: PrometheusResponse<PrometheusMatrixResult> = {
        status: "success",
        data: {
          resultType: "matrix",
          result: [
            {
              metric: { other: "label" },
              values: [[1704067200, "10"]],
            },
          ],
        },
      };

      const timeSeries = matrixToTimeSeries(result);

      expect(timeSeries[0].values.value).toBe(10);
    });

    it("should return empty array for error response", () => {
      const result: PrometheusResponse<PrometheusMatrixResult> = {
        status: "error",
        error: "query error",
      };

      const timeSeries = matrixToTimeSeries(result);
      expect(timeSeries).toEqual([]);
    });

    it("should handle series with no values", () => {
      const result: PrometheusResponse<PrometheusMatrixResult> = {
        status: "success",
        data: {
          resultType: "matrix",
          result: [
            {
              metric: { agent: "test" },
              values: undefined as unknown as [number, string][],
            },
          ],
        },
      };

      const timeSeries = matrixToTimeSeries(result);
      expect(timeSeries).toEqual([]);
    });
  });
});
