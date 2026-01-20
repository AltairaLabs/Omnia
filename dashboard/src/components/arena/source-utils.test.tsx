/**
 * Tests for shared Arena source utilities.
 */

import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import {
  formatDate,
  formatInterval,
  formatBytes,
  getSourceTypeIcon,
  getSourceTypeBadge,
  getStatusBadge,
  getSourceUrl,
  getConditionIcon,
} from "./source-utils";
import type { ArenaSource } from "@/types/arena";

describe("formatDate", () => {
  it("returns dash for undefined input", () => {
    expect(formatDate(undefined)).toBe("-");
  });

  it("formats date without year by default", () => {
    const result = formatDate("2026-01-20T10:30:00Z");
    expect(result).toContain("Jan");
    expect(result).toContain("20");
    expect(result).not.toContain("2026");
  });

  it("formats date with year when includeYear is true", () => {
    const result = formatDate("2026-01-20T10:30:00Z", true);
    expect(result).toContain("Jan");
    expect(result).toContain("20");
    expect(result).toContain("2026");
  });
});

describe("formatInterval", () => {
  it("returns dash for undefined input", () => {
    expect(formatInterval(undefined)).toBe("-");
  });

  it("returns original string for non-matching format", () => {
    expect(formatInterval("invalid")).toBe("invalid");
  });

  it("formats seconds correctly", () => {
    expect(formatInterval("30s")).toBe("30 secs");
    expect(formatInterval("1s")).toBe("1 sec");
  });

  it("formats minutes correctly", () => {
    expect(formatInterval("5m")).toBe("5 mins");
    expect(formatInterval("1m")).toBe("1 min");
  });

  it("formats hours correctly", () => {
    expect(formatInterval("2h")).toBe("2 hours");
    expect(formatInterval("1h")).toBe("1 hour");
  });

  it("formats days correctly", () => {
    expect(formatInterval("7d")).toBe("7 days");
    expect(formatInterval("1d")).toBe("1 day");
  });
});

describe("formatBytes", () => {
  it("returns dash for undefined input", () => {
    expect(formatBytes(undefined)).toBe("-");
  });

  it("returns dash for null input", () => {
    expect(formatBytes(null as unknown as number)).toBe("-");
  });

  it("formats bytes", () => {
    expect(formatBytes(500)).toBe("500 B");
  });

  it("formats kilobytes", () => {
    expect(formatBytes(2048)).toBe("2.0 KB");
  });

  it("formats megabytes", () => {
    expect(formatBytes(1048576)).toBe("1.0 MB");
    expect(formatBytes(2621440)).toBe("2.5 MB");
  });
});

describe("getSourceTypeIcon", () => {
  it("returns git icon for git type", () => {
    const { container } = render(<>{getSourceTypeIcon("git")}</>);
    expect(container.querySelector("svg")).toBeInTheDocument();
  });

  it("returns oci icon for oci type", () => {
    const { container } = render(<>{getSourceTypeIcon("oci")}</>);
    expect(container.querySelector("svg")).toBeInTheDocument();
  });

  it("returns s3 icon for s3 type", () => {
    const { container } = render(<>{getSourceTypeIcon("s3")}</>);
    expect(container.querySelector("svg")).toBeInTheDocument();
  });

  it("returns configmap icon for configmap type", () => {
    const { container } = render(<>{getSourceTypeIcon("configmap")}</>);
    expect(container.querySelector("svg")).toBeInTheDocument();
  });

  it("returns default icon for undefined type", () => {
    const { container } = render(<>{getSourceTypeIcon(undefined)}</>);
    expect(container.querySelector("svg")).toBeInTheDocument();
  });

  it("returns larger icon when size is md", () => {
    const { container } = render(<>{getSourceTypeIcon("git", "md")}</>);
    const svg = container.querySelector("svg");
    expect(svg).toHaveClass("h-5", "w-5");
  });

  it("returns smaller icon when size is sm", () => {
    const { container } = render(<>{getSourceTypeIcon("git", "sm")}</>);
    const svg = container.querySelector("svg");
    expect(svg).toHaveClass("h-4", "w-4");
  });
});

