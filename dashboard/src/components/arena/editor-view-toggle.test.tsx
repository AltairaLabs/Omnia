import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { EditorViewToggle } from "./editor-view-toggle";

describe("EditorViewToggle", () => {
  it("renders both options and reports the chosen view", () => {
    const onChange = vi.fn();
    render(<EditorViewToggle view="yaml" onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: /workload/i }));
    expect(onChange).toHaveBeenCalledWith("workload");
  });

  it("marks the active view", () => {
    render(<EditorViewToggle view="workload" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: /workload/i })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: /yaml/i })).toHaveAttribute("aria-pressed", "false");
  });
});
