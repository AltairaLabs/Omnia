import { describe, it, expect } from "vitest";
import fs from "fs";
import path from "path";
import { computeWorkspaceRole } from "./compute-workspace-role";
import type {
  RoleBinding,
  DirectGrant,
  AnonymousAccessConfig,
  WorkspaceRole,
} from "@/types/workspace";

/**
 * Shared TS↔Go parity fixture. The SAME JSON file is consumed by the Go
 * pkg/workspaceauth parity_test.go so the two implementations cannot drift.
 */
interface ParityCase {
  name: string;
  roleBindings?: RoleBinding[];
  directGrants?: DirectGrant[];
  anonymousAccess?: AnonymousAccessConfig;
  userGroups: string[];
  userIdentity: string;
  anonymous: boolean;
  expectedRole: WorkspaceRole | "";
}

const fixturePath = path.join(
  __dirname,
  "../../../../pkg/workspaceauth/testdata/parity_cases.json"
);

const cases: ParityCase[] = JSON.parse(fs.readFileSync(fixturePath, "utf-8"));

describe("computeWorkspaceRole parity with Go ComputeRole", () => {
  it("loads the shared fixture", () => {
    expect(cases.length).toBeGreaterThan(0);
  });

  for (const tc of cases) {
    it(tc.name, () => {
      const got = computeWorkspaceRole(
        {
          roleBindings: tc.roleBindings,
          directGrants: tc.directGrants,
          anonymousAccess: tc.anonymousAccess,
          userGroups: tc.userGroups,
          userIdentity: tc.userIdentity,
          isAnonymous: tc.anonymous,
        },
        Date.now()
      );
      const want = tc.expectedRole === "" ? null : tc.expectedRole;
      expect(got).toBe(want);
    });
  }
});
