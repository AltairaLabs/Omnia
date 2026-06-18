import { describe, it, expect } from "vitest";
import { computeWorkspaceRole } from "./compute-workspace-role";

const NOW = Date.UTC(2026, 0, 1);

describe("computeWorkspaceRole", () => {
  it("returns role from a matching role binding", () => {
    const role = computeWorkspaceRole(
      {
        roleBindings: [{ groups: ["eng"], role: "viewer" }],
        userGroups: ["eng"],
        userIdentity: "u@x.io",
        isAnonymous: false,
      },
      NOW
    );
    expect(role).toBe("viewer");
  });

  it("returns the highest of multiple matching bindings", () => {
    const role = computeWorkspaceRole(
      {
        roleBindings: [
          { groups: ["eng"], role: "viewer" },
          { groups: ["admins"], role: "owner" },
        ],
        userGroups: ["eng", "admins"],
        userIdentity: "u@x.io",
        isAnonymous: false,
      },
      NOW
    );
    expect(role).toBe("owner");
  });

  it("returns null when no group matches and no grant", () => {
    const role = computeWorkspaceRole(
      {
        roleBindings: [{ groups: ["admins"], role: "owner" }],
        userGroups: ["other"],
        userIdentity: "u@x.io",
        isAnonymous: false,
      },
      NOW
    );
    expect(role).toBeNull();
  });

  it("matches a direct grant case-insensitively", () => {
    const role = computeWorkspaceRole(
      {
        directGrants: [{ user: "U@X.io", role: "editor" }],
        userGroups: [],
        userIdentity: "u@x.io",
        isAnonymous: false,
      },
      NOW
    );
    expect(role).toBe("editor");
  });

  it("takes the max of binding and grant", () => {
    const role = computeWorkspaceRole(
      {
        roleBindings: [{ groups: ["eng"], role: "viewer" }],
        directGrants: [{ user: "u@x.io", role: "owner" }],
        userGroups: ["eng"],
        userIdentity: "u@x.io",
        isAnonymous: false,
      },
      NOW
    );
    expect(role).toBe("owner");
  });

  it("ignores an expired direct grant", () => {
    const role = computeWorkspaceRole(
      {
        directGrants: [
          { user: "u@x.io", role: "owner", expires: "2020-01-01T00:00:00Z" },
        ],
        userGroups: [],
        userIdentity: "u@x.io",
        isAnonymous: false,
      },
      NOW
    );
    expect(role).toBeNull();
  });

  it("honors a non-expired direct grant", () => {
    const role = computeWorkspaceRole(
      {
        directGrants: [
          { user: "u@x.io", role: "owner", expires: "2099-01-01T00:00:00Z" },
        ],
        userGroups: [],
        userIdentity: "u@x.io",
        isAnonymous: false,
      },
      NOW
    );
    expect(role).toBe("owner");
  });

  it("returns the configured anonymous role when anonymous and enabled", () => {
    const role = computeWorkspaceRole(
      {
        anonymousAccess: { enabled: true, role: "editor" },
        userGroups: [],
        userIdentity: "",
        isAnonymous: true,
      },
      NOW
    );
    expect(role).toBe("editor");
  });

  it("defaults anonymous role to viewer", () => {
    const role = computeWorkspaceRole(
      {
        anonymousAccess: { enabled: true },
        userGroups: [],
        userIdentity: "",
        isAnonymous: true,
      },
      NOW
    );
    expect(role).toBe("viewer");
  });

  it("returns null when anonymous access disabled", () => {
    const role = computeWorkspaceRole(
      {
        anonymousAccess: { enabled: false },
        userGroups: [],
        userIdentity: "",
        isAnonymous: true,
      },
      NOW
    );
    expect(role).toBeNull();
  });

  it("returns null for identity-less non-anonymous principal with no anon config", () => {
    const role = computeWorkspaceRole(
      {
        roleBindings: [{ groups: ["eng"], role: "owner" }],
        userGroups: ["eng"],
        userIdentity: undefined,
        isAnonymous: false,
      },
      NOW
    );
    expect(role).toBeNull();
  });
});
