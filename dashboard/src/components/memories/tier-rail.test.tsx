import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { TierRail } from "./tier-rail";

const counts = { institutional: 3, agent: 0, user: 5, user_for_agent: 1 };

describe("TierRail", () => {
  it("renders all four tiers with counts, even zero", () => {
    render(<TierRail counts={counts} hidden={new Set()} onToggle={() => {}} />);
    expect(screen.getByText("Institutional")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByText("User-for-agent")).toBeInTheDocument();
    expect(screen.getByTestId("tier-count-agent")).toHaveTextContent("0");
  });

  it("calls onToggle with the tier when a chip is clicked", async () => {
    const onToggle = vi.fn();
    render(<TierRail counts={counts} hidden={new Set()} onToggle={onToggle} />);
    await userEvent.click(screen.getByTestId("tier-chip-user"));
    expect(onToggle).toHaveBeenCalledWith("user");
  });

  it("marks hidden tiers with aria-pressed=false", () => {
    render(<TierRail counts={counts} hidden={new Set(["agent"])} onToggle={() => {}} />);
    expect(screen.getByTestId("tier-chip-agent")).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByTestId("tier-chip-user")).toHaveAttribute("aria-pressed", "true");
  });
});
