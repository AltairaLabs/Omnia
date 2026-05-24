import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { SystemPackBadge } from "./system-pack-badge";

describe("SystemPackBadge", () => {
  it("renders when pack-class label is system", () => {
    render(
      <SystemPackBadge
        labels={{ "omnia.altairalabs.ai/pack-class": "system" }}
      />,
    );
    expect(screen.getByText(/System/)).toBeInTheDocument();
  });

  it("includes the pack-role annotation as a tooltip when present", () => {
    render(
      <SystemPackBadge
        labels={{ "omnia.altairalabs.ai/pack-class": "system" }}
        annotations={{
          "omnia.altairalabs.ai/pack-role": "consolidation-safe-default",
        }}
      />,
    );
    const badge = screen.getByText(/System/);
    expect(badge).toHaveAttribute("title", "consolidation-safe-default");
  });

  it("returns null when pack-class label is absent", () => {
    const { container } = render(<SystemPackBadge labels={{}} />);
    expect(container.firstChild).toBeNull();
  });

  it("returns null when labels is undefined", () => {
    const { container } = render(<SystemPackBadge />);
    expect(container.firstChild).toBeNull();
  });

  it("returns null when pack-class is a non-system value", () => {
    const { container } = render(
      <SystemPackBadge
        labels={{ "omnia.altairalabs.ai/pack-class": "user" }}
      />,
    );
    expect(container.firstChild).toBeNull();
  });
});
