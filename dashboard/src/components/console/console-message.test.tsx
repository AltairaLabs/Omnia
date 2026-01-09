import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ConsoleMessage } from "./console-message";
import type { ConsoleMessage as ConsoleMessageType } from "@/types/websocket";

describe("ConsoleMessage", () => {
  const baseMessage: ConsoleMessageType = {
    id: "test-1",
    role: "user",
    content: "Hello, agent!",
    timestamp: new Date("2024-01-15T10:00:00Z"),
  };

  describe("text messages", () => {
    it("should render user message with content", () => {
      render(<ConsoleMessage message={baseMessage} />);
      expect(screen.getByText("Hello, agent!")).toBeInTheDocument();
    });

    it("should render assistant message", () => {
      const assistantMessage: ConsoleMessageType = {
        ...baseMessage,
        role: "assistant",
        content: "How can I help you?",
      };
      render(<ConsoleMessage message={assistantMessage} />);
      expect(screen.getByText("How can I help you?")).toBeInTheDocument();
    });

    it("should render system message as divider", () => {
      const systemMessage: ConsoleMessageType = {
        ...baseMessage,
        role: "system",
        content: "Connected to agent",
      };
      render(<ConsoleMessage message={systemMessage} />);
      expect(screen.getByText("Connected to agent")).toBeInTheDocument();
    });

    it("should show streaming indicator when streaming", () => {
      const streamingMessage: ConsoleMessageType = {
        ...baseMessage,
        role: "assistant",
        content: "I am thinking",
        isStreaming: true,
      };
      render(<ConsoleMessage message={streamingMessage} />);
      expect(screen.getByText("I am thinking")).toBeInTheDocument();
    });

    it("should show 'Thinking...' for empty streaming message", () => {
      const emptyStreamingMessage: ConsoleMessageType = {
        ...baseMessage,
        role: "assistant",
        content: "",
        isStreaming: true,
      };
      render(<ConsoleMessage message={emptyStreamingMessage} />);
      expect(screen.getByText("Thinking...")).toBeInTheDocument();
    });
  });

  describe("tool calls", () => {
    it("should render tool calls", () => {
      const messageWithTools: ConsoleMessageType = {
        ...baseMessage,
        role: "assistant",
        content: "Let me check that",
        toolCalls: [
          {
            id: "tool-1",
            name: "search_database",
            arguments: { query: "test" },
            status: "success",
            result: { found: true },
          },
        ],
      };

      render(<ConsoleMessage message={messageWithTools} />);

      expect(screen.getByText("search_database")).toBeInTheDocument();
    });
  });

  describe("timestamp", () => {
    it("should display formatted time", () => {
      render(<ConsoleMessage message={baseMessage} />);

      // Time should be formatted (exact format depends on locale)
      // Just check that something time-like is present
      const timeElements = screen.getAllByText(/\d{1,2}:\d{2}/);
      expect(timeElements.length).toBeGreaterThan(0);
    });
  });
});
