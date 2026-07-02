import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useFlowColorMode } from "./use-color-mode";

const mockUseTheme = vi.fn();
vi.mock("next-themes", () => ({ useTheme: () => mockUseTheme() }));

describe("useFlowColorMode", () => {
  it("returns dark when the resolved theme is dark", () => {
    mockUseTheme.mockReturnValue({ resolvedTheme: "dark" });
    const { result } = renderHook(() => useFlowColorMode());
    expect(result.current).toBe("dark");
  });

  it("returns light for the light theme", () => {
    mockUseTheme.mockReturnValue({ resolvedTheme: "light" });
    const { result } = renderHook(() => useFlowColorMode());
    expect(result.current).toBe("light");
  });

  it("defaults to light when the theme is unresolved", () => {
    mockUseTheme.mockReturnValue({ resolvedTheme: undefined });
    const { result } = renderHook(() => useFlowColorMode());
    expect(result.current).toBe("light");
  });
});
