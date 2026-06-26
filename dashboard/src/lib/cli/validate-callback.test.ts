/* eslint-disable sonarjs/no-clear-text-protocols */
import { describe, it, expect } from "vitest";
import { parseLoopbackCallback, isValidCliState } from "./validate-callback";

describe("parseLoopbackCallback", () => {
  it.each([
    "http://127.0.0.1:5000/cb",
    "http://localhost:8080/",
    "http://[::1]:9000/callback",
  ])("accepts loopback %s", (u) => {
    expect(parseLoopbackCallback(u)?.toString()).toBe(new URL(u).toString());
  });

  it.each([
    null,
    "",
    "not-a-url",
    "https://127.0.0.1:5000/cb",      // not http
    "http://evil.example.com:5000/cb", // not loopback
    "http://127.0.0.1/cb",            // no port
  ])("rejects %s", (u) => {
    expect(parseLoopbackCallback(u)).toBeNull();
  });
});

describe("isValidCliState", () => {
  it("accepts a URL-safe nonce of sane length", () => {
    expect(isValidCliState("abcd1234_~.-")).toBe(true);
  });
  it.each([null, "", "short", "x".repeat(257), "has space", "has/slash"])(
    "rejects %s",
    (s) => expect(isValidCliState(s)).toBe(false)
  );
});
