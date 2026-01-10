import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useReadOnly } from "./use-read-only";
import { renderHook } from "@testing-library/react";

describe("useReadOnly", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    vi.resetModules();
    process.env = { ...originalEnv };
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  describe("isReadOnly", () => {
    it("should return false when NEXT_PUBLIC_READ_ONLY_MODE is not set", () => {
      delete process.env.NEXT_PUBLIC_READ_ONLY_MODE;
      const { result } = renderHook(() => useReadOnly());
      expect(result.current.isReadOnly).toBe(false);
    });

    it("should return false when NEXT_PUBLIC_READ_ONLY_MODE is 'false'", () => {
      process.env.NEXT_PUBLIC_READ_ONLY_MODE = "false";
      const { result } = renderHook(() => useReadOnly());
      expect(result.current.isReadOnly).toBe(false);
    });

    it("should return true when NEXT_PUBLIC_READ_ONLY_MODE is 'true'", () => {
      process.env.NEXT_PUBLIC_READ_ONLY_MODE = "true";
      const { result } = renderHook(() => useReadOnly());
      expect(result.current.isReadOnly).toBe(true);
    });
  });

  describe("message", () => {
    it("should return default message when NEXT_PUBLIC_READ_ONLY_MESSAGE is not set", () => {
      delete process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE;
      const { result } = renderHook(() => useReadOnly());
      expect(result.current.message).toBe(
        "Dashboard is in read-only mode. Changes must be made through GitOps."
      );
    });

    it("should return custom message when NEXT_PUBLIC_READ_ONLY_MESSAGE is set", () => {
      process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE = "Custom read-only message";
      const { result } = renderHook(() => useReadOnly());
      expect(result.current.message).toBe("Custom read-only message");
    });
  });

  describe("combined config", () => {
    it("should return correct config when in read-only mode with custom message", () => {
      process.env.NEXT_PUBLIC_READ_ONLY_MODE = "true";
      process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE = "Managed by ArgoCD";
      const { result } = renderHook(() => useReadOnly());
      expect(result.current).toEqual({
        isReadOnly: true,
        message: "Managed by ArgoCD",
      });
    });

    it("should return correct config when not in read-only mode", () => {
      process.env.NEXT_PUBLIC_READ_ONLY_MODE = "false";
      delete process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE;
      const { result } = renderHook(() => useReadOnly());
      expect(result.current).toEqual({
        isReadOnly: false,
        message: "Dashboard is in read-only mode. Changes must be made through GitOps.",
      });
    });
  });
});
