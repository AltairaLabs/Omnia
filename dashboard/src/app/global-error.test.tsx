/**
 * Tests for the app-level global-error boundary. Because it replaces the root
 * layout, it renders its own <html>/<body> and its own BrandProvider so it
 * stays white-labeled even when the root layout itself throws.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import GlobalError from "./global-error";

// The self-hosted BrandProvider fetches runtime config; resolve it to empty so
// the default Omnia brand is used deterministically in tests.
vi.mock("@/lib/config", () => ({
  getRuntimeConfig: () => Promise.resolve({}),
}));

describe("GlobalError boundary", () => {
  beforeEach(() => {
    vi.spyOn(console, "error").mockImplementation(() => {});
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders a branded critical-error page", () => {
    render(<GlobalError error={new Error("fatal")} reset={() => {}} />);
    expect(screen.getByText("Omnia")).toBeInTheDocument();
    expect(screen.getByText(/something went wrong/i)).toBeInTheDocument();
  });

  it("invokes reset when the retry action is clicked", () => {
    const reset = vi.fn();
    render(<GlobalError error={new Error("fatal")} reset={reset} />);
    fireEvent.click(screen.getByRole("button", { name: /try again/i }));
    expect(reset).toHaveBeenCalledTimes(1);
  });
});
