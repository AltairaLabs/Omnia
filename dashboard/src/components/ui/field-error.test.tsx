import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { FieldError } from "./field-error";

describe("FieldError", () => {
  it("renders nothing when there is no message", () => {
    const { container } = render(<FieldError message={null} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders the message with an id for aria-describedby", () => {
    render(<FieldError id="name-error" message="Invalid format." />);
    const el = screen.getByText("Invalid format.");
    expect(el).toHaveAttribute("id", "name-error");
    expect(el).toHaveAttribute("role", "alert");
  });
});
