import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { CliWorkspacePicker } from "./cli-workspace-picker";

describe("CliWorkspacePicker", () => {
  it("renders a form posting to /api/cli/grant with the flow + workspace radios", () => {
    const { container } = render(
      <CliWorkspacePicker
        flow="f-123"
        workspaces={[
          { name: "team-acme", role: "owner" },
          { name: "team-beta", role: "editor" },
        ]}
      />
    );
    const form = container.querySelector("form")!;
    expect(form.getAttribute("action")).toBe("/api/cli/grant");
    expect(form.getAttribute("method")).toBe("post");
    expect(container.querySelector('input[name="flow"]')!.getAttribute("value")).toBe("f-123");
    const radios = container.querySelectorAll('input[name="workspace"]');
    expect(Array.from(radios).map((r) => r.getAttribute("value"))).toEqual(["team-acme", "team-beta"]);
    expect(screen.getByRole("button", { name: /authorize/i })).toBeTruthy();
  });

  it("shows an empty-state and no form when there are no workspaces", () => {
    const { container } = render(<CliWorkspacePicker flow="f-1" workspaces={[]} />);
    expect(container.querySelector("form")).toBeNull();
    expect(screen.getByText(/no workspaces/i)).toBeTruthy();
  });
});
