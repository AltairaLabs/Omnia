/**
 * Email sender abstraction.
 *
 * When OMNIA_SMTP_HOST is set, sends via SMTP using nodemailer.
 * When not set, logs a warning with the email content (development fallback).
 */

import nodemailer from "nodemailer";

export interface EmailMessage {
  to: string;
  subject: string;
  text: string;
  html?: string;
}

export interface EmailResult {
  accepted: boolean;
  to: string;
  subject: string;
}

export async function sendEmail(message: EmailMessage): Promise<EmailResult> {
  const smtpHost = process.env.OMNIA_SMTP_HOST;

  if (!smtpHost) {
    console.warn(
      `[email] OMNIA_SMTP_HOST not configured — email not sent. to=${message.to} subject="${message.subject}"`
    );
    return { accepted: false, to: message.to, subject: message.subject };
  }

  const port = Number.parseInt(process.env.OMNIA_SMTP_PORT ?? "587", 10);
  const secure = process.env.OMNIA_SMTP_SECURE === "true";
  const from = process.env.OMNIA_SMTP_FROM ?? "noreply@omnia.local";

  const authUser = process.env.OMNIA_SMTP_USER;
  const authPass = process.env.OMNIA_SMTP_PASS;

  const transport = nodemailer.createTransport({
    host: smtpHost,
    port,
    secure,
    ...(authUser && authPass ? { auth: { user: authUser, pass: authPass } } : {}),
  });

  const info = await transport.sendMail({
    from,
    to: message.to,
    subject: message.subject,
    text: message.text,
    ...(message.html ? { html: message.html } : {}),
  });

  const accepted = Array.isArray(info.accepted) && info.accepted.length > 0;

  return { accepted, to: message.to, subject: message.subject };
}
