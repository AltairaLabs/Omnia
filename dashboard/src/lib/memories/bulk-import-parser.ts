/**
 * Parsers for bulk-importing institutional memories.
 *
 * Supports two formats:
 *   - JSON array of `{ type, content, confidence?, metadata? }` objects.
 *   - Markdown with `##` section headers (each section becomes one memory;
 *     header text is the type, body is the content).
 */

export interface ParsedMemory {
  type: string;
  content: string;
  confidence?: number;
  metadata?: Record<string, unknown>;
  /**
   * Optional ISO-8601 UTC timestamp. Forwarded verbatim to the institutional
   * POST body; the memory-api rejects past timestamps. Only emitted from the
   * JSON parser — Markdown import has no syntax for it.
   */
  expiresAt?: string;
}

export interface ParseError {
  format: "json" | "markdown";
  message: string;
}

export interface ParseResult {
  memories: ParsedMemory[];
  errors: ParseError[];
}

/**
 * Parse a JSON array body. Each entry must have `type` and `content` strings.
 * Unknown extra fields are forwarded to metadata untouched.
 */
export function parseJsonBulk(input: string): ParseResult {
  let raw: unknown;
  try {
    raw = JSON.parse(input);
  } catch (e) {
    return {
      memories: [],
      errors: [{ format: "json", message: e instanceof Error ? e.message : "Invalid JSON" }],
    };
  }
  if (!Array.isArray(raw)) {
    return {
      memories: [],
      errors: [{ format: "json", message: "Expected a JSON array at the top level" }],
    };
  }

  const memories: ParsedMemory[] = [];
  const errors: ParseError[] = [];
  raw.forEach((entry, i) => {
    if (entry === null || typeof entry !== "object") {
      errors.push({ format: "json", message: `Entry ${i}: not an object` });
      return;
    }
    const rec = entry as Record<string, unknown>;
    const type = rec.type;
    const content = rec.content;
    if (typeof type !== "string" || !type) {
      errors.push({ format: "json", message: `Entry ${i}: missing "type"` });
      return;
    }
    if (typeof content !== "string" || !content) {
      errors.push({ format: "json", message: `Entry ${i}: missing "content"` });
      return;
    }
    const out: ParsedMemory = { type, content };
    if (typeof rec.confidence === "number") {
      out.confidence = rec.confidence;
    }
    if (rec.metadata && typeof rec.metadata === "object" && !Array.isArray(rec.metadata)) {
      out.metadata = rec.metadata as Record<string, unknown>;
    }
    // Accept camelCase and snake_case; the API expects snake_case.
    const rawExpiresAt = rec.expiresAt ?? rec.expires_at;
    if (typeof rawExpiresAt === "string" && rawExpiresAt) {
      out.expiresAt = rawExpiresAt;
    }
    memories.push(out);
  });

  return { memories, errors };
}

/**
 * Parse markdown with `## Section` headers. Each top-level `##` section
 * becomes a memory; the header text becomes the memory `type` (slugified —
 * lowercased, hyphenated) and the body becomes `content`. Anything above
 * the first `##` is ignored (can be used for a file-level title/description).
 *
 * Empty sections are dropped silently; a file with no `##` headers produces
 * a single error describing the mis-formatted input.
 */
export function parseMarkdownBulk(input: string): ParseResult {
  const lines = input.split(/\r?\n/);
  const memories: ParsedMemory[] = [];
  const errors: ParseError[] = [];

  let currentHeader: string | null = null;
  let currentBody: string[] = [];

  const flush = () => {
    if (currentHeader === null) return;
    const content = currentBody.join("\n").trim();
    if (!content) {
      currentHeader = null;
      currentBody = [];
      return;
    }
    memories.push({
      type: slugify(currentHeader),
      content,
      metadata: { source: "markdown", heading: currentHeader },
    });
    currentHeader = null;
    currentBody = [];
  };

  const headerRe = /^##\s+(\S[^\n]*?)$/;
  for (const rawLine of lines) {
    const match = headerRe.exec(rawLine);
    if (match) {
      flush();
      currentHeader = match[1];
      continue;
    }
    if (currentHeader !== null) {
      currentBody.push(rawLine);
    }
  }
  flush();

  if (memories.length === 0 && errors.length === 0) {
    errors.push({
      format: "markdown",
      message: "No `## Section` headers found — the parser expects each memory to start with `##`.",
    });
  }

  return { memories, errors };
}

function slugify(raw: string): string {
  const compact = raw.toLowerCase().replaceAll(/[^a-z0-9]+/g, "-");
  let start = 0;
  while (start < compact.length && compact[start] === "-") start++;
  let end = compact.length;
  while (end > start && compact[end - 1] === "-") end--;
  const trimmed = compact.slice(start, end);
  return trimmed || "untitled";
}
