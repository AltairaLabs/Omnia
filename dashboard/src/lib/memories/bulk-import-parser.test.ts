import { describe, it, expect } from "vitest";
import { parseJsonBulk, parseMarkdownBulk } from "./bulk-import-parser";

describe("parseJsonBulk", () => {
  it("parses a well-formed array", () => {
    const res = parseJsonBulk(`[
      {"type":"policy","content":"snake_case","confidence":1.0},
      {"type":"glossary","content":"API terms","metadata":{"source":"doc"}}
    ]`);
    expect(res.errors).toHaveLength(0);
    expect(res.memories).toHaveLength(2);
    expect(res.memories[0]).toMatchObject({ type: "policy", content: "snake_case", confidence: 1.0 });
    expect(res.memories[1].metadata).toEqual({ source: "doc" });
  });

  it("returns an error on invalid JSON", () => {
    const res = parseJsonBulk("not json");
    expect(res.memories).toHaveLength(0);
    expect(res.errors).toHaveLength(1);
    expect(res.errors[0].format).toBe("json");
  });

  it("rejects non-array top-level", () => {
    const res = parseJsonBulk(`{"type":"x","content":"y"}`);
    expect(res.memories).toHaveLength(0);
    expect(res.errors[0].message).toMatch(/array/i);
  });

  it("reports per-entry errors", () => {
    const res = parseJsonBulk(`[
      {"type":"ok","content":"ok"},
      null,
      {"type":"missing-content"},
      {"content":"missing-type"},
      "string entry"
    ]`);
    expect(res.memories).toHaveLength(1);
    expect(res.errors).toHaveLength(4);
  });

  it("drops non-object metadata silently", () => {
    const res = parseJsonBulk(`[{"type":"t","content":"c","metadata":"not-an-object"}]`);
    expect(res.memories).toHaveLength(1);
    expect(res.memories[0].metadata).toBeUndefined();
  });
});

describe("parseMarkdownBulk", () => {
  it("splits on ## headers", () => {
    const md = `# Title
Intro paragraph (ignored).

## API Style
Use snake_case for all new endpoints.

## Runbook: On-call
Check the dashboard at example.com first.`;
    const res = parseMarkdownBulk(md);
    expect(res.errors).toHaveLength(0);
    expect(res.memories).toHaveLength(2);
    expect(res.memories[0]).toMatchObject({
      type: "api-style",
      content: expect.stringContaining("snake_case"),
    });
    expect(res.memories[0].metadata).toEqual({ source: "markdown", heading: "API Style" });
    expect(res.memories[1].type).toBe("runbook-on-call");
  });

  it("errors when no ## headers", () => {
    const res = parseMarkdownBulk("Just plain text.\nNo headers here.");
    expect(res.memories).toHaveLength(0);
    expect(res.errors).toHaveLength(1);
    expect(res.errors[0].format).toBe("markdown");
  });

  it("drops empty sections silently", () => {
    const md = `## Empty

## Real
Has content.`;
    const res = parseMarkdownBulk(md);
    expect(res.memories).toHaveLength(1);
    expect(res.memories[0].type).toBe("real");
  });

  it("slugifies unusual heading characters", () => {
    const res = parseMarkdownBulk(`## Hello, world!! (test)\nbody`);
    expect(res.memories[0].type).toBe("hello-world-test");
  });

  it("handles CRLF line endings", () => {
    const res = parseMarkdownBulk("## A\r\nbody A\r\n\r\n## B\r\nbody B");
    expect(res.memories).toHaveLength(2);
    expect(res.memories[0].content).toBe("body A");
  });

  it("falls back to untitled when heading slugifies to empty", () => {
    const res = parseMarkdownBulk(`## !!!\nbody`);
    expect(res.memories[0].type).toBe("untitled");
  });
});
