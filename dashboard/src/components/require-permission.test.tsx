import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { RequirePermission } from "./require-permission";

// Mock workspace permissions hook
const mockPermissions = {
  read: true,
  write: true,
  delete: true,
  manageMembers: true,
};

vi.mock("@/hooks/use-workspace-permissions", () => ({
  useWorkspacePermissions: () => ({
    permissions: mockPermissions,
    loading: false,
  }),
}));

describe("RequirePermission", () => {
  it("renders children when permission is granted", () => {
    render(
      <RequirePermission permission="write">
        <button>Create</button>
      </RequirePermission>
    );

    expect(screen.getByText("Create")).toBeInTheDocument();
  });

  it("hides children when permission is denied with default fallback", () => {
    mockPermissions.write = false;

    const { container } = render(
      <RequirePermission permission="write">
        <button>Create</button>
      </RequirePermission>
    );

    expect(container.innerHTML).toBe("");
    mockPermissions.write = true;
  });

  it("renders custom fallback when permission is denied", () => {
    mockPermissions.delete = false;

    render(
      <RequirePermission permission="delete" fallback={<span>No access</span>}>
        <button>Delete</button>
      </RequirePermission>
    );

    expect(screen.getByText("No access")).toBeInTheDocument();
    expect(screen.queryByText("Delete")).not.toBeInTheDocument();
    mockPermissions.delete = true;
  });

  it("disables children when fallback is 'disable' and permission denied", () => {
    mockPermissions.write = false;

    render(
      <RequirePermission permission="write" fallback="disable">
        <button>Scale</button>
      </RequirePermission>
    );

    const button = screen.getByText("Scale");
    expect(button).toBeDisabled();
    mockPermissions.write = true;
  });

  it("returns children as-is when fallback is 'disable' but children is not a valid element", () => {
    mockPermissions.write = false;

    render(
      <RequirePermission permission="write" fallback="disable">
        plain text
      </RequirePermission>
    );

    expect(screen.getByText("plain text")).toBeInTheDocument();
    mockPermissions.write = true;
  });
});
