import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act, cleanup } from "@testing-library/react";
import { Toaster } from "./toaster";
import { toast, dismissToast, useToast } from "@/hooks/use-toast";
import { renderHook } from "@testing-library/react";

function clearToasts() {
  const { result } = renderHook(() => useToast());
  act(() => {
    for (const t of result.current.toasts) dismissToast(t.id);
  });
}

describe("Toaster", () => {
  beforeEach(clearToasts);
  afterEach(cleanup);

  it("renders nothing when there are no toasts", () => {
    const { container } = render(<Toaster />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders a raised toast's title and description", () => {
    render(<Toaster />);
    act(() => {
      toast({ title: "Saved", description: "Tool registry created" });
    });
    expect(screen.getByText("Saved")).toBeInTheDocument();
    expect(screen.getByText("Tool registry created")).toBeInTheDocument();
  });

  it("dismiss button removes the toast", () => {
    render(<Toaster />);
    act(() => {
      toast({ title: "Bye", duration: 0 });
    });
    expect(screen.getByText("Bye")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Dismiss notification" }));
    expect(screen.queryByText("Bye")).not.toBeInTheDocument();
  });
});
