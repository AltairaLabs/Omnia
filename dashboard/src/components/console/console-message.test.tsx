import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ConsoleMessage } from "./console-message";
import type { ConsoleMessage as ConsoleMessageType, FileAttachment } from "@/types/websocket";

// Mock ResizeObserver for Radix components
class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}
global.ResizeObserver = ResizeObserverMock;

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

  describe("image attachments", () => {
    const mockImageAttachment: FileAttachment = {
      id: "img-1",
      name: "test-image.png",
      type: "image/png",
      size: 1024,
      dataUrl: "data:image/png;base64,iVBORw0KGgo=",
    };

    const mockNonImageAttachment: FileAttachment = {
      id: "file-1",
      name: "document.pdf",
      type: "application/pdf",
      size: 2048,
      dataUrl: "data:application/pdf;base64,abc123",
    };

    it("should render image attachments", () => {
      const messageWithImages: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockImageAttachment],
      };

      render(<ConsoleMessage message={messageWithImages} />);

      const img = screen.getByRole("img");
      expect(img).toBeInTheDocument();
      expect(img).toHaveAttribute("src", mockImageAttachment.dataUrl);
    });

    it("should not render non-image attachments as images", () => {
      const messageWithNonImage: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockNonImageAttachment],
      };

      render(<ConsoleMessage message={messageWithNonImage} />);

      // Non-image attachments should not create img elements in the attachments section
      expect(screen.queryByRole("img")).not.toBeInTheDocument();
    });

    it("should render multiple image attachments", () => {
      const secondImage: FileAttachment = {
        id: "img-2",
        name: "test-image-2.jpg",
        type: "image/jpeg",
        size: 2048,
        dataUrl: "data:image/jpeg;base64,xyz789",
      };

      const messageWithMultipleImages: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockImageAttachment, secondImage],
      };

      render(<ConsoleMessage message={messageWithMultipleImages} />);

      const images = screen.getAllByRole("img");
      expect(images).toHaveLength(2);
    });

    it("should open lightbox when image is clicked", () => {
      const messageWithImages: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockImageAttachment],
      };

      render(<ConsoleMessage message={messageWithImages} />);

      const imageButton = screen.getByRole("button", { name: `View ${mockImageAttachment.name}` });
      fireEvent.click(imageButton);

      // Lightbox should be open - check for zoom controls
      expect(screen.getByRole("button", { name: "Zoom in" })).toBeInTheDocument();
    });
  });
});
