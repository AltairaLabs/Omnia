import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccessTab } from "./access-tab";
import type { Workspace } from "@/types/workspace";

const workspace: Workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "test-ws" },
  spec: {
    displayName: "Test",
    environment: "development",
    namespace: { name: "test-ns" },
    anonymousAccess: { enabled: true, role: "viewer" },
    roleBindings: [
      { groups: ["dev-team"], role: "editor" },
      { groups: ["ops-team"], role: "owner" },
    ],
    directGrants: [
      { user: "alice@example.com", role: "owner" },
      { user: "bob@example.com", role: "viewer", expires: "2026-12-31T00:00:00Z" },
    ],
  },
};

describe("AccessTab", () => {
  it("renders anonymous access toggle as checked", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    const toggle = screen.getByRole("switch");
    expect(toggle).toHaveAttribute("data-state", "checked");
  });

  it("renders role bindings group names", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    expect(screen.getByText("dev-team")).toBeInTheDocument();
    expect(screen.getByText("ops-team")).toBeInTheDocument();
  });

  it("renders direct grants user emails", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    expect(screen.getByText("bob@example.com")).toBeInTheDocument();
  });

  it("calls onPatch with disabled anonymous access when toggled", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    const toggle = screen.getByRole("switch");
    fireEvent.click(toggle);
    expect(onPatch).toHaveBeenCalledWith({
      anonymousAccess: { enabled: false },
    });
  });

  it("calls onPatch with remaining bindings when first binding is deleted", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    const deleteButtons = screen.getAllByTestId("delete-binding");
    fireEvent.click(deleteButtons[0]);
    expect(onPatch).toHaveBeenCalledWith({
      roleBindings: [{ groups: ["ops-team"], role: "owner" }],
    });
  });

  it("calls onPatch with remaining grants when first grant is deleted", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    const deleteButtons = screen.getAllByTestId("delete-grant");
    fireEvent.click(deleteButtons[0]);
    expect(onPatch).toHaveBeenCalledWith({
      directGrants: [
        { user: "bob@example.com", role: "viewer", expires: "2026-12-31T00:00:00Z" },
      ],
    });
  });

  it("shows Never for grants without expiration", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    expect(screen.getByText("Never")).toBeInTheDocument();
  });

  it("renders section titles", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    expect(screen.getByText("Anonymous Access")).toBeInTheDocument();
    expect(screen.getByText("Role Bindings")).toBeInTheDocument();
    expect(screen.getByText("Direct Grants")).toBeInTheDocument();
  });

  it("hides role select when anonymous access is disabled", () => {
    const onPatch = vi.fn();
    const ws: Workspace = {
      ...workspace,
      spec: {
        ...workspace.spec,
        anonymousAccess: { enabled: false },
      },
    };
    render(<AccessTab workspace={ws} onPatch={onPatch} />);
    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  it("shows elevated access warning for editor/owner anonymous role", () => {
    const onPatch = vi.fn();
    const ws: Workspace = {
      ...workspace,
      spec: {
        ...workspace.spec,
        anonymousAccess: { enabled: true, role: "editor" },
      },
    };
    render(<AccessTab workspace={ws} onPatch={onPatch} />);
    expect(screen.getByText("Elevated access")).toBeInTheDocument();
  });

  it("does not show elevated access warning for viewer role", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    expect(screen.queryByText("Elevated access")).not.toBeInTheDocument();
  });

  it("adds a new role binding when add button is clicked", async () => {
    const onPatch = vi.fn();
    const user = userEvent.setup();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);

    const groupInput = screen.getByPlaceholderText("Group name");
    await user.type(groupInput, "new-group");

    // Find the add button (Plus) - it's in the same row as the input
    const addButtons = screen.getAllByRole("button");
    // The add button for bindings is after the delete buttons for bindings
    const bindingAddButton = addButtons.find(
      (btn) =>
        btn.closest("tr")?.querySelector("input[placeholder='Group name']") !==
        null
    );
    expect(bindingAddButton).toBeDefined();
    await user.click(bindingAddButton!);

    expect(onPatch).toHaveBeenCalledWith({
      roleBindings: [
        ...workspace.spec.roleBindings!,
        { groups: ["new-group"], role: "viewer" },
      ],
    });
  });

  it("adds a new direct grant when add button is clicked", async () => {
    const onPatch = vi.fn();
    const user = userEvent.setup();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);

    const userInput = screen.getByPlaceholderText("User email");
    await user.type(userInput, "charlie@example.com");

    const addButtons = screen.getAllByRole("button");
    const grantAddButton = addButtons.find(
      (btn) =>
        btn.closest("tr")?.querySelector("input[placeholder='User email']") !==
        null
    );
    expect(grantAddButton).toBeDefined();
    await user.click(grantAddButton!);

    expect(onPatch).toHaveBeenCalledWith({
      directGrants: [
        ...workspace.spec.directGrants!,
        { user: "charlie@example.com", role: "viewer" },
      ],
    });
  });

  it("handles workspace with no roleBindings or directGrants", () => {
    const onPatch = vi.fn();
    const ws: Workspace = {
      ...workspace,
      spec: {
        ...workspace.spec,
        roleBindings: undefined,
        directGrants: undefined,
        anonymousAccess: undefined,
      },
    };
    render(<AccessTab workspace={ws} onPatch={onPatch} />);
    // Switch should be unchecked
    const toggle = screen.getByRole("switch");
    expect(toggle).toHaveAttribute("data-state", "unchecked");
    // Tables render without data rows (just the add row)
    expect(screen.queryByText("dev-team")).not.toBeInTheDocument();
    expect(screen.queryByText("alice@example.com")).not.toBeInTheDocument();
  });

  it("displays formatted expiry date for grants with expiration", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    // Bob's grant has an expiry - check it renders as a formatted date
    const dateCell = screen.getByText(/12\/31\/2026|31\/12\/2026|2026/);
    expect(dateCell).toBeInTheDocument();
  });

  it("renders role badges for bindings and grants", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    // Role badges: editor (binding), owner (binding + grant), viewer (grant)
    const editorBadges = screen.getAllByText("editor");
    expect(editorBadges.length).toBeGreaterThanOrEqual(1);
    const ownerBadges = screen.getAllByText("owner");
    expect(ownerBadges.length).toBeGreaterThanOrEqual(2);
  });

  it("disables add binding button when group name is empty", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    const addButtons = screen.getAllByRole("button");
    const bindingAddButton = addButtons.find(
      (btn) =>
        btn.closest("tr")?.querySelector("input[placeholder='Group name']") !==
        null
    );
    expect(bindingAddButton).toBeDisabled();
  });

  it("disables add grant button when user email is empty", () => {
    const onPatch = vi.fn();
    render(<AccessTab workspace={workspace} onPatch={onPatch} />);
    const addButtons = screen.getAllByRole("button");
    const grantAddButton = addButtons.find(
      (btn) =>
        btn.closest("tr")?.querySelector("input[placeholder='User email']") !==
        null
    );
    expect(grantAddButton).toBeDisabled();
  });
});
