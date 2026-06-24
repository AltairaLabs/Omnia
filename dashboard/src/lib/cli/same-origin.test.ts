import { describe, it, expect } from "vitest";
import { NextRequest } from "next/server";
import { isSameOrigin } from "./same-origin";

function reqWith(headers: Record<string, string>): NextRequest {
  return new NextRequest("http://localhost:3000/api/cli/grant", { method: "POST", headers });
}

describe("isSameOrigin", () => {
  it("allows a missing Origin header (non-browser / native nav)", () => {
    expect(isSameOrigin(reqWith({}))).toBe(true);
  });
  it("allows Origin whose host matches the forwarded host", () => {
    expect(isSameOrigin(reqWith({ origin: "https://omnia.example.com", "x-forwarded-host": "omnia.example.com" }))).toBe(true);
  });
  it("allows Origin matching the request host when no forwarded host", () => {
    expect(isSameOrigin(reqWith({ origin: "http://localhost:3000", host: "localhost:3000" }))).toBe(true);
  });
  it("rejects a cross-origin host", () => {
    expect(isSameOrigin(reqWith({ origin: "https://evil.example.com", "x-forwarded-host": "omnia.example.com" }))).toBe(false);
  });
});
