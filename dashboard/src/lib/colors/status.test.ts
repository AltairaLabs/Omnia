import { describe, it, expect } from "vitest";
import { getStatusColorVar, getStatusClasses } from "./status";

describe("status colors", () => {
  it("maps each kind to a CSS variable", () => {
    expect(getStatusColorVar("success")).toBe("var(--success)");
    expect(getStatusColorVar("warning")).toBe("var(--warning)");
    expect(getStatusColorVar("info")).toBe("var(--info)");
    expect(getStatusColorVar("error")).toBe("var(--destructive)");
    expect(getStatusColorVar("neutral")).toBe("var(--muted-foreground)");
  });

  it("returns token utility classes, never raw palette classes", () => {
    const cls = getStatusClasses("success");
    expect(cls.text).toBe("text-success");
    expect(cls.bg).toBe("bg-success/15");
    expect(cls.border).toBe("border-success/30");
    // guard: no hardcoded palette like green-500 leaks through
    expect(JSON.stringify(cls)).not.toMatch(/-(green|red|amber|yellow)-\d/);
  });

  it("maps error kind to the destructive token classes", () => {
    const cls = getStatusClasses("error");
    expect(cls.text).toBe("text-destructive");
    expect(cls.bg).toBe("bg-destructive/15");
    expect(cls.border).toBe("border-destructive/30");
  });
});
