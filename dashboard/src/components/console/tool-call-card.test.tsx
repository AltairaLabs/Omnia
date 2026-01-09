import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { ToolCallCard } from "./tool-call-card";
import type { ToolCallWithResult } from "@/types/websocket";

describe("ToolCallCard", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  const baseToolCall: ToolCallWithResult = {
    id: "tool-1",
    name: "search_database",
    arguments: { query: "test" },
    status: "pending",
  };

  describe("rendering", () => {
    it("should render tool name", () => {
      render(<ToolCallCard toolCall={baseToolCall} />);
      expect(screen.getByText("search_database")).toBeInTheDocument();
    });

    it("should render pending status with spinner", () => {
      render(<ToolCallCard toolCall={baseToolCall} />);
      // The Loader2 icon has animate-spin class
      const card = screen.getByText("search_database").closest("div");
      expect(card).toHaveClass("bg-muted/30");
    });

    it("should render success status with check icon", () => {
      const successToolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "success",
        result: { found: true },
      };
      render(<ToolCallCard toolCall={successToolCall} />);
      const card = screen.getByText("search_database").closest("div");
      expect(card).toHaveClass("bg-green-500/10");
    });

    it("should render error status with X icon", () => {
      const errorToolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "error",
        error: "Database connection failed",
      };
      render(<ToolCallCard toolCall={errorToolCall} />);
      const card = screen.getByText("search_database").closest("div");
      expect(card).toHaveClass("bg-red-500/10");
    });
  });

  describe("expand/collapse", () => {
    it("should start collapsed for pending status", () => {
      render(<ToolCallCard toolCall={baseToolCall} />);
      // Arguments should not be visible when collapsed
      expect(screen.queryByText("Arguments:")).not.toBeInTheDocument();
    });

    it("should start expanded for success status", () => {
      const successToolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "success",
        result: { found: true },
      };
      render(<ToolCallCard toolCall={successToolCall} />);
      // Arguments should be visible when expanded
      expect(screen.getByText("Arguments:")).toBeInTheDocument();
    });

    it("should toggle expand/collapse on click", () => {
      const successToolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "success",
        result: { found: true },
      };
      render(<ToolCallCard toolCall={successToolCall} />);

      // Initially expanded
      expect(screen.getByText("Arguments:")).toBeInTheDocument();

      // Click to collapse
      fireEvent.click(screen.getByRole("button"));
      expect(screen.queryByText("Arguments:")).not.toBeInTheDocument();

      // Click to expand again
      fireEvent.click(screen.getByRole("button"));
      expect(screen.getByText("Arguments:")).toBeInTheDocument();
    });

    it("should auto-expand when status changes from pending to success", async () => {
      const { rerender } = render(<ToolCallCard toolCall={baseToolCall} />);

      // Initially collapsed
      expect(screen.queryByText("Arguments:")).not.toBeInTheDocument();

      // Update to success status
      const successToolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "success",
        result: { found: true },
      };

      rerender(<ToolCallCard toolCall={successToolCall} />);

      // Run requestAnimationFrame callback
      await act(async () => {
        vi.runAllTimers();
      });

      // Should now be expanded
      expect(screen.getByText("Arguments:")).toBeInTheDocument();
    });

    it("should auto-expand when status changes from pending to error", async () => {
      const { rerender } = render(<ToolCallCard toolCall={baseToolCall} />);

      // Initially collapsed
      expect(screen.queryByText("Arguments:")).not.toBeInTheDocument();

      // Update to error status
      const errorToolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "error",
        error: "Failed",
      };

      rerender(<ToolCallCard toolCall={errorToolCall} />);

      // Run requestAnimationFrame callback
      await act(async () => {
        vi.runAllTimers();
      });

      // Should now be expanded
      expect(screen.getByText("Arguments:")).toBeInTheDocument();
    });
  });

  describe("content display", () => {
    it("should display arguments when expanded", () => {
      const toolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "success",
        result: "done",
      };
      render(<ToolCallCard toolCall={toolCall} />);

      expect(screen.getByText("Arguments:")).toBeInTheDocument();
      expect(screen.getByText(/"query": "test"/)).toBeInTheDocument();
    });

    it("should display result for success status", () => {
      const toolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "success",
        result: { found: true, count: 5 },
      };
      render(<ToolCallCard toolCall={toolCall} />);

      expect(screen.getByText("Result:")).toBeInTheDocument();
      expect(screen.getByText(/"found": true/)).toBeInTheDocument();
    });

    it("should display string result directly", () => {
      const toolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "success",
        result: "Operation completed successfully",
      };
      render(<ToolCallCard toolCall={toolCall} />);

      expect(screen.getByText("Result:")).toBeInTheDocument();
      expect(screen.getByText("Operation completed successfully")).toBeInTheDocument();
    });

    it("should display error message for error status", () => {
      const toolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "error",
        error: "Connection timeout",
      };
      render(<ToolCallCard toolCall={toolCall} />);

      expect(screen.getByText("Error:")).toBeInTheDocument();
      expect(screen.getByText("Connection timeout")).toBeInTheDocument();
    });

    it("should not display arguments section when arguments are empty", () => {
      const toolCall: ToolCallWithResult = {
        id: "tool-1",
        name: "ping",
        arguments: {},
        status: "success",
        result: "pong",
      };
      render(<ToolCallCard toolCall={toolCall} />);

      expect(screen.queryByText("Arguments:")).not.toBeInTheDocument();
      expect(screen.getByText("Result:")).toBeInTheDocument();
    });

    it("should not display result section when result is undefined", () => {
      const toolCall: ToolCallWithResult = {
        ...baseToolCall,
        status: "success",
        result: undefined,
      };
      render(<ToolCallCard toolCall={toolCall} />);

      expect(screen.getByText("Arguments:")).toBeInTheDocument();
      expect(screen.queryByText("Result:")).not.toBeInTheDocument();
    });
  });

  describe("custom className", () => {
    it("should apply custom className", () => {
      const { container } = render(
        <ToolCallCard toolCall={baseToolCall} className="custom-class" />
      );
      expect(container.firstChild).toHaveClass("custom-class");
    });
  });
});
