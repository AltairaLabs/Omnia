import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { AgentConsole } from "./agent-console";

// Mock the hooks
vi.mock("@/hooks", () => ({
  useAgentConsole: () => ({
    sessionId: "test-session",
    status: "connected",
    messages: [],
    error: null,
    sendMessage: vi.fn(),
    connect: vi.fn(),
    disconnect: vi.fn(),
    clearMessages: vi.fn(),
  }),
}));

// Mock crypto.randomUUID
vi.stubGlobal("crypto", {
  randomUUID: () => "test-uuid-1234",
});

describe("AgentConsole", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("paste handling", () => {
    it("should render the console with paste-enabled textarea", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const textarea = screen.getByRole("textbox");
      expect(textarea).toBeInTheDocument();
      expect(textarea).toHaveAttribute(
        "placeholder",
        expect.stringContaining("Paste images")
      );
    });

    it("should not prevent default for text-only paste", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const textarea = screen.getByRole("textbox");

      // Create mock text item (no image)
      const mockItem = {
        type: "text/plain",
        getAsFile: () => null,
      };

      const pasteEvent = {
        clipboardData: {
          items: [mockItem],
        },
        preventDefault: vi.fn(),
      };

      fireEvent.paste(textarea, pasteEvent);

      // preventDefault should NOT be called for text paste
      expect(pasteEvent.preventDefault).not.toHaveBeenCalled();
    });

    it("should ignore unsupported image types", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const textarea = screen.getByRole("textbox");

      // Create mock item with unsupported type
      const mockItem = {
        type: "image/bmp", // Not in ALLOWED_TYPES
        getAsFile: () =>
          new File(["data"], "test.bmp", { type: "image/bmp" }),
      };

      const pasteEvent = {
        clipboardData: {
          items: [mockItem],
        },
        preventDefault: vi.fn(),
      };

      fireEvent.paste(textarea, pasteEvent);

      // preventDefault should NOT be called for unsupported type
      expect(pasteEvent.preventDefault).not.toHaveBeenCalled();
    });

    it("should ignore files that are too large", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const textarea = screen.getByRole("textbox");

      // Create a mock file larger than 10MB
      const largeFile = new File(["x".repeat(11 * 1024 * 1024)], "large.png", {
        type: "image/png",
      });
      // Override size property since File doesn't actually compute it from content
      Object.defineProperty(largeFile, "size", { value: 11 * 1024 * 1024 });

      const mockItem = {
        type: "image/png",
        getAsFile: () => largeFile,
      };

      const pasteEvent = {
        clipboardData: {
          items: [mockItem],
        },
        preventDefault: vi.fn(),
      };

      fireEvent.paste(textarea, pasteEvent);

      // preventDefault should NOT be called for oversized file
      expect(pasteEvent.preventDefault).not.toHaveBeenCalled();
    });
  });

  describe("connection status", () => {
    it("should show connected status badge", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      expect(screen.getByText("Connected")).toBeInTheDocument();
    });

    it("should show session ID", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      expect(screen.getByText(/Session:/)).toBeInTheDocument();
    });
  });

  describe("input handling", () => {
    it("should enable send button when text is entered", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const textarea = screen.getByRole("textbox");
      // Get all buttons and find the send button (the one next to the textarea)
      const buttons = screen.getAllByRole("button");
      // The send button is the last button (after the textarea)
      const sendButton = buttons[buttons.length - 1];

      // Initially disabled (no input)
      expect(sendButton).toBeDisabled();

      // Type some text
      fireEvent.change(textarea, { target: { value: "Hello" } });

      // Button should be enabled
      expect(sendButton).toBeEnabled();
    });

    it("should handle Enter key to send message", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const textarea = screen.getByRole("textbox");

      // Type some text
      fireEvent.change(textarea, { target: { value: "Hello" } });

      // Press Enter (should send and clear)
      fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

      // Input should be cleared after send
      expect(textarea).toHaveValue("");
    });

    it("should allow Shift+Enter for new line", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const textarea = screen.getByRole("textbox");

      // Type some text
      fireEvent.change(textarea, { target: { value: "Hello" } });

      // Press Shift+Enter (should NOT send)
      fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true });

      // Input should still have text
      expect(textarea).toHaveValue("Hello");
    });
  });

  describe("drag and drop", () => {
    it("should show drop zone overlay when dragging files", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const console = screen.getByText(/Start a conversation/).closest("div")?.parentElement?.parentElement;

      // Simulate drag enter with files
      fireEvent.dragEnter(console!, {
        dataTransfer: { types: ["Files"] },
      });

      // Drop zone overlay should be visible
      expect(screen.getByText("Drop files here")).toBeInTheDocument();
    });

    it("should hide drop zone overlay when dragging leaves", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const console = screen.getByText(/Start a conversation/).closest("div")?.parentElement?.parentElement;

      // Simulate drag enter
      fireEvent.dragEnter(console!, {
        dataTransfer: { types: ["Files"] },
      });

      // Simulate drag leave
      fireEvent.dragLeave(console!, {});

      // Drop zone overlay should be hidden
      expect(screen.queryByText("Drop files here")).not.toBeInTheDocument();
    });

    it("should handle drag over without error", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const consoleElement = screen.getByText(/Start a conversation/).closest("div")?.parentElement?.parentElement;

      // Simulate drag over (should not throw and element should still exist)
      fireEvent.dragOver(consoleElement!, {});

      // Console should still be rendered
      expect(screen.getByText(/Start a conversation/)).toBeInTheDocument();
    });

    it("should handle drop event", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const console = screen.getByText(/Start a conversation/).closest("div")?.parentElement?.parentElement;

      // First enter drag mode
      fireEvent.dragEnter(console!, {
        dataTransfer: { types: ["Files"] },
      });

      // Then drop (with no files for simplicity)
      fireEvent.drop(console!, {
        dataTransfer: { files: [] },
      });

      // Drop zone should be hidden after drop
      expect(screen.queryByText("Drop files here")).not.toBeInTheDocument();
    });
  });

  describe("empty state", () => {
    it("should show empty state message", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      expect(screen.getByText(/Start a conversation with/)).toBeInTheDocument();
      expect(screen.getByText("test-agent")).toBeInTheDocument();
    });
  });
});
