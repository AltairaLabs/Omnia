import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { axe } from "vitest-axe";
import { DocumentPreview } from "./document-preview";

// Mock ResizeObserver for Radix UI components
class MockResizeObserver {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
global.ResizeObserver = MockResizeObserver;

describe("DocumentPreview", () => {
  const mockPdfAttachment = {
    src: "data:application/pdf;base64,JVBERi0xLjQK",
    filename: "report.pdf",
    type: "application/pdf",
    size: 2048000,
  };

  const mockJsonAttachment = {
    src: "data:application/json;base64,eyJuYW1lIjoidGVzdCJ9",
    filename: "data.json",
    type: "application/json",
    size: 1024,
  };

  const mockTextAttachment = {
    src: "data:text/plain;base64,SGVsbG8gV29ybGQh",
    filename: "readme.txt",
    type: "text/plain",
    size: 512,
  };

  const mockWordAttachment = {
    src: "data:application/vnd.openxmlformats-officedocument.wordprocessingml.document;base64,abc",
    filename: "document.docx",
    type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    size: 50000,
  };

  const mockExcelAttachment = {
    src: "data:application/vnd.openxmlformats-officedocument.spreadsheetml.sheet;base64,xyz",
    filename: "spreadsheet.xlsx",
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    size: 75000,
  };

  const mockArchiveAttachment = {
    src: "data:application/zip;base64,UEsDBBQ=",
    filename: "archive.zip",
    type: "application/zip",
    size: 1000000,
  };

  describe("rendering", () => {
    it("should render PDF document with correct info", () => {
      render(<DocumentPreview {...mockPdfAttachment} />);

      expect(screen.getByText("report.pdf")).toBeInTheDocument();
      expect(screen.getByText("2.0 MB • PDF Document")).toBeInTheDocument();
    });

    it("should render JSON file with correct info", () => {
      render(<DocumentPreview {...mockJsonAttachment} />);

      expect(screen.getByText("data.json")).toBeInTheDocument();
      expect(screen.getByText("1.0 KB • JSON File")).toBeInTheDocument();
    });

    it("should render text file with correct info", () => {
      render(<DocumentPreview {...mockTextAttachment} />);

      expect(screen.getByText("readme.txt")).toBeInTheDocument();
      expect(screen.getByText("512 B • Text File")).toBeInTheDocument();
    });

    it("should render Word document with correct info", () => {
      render(<DocumentPreview {...mockWordAttachment} />);

      expect(screen.getByText("document.docx")).toBeInTheDocument();
      expect(screen.getByText("48.8 KB • Word Document")).toBeInTheDocument();
    });

    it("should render Excel file with correct info", () => {
      render(<DocumentPreview {...mockExcelAttachment} />);

      expect(screen.getByText("spreadsheet.xlsx")).toBeInTheDocument();
      expect(screen.getByText("73.2 KB • Spreadsheet")).toBeInTheDocument();
    });

    it("should render archive file with correct info", () => {
      render(<DocumentPreview {...mockArchiveAttachment} />);

      expect(screen.getByText("archive.zip")).toBeInTheDocument();
      expect(screen.getByText("976.6 KB • Archive")).toBeInTheDocument();
    });

    it("should apply custom className", () => {
      const { container } = render(
        <DocumentPreview {...mockPdfAttachment} className="custom-class" />
      );

      expect(container.firstChild).toHaveClass("custom-class");
    });
  });

  describe("download functionality", () => {
    it("should render download link with correct attributes", () => {
      render(<DocumentPreview {...mockPdfAttachment} />);

      const downloadLink = screen.getByRole("link", { name: `Download ${mockPdfAttachment.filename}` });
      expect(downloadLink).toHaveAttribute("href", mockPdfAttachment.src);
      expect(downloadLink).toHaveAttribute("download", mockPdfAttachment.filename);
    });
  });

  describe("preview functionality", () => {
    it("should show preview button for PDF files", () => {
      render(<DocumentPreview {...mockPdfAttachment} />);

      expect(screen.getByRole("button", { name: "Show preview" })).toBeInTheDocument();
    });

    it("should show preview button for JSON files", () => {
      render(<DocumentPreview {...mockJsonAttachment} />);

      expect(screen.getByRole("button", { name: "Show preview" })).toBeInTheDocument();
    });

    it("should show preview button for text files", () => {
      render(<DocumentPreview {...mockTextAttachment} />);

      expect(screen.getByRole("button", { name: "Show preview" })).toBeInTheDocument();
    });

    it("should not show preview button for Word documents", () => {
      render(<DocumentPreview {...mockWordAttachment} />);

      expect(screen.queryByRole("button", { name: "Show preview" })).not.toBeInTheDocument();
    });

    it("should not show preview button for Excel files", () => {
      render(<DocumentPreview {...mockExcelAttachment} />);

      expect(screen.queryByRole("button", { name: "Show preview" })).not.toBeInTheDocument();
    });

    it("should not show preview button for archive files", () => {
      render(<DocumentPreview {...mockArchiveAttachment} />);

      expect(screen.queryByRole("button", { name: "Show preview" })).not.toBeInTheDocument();
    });

    it("should toggle preview when button is clicked", async () => {
      render(<DocumentPreview {...mockPdfAttachment} />);

      const previewButton = screen.getByRole("button", { name: "Show preview" });
      fireEvent.click(previewButton);

      // After clicking, button should change to "Hide preview"
      expect(screen.getByRole("button", { name: "Hide preview" })).toBeInTheDocument();

      // PDF preview should be visible (object element)
      expect(screen.getByLabelText(`Preview of ${mockPdfAttachment.filename}`)).toBeInTheDocument();
    });

    it("should hide preview when button is clicked again", async () => {
      render(<DocumentPreview {...mockPdfAttachment} />);

      // Show preview
      const showButton = screen.getByRole("button", { name: "Show preview" });
      fireEvent.click(showButton);

      // Hide preview
      const hideButton = screen.getByRole("button", { name: "Hide preview" });
      fireEvent.click(hideButton);

      // Should be back to "Show preview"
      expect(screen.getByRole("button", { name: "Show preview" })).toBeInTheDocument();
    });

    it("should show text content preview for text files", async () => {
      render(<DocumentPreview {...mockTextAttachment} />);

      const previewButton = screen.getByRole("button", { name: "Show preview" });
      fireEvent.click(previewButton);

      // Wait for the content to be decoded and displayed
      await waitFor(() => {
        expect(screen.getByText("Hello World!")).toBeInTheDocument();
      });
    });

    it("should show JSON content preview", async () => {
      render(<DocumentPreview {...mockJsonAttachment} />);

      const previewButton = screen.getByRole("button", { name: "Show preview" });
      fireEvent.click(previewButton);

      // Wait for the content to be decoded and displayed
      await waitFor(() => {
        expect(screen.getByText(/name.*test/)).toBeInTheDocument();
      });
    });
  });

  describe("file size formatting", () => {
    it("should format bytes correctly", () => {
      const { rerender } = render(
        <DocumentPreview
          src="data:application/octet-stream;base64,test"
          filename="file.bin"
          type="application/octet-stream"
          size={500}
        />
      );
      expect(screen.getByText(/500 B/)).toBeInTheDocument();

      rerender(
        <DocumentPreview
          src="data:application/octet-stream;base64,test"
          filename="file.bin"
          type="application/octet-stream"
          size={2048}
        />
      );
      expect(screen.getByText(/2.0 KB/)).toBeInTheDocument();

      rerender(
        <DocumentPreview
          src="data:application/octet-stream;base64,test"
          filename="file.bin"
          type="application/octet-stream"
          size={5242880}
        />
      );
      expect(screen.getByText(/5.0 MB/)).toBeInTheDocument();
    });
  });

  describe("file type detection", () => {
    it("should detect PDF by extension when MIME type is generic", () => {
      render(
        <DocumentPreview
          src="data:application/octet-stream;base64,test"
          filename="document.pdf"
          type="application/octet-stream"
          size={1000}
        />
      );

      expect(screen.getByText("1000 B • PDF Document")).toBeInTheDocument();
    });

    it("should detect CSV files", () => {
      render(
        <DocumentPreview
          src="data:text/csv;base64,test"
          filename="data.csv"
          type="text/csv"
          size={1000}
        />
      );

      expect(screen.getByText(/Spreadsheet/)).toBeInTheDocument();
    });

    it("should detect code files by extension", () => {
      render(
        <DocumentPreview
          src="data:application/octet-stream;base64,test"
          filename="script.py"
          type="application/octet-stream"
          size={1000}
        />
      );

      expect(screen.getByText(/Text File/)).toBeInTheDocument();
    });
  });

  describe("accessibility", () => {
    it("should have no accessibility violations", async () => {
      const { container } = render(<DocumentPreview {...mockPdfAttachment} />);

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });

    it("should have no accessibility violations with preview open", async () => {
      const { container } = render(<DocumentPreview {...mockTextAttachment} />);

      const previewButton = screen.getByRole("button", { name: "Show preview" });
      fireEvent.click(previewButton);

      // Wait for preview to render
      await waitFor(() => {
        expect(screen.getByText("Hello World!")).toBeInTheDocument();
      });

      const results = await axe(container);
      expect(results).toHaveNoViolations();
    });

    it("should have accessible download link", () => {
      render(<DocumentPreview {...mockPdfAttachment} />);

      const downloadLink = screen.getByRole("link", { name: `Download ${mockPdfAttachment.filename}` });
      expect(downloadLink).toBeInTheDocument();
    });

    it("should have accessible preview button", () => {
      render(<DocumentPreview {...mockPdfAttachment} />);

      const previewButton = screen.getByRole("button", { name: "Show preview" });
      expect(previewButton).toBeInTheDocument();
    });
  });
});
