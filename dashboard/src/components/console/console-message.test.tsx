import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { axe } from "vitest-axe";
import { ConsoleMessage } from "./console-message";
import type { ConsoleMessage as ConsoleMessageType, FileAttachment } from "@/types/websocket";

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

  describe("audio attachments", () => {
    const mockAudioAttachment: FileAttachment = {
      id: "audio-1",
      name: "recording.mp3",
      type: "audio/mpeg",
      size: 5000,
      dataUrl: "data:audio/mpeg;base64,audiodata123",
    };

    it("should render audio player for audio attachments", () => {
      const messageWithAudio: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockAudioAttachment],
      };

      render(<ConsoleMessage message={messageWithAudio} />);

      const audioElement = document.querySelector("audio");
      expect(audioElement).toBeInTheDocument();
      expect(audioElement).toHaveAttribute("controls");
      expect(audioElement).toHaveAttribute("aria-label", `Audio: ${mockAudioAttachment.name}`);
    });

    it("should display audio filename", () => {
      const messageWithAudio: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockAudioAttachment],
      };

      render(<ConsoleMessage message={messageWithAudio} />);

      expect(screen.getByText("recording.mp3")).toBeInTheDocument();
    });

    it("should render multiple audio attachments", () => {
      const secondAudio: FileAttachment = {
        id: "audio-2",
        name: "song.wav",
        type: "audio/wav",
        size: 10000,
        dataUrl: "data:audio/wav;base64,wavdata456",
      };

      const messageWithMultipleAudio: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockAudioAttachment, secondAudio],
      };

      render(<ConsoleMessage message={messageWithMultipleAudio} />);

      const audioElements = document.querySelectorAll("audio");
      expect(audioElements).toHaveLength(2);
    });
  });

  describe("video attachments", () => {
    const mockVideoAttachment: FileAttachment = {
      id: "video-1",
      name: "clip.mp4",
      type: "video/mp4",
      size: 50000,
      dataUrl: "data:video/mp4;base64,videodata789",
    };

    it("should render video player for video attachments", () => {
      const messageWithVideo: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockVideoAttachment],
      };

      render(<ConsoleMessage message={messageWithVideo} />);

      const videoElement = document.querySelector("video");
      expect(videoElement).toBeInTheDocument();
      expect(videoElement).toHaveAttribute("controls");
      expect(videoElement).toHaveAttribute("aria-label", `Video: ${mockVideoAttachment.name}`);
    });

    it("should display video filename", () => {
      const messageWithVideo: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockVideoAttachment],
      };

      render(<ConsoleMessage message={messageWithVideo} />);

      expect(screen.getByText("clip.mp4")).toBeInTheDocument();
    });
  });

  describe("file attachments (download links)", () => {
    const mockPdfAttachment: FileAttachment = {
      id: "pdf-1",
      name: "document.pdf",
      type: "application/pdf",
      size: 2048,
      dataUrl: "data:application/pdf;base64,pdfdata123",
    };

    const mockJsonAttachment: FileAttachment = {
      id: "json-1",
      name: "data.json",
      type: "application/json",
      size: 512,
      dataUrl: "data:application/json;base64,jsondata456",
    };

    const mockCodeAttachment: FileAttachment = {
      id: "code-1",
      name: "script.py",
      type: "text/x-python",
      size: 1024,
      dataUrl: "data:text/x-python;base64,codedata789",
    };

    it("should render download link for PDF attachments", () => {
      const messageWithPdf: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockPdfAttachment],
      };

      render(<ConsoleMessage message={messageWithPdf} />);

      const link = screen.getByRole("link");
      expect(link).toHaveAttribute("href", mockPdfAttachment.dataUrl);
      expect(link).toHaveAttribute("download", "document.pdf");
      expect(screen.getByText("document.pdf")).toBeInTheDocument();
    });

    it("should display file size", () => {
      const messageWithPdf: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockPdfAttachment],
      };

      render(<ConsoleMessage message={messageWithPdf} />);

      expect(screen.getByText("2.0 KB")).toBeInTheDocument();
    });

    it("should render multiple file attachments", () => {
      const messageWithFiles: ConsoleMessageType = {
        ...baseMessage,
        attachments: [mockPdfAttachment, mockJsonAttachment, mockCodeAttachment],
      };

      render(<ConsoleMessage message={messageWithFiles} />);

      const links = screen.getAllByRole("link");
      expect(links).toHaveLength(3);
      expect(screen.getByText("document.pdf")).toBeInTheDocument();
      expect(screen.getByText("data.json")).toBeInTheDocument();
      expect(screen.getByText("script.py")).toBeInTheDocument();
    });

    it("should format bytes correctly", () => {
      const smallFile: FileAttachment = {
        id: "small-1",
        name: "tiny.txt",
        type: "text/plain",
        size: 100,
        dataUrl: "data:text/plain;base64,abc",
      };

      const largeFile: FileAttachment = {
        id: "large-1",
        name: "big.bin",
        type: "application/octet-stream",
        size: 5 * 1024 * 1024,
        dataUrl: "data:application/octet-stream;base64,xyz",
      };

      const messageWithFiles: ConsoleMessageType = {
        ...baseMessage,
        attachments: [smallFile, largeFile],
      };

      render(<ConsoleMessage message={messageWithFiles} />);

      expect(screen.getByText("100 B")).toBeInTheDocument();
      expect(screen.getByText("5.0 MB")).toBeInTheDocument();
    });
  });

  describe("mixed attachments", () => {
    it("should render all attachment types in one message", () => {
      const messageWithMixed: ConsoleMessageType = {
        ...baseMessage,
        attachments: [
          { id: "img-1", name: "photo.png", type: "image/png", size: 1000, dataUrl: "data:image/png;base64,img" },
          { id: "audio-1", name: "voice.mp3", type: "audio/mpeg", size: 2000, dataUrl: "data:audio/mpeg;base64,audio" },
          { id: "video-1", name: "movie.mp4", type: "video/mp4", size: 3000, dataUrl: "data:video/mp4;base64,video" },
          { id: "file-1", name: "doc.pdf", type: "application/pdf", size: 4000, dataUrl: "data:application/pdf;base64,pdf" },
        ],
      };

      render(<ConsoleMessage message={messageWithMixed} />);

      // Image
      expect(screen.getByRole("img")).toBeInTheDocument();
      // Audio
      expect(document.querySelector("audio")).toBeInTheDocument();
      // Video
      expect(document.querySelector("video")).toBeInTheDocument();
      // File download
      expect(screen.getByRole("link")).toBeInTheDocument();
    });
  });

  describe("accessibility", () => {
    it("should have no accessibility violations for user message", async () => {
      const { container } = render(<ConsoleMessage message={baseMessage} />);

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });

    it("should have no accessibility violations for assistant message", async () => {
      const assistantMessage: ConsoleMessageType = {
        ...baseMessage,
        role: "assistant",
        content: "How can I help you?",
      };
      const { container } = render(<ConsoleMessage message={assistantMessage} />);

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });

    it("should have no accessibility violations for system message", async () => {
      const systemMessage: ConsoleMessageType = {
        ...baseMessage,
        role: "system",
        content: "Connected to agent",
      };
      const { container } = render(<ConsoleMessage message={systemMessage} />);

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });
  });
});
