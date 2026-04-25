import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { TierBadge } from "./tier-badge";

describe("TierBadge", () => {
  it.each([
    ["institutional", "Institutional"],
    ["agent", "Agent"],
    ["user", "User"],
  ] as const)("renders the %s tier as %s", (tier, label) => {
    render(<TierBadge tier={tier} />);
    expect(screen.getByText(label)).toBeInTheDocument();
  });

  it("renders nothing when tier is undefined", () => {
    const { container } = render(<TierBadge tier={undefined} />);
    expect(container.firstChild).toBeNull();
  });

  it("forwards className", () => {
    render(<TierBadge tier="user" className="custom-class" />);
    const badge = screen.getByText("User");
    expect(badge.className).toContain("custom-class");
  });
});
