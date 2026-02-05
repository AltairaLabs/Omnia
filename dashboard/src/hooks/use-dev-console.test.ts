import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// Mock console store
const mockAddMessage = vi.fn();
const mockUpdateLastMessage = vi.fn();
const mockSetStatus = vi.fn();
const mockSetSessionId = vi.fn();
const mockClearMessages = vi.fn();

vi.mock("@/stores", () => ({
  useConsoleStore: vi.fn(() => ({
    addMessage: mockAddMessage,
    updateLastMessage: mockUpdateLastMessage,
    setStatus: mockSetStatus,
    setSessionId: mockSetSessionId,
    clearMessages: mockClearMessages,
  })),
  useSession: vi.fn(() => ({
    sessionId: null,
    status: "disconnected",
    messages: [],
    error: null,
  })),
}));

// Import after mocks
import { useDevConsole } from "./use-dev-console";

describe("useDevConsole", () => {
  let mockWsInstance: {
    send: ReturnType<typeof vi.fn>;
    close: ReturnType<typeof vi.fn>;
    readyState: number;
    onopen: ((ev: Event) => void) | null;
    onclose: ((ev: CloseEvent) => void) | null;
    onmessage: ((ev: MessageEvent) => void) | null;
    onerror: ((ev: Event) => void) | null;
  } | null = null;

  beforeEach(() => {
    vi.clearAllMocks();
    mockWsInstance = null;

    // Mock WebSocket constructor
    const MockWebSocket = vi.fn().mockImplementation((_url: string) => {
      mockWsInstance = {
        send: vi.fn(),
        close: vi.fn(),
        readyState: 1, // OPEN
        onopen: null,
        onclose: null,
        onmessage: null,
        onerror: null,
      };
      // Simulate connection after constructor
      setTimeout(() => {
        mockWsInstance?.onopen?.(new Event("open"));
      }, 0);
      return mockWsInstance;
    });
    (MockWebSocket as unknown as { OPEN: number }).OPEN = 1;
    (MockWebSocket as unknown as { CLOSED: number }).CLOSED = 3;
    vi.stubGlobal("WebSocket", MockWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("should return initial state", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    expect(result.current.status).toBe("disconnected");
    expect(result.current.messages).toEqual([]);
    expect(result.current.sessionId).toBeNull();
  });

  it("should have connect function", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    expect(typeof result.current.connect).toBe("function");
  });

  it("should have disconnect function", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    expect(typeof result.current.disconnect).toBe("function");
  });

  it("should have sendMessage function", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    expect(typeof result.current.sendMessage).toBe("function");
  });

  it("should have clearMessages function", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    expect(typeof result.current.clearMessages).toBe("function");
  });

  it("should have reload function", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    expect(typeof result.current.reload).toBe("function");
  });

  it("should have resetConversation function", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    expect(typeof result.current.resetConversation).toBe("function");
  });

  it("should have setProvider function", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    expect(typeof result.current.setProvider).toBe("function");
  });

  it("should call clearMessages on store when clearMessages is called", () => {
    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project" })
    );

    act(() => {
      result.current.clearMessages();
    });

    expect(mockClearMessages).toHaveBeenCalledWith("dev-console-test-project");
  });

  it("should use custom sessionId when provided", () => {
    const { result } = renderHook(() =>
      useDevConsole({
        sessionId: "custom-session-id",
        projectId: "test-project",
      })
    );

    act(() => {
      result.current.clearMessages();
    });

    expect(mockClearMessages).toHaveBeenCalledWith("custom-session-id");
  });

  it("should set status to connecting when connect is called", () => {
    // Create a proper mock class for WebSocket
    class MockWebSocketClass {
      static readonly OPEN = 1;
      static readonly CLOSED = 3;
      readyState = 1;
      onopen: ((ev: Event) => void) | null = null;
      onclose: ((ev: CloseEvent) => void) | null = null;
      onmessage: ((ev: MessageEvent) => void) | null = null;
      onerror: ((ev: Event) => void) | null = null;
      send = vi.fn();
      close = vi.fn();
      constructor() {
        setTimeout(() => this.onopen?.(new Event("open")), 0);
      }
    }
    vi.stubGlobal("WebSocket", MockWebSocketClass);

    const { result } = renderHook(() =>
      useDevConsole({ projectId: "test-project", service: "test-service" })
    );

    act(() => {
      result.current.connect();
    });

    expect(mockSetStatus).toHaveBeenCalledWith(
      "dev-console-test-project",
      "connecting",
      undefined
    );
  });
});