describe("getSourceTypeBadge", () => {
  it("renders git badge with correct styling", () => {
    const { container } = render(<>{getSourceTypeBadge("git")}</>);
    expect(container.textContent).toContain("git");
  });

  it("renders oci badge with correct styling", () => {
    const { container } = render(<>{getSourceTypeBadge("oci")}</>);
    expect(container.textContent).toContain("oci");
  });

  it("renders s3 badge with correct styling", () => {
    const { container } = render(<>{getSourceTypeBadge("s3")}</>);
    expect(container.textContent).toContain("s3");
  });

  it("renders configmap badge with correct styling", () => {
    const { container } = render(<>{getSourceTypeBadge("configmap")}</>);
    expect(container.textContent).toContain("configmap");
  });

  it("renders unknown badge for undefined type", () => {
    const { container } = render(<>{getSourceTypeBadge(undefined)}</>);
    expect(container.textContent).toContain("Unknown");
  });
});

describe("getStatusBadge", () => {
  it("renders Ready badge with green styling", () => {
    const { container } = render(<>{getStatusBadge("Ready")}</>);
    expect(container.textContent).toContain("Ready");
  });

  it("renders Failed badge with destructive styling", () => {
    const { container } = render(<>{getStatusBadge("Failed")}</>);
    expect(container.textContent).toContain("Failed");
  });

  it("renders Pending badge with outline styling", () => {
    const { container } = render(<>{getStatusBadge("Pending")}</>);
    expect(container.textContent).toContain("Pending");
  });

  it("renders unknown badge for undefined phase", () => {
    const { container } = render(<>{getStatusBadge(undefined)}</>);
    expect(container.textContent).toContain("Unknown");
  });

  it("renders custom phase in badge", () => {
    const { container } = render(<>{getStatusBadge("Reconciling")}</>);
    expect(container.textContent).toContain("Reconciling");
  });
});

describe("getSourceUrl", () => {
  it("returns git URL for git source", () => {
    const source = {
      spec: { type: "git", git: { url: "https://github.com/org/repo.git" } },
    } as ArenaSource;
    expect(getSourceUrl(source)).toBe("https://github.com/org/repo.git");
  });

  it("returns OCI URL for oci source", () => {
    const source = {
      spec: { type: "oci", oci: { url: "oci://ghcr.io/org/pkg" } },
    } as ArenaSource;
    expect(getSourceUrl(source)).toBe("oci://ghcr.io/org/pkg");
  });

  it("returns S3 bucket URL for s3 source", () => {
    const source = {
      spec: { type: "s3", s3: { bucket: "my-bucket" } },
    } as ArenaSource;
    expect(getSourceUrl(source)).toBe("s3://my-bucket");
  });

  it("returns S3 bucket URL with prefix for s3 source", () => {
    const source = {
      spec: { type: "s3", s3: { bucket: "my-bucket", prefix: "prompts/" } },
    } as ArenaSource;
    expect(getSourceUrl(source)).toBe("s3://my-bucket/prompts/");
  });

  it("returns configmap name for configmap source", () => {
    const source = {
      spec: { type: "configmap", configMapRef: { name: "my-config" } },
    } as ArenaSource;
    expect(getSourceUrl(source)).toBe("my-config");
  });

  it("returns dash for unknown source type", () => {
    const source = {
      spec: { type: "unknown" },
    } as unknown as ArenaSource;
    expect(getSourceUrl(source)).toBe("-");
  });

  it("returns dash for git source without git config", () => {
    const source = {
      spec: { type: "git" },
    } as ArenaSource;
    expect(getSourceUrl(source)).toBe("-");
  });
});

describe("getConditionIcon", () => {
  it("returns green check for True status", () => {
    const { container } = render(<>{getConditionIcon("True")}</>);
    const svg = container.querySelector("svg");
    expect(svg).toHaveClass("text-green-500");
  });

  it("returns red alert for False status", () => {
    const { container } = render(<>{getConditionIcon("False")}</>);
    const svg = container.querySelector("svg");
    expect(svg).toHaveClass("text-red-500");
  });

  it("returns yellow clock for Unknown status", () => {
    const { container } = render(<>{getConditionIcon("Unknown")}</>);
    const svg = container.querySelector("svg");
    expect(svg).toHaveClass("text-yellow-500");
  });
});
