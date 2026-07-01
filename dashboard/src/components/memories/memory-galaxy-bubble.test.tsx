import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { MemoryGalaxyBubble } from "./memory-galaxy-bubble";
import type { GalaxyPoint } from "@/lib/memory-galaxy/types";

function pt(id: string, over: Partial<GalaxyPoint> = {}): GalaxyPoint {
  return {
    id,
    x: 0,
    y: 0,
    tier: "user",
    confidence: 0.5,
    title: `title-${id}`,
    ...over,
  } as GalaxyPoint;
}

const base = {
  left: 100,
  top: 100,
  placement: "below" as const,
  tailOffset: 0,
  onPrev: () => {},
  onNext: () => {},
  onClose: () => {},
  onDelete: () => {},
};

afterEach(cleanup);

describe("MemoryGalaxyBubble", () => {
  it("renders the memory at the current index", () => {
    render(<MemoryGalaxyBubble {...base} stack={[pt("a"), pt("b")]} index={1} />);
    expect(screen.getByText("title-b")).toBeInTheDocument();
  });

  it("hides the carousel nav for a single memory", () => {
    render(<MemoryGalaxyBubble {...base} stack={[pt("solo")]} index={0} />);
    expect(screen.queryByTestId("bubble-position")).not.toBeInTheDocument();
    expect(screen.queryByTestId("bubble-next")).not.toBeInTheDocument();
  });

  it("shows position and both arrows when several memories are stacked", () => {
    render(<MemoryGalaxyBubble {...base} stack={[pt("a"), pt("b"), pt("c")]} index={1} />);
    expect(screen.getByTestId("bubble-position")).toHaveTextContent("2 of 3");
    expect(screen.getByTestId("bubble-prev")).toBeEnabled();
    expect(screen.getByTestId("bubble-next")).toBeEnabled();
  });

  it("disables prev at the start and next at the end", () => {
    const { rerender } = render(
      <MemoryGalaxyBubble {...base} stack={[pt("a"), pt("b"), pt("c")]} index={0} />,
    );
    expect(screen.getByTestId("bubble-prev")).toBeDisabled();
    expect(screen.getByTestId("bubble-next")).toBeEnabled();
    rerender(<MemoryGalaxyBubble {...base} stack={[pt("a"), pt("b"), pt("c")]} index={2} />);
    expect(screen.getByTestId("bubble-next")).toBeDisabled();
    expect(screen.getByTestId("bubble-prev")).toBeEnabled();
  });

  it("calls onNext / onPrev when the arrows are clicked", () => {
    const onNext = vi.fn();
    const onPrev = vi.fn();
    render(
      <MemoryGalaxyBubble
        {...base}
        onNext={onNext}
        onPrev={onPrev}
        stack={[pt("a"), pt("b"), pt("c")]}
        index={1}
      />,
    );
    fireEvent.click(screen.getByTestId("bubble-next"));
    fireEvent.click(screen.getByTestId("bubble-prev"));
    expect(onNext).toHaveBeenCalledTimes(1);
    expect(onPrev).toHaveBeenCalledTimes(1);
  });

  it("deletes the memory currently shown, not the first in the stack", () => {
    const onDelete = vi.fn();
    render(
      <MemoryGalaxyBubble
        {...base}
        onDelete={onDelete}
        stack={[pt("a"), pt("b"), pt("c")]}
        index={2}
      />,
    );
    fireEvent.click(screen.getByTestId("bubble-delete"));
    expect(onDelete).toHaveBeenCalledWith("c");
  });

  it("calls onClose from the close button", () => {
    const onClose = vi.fn();
    render(<MemoryGalaxyBubble {...base} onClose={onClose} stack={[pt("a")]} index={0} />);
    fireEvent.click(screen.getByTestId("bubble-close"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("renders populated metadata rows (preview, type, user, dates)", () => {
    const full = pt("full", {
      preview: "some preview text",
      type: "preference",
      userRef: "user-42",
      observedAt: "2026-06-08T00:00:00Z",
      expiresAt: "2026-07-12T00:00:00Z",
    });
    render(<MemoryGalaxyBubble {...base} stack={[full]} index={0} />);
    expect(screen.getByText("some preview text")).toBeInTheDocument();
    expect(screen.getByText("preference")).toBeInTheDocument();
    expect(screen.getByText("user-42")).toBeInTheDocument();
  });

  it("falls back to 'Memory' when a point has no title", () => {
    render(<MemoryGalaxyBubble {...base} stack={[pt("x", { title: undefined })]} index={0} />);
    expect(screen.getByText("Memory")).toBeInTheDocument();
  });

  it("supports the 'above' placement", () => {
    render(<MemoryGalaxyBubble {...base} placement="above" stack={[pt("a")]} index={0} />);
    expect(screen.getByText("title-a")).toBeInTheDocument();
  });

  it("renders nothing when the index is out of range", () => {
    const { container } = render(
      <MemoryGalaxyBubble {...base} stack={[pt("a")]} index={5} />,
    );
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText(/title-/)).not.toBeInTheDocument();
  });
});
