/**
 * Tests for audit logger.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  logAudit,
  logCrdSuccess,
  logCrdDenied,
  logCrdError,
  logWarn,
  logError,
  createAuditLogger,
  methodToAction,
  isAuditLoggingEnabled,
} from "./logger";

describe("audit logger", () => {
  let consoleSpy: ReturnType<typeof vi.spyOn>;
  const originalEnv = process.env.OMNIA_AUDIT_LOGGING_ENABLED;

  beforeEach(() => {
    consoleSpy = vi.spyOn(console, "log").mockImplementation(() => {});
    // Reset env var for each test
    delete process.env.OMNIA_AUDIT_LOGGING_ENABLED;
  });

  afterEach(() => {
    consoleSpy.mockRestore();
    if (originalEnv !== undefined) {
      process.env.OMNIA_AUDIT_LOGGING_ENABLED = originalEnv;
    } else {
      delete process.env.OMNIA_AUDIT_LOGGING_ENABLED;
    }
  });

  describe("isAuditLoggingEnabled", () => {
    it("returns true by default", () => {
      expect(isAuditLoggingEnabled()).toBe(true);
    });

    it("returns false when explicitly disabled", () => {
      process.env.OMNIA_AUDIT_LOGGING_ENABLED = "false";
      expect(isAuditLoggingEnabled()).toBe(false);
    });

    it("returns true when set to true", () => {
      process.env.OMNIA_AUDIT_LOGGING_ENABLED = "true";
      expect(isAuditLoggingEnabled()).toBe(true);
    });
  });

  describe("logAudit", () => {
    it("logs audit entry as JSON to console", () => {
      logAudit({
        action: "create",
        resourceType: "AgentRuntime",
        resourceName: "my-agent",
        workspace: "test-workspace",
        namespace: "test-ns",
        user: "user@example.com",
        result: "success",
      });

      expect(consoleSpy).toHaveBeenCalledTimes(1);
      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.level).toBe("audit");
      expect(loggedJson.action).toBe("create");
      expect(loggedJson.resourceType).toBe("AgentRuntime");
      expect(loggedJson.resourceName).toBe("my-agent");
      expect(loggedJson.workspace).toBe("test-workspace");
      expect(loggedJson.namespace).toBe("test-ns");
      expect(loggedJson.user).toBe("user@example.com");
      expect(loggedJson.result).toBe("success");
      expect(loggedJson.timestamp).toBeDefined();
    });

    it("does not log when audit logging is disabled", () => {
      process.env.OMNIA_AUDIT_LOGGING_ENABLED = "false";

      logAudit({
        action: "create",
        resourceType: "AgentRuntime",
        workspace: "test",
        namespace: "test-ns",
        user: "user@example.com",
        result: "success",
      });

      expect(consoleSpy).not.toHaveBeenCalled();
    });

    it("includes optional fields when provided", () => {
      logAudit({
        action: "delete",
        resourceType: "PromptPack",
        resourceName: "my-pack",
        workspace: "prod",
        namespace: "prod-ns",
        user: "admin@example.com",
        role: "owner",
        authProvider: "oauth",
        result: "success",
        path: "/api/workspaces/prod/promptpacks/my-pack",
        method: "DELETE",
        metadata: { reason: "cleanup" },
      });

      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.role).toBe("owner");
      expect(loggedJson.authProvider).toBe("oauth");
      expect(loggedJson.path).toBe("/api/workspaces/prod/promptpacks/my-pack");
      expect(loggedJson.method).toBe("DELETE");
      expect(loggedJson.metadata).toEqual({ reason: "cleanup" });
    });

    it("includes error fields for failed operations", () => {
      logAudit({
        action: "update",
        resourceType: "AgentRuntime",
        resourceName: "broken-agent",
        workspace: "dev",
        namespace: "dev-ns",
        user: "user@example.com",
        result: "error",
        errorMessage: "Resource not found",
        statusCode: 404,
      });

      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.result).toBe("error");
      expect(loggedJson.errorMessage).toBe("Resource not found");
      expect(loggedJson.statusCode).toBe(404);
    });
  });

  describe("logCrdSuccess", () => {
    it("logs successful CRD operation", () => {
      logCrdSuccess(
        "create",
        "AgentRuntime",
        "my-agent",
        "workspace",
        "namespace",
        "user@example.com",
        "editor"
      );

      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.action).toBe("create");
      expect(loggedJson.resourceType).toBe("AgentRuntime");
      expect(loggedJson.resourceName).toBe("my-agent");
      expect(loggedJson.result).toBe("success");
      expect(loggedJson.role).toBe("editor");
    });

    it("includes metadata when provided", () => {
      logCrdSuccess(
        "list",
        "PromptPack",
        undefined,
        "workspace",
        "namespace",
        "user@example.com",
        "viewer",
        { count: 5 }
      );

      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.metadata).toEqual({ count: 5 });
    });
  });

  describe("logCrdDenied", () => {
    it("logs denied CRD operation with 403 status", () => {
      logCrdDenied(
        "delete",
        "AgentRuntime",
        "protected-agent",
        "workspace",
        "namespace",
        "viewer@example.com",
        "viewer",
        "Insufficient permissions"
      );

      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.action).toBe("delete");
      expect(loggedJson.result).toBe("denied");
      expect(loggedJson.statusCode).toBe(403);
      expect(loggedJson.errorMessage).toBe("Insufficient permissions");
    });
  });

  describe("logCrdError", () => {
    it("logs failed CRD operation with error details", () => {
      logCrdError(
        "update",
        "PromptPack",
        "broken-pack",
        "workspace",
        "namespace",
        "user@example.com",
        "editor",
        "Connection timeout",
        500
      );

      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.action).toBe("update");
      expect(loggedJson.result).toBe("error");
      expect(loggedJson.errorMessage).toBe("Connection timeout");
      expect(loggedJson.statusCode).toBe(500);
    });
  });

  describe("createAuditLogger", () => {
    it("creates a logger with bound context", () => {
      const log = createAuditLogger({
        workspace: "my-workspace",
        namespace: "my-namespace",
        user: "user@example.com",
        role: "editor",
        authProvider: "oauth",
        path: "/api/workspaces/my-workspace/agents",
        method: "GET",
      });

      log({
        action: "list",
        resourceType: "AgentRuntime",
        result: "success",
      });

      const loggedJson = JSON.parse(consoleSpy.mock.calls[0][0]);
      expect(loggedJson.workspace).toBe("my-workspace");
      expect(loggedJson.namespace).toBe("my-namespace");
      expect(loggedJson.user).toBe("user@example.com");
      expect(loggedJson.role).toBe("editor");
      expect(loggedJson.authProvider).toBe("oauth");
      expect(loggedJson.action).toBe("list");
      expect(loggedJson.resourceType).toBe("AgentRuntime");
    });
  });

  describe("methodToAction", () => {
    it("maps GET to get", () => {
      expect(methodToAction("GET")).toBe("get");
    });

    it("maps POST to create", () => {
      expect(methodToAction("POST")).toBe("create");
    });

    it("maps PUT to update", () => {
      expect(methodToAction("PUT")).toBe("update");
    });

    it("maps PATCH to patch", () => {
      expect(methodToAction("PATCH")).toBe("patch");
    });

    it("maps DELETE to delete", () => {
      expect(methodToAction("DELETE")).toBe("delete");
    });

    it("handles lowercase methods", () => {
      expect(methodToAction("get")).toBe("get");
      expect(methodToAction("post")).toBe("create");
    });

    it("defaults unknown methods to get", () => {
      expect(methodToAction("OPTIONS")).toBe("get");
      expect(methodToAction("HEAD")).toBe("get");
    });
  });

  describe("logWarn", () => {
    it("logs structured warning to stderr", () => {
      const consoleWarnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

      logWarn("Test warning message", "test-context", { key: "value" });

      expect(consoleWarnSpy).toHaveBeenCalledTimes(1);
      const loggedJson = JSON.parse(consoleWarnSpy.mock.calls[0][0]);
      expect(loggedJson.level).toBe("warn");
      expect(loggedJson.message).toBe("Test warning message");
      expect(loggedJson.context).toBe("test-context");
      expect(loggedJson.metadata).toEqual({ key: "value" });
      expect(loggedJson.timestamp).toBeDefined();

      consoleWarnSpy.mockRestore();
    });

    it("logs without optional fields", () => {
      const consoleWarnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

      logWarn("Simple warning");

      const loggedJson = JSON.parse(consoleWarnSpy.mock.calls[0][0]);
      expect(loggedJson.message).toBe("Simple warning");
      expect(loggedJson.context).toBeUndefined();
      expect(loggedJson.metadata).toBeUndefined();

      consoleWarnSpy.mockRestore();
    });
  });

  describe("logError", () => {
    it("logs structured error with Error object", () => {
      const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
      const testError = new Error("Test error");

      logError("Operation failed", testError, "test-context", { operation: "test" });

      expect(consoleErrorSpy).toHaveBeenCalledTimes(1);
      const loggedJson = JSON.parse(consoleErrorSpy.mock.calls[0][0]);
      expect(loggedJson.level).toBe("error");
      expect(loggedJson.message).toBe("Operation failed");
      expect(loggedJson.error).toBe("Test error");
      expect(loggedJson.stack).toBeDefined();
      expect(loggedJson.context).toBe("test-context");
      expect(loggedJson.metadata).toEqual({ operation: "test" });
      expect(loggedJson.timestamp).toBeDefined();

      consoleErrorSpy.mockRestore();
    });

    it("logs structured error with string error", () => {
      const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      logError("Operation failed", "String error message", "context");

      const loggedJson = JSON.parse(consoleErrorSpy.mock.calls[0][0]);
      expect(loggedJson.error).toBe("String error message");
      expect(loggedJson.stack).toBeUndefined();

      consoleErrorSpy.mockRestore();
    });

    it("logs without error object", () => {
      const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      logError("Simple error message");

      const loggedJson = JSON.parse(consoleErrorSpy.mock.calls[0][0]);
      expect(loggedJson.message).toBe("Simple error message");
      expect(loggedJson.error).toBeUndefined();
      expect(loggedJson.context).toBeUndefined();

      consoleErrorSpy.mockRestore();
    });
  });
});
