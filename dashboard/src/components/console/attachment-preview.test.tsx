import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { axe } from "vitest-axe";
import { AttachmentPreview } from "./attachment-preview";
import type { FileAttachment } from "@/types/websocket";

describe("AttachmentPreview", () => {
  const mockImageAttachment: FileAttachment = {
    id: "img-1",
    name: "test-image.png",
    type: "image/png",
    size: 1024,
    dataUrl: "data:image/png;base64,iVBORw0KGgo=",
  };

  const mockAudioAttachment: FileAttachment = {
    id: "audio-1",
    name: "test-audio.mp3",
    type: "audio/mpeg",
    size: 2048,
    dataUrl: "data:audio/mpeg;base64,abc123",
  };

  const mockFileAttachment: FileAttachment = {
    id: "file-1",
    name: "document.pdf",
    type: "application/pdf",
    size: 10240,
    dataUrl: "data:application/pdf;base64,xyz789",
  };

  describe("rendering", () => {
    it("should return null when attachments array is empty", () => {
      const { container } = render(<AttachmentPreview attachments={[]} />);
      expect(container.firstChild).toBeNull();
    });

    it("should render image attachment as thumbnail", () => {
      render(<AttachmentPreview attachments={[mockImageAttachment]} />);

      const img = screen.getByRole("img");
      expect(img).toBeInTheDocument();
      expect(img).toHaveAttribute("src", mockImageAttachment.dataUrl);
      expect(img).toHaveAttribute("alt", mockImageAttachment.name);
    });

    it("should render audio attachment with icon and file info", () => {
      render(<AttachmentPreview attachments={[mockAudioAttachment]} />);

      expect(screen.getByText("test-audio.mp3")).toBeInTheDocument();
      expect(screen.getByText("2.0 KB")).toBeInTheDocument();
    });

    it("should render generic file attachment with icon and file info", () => {
      render(<AttachmentPreview attachments={[mockFileAttachment]} />);

      expect(screen.getByText("document.pdf")).toBeInTheDocument();
      expect(screen.getByText("10.0 KB")).toBeInTheDocument();
    });

    it("should render multiple attachments", () => {
      render(
        <AttachmentPreview
          attachments={[mockImageAttachment, mockAudioAttachment, mockFileAttachment]}
        />
      );

      expect(screen.getByRole("img")).toBeInTheDocument();
      expect(screen.getByText("test-audio.mp3")).toBeInTheDocument();
      expect(screen.getByText("document.pdf")).toBeInTheDocument();
    });

    it("should apply custom className", () => {
      const { container } = render(
        <AttachmentPreview
          attachments={[mockImageAttachment]}
          className="custom-class"
        />
      );

      expect(container.firstChild).toHaveClass("custom-class");
    });
  });

  describe("file size formatting", () => {
    it("should format bytes correctly", () => {
      const smallFile: FileAttachment = {
        ...mockFileAttachment,
        size: 500,
      };
      render(<AttachmentPreview attachments={[smallFile]} />);
      expect(screen.getByText("500 B")).toBeInTheDocument();
    });

    it("should format kilobytes correctly", () => {
      const kbFile: FileAttachment = {
        ...mockFileAttachment,
        size: 1536, // 1.5 KB
      };
      render(<AttachmentPreview attachments={[kbFile]} />);
      expect(screen.getByText("1.5 KB")).toBeInTheDocument();
    });

    it("should format megabytes correctly", () => {
      const mbFile: FileAttachment = {
        ...mockFileAttachment,
        size: 2 * 1024 * 1024, // 2 MB
      };
      render(<AttachmentPreview attachments={[mbFile]} />);
      expect(screen.getByText("2.0 MB")).toBeInTheDocument();
    });
  });

  describe("remove functionality", () => {
    it("should render remove button when onRemove is provided", () => {
      const onRemove = vi.fn();
      render(
        <AttachmentPreview
          attachments={[mockImageAttachment]}
          onRemove={onRemove}
        />
      );

      const removeButton = screen.getByRole("button", {
        name: `Remove ${mockImageAttachment.name}`,
      });
      expect(removeButton).toBeInTheDocument();
    });

    it("should call onRemove with attachment id when remove button is clicked", () => {
      const onRemove = vi.fn();
      render(
        <AttachmentPreview
          attachments={[mockImageAttachment]}
          onRemove={onRemove}
        />
      );

      const removeButton = screen.getByRole("button", {
        name: `Remove ${mockImageAttachment.name}`,
      });
      fireEvent.click(removeButton);

      expect(onRemove).toHaveBeenCalledWith(mockImageAttachment.id);
    });

    it("should not render remove button when readonly is true", () => {
      const onRemove = vi.fn();
      render(
        <AttachmentPreview
          attachments={[mockImageAttachment]}
          onRemove={onRemove}
          readonly={true}
        />
      );

      expect(
        screen.queryByRole("button", {
          name: `Remove ${mockImageAttachment.name}`,
        })
      ).not.toBeInTheDocument();
    });

    it("should not render remove button when onRemove is not provided", () => {
      render(<AttachmentPreview attachments={[mockImageAttachment]} />);

      expect(
        screen.queryByRole("button", {
          name: `Remove ${mockImageAttachment.name}`,
        })
      ).not.toBeInTheDocument();
    });
  });

  describe("image overlay", () => {
    it("should show filename overlay on image attachments", () => {
      render(<AttachmentPreview attachments={[mockImageAttachment]} />);

      // The filename appears in an overlay div for images
      const filenameElements = screen.getAllByText(mockImageAttachment.name);
      expect(filenameElements.length).toBeGreaterThanOrEqual(1);
    });
  });

  describe("accessibility", () => {
    it("should have no accessibility violations with image attachments", async () => {
      const { container } = render(
        <AttachmentPreview attachments={[mockImageAttachment]} />
      );

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });

    it("should have no accessibility violations with mixed attachments", async () => {
      const { container } = render(
        <AttachmentPreview
          attachments={[mockImageAttachment, mockAudioAttachment, mockFileAttachment]}
        />
      );

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });
  });
});
