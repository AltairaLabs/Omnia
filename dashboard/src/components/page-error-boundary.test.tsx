/**
 * Tests for PageErrorBoundary component.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { PageErrorBoundary } from "./page-error-boundary";

// Suppress console.error for expected React error boundary logs
const originalConsoleError = console.error;
beforeEach(() => {
  console.error = vi.fn();
});
afterEach(() => {
  console.error = originalConsoleError;
});

describe("PageErrorBoundary", () => {
  it("renders children when no error occurs", () => {
    render(
      <PageErrorBoundary>
        <div>Page content</div>
      </PageErrorBoundary>
    );

    expect(screen.getByText("Page content")).toBeInTheDocument();
  });

  it("renders page error fallback when a child throws", () => {
    function ThrowingChild() {
      throw new Error("Page error");
      return null;
    }

    render(
      <PageErrorBoundary>
        <ThrowingChild />
      </PageErrorBoundary>
    );

    expect(screen.getByText("Page error")).toBeInTheDocument();
    expect(
      screen.getByText(
        "This page encountered an unexpected error. You can try going back or returning to the dashboard."
      )
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to dashboard" })).toHaveAttribute(
      "href",
      "/"
    );
  });

  it("does not show default ErrorBoundary fallback", () => {
    function ThrowingChild() {
      throw new Error("Page error");
      return null;
    }

    render(
      <PageErrorBoundary>
        <ThrowingChild />
      </PageErrorBoundary>
    );

    // Should show page-level fallback, not the default "Something went wrong"
    expect(screen.queryByText("Something went wrong")).not.toBeInTheDocument();
    expect(screen.queryByText("Try again")).not.toBeInTheDocument();
  });
});
