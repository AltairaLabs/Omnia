import { describe, it, expect } from "vitest";
import { parseLogLine } from "./log-parser";

describe("parseLogLine", () => {
  describe("Zap production JSON", () => {
    it("extracts level, msg, ts, and fields from JSON body", () => {
      const line =
        '2024-01-15T10:30:00.123456789Z {"level":"info","ts":1706789012.123,"caller":"server/main.go:42","msg":"pool created","maxConns":25,"host":"localhost"}';
      const result = parseLogLine(line, "runtime");

      expect(result.level).toBe("info");
      expect(result.message).toBe("pool created");
      expect(result.container).toBe("runtime");
      // Uses app timestamp from ts field, not K8s prefix
      expect(result.timestamp).toBe(new Date(1706789012.123 * 1000).toISOString());
      expect(result.fields).toEqual({ maxConns: 25, host: "localhost" });
    });

    it("maps warn level correctly", () => {
      const line =
        '2024-01-15T10:30:00Z {"level":"warn","ts":1706789012.0,"msg":"high latency"}';
      const result = parseLogLine(line, "facade");

      expect(result.level).toBe("warn");
      expect(result.message).toBe("high latency");
    });

    it("maps error level correctly", () => {
      const line =
        '2024-01-15T10:30:00Z {"level":"error","ts":1706789012.0,"msg":"connection failed","err":"timeout"}';
      const result = parseLogLine(line, "facade");

      expect(result.level).toBe("error");
      expect(result.fields).toEqual({ err: "timeout" });
    });

    it("maps dpanic/panic/fatal to error", () => {
      const line =
        '2024-01-15T10:30:00Z {"level":"dpanic","ts":1706789012.0,"msg":"critical failure"}';
      expect(parseLogLine(line, "runtime").level).toBe("error");
    });

    it("maps debug level correctly", () => {
      const line =
        '2024-01-15T10:30:00Z {"level":"debug","ts":1706789012.0,"msg":"trace info"}';
      expect(parseLogLine(line, "runtime").level).toBe("debug");
    });

    it("excludes caller, stacktrace, logger from fields", () => {
      const line =
        '2024-01-15T10:30:00Z {"level":"info","ts":1706789012.0,"msg":"ok","caller":"main.go:1","stacktrace":"...","logger":"test","extra":"val"}';
      const result = parseLogLine(line, "runtime");

      expect(result.fields).toEqual({ extra: "val" });
    });

    it("returns undefined fields when no extra keys exist", () => {
      const line =
        '2024-01-15T10:30:00Z {"level":"info","ts":1706789012.0,"msg":"ok","caller":"main.go:1"}';
      const result = parseLogLine(line, "runtime");

      expect(result.fields).toBeUndefined();
    });

    it("uses time field if ts is missing", () => {
      const line =
        '2024-01-15T10:30:00Z {"level":"info","time":"2024-01-15T10:30:12.000Z","msg":"ok"}';
      const result = parseLogLine(line, "runtime");

      expect(result.timestamp).toBe("2024-01-15T10:30:12.000Z");
    });

    it("falls back to K8s timestamp if ts and time are missing", () => {
      const line =
        '2024-01-15T10:30:00.123Z {"level":"info","msg":"ok"}';
      const result = parseLogLine(line, "runtime");

      expect(result.timestamp).toBe("2024-01-15T10:30:00.123Z");
    });
  });

  describe("Zap development format", () => {
    it("extracts level, message, and JSON fields from tab-separated format", () => {
      const line =
        '2024-01-15T10:30:00Z 2024-01-15T10:30:12.000Z\tDEBUG\tsession-service\tsession retrieved\t{"sessionID":"abc","tier":"warm"}';
      const result = parseLogLine(line, "facade");

      expect(result.level).toBe("debug");
      expect(result.message).toBe("session retrieved");
      expect(result.container).toBe("facade");
      expect(result.timestamp).toBe("2024-01-15T10:30:12.000Z");
      expect(result.fields).toEqual({ sessionID: "abc", tier: "warm" });
    });

    it("handles dev format without trailing JSON fields", () => {
      const line =
        '2024-01-15T10:30:00Z 2024-01-15T10:30:12.000Z\tINFO\tserver\tstarting up';
      const result = parseLogLine(line, "runtime");

      expect(result.level).toBe("info");
      expect(result.message).toBe("starting up");
      expect(result.fields).toBeUndefined();
    });

    it("handles WARN level", () => {
      const line =
        '2024-01-15T10:30:00Z 2024-01-15T10:30:12.000Z\tWARN\tserver\thigh memory';
      const result = parseLogLine(line, "runtime");

      expect(result.level).toBe("warn");
    });
  });

  describe("unstructured text", () => {
    it("falls back to raw message with K8s timestamp", () => {
      const line = "2024-01-15T10:30:00.123Z Starting server on port 8080";
      const result = parseLogLine(line, "facade");

      expect(result.timestamp).toBe("2024-01-15T10:30:00.123Z");
      expect(result.level).toBe("");
      expect(result.message).toBe("Starting server on port 8080");
      expect(result.container).toBe("facade");
      expect(result.fields).toBeUndefined();
    });

    it("handles lines with no K8s timestamp prefix", () => {
      const line = "some random log output";
      const result = parseLogLine(line, "runtime");

      expect(result.level).toBe("");
      expect(result.message).toBe("some random log output");
      expect(result.fields).toBeUndefined();
    });
  });

  describe("malformed input", () => {
    it("handles malformed JSON gracefully", () => {
      const line = '2024-01-15T10:30:00Z {"level":"info","msg":"broken';
      const result = parseLogLine(line, "runtime");

      // Falls through to unstructured since JSON parse fails
      expect(result.level).toBe("");
      expect(result.message).toContain("broken");
    });

    it("handles empty line", () => {
      const result = parseLogLine("", "runtime");
      expect(result.message).toBe("");
      expect(result.container).toBe("runtime");
    });

    it("handles JSON that is not an object", () => {
      const line = '2024-01-15T10:30:00Z "just a string"';
      const result = parseLogLine(line, "runtime");

      // Falls through since it doesn't start with {
      expect(result.level).toBe("");
    });

    it("handles JSON array body", () => {
      const line = "2024-01-15T10:30:00Z [1,2,3]";
      const result = parseLogLine(line, "runtime");

      // Falls through since it doesn't start with {
      expect(result.level).toBe("");
      expect(result.message).toBe("[1,2,3]");
    });
  });

  describe("no K8s timestamp prefix", () => {
    it("parses JSON body without K8s prefix", () => {
      const line = '{"level":"info","ts":1706789012.0,"msg":"direct log"}';
      const result = parseLogLine(line, "runtime");

      expect(result.level).toBe("info");
      expect(result.message).toBe("direct log");
    });
  });
});
