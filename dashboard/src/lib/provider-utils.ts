/**
 * Shared provider utilities for consistent display across cost components.
 */

// Provider brand colors
export const PROVIDER_COLORS: Record<string, string> = {
  anthropic: "#D97757", // Coral/orange (Claude brand)
  openai: "#10A37F",    // Green (OpenAI brand)
  gemini: "#4285F4",    // Blue (Google brand)
  ollama: "#1A1A1A",    // Dark gray
  mock: "#6B7280",      // Muted gray (demo data)
};

// Fallback colors for unknown providers
const FALLBACK_COLORS = ["#3B82F6", "#8B5CF6", "#EC4899", "#F59E0B", "#06B6D4"];

/**
 * Get color for a provider, with fallback for unknown providers.
 */
export function getProviderColor(provider: string, index = 0): string {
  const normalizedProvider = provider.toLowerCase();
  if (PROVIDER_COLORS[normalizedProvider]) {
    return PROVIDER_COLORS[normalizedProvider];
  }
  return FALLBACK_COLORS[index % FALLBACK_COLORS.length];
}

/**
 * Get display name for a provider.
 */
export function getProviderDisplayName(provider: string): string {
  const displayNames: Record<string, string> = {
    anthropic: "Anthropic",
    openai: "OpenAI",
    gemini: "Gemini",
    ollama: "Ollama",
    mock: "Mock",
    claude: "Anthropic", // Alias
  };
  const normalizedProvider = provider.toLowerCase();
  return displayNames[normalizedProvider] || provider.charAt(0).toUpperCase() + provider.slice(1);
}
