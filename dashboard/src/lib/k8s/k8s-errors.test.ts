import { describe, it, expect } from "vitest";
import { extractStatusCode, extractStatusMessage, isAuthError } from "./k8s-errors";

describe("extractStatusCode", () => {
  it("returns null for non-object errors", () => {
    expect(extractStatusCode(null)).toBeNull();
    expect(extractStatusCode(undefined)).toBeNull();
    expect(extractStatusCode("boom")).toBeNull();
    expect(extractStatusCode(42)).toBeNull();
  });

  it("reads a direct statusCode property", () => {
    expect(extractStatusCode({ statusCode: 401 })).toBe(401);
    expect(extractStatusCode({ statusCode: 404 })).toBe(404);
  });

  it("reads a nested response.statusCode", () => {
    expect(extractStatusCode({ response: { statusCode: 403 } })).toBe(403);
  });

  it("parses 'HTTP-Code: <n>' from the Error message (real k8s client shape)", () => {
    const err = new Error("HTTP-Code: 401\nMessage: Unauthorized\nBody: undefined");
    expect(extractStatusCode(err)).toBe(401);
    expect(extractStatusCode({ message: "HTTP-Code: 404\nMessage: Not Found" })).toBe(404);
  });

  it("parses a JSON string body's code", () => {
    expect(extractStatusCode({ body: '{"code":409,"message":"conflict"}' })).toBe(409);
  });

  it("ignores a non-JSON string body", () => {
    expect(extractStatusCode({ body: "not json" })).toBeNull();
  });

  it("reads an object body's code", () => {
    expect(extractStatusCode({ body: { code: 500 } })).toBe(500);
  });

  it("returns null when no status is present", () => {
    expect(extractStatusCode({ foo: "bar" })).toBeNull();
  });
});

describe("isAuthError", () => {
  it("is true for 401 in every shape", () => {
    expect(isAuthError({ statusCode: 401 })).toBe(true);
    expect(isAuthError({ response: { statusCode: 401 } })).toBe(true);
    expect(isAuthError(new Error("HTTP-Code: 401\nMessage: Unauthorized"))).toBe(true);
    expect(isAuthError({ body: '{"code":401}' })).toBe(true);
  });

  it("is false for non-401 / non-auth errors", () => {
    expect(isAuthError({ statusCode: 403 })).toBe(false);
    expect(isAuthError({ statusCode: 404 })).toBe(false);
    expect(isAuthError(new Error("HTTP-Code: 500"))).toBe(false);
    expect(isAuthError(new Error("plain error"))).toBe(false);
    expect(isAuthError(null)).toBe(false);
  });
});

describe("extractStatusMessage", () => {
  it("parses the Kubernetes Status.message from a JSON-string body", () => {
    const error = Object.assign(new Error("HTTP-Code: 422\nMessage: Unknown API Status Code!"), {
      body: JSON.stringify({ message: "ToolRegistry is invalid: spec.handlers[0].name", code: 422 }),
    });
    expect(extractStatusMessage(error, "fallback")).toBe(
      "ToolRegistry is invalid: spec.handlers[0].name"
    );
  });

  it("parses the message from an object body", () => {
    const error = Object.assign(new Error("x"), { body: { message: "already exists", code: 409 } });
    expect(extractStatusMessage(error, "fallback")).toBe("already exists");
  });

  it("falls back to the error message when the body has none", () => {
    expect(extractStatusMessage(new Error("plain boom"), "fallback")).toBe("plain boom");
  });

  it("falls back to the provided default for non-error input", () => {
    expect(extractStatusMessage(null, "fallback")).toBe("fallback");
    expect(extractStatusMessage("nope", "fallback")).toBe("fallback");
  });
});
