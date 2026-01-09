import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { axe } from "vitest-axe";
import { ImageLightbox } from "./image-lightbox";

describe("ImageLightbox", () => {
  const mockImages = [
    {
      src: "data:image/png;base64,abc123",
      alt: "Test image 1",
      filename: "test1.png",
    },
    {
      src: "data:image/jpeg;base64,def456",
      alt: "Test image 2",
      filename: "test2.jpg",
    },
    {
      src: "data:image/gif;base64,ghi789",
      alt: "Test image 3",
    },
  ];

  const defaultProps = {
    images: mockImages,
    initialIndex: 0,
    open: true,
    onOpenChange: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("rendering", () => {
    it("should return null when images array is empty", () => {
      const { container } = render(
        <ImageLightbox {...defaultProps} images={[]} />
      );
      expect(container.firstChild).toBeNull();
    });

    it("should render the lightbox when open is true", () => {
      render(<ImageLightbox {...defaultProps} />);

      const img = screen.getByRole("img");
      expect(img).toBeInTheDocument();
      expect(img).toHaveAttribute("src", mockImages[0].src);
      expect(img).toHaveAttribute("alt", mockImages[0].alt);
    });

    it("should not render content when open is false", () => {
      render(<ImageLightbox {...defaultProps} open={false} />);

      expect(screen.queryByRole("img")).not.toBeInTheDocument();
    });

    it("should display the current image at initialIndex", () => {
      render(<ImageLightbox {...defaultProps} initialIndex={1} />);

      const img = screen.getByRole("img");
      expect(img).toHaveAttribute("src", mockImages[1].src);
      expect(img).toHaveAttribute("alt", mockImages[1].alt);
    });

    it("should display image counter for multiple images", () => {
      render(<ImageLightbox {...defaultProps} />);

      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });

    it("should not display image counter for single image", () => {
      render(<ImageLightbox {...defaultProps} images={[mockImages[0]]} />);

      // No counter like "1 / 1" should appear for single image
      expect(screen.queryByText("1 / 1")).not.toBeInTheDocument();
    });

    it("should display filename in footer when available", () => {
      render(<ImageLightbox {...defaultProps} />);

      expect(screen.getByText("test1.png")).toBeInTheDocument();
    });

    it("should not display filename when not available", () => {
      render(<ImageLightbox {...defaultProps} initialIndex={2} />);

      // Image 3 has no filename
      expect(screen.queryByText("test3.png")).not.toBeInTheDocument();
    });
  });

  describe("zoom controls", () => {
    it("should display initial zoom level as 100%", () => {
      render(<ImageLightbox {...defaultProps} />);

      expect(screen.getByText("100%")).toBeInTheDocument();
    });

    it("should have zoom in button", () => {
      render(<ImageLightbox {...defaultProps} />);

      const zoomInButton = screen.getByRole("button", { name: "Zoom in" });
      expect(zoomInButton).toBeInTheDocument();
    });

    it("should have zoom out button", () => {
      render(<ImageLightbox {...defaultProps} />);

      const zoomOutButton = screen.getByRole("button", { name: "Zoom out" });
      expect(zoomOutButton).toBeInTheDocument();
    });

    it("should have reset zoom button", () => {
      render(<ImageLightbox {...defaultProps} />);

      const resetButton = screen.getByRole("button", { name: "Reset zoom" });
      expect(resetButton).toBeInTheDocument();
    });

    it("should increase zoom when zoom in button is clicked", () => {
      render(<ImageLightbox {...defaultProps} />);

      const zoomInButton = screen.getByRole("button", { name: "Zoom in" });
      fireEvent.click(zoomInButton);

      expect(screen.getByText("125%")).toBeInTheDocument();
    });

    it("should decrease zoom when zoom out button is clicked", () => {
      render(<ImageLightbox {...defaultProps} />);

      const zoomOutButton = screen.getByRole("button", { name: "Zoom out" });
      fireEvent.click(zoomOutButton);

      expect(screen.getByText("75%")).toBeInTheDocument();
    });

    it("should reset zoom when reset button is clicked", () => {
      render(<ImageLightbox {...defaultProps} />);

      // Zoom in first
      const zoomInButton = screen.getByRole("button", { name: "Zoom in" });
      fireEvent.click(zoomInButton);
      fireEvent.click(zoomInButton);
      expect(screen.getByText("150%")).toBeInTheDocument();

      // Reset
      const resetButton = screen.getByRole("button", { name: "Reset zoom" });
      fireEvent.click(resetButton);

      expect(screen.getByText("100%")).toBeInTheDocument();
    });

    it("should disable zoom out at minimum zoom (50%)", () => {
      render(<ImageLightbox {...defaultProps} />);

      const zoomOutButton = screen.getByRole("button", { name: "Zoom out" });

      // Zoom out to minimum
      fireEvent.click(zoomOutButton); // 75%
      fireEvent.click(zoomOutButton); // 50%

      expect(screen.getByText("50%")).toBeInTheDocument();
      expect(zoomOutButton).toBeDisabled();
    });

    it("should disable zoom in at maximum zoom (300%)", () => {
      render(<ImageLightbox {...defaultProps} />);

      const zoomInButton = screen.getByRole("button", { name: "Zoom in" });

      // Zoom in to maximum
      for (let i = 0; i < 10; i++) {
        fireEvent.click(zoomInButton);
      }

      expect(screen.getByText("300%")).toBeInTheDocument();
      expect(zoomInButton).toBeDisabled();
    });
  });

  describe("navigation", () => {
    it("should show navigation arrows for multiple images", () => {
      render(<ImageLightbox {...defaultProps} />);

      expect(screen.getByRole("button", { name: "Previous image" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Next image" })).toBeInTheDocument();
    });

    it("should not show navigation arrows for single image", () => {
      render(<ImageLightbox {...defaultProps} images={[mockImages[0]]} />);

      expect(screen.queryByRole("button", { name: "Previous image" })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Next image" })).not.toBeInTheDocument();
    });

    it("should navigate to next image when next button is clicked", () => {
      render(<ImageLightbox {...defaultProps} />);

      const nextButton = screen.getByRole("button", { name: "Next image" });
      fireEvent.click(nextButton);

      expect(screen.getByText("2 / 3")).toBeInTheDocument();
      const img = screen.getByRole("img");
      expect(img).toHaveAttribute("src", mockImages[1].src);
    });

    it("should navigate to previous image when previous button is clicked", () => {
      render(<ImageLightbox {...defaultProps} initialIndex={1} />);

      const prevButton = screen.getByRole("button", { name: "Previous image" });
      fireEvent.click(prevButton);

      expect(screen.getByText("1 / 3")).toBeInTheDocument();
      const img = screen.getByRole("img");
      expect(img).toHaveAttribute("src", mockImages[0].src);
    });

    it("should wrap to last image when previous is clicked on first image", () => {
      render(<ImageLightbox {...defaultProps} initialIndex={0} />);

      const prevButton = screen.getByRole("button", { name: "Previous image" });
      fireEvent.click(prevButton);

      expect(screen.getByText("3 / 3")).toBeInTheDocument();
    });

    it("should wrap to first image when next is clicked on last image", () => {
      render(<ImageLightbox {...defaultProps} initialIndex={2} />);

      const nextButton = screen.getByRole("button", { name: "Next image" });
      fireEvent.click(nextButton);

      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });

    it("should reset zoom when navigating to another image", () => {
      render(<ImageLightbox {...defaultProps} />);

      // Zoom in first
      const zoomInButton = screen.getByRole("button", { name: "Zoom in" });
      fireEvent.click(zoomInButton);
      expect(screen.getByText("125%")).toBeInTheDocument();

      // Navigate
      const nextButton = screen.getByRole("button", { name: "Next image" });
      fireEvent.click(nextButton);

      // Zoom should be reset
      expect(screen.getByText("100%")).toBeInTheDocument();
    });
  });

  describe("keyboard navigation", () => {
    it("should navigate with arrow keys", () => {
      render(<ImageLightbox {...defaultProps} />);

      // Press right arrow
      fireEvent.keyDown(window, { key: "ArrowRight" });
      expect(screen.getByText("2 / 3")).toBeInTheDocument();

      // Press left arrow
      fireEvent.keyDown(window, { key: "ArrowLeft" });
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });

    it("should zoom with + and - keys", () => {
      render(<ImageLightbox {...defaultProps} />);

      // Press + to zoom in
      fireEvent.keyDown(window, { key: "+" });
      expect(screen.getByText("125%")).toBeInTheDocument();

      // Press - to zoom out
      fireEvent.keyDown(window, { key: "-" });
      expect(screen.getByText("100%")).toBeInTheDocument();
    });

    it("should reset zoom with 0 key", () => {
      render(<ImageLightbox {...defaultProps} />);

      // Zoom in first
      fireEvent.keyDown(window, { key: "+" });
      fireEvent.keyDown(window, { key: "+" });
      expect(screen.getByText("150%")).toBeInTheDocument();

      // Press 0 to reset
      fireEvent.keyDown(window, { key: "0" });
      expect(screen.getByText("100%")).toBeInTheDocument();
    });

    it("should zoom with = key (same as +)", () => {
      render(<ImageLightbox {...defaultProps} />);

      fireEvent.keyDown(window, { key: "=" });
      expect(screen.getByText("125%")).toBeInTheDocument();
    });

    it("should not navigate with arrow keys for single image", () => {
      render(<ImageLightbox {...defaultProps} images={[mockImages[0]]} />);

      // Press right arrow - should not change anything
      fireEvent.keyDown(window, { key: "ArrowRight" });

      // No counter should appear for single image
      expect(screen.queryByText("1 / 1")).not.toBeInTheDocument();
    });
  });

  describe("mouse interactions", () => {
    it("should zoom with mouse wheel", () => {
      render(<ImageLightbox {...defaultProps} />);

      const imageContainer = screen.getByRole("img").parentElement!;

      // Scroll up to zoom in
      fireEvent.wheel(imageContainer, { deltaY: -100 });
      expect(screen.getByText("125%")).toBeInTheDocument();

      // Scroll down to zoom out
      fireEvent.wheel(imageContainer, { deltaY: 100 });
      expect(screen.getByText("100%")).toBeInTheDocument();
    });
  });

  describe("download", () => {
    it("should have download button", () => {
      render(<ImageLightbox {...defaultProps} />);

      const downloadButton = screen.getByRole("button", { name: "Download image" });
      expect(downloadButton).toBeInTheDocument();
    });

    it("should trigger download when download button is clicked", () => {
      const clickMock = vi.fn();
      const originalCreateElement = document.createElement.bind(document);
      vi.spyOn(document, "createElement").mockImplementation((tagName: string) => {
        if (tagName === "a") {
          const link = originalCreateElement("a");
          link.click = clickMock;
          return link;
        }
        return originalCreateElement(tagName);
      });

      render(<ImageLightbox {...defaultProps} />);

      const downloadButton = screen.getByRole("button", { name: "Download image" });
      fireEvent.click(downloadButton);

      expect(clickMock).toHaveBeenCalled();

      vi.restoreAllMocks();
    });
  });

  describe("close", () => {
    it("should have close button", () => {
      render(<ImageLightbox {...defaultProps} />);

      const closeButton = screen.getByRole("button", { name: "Close" });
      expect(closeButton).toBeInTheDocument();
    });
  });

  describe("state reset", () => {
    it("should reset to initialIndex when dialog reopens", () => {
      const { rerender } = render(<ImageLightbox {...defaultProps} initialIndex={0} />);

      // Navigate to image 2
      const nextButton = screen.getByRole("button", { name: "Next image" });
      fireEvent.click(nextButton);
      expect(screen.getByText("2 / 3")).toBeInTheDocument();

      // Close dialog
      rerender(<ImageLightbox {...defaultProps} open={false} initialIndex={0} />);

      // Reopen dialog
      rerender(<ImageLightbox {...defaultProps} open={true} initialIndex={0} />);

      // Should be back at image 1
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });

    it("should start at new initialIndex when reopened", () => {
      const { rerender } = render(<ImageLightbox {...defaultProps} initialIndex={0} />);

      expect(screen.getByText("1 / 3")).toBeInTheDocument();

      // Close dialog
      rerender(<ImageLightbox {...defaultProps} open={false} initialIndex={2} />);

      // Reopen with new initialIndex
      rerender(<ImageLightbox {...defaultProps} open={true} initialIndex={2} />);

      // Should be at image 3
      expect(screen.getByText("3 / 3")).toBeInTheDocument();
    });
  });

  describe("accessibility", () => {
    it("should have no accessibility violations when open", async () => {
      const { container } = render(<ImageLightbox {...defaultProps} />);

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });

    it("should have no accessibility violations with single image", async () => {
      const { container } = render(
        <ImageLightbox {...defaultProps} images={[mockImages[0]]} />
      );

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });
  });
});
