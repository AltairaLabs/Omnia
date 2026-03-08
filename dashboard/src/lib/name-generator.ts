/**
 * Docker-style random name generator for Kubernetes resources.
 * Produces lowercase hyphenated names like "swift-falcon" or "calm-otter".
 * Names are valid Kubernetes resource names (RFC 1123 label).
 */

const ADJECTIVES = [
  "agile", "bold", "brave", "bright", "calm", "clever", "cool", "crisp",
  "eager", "epic", "fair", "fast", "fierce", "firm", "fleet", "fresh",
  "glad", "grand", "great", "happy", "keen", "kind", "lively", "lucid",
  "merry", "mighty", "neat", "noble", "prime", "proud", "quick", "quiet",
  "rapid", "ready", "sharp", "sleek", "smart", "smooth", "solid", "steady",
  "still", "stoic", "stout", "strong", "super", "sure", "swift", "vivid",
  "warm", "wise",
];

const NOUNS = [
  "arrow", "atlas", "badge", "beam", "blaze", "bolt", "cedar", "cliff",
  "comet", "coral", "crane", "crest", "drift", "eagle", "ember", "falcon",
  "flame", "flare", "forge", "frost", "gale", "grove", "hawk", "heron",
  "iris", "lance", "lark", "lotus", "maple", "marsh", "nova", "opal",
  "orbit", "otter", "peak", "pine", "pulse", "quill", "reef", "ridge",
  "river", "robin", "rover", "sage", "shore", "spark", "spire", "steel",
  "stone", "storm", "surge", "thorn", "tide", "trail", "vale", "viper",
  "wave", "wren",
];

function pick<T>(arr: readonly T[]): T {
  const randomValues = new Uint32Array(1);
  crypto.getRandomValues(randomValues);
  return arr[randomValues[0] % arr.length];
}

/**
 * Generate a random name in the format "adjective-noun" (e.g., "swift-falcon").
 * Optionally accepts a prefix that will be prepended (e.g., "eval-swift-falcon").
 */
export function generateName(prefix?: string): string {
  const name = `${pick(ADJECTIVES)}-${pick(NOUNS)}`;
  return prefix ? `${prefix}-${name}` : name;
}
