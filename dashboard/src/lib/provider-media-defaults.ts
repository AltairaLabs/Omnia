/**
 * Provider-specific media requirements defaults.
 *
 * These are based on documented provider capabilities and limits.
 * Values can be overridden via ConsoleConfig.mediaRequirements in the CRD.
 */

import type { ProviderType, MediaRequirements, VideoProcessingMode, CompressionGuidance } from "@/types/agent-runtime";

// Constants to avoid duplication
const VIDEO_MODE_FRAMES: VideoProcessingMode = "frames";
const VIDEO_MODE_NATIVE: VideoProcessingMode = "native";
const COMPRESSION_LOSSY_MEDIUM: CompressionGuidance = "lossy-medium";

/**
 * Provider-specific media requirements defaults.
 * These values are based on each provider's documented capabilities and limits.
 */
export const PROVIDER_MEDIA_DEFAULTS: Record<
  ProviderType,
  MediaRequirements
> = {
  claude: {
    image: {
      maxSizeBytes: 5 * 1024 * 1024, // 5MB per image
      maxDimensions: { width: 8000, height: 8000 },
      recommendedDimensions: { width: 1568, height: 1568 },
      supportedFormats: ["png", "jpeg", "gif", "webp"],
      preferredFormat: "png",
      compressionGuidance: "lossless",
    },
    video: {
      // Claude doesn't natively support video - requires frame extraction
      maxDurationSeconds: 60,
      supportsSegmentSelection: false,
      processingMode: VIDEO_MODE_FRAMES,
      frameExtractionInterval: 1,
    },
    audio: {
      // Claude doesn't natively support audio
      maxDurationSeconds: undefined,
      supportsSegmentSelection: false,
    },
    document: {
      maxPages: 100,
      supportsOCR: true, // Via vision
    },
  },
  openai: {
    image: {
      maxSizeBytes: 20 * 1024 * 1024, // 20MB
      maxDimensions: { width: 2048, height: 2048 },
      recommendedDimensions: { width: 1024, height: 1024 },
      supportedFormats: ["png", "jpeg", "gif", "webp"],
      preferredFormat: "png",
      compressionGuidance: "lossy-high",
    },
    video: {
      // GPT-4V doesn't support video natively
      maxDurationSeconds: 30,
      supportsSegmentSelection: false,
      processingMode: VIDEO_MODE_FRAMES,
      frameExtractionInterval: 2,
    },
    audio: {
      maxDurationSeconds: 300, // Whisper limit
      recommendedSampleRate: 16000,
      supportsSegmentSelection: true,
    },
    document: {
      maxPages: 50,
      supportsOCR: true,
    },
  },
  gemini: {
    image: {
      maxSizeBytes: 20 * 1024 * 1024,
      maxDimensions: { width: 3072, height: 3072 },
      recommendedDimensions: { width: 1024, height: 1024 },
      supportedFormats: ["png", "jpeg", "gif", "webp"],
      preferredFormat: "jpeg",
      compressionGuidance: COMPRESSION_LOSSY_MEDIUM,
    },
    video: {
      maxDurationSeconds: 7200, // 2 hours for Gemini 1.5
      supportsSegmentSelection: true,
      processingMode: VIDEO_MODE_NATIVE,
    },
    audio: {
      maxDurationSeconds: 34200, // 9.5 hours
      recommendedSampleRate: 16000,
      supportsSegmentSelection: true,
    },
    document: {
      maxPages: 3600,
      supportsOCR: true,
    },
  },
  ollama: {
    // Ollama capabilities depend on the model
    // Using conservative defaults
    image: {
      maxSizeBytes: 10 * 1024 * 1024,
      supportedFormats: ["png", "jpeg"],
      compressionGuidance: COMPRESSION_LOSSY_MEDIUM,
    },
    video: undefined,
    audio: undefined,
    document: undefined,
  },
  mock: {
    // Mock provider for testing - permissive defaults
    image: {
      maxSizeBytes: 100 * 1024 * 1024,
      supportedFormats: ["png", "jpeg", "gif", "webp"],
    },
    video: {
      maxDurationSeconds: 3600,
      processingMode: VIDEO_MODE_NATIVE,
    },
    audio: {
      maxDurationSeconds: 3600,
    },
    document: {
      maxPages: 1000,
      supportsOCR: true,
    },
  },
};

/**
 * Conservative defaults used when provider type is unknown.
 * These values are safe across all providers.
 */
export const CONSERVATIVE_MEDIA_DEFAULTS: MediaRequirements = {
  image: {
    maxSizeBytes: 5 * 1024 * 1024,
    maxDimensions: { width: 2048, height: 2048 },
    supportedFormats: ["png", "jpeg"],
    compressionGuidance: COMPRESSION_LOSSY_MEDIUM,
  },
  video: {
    maxDurationSeconds: 30,
    processingMode: VIDEO_MODE_FRAMES,
    frameExtractionInterval: 2,
  },
  audio: {
    maxDurationSeconds: 60,
  },
  document: {
    maxPages: 20,
  },
};

/**
 * Deep merge two MediaRequirements objects.
 * Values from `overrides` take precedence over `defaults`.
 */
function mergeMediaRequirements(
  defaults: MediaRequirements,
  overrides: MediaRequirements
): MediaRequirements {
  return {
    image: overrides.image ?? defaults.image,
    video: overrides.video ?? defaults.video,
    audio: overrides.audio ?? defaults.audio,
    document: overrides.document ?? defaults.document,
  };
}

/**
 * Get media requirements for a provider, with explicit overrides taking precedence.
 *
 * @param providerType - The provider type (e.g., "claude", "openai", "gemini")
 * @param overrides - Optional explicit overrides from the CRD
 * @returns Resolved media requirements
 */
export function getMediaRequirements(
  providerType: ProviderType | undefined,
  overrides?: MediaRequirements
): MediaRequirements {
  // Get provider-specific defaults or conservative defaults
  const defaults = providerType
    ? PROVIDER_MEDIA_DEFAULTS[providerType]
    : CONSERVATIVE_MEDIA_DEFAULTS;

  // If no overrides, return defaults
  if (!overrides) {
    return defaults;
  }

  // Merge overrides with defaults
  return mergeMediaRequirements(defaults, overrides);
}
