/**
 * Tests for email sender.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { sendEmail } from "./sender";

describe("sendEmail", () => {
  const originalSmtpHost = process.env.OMNIA_SMTP_HOST;

  beforeEach(() => {
    delete process.env.OMNIA_SMTP_HOST;
  });

  afterEach(() => {
    if (originalSmtpHost === undefined) {
      delete process.env.OMNIA_SMTP_HOST;
    } else {
      process.env.OMNIA_SMTP_HOST = originalSmtpHost;
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
});
