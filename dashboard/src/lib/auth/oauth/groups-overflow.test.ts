import { describe, expect, it, vi } from "vitest";
import { resolveGroupsOverflow, type GraphTransport } from "./groups-overflow";

// Tests for the Entra "groups overage" handler (issue #855).
//
// We never hit the real Microsoft Graph from tests; transport is a
// mock function that asserts on URL/headers and returns canned bodies.

const accessToken = "fake-access-token";

function makeClaims(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    sub: "00000000-0000-0000-0000-000000000001",
    oid: "11111111-1111-1111-1111-111111111111",
    ...overrides,
  };
}

function overageClaims(): Record<string, unknown> {
  return makeClaims({
    _claim_names: { groups: "src1" },
    _claim_sources: {
      src1: {
        endpoint:
          "https://graph.windows.net/contoso/users/11111111-1111-1111-1111-111111111111/getMemberObjects",
      },
    },
  });
}

describe("resolveGroupsOverflow", () => {
  it("returns inline groups when claims have no _claim_names pointer", async () => {
    const inline = ["g1", "g2"];
    const transport = vi.fn() as unknown as GraphTransport;

    const result = await resolveGroupsOverflow(makeClaims(), inline, accessToken, transport);

    expect(result).toEqual({ kind: "inline", groups: inline });
    expect(transport).not.toHaveBeenCalled();
  });

  it("returns inline groups when _claim_names exists but doesn't point at groups", async () => {
    const claims = makeClaims({ _claim_names: { other: "src2" } });
    const transport = vi.fn() as unknown as GraphTransport;

    const result = await resolveGroupsOverflow(claims, ["x"], accessToken, transport);

    expect(result).toEqual({ kind: "inline", groups: ["x"] });
    expect(transport).not.toHaveBeenCalled();
  });

  it("returns no_token when overage detected without an access token", async () => {
    const transport = vi.fn() as unknown as GraphTransport;

    const result = await resolveGroupsOverflow(overageClaims(), [], undefined, transport);

    expect(result.kind).toBe("no_token");
    expect(result.groups).toEqual([]);
    expect(result.reason).toContain("no access_token");
    expect(transport).not.toHaveBeenCalled();
  });

  it("calls Graph getMemberObjects and returns the resolved groups", async () => {
    const transport = vi.fn().mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ value: ["group-1", "group-2", "group-3"] }),
    });

    const result = await resolveGroupsOverflow(overageClaims(), [], accessToken, transport);

    expect(result.kind).toBe("resolved");
    expect(result.groups).toEqual(["group-1", "group-2", "group-3"]);
    expect(transport).toHaveBeenCalledTimes(1);

    const [url, init] = transport.mock.calls[0];
    expect(url).toBe(
      "https://graph.microsoft.com/v1.0/users/11111111-1111-1111-1111-111111111111/getMemberObjects",
    );
    expect(init.method).toBe("POST");
    expect(init.headers.Authorization).toBe(`Bearer ${accessToken}`);
    expect(init.headers["Content-Type"]).toBe("application/json");
    // The Graph contract: securityEnabledOnly=false returns ALL groups
    // (security + Microsoft 365), matching the inline `groups` claim's
    // default behaviour.
    expect(JSON.parse(init.body!)).toEqual({ securityEnabledOnly: false });
  });

  it("follows @odata.nextLink for pagination and de-duplicates", async () => {
    const transport = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          value: ["g1", "g2", "g3"],
          "@odata.nextLink":
            "https://graph.microsoft.com/v1.0/users/.../getMemberObjects?$skiptoken=abc",
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          // g3 is repeated on purpose — must be de-duplicated.
          value: ["g3", "g4", "g5"],
        }),
      });

    const result = await resolveGroupsOverflow(overageClaims(), [], accessToken, transport);

    expect(result.kind).toBe("resolved");
    expect(result.groups.sort()).toEqual(["g1", "g2", "g3", "g4", "g5"]);
    expect(transport).toHaveBeenCalledTimes(2);

    // Pagination follow-up uses GET (Graph quirk: getMemberObjects is
    // POST for the initial call, but @odata.nextLink is GET).
    expect(transport.mock.calls[1][1].method).toBe("GET");
    expect(transport.mock.calls[1][1].body).toBeUndefined();
  });

  it("graceful degrade when Graph returns 5xx", async () => {
    const transport = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      json: async () => ({ error: "service unavailable" }),
    });

    const result = await resolveGroupsOverflow(
      overageClaims(),
      ["fallback-from-inline-which-was-empty"],
      accessToken,
      transport,
    );

    expect(result.kind).toBe("graph_failed");
    expect(result.reason).toContain("503");
    // Returns the inline groups (which the caller passed in) so a
    // failed Graph call keeps the user at whatever the token's inline
    // claim said, rather than zeroing them out.
    expect(result.groups).toEqual(["fallback-from-inline-which-was-empty"]);
  });

  it("graceful degrade when Graph returns 429", async () => {
    const transport = vi.fn().mockResolvedValue({
      ok: false,
      status: 429,
      json: async () => ({ error: "too many requests" }),
    });

    const result = await resolveGroupsOverflow(overageClaims(), [], accessToken, transport);

    expect(result.kind).toBe("graph_failed");
    expect(result.reason).toContain("429");
  });

  it("graceful degrade when transport throws", async () => {
    const transport = vi.fn().mockRejectedValue(new Error("ECONNRESET"));

    const result = await resolveGroupsOverflow(overageClaims(), [], accessToken, transport);

    expect(result.kind).toBe("graph_failed");
    expect(result.reason).toContain("ECONNRESET");
  });

  it("uses sub when oid is missing", async () => {
    const claims = makeClaims({
      _claim_names: { groups: "src1" },
      oid: undefined,
    });
    const transport = vi.fn().mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({ value: ["g1"] }),
    });

    const result = await resolveGroupsOverflow(claims, [], accessToken, transport);

    expect(result.kind).toBe("resolved");
    const [url] = transport.mock.calls[0];
    expect(url).toContain("/users/00000000-0000-0000-0000-000000000001/");
  });

  it("returns graph_failed when both oid and sub are missing", async () => {
    const claims = {
      _claim_names: { groups: "src1" },
    };
    const transport = vi.fn() as unknown as GraphTransport;

    const result = await resolveGroupsOverflow(claims, [], accessToken, transport);

    expect(result.kind).toBe("graph_failed");
    expect(result.reason).toContain("missing both oid and sub");
    expect(transport).not.toHaveBeenCalled();
  });

  it("stops following @odata.nextLink after MAX_PAGES (10)", async () => {
    // Always return a nextLink — would loop forever without the bound.
    const transport = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        value: ["g"],
        "@odata.nextLink": "https://graph.microsoft.com/v1.0/users/.../getMemberObjects?$skiptoken=looping",
      }),
    });

    const result = await resolveGroupsOverflow(overageClaims(), [], accessToken, transport);

    expect(result.kind).toBe("resolved");
    expect(transport).toHaveBeenCalledTimes(10);
  });
});
