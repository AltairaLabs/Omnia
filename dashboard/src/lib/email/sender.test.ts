/**
 * Tests for email sender.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock nodemailer before importing sender so the mock is in place.
vi.mock("nodemailer", () => {
  const sendMailMock = vi.fn();
  const createTransportMock = vi.fn(() => ({ sendMail: sendMailMock }));
  return { default: { createTransport: createTransportMock } };
});

import nodemailer from "nodemailer";
import { sendEmail } from "./sender";

// Typed references to the nodemailer mocks for assertions.
const createTransportMock = nodemailer.createTransport as ReturnType<typeof vi.fn>;

function getSendMailMock(): ReturnType<typeof vi.fn> {
  return createTransportMock.mock.results[0]?.value?.sendMail as ReturnType<typeof vi.fn>;
}

describe("sendEmail", () => {
  const originalEnv = {
    OMNIA_SMTP_HOST: process.env.OMNIA_SMTP_HOST,
    OMNIA_SMTP_PORT: process.env.OMNIA_SMTP_PORT,
    OMNIA_SMTP_SECURE: process.env.OMNIA_SMTP_SECURE,
    OMNIA_SMTP_FROM: process.env.OMNIA_SMTP_FROM,
    OMNIA_SMTP_USER: process.env.OMNIA_SMTP_USER,
    OMNIA_SMTP_PASS: process.env.OMNIA_SMTP_PASS,
  };

  beforeEach(() => {
    delete process.env.OMNIA_SMTP_HOST;
    delete process.env.OMNIA_SMTP_PORT;
    delete process.env.OMNIA_SMTP_SECURE;
    delete process.env.OMNIA_SMTP_FROM;
    delete process.env.OMNIA_SMTP_USER;
    delete process.env.OMNIA_SMTP_PASS;
    vi.clearAllMocks();
  });

  afterEach(() => {
    for (const [key, value] of Object.entries(originalEnv)) {
      if (value === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = value;
      }
    }
    vi.restoreAllMocks();
  });

  describe("console fallback (SMTP not configured)", () => {
    it("calls console.warn when OMNIA_SMTP_HOST is not set", async () => {
      const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

      await sendEmail({
        to: "user@example.com",
        subject: "Test Subject",
        text: "Test body text",
      });

      expect(warnSpy).toHaveBeenCalledTimes(1);
    });

    it("returns accepted: false when SMTP is not configured", async () => {
      vi.spyOn(console, "warn").mockImplementation(() => {});

      const result = await sendEmail({
        to: "user@example.com",
        subject: "Test Subject",
        text: "Test body text",
      });

      expect(result.accepted).toBe(false);
    });

    it("returns the to address in the result", async () => {
      vi.spyOn(console, "warn").mockImplementation(() => {});

      const result = await sendEmail({
        to: "user@example.com",
        subject: "Test Subject",
        text: "Test body text",
      });

      expect(result.to).toBe("user@example.com");
    });

    it("returns the subject in the result", async () => {
      vi.spyOn(console, "warn").mockImplementation(() => {});

      const result = await sendEmail({
        to: "user@example.com",
        subject: "Password Reset Request",
        text: "Click here to reset your password.",
      });

      expect(result.subject).toBe("Password Reset Request");
    });

    it("includes to address and subject in the warning output", async () => {
      const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});

      await sendEmail({
        to: "user@example.com",
        subject: "Verify Your Email",
        text: "Click here to verify.",
      });

      const warnArg = warnSpy.mock.calls[0][0] as string;
      expect(warnArg).toContain("user@example.com");
      expect(warnArg).toContain("Verify Your Email");
    });
  });

  describe("SMTP path (OMNIA_SMTP_HOST is set)", () => {
    beforeEach(() => {
      process.env.OMNIA_SMTP_HOST = "smtp.example.com";
      // Configure sendMail mock to return accepted addresses by default.
      const sendMail = vi.fn().mockResolvedValue({ accepted: ["user@example.com"] });
      createTransportMock.mockReturnValue({ sendMail });
    });

    it("calls createTransport with the smtp host", async () => {
      await sendEmail({
        to: "user@example.com",
        subject: "Hello",
        text: "Body",
      });

      expect(createTransportMock).toHaveBeenCalledTimes(1);
      expect(createTransportMock).toHaveBeenCalledWith(
        expect.objectContaining({ host: "smtp.example.com" })
      );
    });

    it("uses default port 587 when OMNIA_SMTP_PORT is not set", async () => {
      await sendEmail({ to: "user@example.com", subject: "Hello", text: "Body" });

      expect(createTransportMock).toHaveBeenCalledWith(
        expect.objectContaining({ port: 587 })
      );
    });

    it("uses OMNIA_SMTP_PORT when set", async () => {
      process.env.OMNIA_SMTP_PORT = "465";
      await sendEmail({ to: "user@example.com", subject: "Hello", text: "Body" });

      expect(createTransportMock).toHaveBeenCalledWith(
        expect.objectContaining({ port: 465 })
      );
    });

    it("sets secure=false by default", async () => {
      await sendEmail({ to: "user@example.com", subject: "Hello", text: "Body" });

      expect(createTransportMock).toHaveBeenCalledWith(
        expect.objectContaining({ secure: false })
      );
    });

    it("sets secure=true when OMNIA_SMTP_SECURE=true", async () => {
      process.env.OMNIA_SMTP_SECURE = "true";
      await sendEmail({ to: "user@example.com", subject: "Hello", text: "Body" });

      expect(createTransportMock).toHaveBeenCalledWith(
        expect.objectContaining({ secure: true })
      );
    });

    it("does not include auth when OMNIA_SMTP_USER is not set", async () => {
      await sendEmail({ to: "user@example.com", subject: "Hello", text: "Body" });

      const callArg = createTransportMock.mock.calls[0][0] as Record<string, unknown>;
      expect(callArg).not.toHaveProperty("auth");
    });

    it("includes auth when OMNIA_SMTP_USER and OMNIA_SMTP_PASS are set", async () => {
      process.env.OMNIA_SMTP_USER = "myuser";
      process.env.OMNIA_SMTP_PASS = "mypass";

      await sendEmail({ to: "user@example.com", subject: "Hello", text: "Body" });

      expect(createTransportMock).toHaveBeenCalledWith(
        expect.objectContaining({
          auth: { user: "myuser", pass: "mypass" },
        })
      );
    });

    it("calls sendMail with correct to, subject, and text", async () => {
      await sendEmail({
        to: "user@example.com",
        subject: "Test Subject",
        text: "Test body",
      });

      const sendMail = getSendMailMock();
      expect(sendMail).toHaveBeenCalledWith(
        expect.objectContaining({
          to: "user@example.com",
          subject: "Test Subject",
          text: "Test body",
        })
      );
    });

    it("uses default from address when OMNIA_SMTP_FROM is not set", async () => {
      await sendEmail({ to: "user@example.com", subject: "Hello", text: "Body" });

      const sendMail = getSendMailMock();
      expect(sendMail).toHaveBeenCalledWith(
        expect.objectContaining({ from: "noreply@omnia.local" })
      );
    });

    it("uses OMNIA_SMTP_FROM when set", async () => {
      process.env.OMNIA_SMTP_FROM = "no-reply@mycompany.com";
      await sendEmail({ to: "user@example.com", subject: "Hello", text: "Body" });

      const sendMail = getSendMailMock();
      expect(sendMail).toHaveBeenCalledWith(
        expect.objectContaining({ from: "no-reply@mycompany.com" })
      );
    });

    it("includes html when provided", async () => {
      await sendEmail({
        to: "user@example.com",
        subject: "Hello",
        text: "Body",
        html: "<p>Body</p>",
      });

      const sendMail = getSendMailMock();
      expect(sendMail).toHaveBeenCalledWith(
        expect.objectContaining({ html: "<p>Body</p>" })
      );
    });

    it("does not include html key when html is not provided", async () => {
      await sendEmail({
        to: "user@example.com",
        subject: "Hello",
        text: "Body",
      });

      const sendMail = getSendMailMock();
      const callArg = sendMail.mock.calls[0][0] as Record<string, unknown>;
      expect(callArg).not.toHaveProperty("html");
    });

    it("returns accepted: true when sendMail returns accepted addresses", async () => {
      const result = await sendEmail({
        to: "user@example.com",
        subject: "Hello",
        text: "Body",
      });

      expect(result.accepted).toBe(true);
      expect(result.to).toBe("user@example.com");
      expect(result.subject).toBe("Hello");
    });

    it("returns accepted: false when sendMail returns empty accepted array", async () => {
      createTransportMock.mockReturnValue({
        sendMail: vi.fn().mockResolvedValue({ accepted: [] }),
      });

      const result = await sendEmail({
        to: "user@example.com",
        subject: "Hello",
        text: "Body",
      });

      expect(result.accepted).toBe(false);
    });

    it("returns accepted: false when sendMail returns non-array accepted", async () => {
      createTransportMock.mockReturnValue({
        sendMail: vi.fn().mockResolvedValue({ accepted: null }),
      });

      const result = await sendEmail({
        to: "user@example.com",
        subject: "Hello",
        text: "Body",
      });

      expect(result.accepted).toBe(false);
    });
  });
});
