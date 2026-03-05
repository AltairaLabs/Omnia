import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ToolCallBadge } from "./tool-call-badge";

describe("ToolCallBadge", () => {
  it("renders success variant", () => {
    render(<ToolCallBadge status="success" />);
    expect(screen.getByText("Success")).toBeInTheDocument();
    expect(screen.getByTestId("tool-call-badge")).toBeInTheDocument();
  });

  it("renders error variant", () => {
    render(<ToolCallBadge status="error" />);
    expect(screen.getByText("Error")).toBeInTheDocument();
  });

  it("renders pending variant", () => {
    render(<ToolCallBadge status="pending" />);
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });
});
