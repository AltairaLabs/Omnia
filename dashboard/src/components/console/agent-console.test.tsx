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
  useConsoleConfig: () => ({
    config: {
      allowedMimeTypes: [
        "image/png", "image/jpeg", "image/gif", "image/webp",
        "audio/mpeg", "audio/wav", "audio/ogg",
        "application/pdf", "text/plain", "text/markdown",
        "text/javascript", "application/javascript",
        "text/x-python", "text/csv", "application/json",
      ],
      allowedExtensions: [
        ".png", ".jpg", ".jpeg", ".gif", ".webp",
        ".mp3", ".wav", ".ogg",
        ".pdf", ".txt", ".md",
        ".js", ".ts", ".jsx", ".tsx", ".py",
        ".csv", ".json",
      ],
      maxFileSize: 10 * 1024 * 1024, // 10MB
      maxFiles: 5,
      acceptString: "image/png,image/jpeg,image/gif,image/webp,audio/mpeg,audio/wav,audio/ogg,application/pdf,text/plain,text/markdown,text/javascript,application/javascript,text/x-python,text/csv,application/json,.png,.jpg,.jpeg,.gif,.webp,.mp3,.wav,.ogg,.pdf,.txt,.md,.js,.ts,.jsx,.tsx,.py,.csv,.json",
    },
    isLoading: false,
    error: null,
    rawConfig: undefined,
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
    it("should render the console with textarea", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const textarea = screen.getByRole("textbox");
      expect(textarea).toBeInTheDocument();
      expect(textarea).toHaveAttribute(
        "placeholder",
        expect.stringContaining("Enter to send")
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

  describe("attachment button", () => {
    it("should render attachment button", () => {
      render(<AgentConsole agentName="test-agent" namespace="default" />);

      const attachButton = screen.getByRole("button", { name: "Attach files" });
      expect(attachButton).toBeInTheDocument();
    });

    it("should have hidden file input", () => {
      const { container } = render(<AgentConsole agentName="test-agent" namespace="default" />);

      const fileInput = container.querySelector('input[type="file"]');
      expect(fileInput).toBeInTheDocument();
      expect(fileInput).toHaveClass("hidden");
    });

    it("should accept correct file types", () => {
      const { container } = render(<AgentConsole agentName="test-agent" namespace="default" />);

      const fileInput = container.querySelector('input[type="file"]');
      expect(fileInput).toHaveAttribute("accept");
      expect(fileInput?.getAttribute("accept")).toContain("image/png");
      expect(fileInput?.getAttribute("accept")).toContain("audio/mpeg");
      // Check for new file types
      expect(fileInput?.getAttribute("accept")).toContain("application/pdf");
      expect(fileInput?.getAttribute("accept")).toContain(".json");
    });

    it("should trigger file input click when attachment button is clicked", () => {
      const { container } = render(<AgentConsole agentName="test-agent" namespace="default" />);

      const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;
      const clickSpy = vi.spyOn(fileInput, "click");

      const attachButton = screen.getByRole("button", { name: "Attach files" });
      fireEvent.click(attachButton);

      expect(clickSpy).toHaveBeenCalled();
    });

    it("should process files when file input changes", async () => {
      const { container } = render(<AgentConsole agentName="test-agent" namespace="default" />);

      const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;

      // Create a mock file
      const file = new File(["test content"], "test.txt", { type: "text/plain" });

      // Trigger file selection
      Object.defineProperty(fileInput, "files", {
        value: [file],
        writable: false,
      });

      fireEvent.change(fileInput);

      // Wait for async file processing
      await vi.waitFor(() => {
        // File input value should be reset after processing
        expect(fileInput.value).toBe("");
      });
    });

    it("should handle empty file input change", () => {
      const { container } = render(<AgentConsole agentName="test-agent" namespace="default" />);

      const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;

      // Trigger change with no files
      Object.defineProperty(fileInput, "files", {
        value: [],
        writable: false,
      });

      // Should not throw - verify component is still rendered
      fireEvent.change(fileInput);
      expect(screen.getByRole("textbox")).toBeInTheDocument();
    });
  });

  describe("file validation", () => {
    it("should accept files with valid extensions", async () => {
      const { container } = render(<AgentConsole agentName="test-agent" namespace="default" />);

      const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;

      // Create a JSON file (text/plain MIME but .json extension)
      const jsonFile = new File(['{"key": "value"}'], "data.json", { type: "application/json" });

      Object.defineProperty(fileInput, "files", {
        value: [jsonFile],
        writable: false,
      });

      fireEvent.change(fileInput);

      // Wait for async processing
      await vi.waitFor(() => {
        expect(fileInput.value).toBe("");
      });
    });

    it("should accept Python files by extension", async () => {
      const { container } = render(<AgentConsole agentName="test-agent" namespace="default" />);

      const fileInput = container.querySelector('input[type="file"]') as HTMLInputElement;

      // Python files may have generic MIME type
      const pyFile = new File(['print("hello")'], "script.py", { type: "text/x-python" });

      Object.defineProperty(fileInput, "files", {
        value: [pyFile],
        writable: false,
      });

      fireEvent.change(fileInput);

      await vi.waitFor(() => {
        expect(fileInput.value).toBe("");
      });
    });
  });
});
