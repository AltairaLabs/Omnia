import { describe, it, expect } from "vitest";
import { isChromelessPath } from "./routes";

describe("isChromelessPath", () => {
  it.each(["/login", "/login/", "/login/error", "/login/oauth/callback"])(
    "treats %s as chromeless",
    (path) => {
      expect(isChromelessPath(path)).toBe(true);
    },
  );

  it.each(["/", "/sessions", "/memory-analytics", "/loginx", "/api/login"])(
    "treats %s as a regular route",
    (path) => {
      expect(isChromelessPath(path)).toBe(false);
    },
  );
});
