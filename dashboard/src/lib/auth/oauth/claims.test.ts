import { describe, expect, it, vi } from "vitest";
import { mapClaimsToUserAsync, mapClaimsToUser } from "./claims";
import type { GraphTransport } from "./groups-overflow";
import type { AuthConfig } from "../config";

// Minimal AuthConfig for the tests — only the fields mapClaimsToUser
// actually reads. Casting through `unknown` keeps us decoupled from
// fields that are required at runtime but irrelevant to claim mapping.
const baseConfig = {
  oauth: {
    claims: {
      username: "preferred_username",
      email: "email",
      displayName: "name",
      groups: "groups",
    },
  },
  roleMapping: {
    admin: ["admins"],
    editor: ["editors"],
  },
} as unknown as AuthConfig;

describe("mapClaimsToUser (sync)", () => {
  it("maps a vanilla token with inline groups", () => {
    const user = mapClaimsToUser(
      {
        sub: "u1",
        preferred_username: "alice",
        email: "alice@example.com",
        name: "Alice",
        groups: ["editors"],
      },
      baseConfig,
    );

    expect(user).toEqual({
      id: "u1",
      username: "alice",
      email: "alice@example.com",
      displayName: "Alice",
      groups: ["editors"],
      role: "editor",
      provider: "oauth",
    });
  });
});

describe("mapClaimsToUserAsync — Entra groups overage (issue #855)", () => {
  it("returns the same shape as the sync mapper when no overage", async () => {
    const user = await mapClaimsToUserAsync(
      {
        sub: "u1",
        preferred_username: "alice",
        email: "alice@example.com",
        name: "Alice",
        groups: ["admins"],
      },
      baseConfig,
      "irrelevant-access-token",
    );

    expect(user.role).toBe("admin");
    expect(user.groups).toEqual(["admins"]);
  });

  it("resolves overage groups via Graph and recomputes role", async () => {
    const transport = vi.fn().mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ value: ["editors", "other-group"] }),
    });

    const user = await mapClaimsToUserAsync(
      {
        sub: "u1",
        oid: "abc-oid",
        preferred_username: "bob",
        email: "bob@example.com",
        name: "Bob",
        // Inline groups missing because of overage; ID token carries
        // the pointer instead.
        _claim_names: { groups: "src1" },
      },
      baseConfig,
      "fake-access-token",
      transport,
    );

    expect(user.groups).toEqual(["editors", "other-group"]);
    // Critical: the role MUST be recomputed off the resolved list.
    // Without that, an admin-overage user would resolve to viewer,
    // which is the original bug.
    expect(user.role).toBe("editor");
  });

  it("falls back to viewer (empty groups) when Graph returns 5xx", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const transport: GraphTransport = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 503,
      json: async () => ({ error: "boom" }),
    });

    const user = await mapClaimsToUserAsync(
      {
        sub: "u1",
        oid: "abc-oid",
        preferred_username: "carol",
        _claim_names: { groups: "src1" },
      },
      baseConfig,
      "fake-access-token",
      transport,
    );

    // Graph failure → fail open: keep whatever inline groups the token
    // had (none, in this case). Operator sees a console.warn.
    expect(user.role).toBe("viewer");
    expect(user.groups).toEqual([]);
    expect(warn).toHaveBeenCalledWith(expect.stringContaining("503"));
    warn.mockRestore();
  });

  it("warns and uses inline groups when overage detected without an access token", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});

    const user = await mapClaimsToUserAsync(
      {
        sub: "u1",
        oid: "abc-oid",
        preferred_username: "dan",
        _claim_names: { groups: "src1" },
      },
      baseConfig,
      undefined,
    );

    expect(user.groups).toEqual([]);
    expect(user.role).toBe("viewer");
    expect(warn).toHaveBeenCalledWith(
      expect.stringContaining("no access_token"),
    );
    warn.mockRestore();
  });
});
