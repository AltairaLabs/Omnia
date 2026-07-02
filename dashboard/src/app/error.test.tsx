/**
 * Tests for the app-level error boundary. It must render a branded, token-styled
 * page and wire the reset() callback to a retry action.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import ErrorBoundary from "./error";
import { OMNIA_BRAND } from "@/lib/branding/types";

describe("Error boundary", () => {
  beforeEach(() => {
    vi.spyOn(console, "error").mockImplementation(() => {});
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders a branded error page", () => {
    render(<ErrorBoundary error={new Error("boom")} reset={() => {}} />);
    expect(screen.getByText(OMNIA_BRAND.productName)).toBeInTheDocument();
    expect(screen.getByText(/something went wrong/i)).toBeInTheDocument();
  });

  it("invokes reset when the retry action is clicked", () => {
    const reset = vi.fn();
    render(<ErrorBoundary error={new Error("boom")} reset={reset} />);
    fireEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it("logs the error", () => {
    const err = new Error("boom");
    render(<ErrorBoundary error={err} reset={() => {}} />);
    expect(console.error).toHaveBeenCalledWith(err);
  });
});
