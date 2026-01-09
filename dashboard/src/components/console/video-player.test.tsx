import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { VideoPlayer } from "./video-player";

// Mock ResizeObserver for Radix UI components (needs to be a class)
class MockResizeObserver {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
global.ResizeObserver = MockResizeObserver;

describe("VideoPlayer", () => {
  describe("rendering", () => {
    it("should render play overlay initially", () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      expect(screen.getByRole("button", { name: "Play video" })).toBeInTheDocument();
    });

    it("should render filename when provided", () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" filename="video.mp4" />);

      expect(screen.getByText("video.mp4")).toBeInTheDocument();
    });

    it("should not render filename when not provided", () => {
      const { container } = render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Check that there's no paragraph with a filename
      const paragraphs = container.querySelectorAll("p");
      expect(paragraphs.length).toBe(0);
    });

    it("should render video element", () => {
      const { container } = render(<VideoPlayer src="data:video/mp4;base64,test" />);

      const videoElement = container.querySelector("video");
      expect(videoElement).toBeInTheDocument();
      expect(videoElement).toHaveAttribute("src", "data:video/mp4;base64,test");
    });

    it("should render file size when provided", () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" fileSize={15200000} />);

      expect(screen.getByText("14.5 MB")).toBeInTheDocument();
    });

    it("should apply custom className", () => {
      const { container } = render(
        <VideoPlayer src="data:video/mp4;base64,test" className="custom-class" />
      );

      expect(container.firstChild).toHaveClass("custom-class");
    });

    it("should have accessible video element", () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" filename="tutorial.mp4" />);

      const video = screen.getByLabelText("Video: tutorial.mp4");
      expect(video).toBeInTheDocument();
    });
  });

  describe("play/pause interaction", () => {
    it("should have play overlay button initially", () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      expect(screen.getByRole("button", { name: "Play video" })).toBeInTheDocument();
    });

    it("should hide overlay when play is clicked", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      const playButton = screen.getByRole("button", { name: "Play video" });

      await act(async () => {
        fireEvent.click(playButton);
      });

      // After clicking, the overlay should be hidden and controls should appear
      // The overlay play button should no longer be visible
      expect(screen.queryByRole("button", { name: "Play video" })).not.toBeInTheDocument();
    });

    it("should show controls after video starts", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      const playButton = screen.getByRole("button", { name: "Play video" });

      await act(async () => {
        fireEvent.click(playButton);
      });

      // Control buttons should now be visible (jsdom doesn't implement play(),
      // so the button stays as "Play" instead of "Pause")
      expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Mute" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Fullscreen" })).toBeInTheDocument();
    });
  });

  describe("mute/unmute interaction", () => {
    it("should toggle mute state when mute button is clicked", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start the video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      const muteButton = screen.getByRole("button", { name: "Mute" });
      await act(async () => {
        fireEvent.click(muteButton);
      });

      expect(screen.getByRole("button", { name: "Unmute" })).toBeInTheDocument();
    });

    it("should toggle back to mute button when unmute is clicked", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start the video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      // Mute
      const muteButton = screen.getByRole("button", { name: "Mute" });
      await act(async () => {
        fireEvent.click(muteButton);
      });

      // Unmute
      const unmuteButton = screen.getByRole("button", { name: "Unmute" });
      await act(async () => {
        fireEvent.click(unmuteButton);
      });

      expect(screen.getByRole("button", { name: "Mute" })).toBeInTheDocument();
    });
  });

  describe("fullscreen", () => {
    it("should have fullscreen button after video starts", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start the video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      expect(screen.getByRole("button", { name: "Fullscreen" })).toBeInTheDocument();
    });

    it("should attempt to enter fullscreen when button is clicked", async () => {
      // Mock requestFullscreen
      const mockRequestFullscreen = vi.fn().mockResolvedValue(undefined);
      HTMLDivElement.prototype.requestFullscreen = mockRequestFullscreen;

      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start the video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      const fullscreenButton = screen.getByRole("button", { name: "Fullscreen" });
      await act(async () => {
        fireEvent.click(fullscreenButton);
      });

      expect(mockRequestFullscreen).toHaveBeenCalled();
    });

    it("should attempt to exit fullscreen when already in fullscreen", async () => {
      // Mock exitFullscreen and fullscreenElement
      const mockExitFullscreen = vi.fn().mockResolvedValue(undefined);
      Object.defineProperty(document, "fullscreenElement", {
        value: document.createElement("div"),
        writable: true,
        configurable: true,
      });
      document.exitFullscreen = mockExitFullscreen;

      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start the video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      const fullscreenButton = screen.getByRole("button", { name: "Fullscreen" });
      await act(async () => {
        fireEvent.click(fullscreenButton);
      });

      expect(mockExitFullscreen).toHaveBeenCalled();

      // Clean up
      Object.defineProperty(document, "fullscreenElement", {
        value: null,
        writable: true,
        configurable: true,
      });
    });

    it("should handle fullscreen error gracefully", async () => {
      // Mock requestFullscreen to throw
      const mockRequestFullscreen = vi.fn().mockRejectedValue(new Error("Fullscreen not allowed"));
      HTMLDivElement.prototype.requestFullscreen = mockRequestFullscreen;
      const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start the video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      const fullscreenButton = screen.getByRole("button", { name: "Fullscreen" });
      await act(async () => {
        fireEvent.click(fullscreenButton);
      });

      // Wait for the error to be logged
      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 0));
      });

      expect(consoleSpy).toHaveBeenCalledWith("Fullscreen error:", expect.any(Error));
      consoleSpy.mockRestore();
    });
  });

  describe("slider interactions", () => {
    it("should render volume slider with correct initial value after video starts", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      const volumeSlider = screen.getByRole("slider", { name: "Volume" });
      expect(volumeSlider).toBeInTheDocument();
      expect(volumeSlider).toHaveAttribute("aria-valuenow", "100");
    });

    it("should show volume at 0 when muted", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      // Mute
      const muteButton = screen.getByRole("button", { name: "Mute" });
      await act(async () => {
        fireEvent.click(muteButton);
      });

      // Volume slider should show 0 when muted
      const volumeSlider = screen.getByRole("slider", { name: "Volume" });
      expect(volumeSlider).toHaveAttribute("aria-valuenow", "0");
    });
  });

  describe("mouse interactions", () => {
    it("should handle mouse move on container", async () => {
      const { container } = render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      // Trigger mouse move
      const containerDiv = container.firstChild as HTMLElement;
      await act(async () => {
        fireEvent.mouseMove(containerDiv);
      });

      // Controls should still be visible after mouse move
      expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
    });

    it("should handle mouse leave on container", async () => {
      const { container } = render(<VideoPlayer src="data:video/mp4;base64,test" />);

      // Start video first
      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      // Trigger mouse leave
      const containerDiv = container.firstChild as HTMLElement;
      await act(async () => {
        fireEvent.mouseLeave(containerDiv);
      });

      // Component should still render correctly
      expect(container.querySelector("video")).toBeInTheDocument();
    });
  });

  describe("accessibility", () => {
    it("should have accessible play button", () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      expect(screen.getByRole("button", { name: "Play video" })).toBeInTheDocument();
    });

    it("should have accessible video element with default label", () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      expect(screen.getByLabelText("Video player")).toBeInTheDocument();
    });

    it("should have accessible seek slider after video starts", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      expect(screen.getByRole("slider", { name: "Seek video" })).toBeInTheDocument();
    });

    it("should have accessible volume slider after video starts", async () => {
      render(<VideoPlayer src="data:video/mp4;base64,test" />);

      const playButton = screen.getByRole("button", { name: "Play video" });
      await act(async () => {
        fireEvent.click(playButton);
      });

      expect(screen.getByRole("slider", { name: "Volume" })).toBeInTheDocument();
    });
  });

  describe("file size formatting", () => {
    it("should format bytes correctly", () => {
      const { rerender } = render(<VideoPlayer src="data:video/mp4;base64,test" fileSize={500} />);
      expect(screen.getByText("500 B")).toBeInTheDocument();

      rerender(<VideoPlayer src="data:video/mp4;base64,test" fileSize={2048} />);
      expect(screen.getByText("2.0 KB")).toBeInTheDocument();

      rerender(<VideoPlayer src="data:video/mp4;base64,test" fileSize={5242880} />);
      expect(screen.getByText("5.0 MB")).toBeInTheDocument();
    });
  });
});
