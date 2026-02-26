/**
 * Tests for ErrorBoundary and withErrorBoundary.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ErrorBoundary, withErrorBoundary } from "./error-boundary";

// Suppress console.error for expected React error boundary logs
const originalConsoleError = console.error;
beforeEach(() => {
  console.error = vi.fn();
});
afterEach(() => {
  console.error = originalConsoleError;
});

function ThrowingComponent({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) {
    throw new Error("Test error");
  }
  return <div>Child content</div>;
}

describe("ErrorBoundary", () => {
  it("renders children when no error occurs", () => {
    render(
      <ErrorBoundary>
        <div>Hello world</div>
      </ErrorBoundary>
    );

    expect(screen.getByText("Hello world")).toBeInTheDocument();
  });

  it("renders default fallback when a child throws", () => {
    render(
      <ErrorBoundary>
        <ThrowingComponent shouldThrow={true} />
      </ErrorBoundary>
    );

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
    expect(
      screen.getByText("An unexpected error occurred. Please try again.")
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Try again" })).toBeInTheDocument();
  });

  it("renders custom fallback when provided", () => {
    render(
      <ErrorBoundary fallback={<div>Custom fallback</div>}>
        <ThrowingComponent shouldThrow={true} />
      </ErrorBoundary>
    );

    expect(screen.getByText("Custom fallback")).toBeInTheDocument();
    expect(screen.queryByText("Something went wrong")).not.toBeInTheDocument();
  });

  it("resets error state when Try again is clicked", () => {
    let shouldThrow = true;

    function ConditionalThrow() {
      if (shouldThrow) {
        throw new Error("Test error");
      }
      return <div>Recovered content</div>;
    }

    render(
      <ErrorBoundary>
        <ConditionalThrow />
      </ErrorBoundary>
    );

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();

    // Fix the error before retrying
    shouldThrow = false;
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(screen.getByText("Recovered content")).toBeInTheDocument();
    expect(screen.queryByText("Something went wrong")).not.toBeInTheDocument();
  });

  it("calls onError callback when an error is caught", () => {
    const onError = vi.fn();

    render(
      <ErrorBoundary onError={onError}>
        <ThrowingComponent shouldThrow={true} />
      </ErrorBoundary>
    );

    expect(onError).toHaveBeenCalledTimes(1);
    expect(onError).toHaveBeenCalledWith(
      expect.objectContaining({ message: "Test error" }),
      expect.objectContaining({ componentStack: expect.any(String) })
    );
  });
});

describe("withErrorBoundary", () => {
  it("wraps a component with an error boundary", () => {
    function MyComponent() {
      return <div>Wrapped component</div>;
    }

    const WrappedComponent = withErrorBoundary(MyComponent);
    render(<WrappedComponent />);

    expect(screen.getByText("Wrapped component")).toBeInTheDocument();
  });

  it("catches errors from the wrapped component", () => {
    function FailingComponent() {
      throw new Error("Wrapped error");
      return null;
    }

    const WrappedComponent = withErrorBoundary(FailingComponent);
    render(<WrappedComponent />);

    expect(screen.getByText("Something went wrong")).toBeInTheDocument();
  });

  it("sets displayName on the wrapped component", () => {
    function MyNamedComponent() {
      return <div>Named</div>;
    }

    const Wrapped = withErrorBoundary(MyNamedComponent);
    expect(Wrapped.displayName).toBe("withErrorBoundary(MyNamedComponent)");
  });

  it("passes errorBoundaryProps to the ErrorBoundary", () => {
    const onError = vi.fn();

    function Failing() {
      throw new Error("props test");
      return null;
    }

    const Wrapped = withErrorBoundary(Failing, {
      fallback: <div>HOC fallback</div>,
      onError,
    });

    render(<Wrapped />);

    expect(screen.getByText("HOC fallback")).toBeInTheDocument();
    expect(onError).toHaveBeenCalledTimes(1);
  });
});
