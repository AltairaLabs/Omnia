import { describe, it, expect } from "vitest";
import { secretErrorResponse } from "./secret-error";

function k8sErr(httpCode: number, message: string): Error {
  const e = new Error(`HTTP-Code: ${httpCode}\nBody: "..."`);
  (e as unknown as { code: number }).code = httpCode;
  (e as unknown as { body: string }).body = JSON.stringify({ message });
  return e;
}

describe("secretErrorResponse", () => {
  it("maps a 403 to status 403 with the real message", async () => {
    const res = secretErrorResponse(k8sErr(403, "secrets is forbidden: ..."), "Failed");
    expect(res.status).toBe(403);
    expect((await res.json()).error).toContain("forbidden");
  });

  it("maps the not-managed conflict to 409", async () => {
    const res = secretErrorResponse(new Error("Secret ns/s exists but is not a managed credential secret"), "Failed");
    expect(res.status).toBe(409);
  });

  it("falls back to 500 with the fallback message for an unknown error", async () => {
    const res = secretErrorResponse(new Error("boom"), "Failed to create/update secret");
    expect(res.status).toBe(500);
    expect((await res.json()).error).toContain("boom");
  });

  it("uses error.message when body is absent", async () => {
    const err = new Error("plain error message");
    const res = secretErrorResponse(err, "Fallback");
    expect(res.status).toBe(500);
    expect((await res.json()).error).toContain("plain error message");
  });

  it("extracts message from object body", async () => {
    const err = new Error("HTTP request failed");
    (err as unknown as { statusCode: number }).statusCode = 403;
    (err as unknown as { body: Record<string, unknown> }).body = {
      message: "secrets is forbidden: user does not have permission",
    };
    const res = secretErrorResponse(err, "Failed");
    expect(res.status).toBe(403);
    expect((await res.json()).error).toContain("forbidden");
  });
});
