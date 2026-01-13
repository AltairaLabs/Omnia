/**
 * Provider icons for topology view.
 *
 * Simple colored circles with letter identifiers for each provider type.
 * Uses brand-appropriate colors while avoiding trademark issues.
 */

import type { ProviderType } from "@/types";

interface ProviderIconProps {
  type: ProviderType;
  size?: number;
  className?: string;
}

/**
 * Provider brand colors and letters.
 */
const providerConfig: Record<ProviderType, { color: string; letter: string; label: string }> = {
  claude: { color: "#D97757", letter: "C", label: "Claude" },
  openai: { color: "#10A37F", letter: "O", label: "OpenAI" },
  gemini: { color: "#4285F4", letter: "G", label: "Gemini" },
  ollama: { color: "#1A1A1A", letter: "L", label: "Ollama" },
  mock: { color: "#6B7280", letter: "M", label: "Mock" },
};

/**
 * Provider icon component.
 * Renders a colored circle with the provider's initial letter.
 */
export function ProviderIcon({ type, size = 24, className = "" }: Readonly<ProviderIconProps>) {
  const config = providerConfig[type] || providerConfig.mock;

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      className={className}
      aria-label={config.label}
    >
      <circle cx="12" cy="12" r="11" fill={config.color} />
      <text
        x="12"
        y="12"
        textAnchor="middle"
        dominantBaseline="central"
        fill="white"
        fontSize="12"
        fontWeight="600"
        fontFamily="system-ui, sans-serif"
      >
        {config.letter}
      </text>
    </svg>
  );
}

/**
 * Get provider color for use in edges and other styling.
 */
export function getProviderColor(type: ProviderType): string {
  return providerConfig[type]?.color || providerConfig.mock.color;
}

/**
 * Get provider label for display.
 */
export function getProviderLabel(type: ProviderType): string {
  return providerConfig[type]?.label || "Unknown";
}
