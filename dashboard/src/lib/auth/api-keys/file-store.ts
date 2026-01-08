/**
 * File-based API key store.
 *
 * Reads API keys from a JSON file mounted from a Kubernetes Secret.
 * This enables GitOps workflows where keys are managed via manifests.
 *
 * The store is read-only - keys must be provisioned via the Secret.
 * Use the provided helper script to generate keys and hashes.
 *
 * Expected file format (keys.json):
 * {
 *   "keys": [
 *     {
 *       "id": "unique-id",
 *       "userId": "user-id",
 *       "name": "Key Name",
 *       "keyPrefix": "omnia_sk_abc12345...",
 *       "keyHash": "$2a$10$...",
 *       "role": "editor",
 *       "expiresAt": "2024-12-31T23:59:59Z",
 *       "createdAt": "2024-01-01T00:00:00Z"
 *     }
 *   ]
 * }
 */

import { readFileSync, existsSync, watchFile, unwatchFile } from "node:fs";
import bcrypt from "bcryptjs";
import {
  API_KEY_PREFIX,
  toApiKeyInfo,
  type ApiKey,
  type ApiKeyInfo,
  type ApiKeyStore,
  type CreateApiKeyOptions,
  type NewApiKey,
} from "./types";

interface StoredKey {
  id: string;
  userId: string;
  name: string;
  keyPrefix: string;
  keyHash: string;
  role: string;
  expiresAt: string | null;
  createdAt: string;
  lastUsedAt?: string | null;
}

interface KeysFile {
  keys: StoredKey[];
}

/**
 * Parse a stored key into an ApiKey.
 */
function parseStoredKey(stored: StoredKey): ApiKey {
  return {
    id: stored.id,
    userId: stored.userId,
    name: stored.name,
    keyPrefix: stored.keyPrefix,
    keyHash: stored.keyHash,
    role: stored.role as ApiKey["role"],
    expiresAt: stored.expiresAt ? new Date(stored.expiresAt) : null,
    createdAt: new Date(stored.createdAt),
    lastUsedAt: stored.lastUsedAt ? new Date(stored.lastUsedAt) : null,
  };
}

/**
 * File-based API key store.
 *
 * Reads keys from a JSON file (typically mounted from a K8s Secret).
 * The store watches the file for changes and reloads automatically.
 */
export class FileApiKeyStore implements ApiKeyStore {
  private readonly filePath: string;
  private readonly keys: Map<string, ApiKey> = new Map();
  private lastModified: number = 0;
  private readonly watchEnabled: boolean;

  constructor(filePath: string, options: { watch?: boolean } = {}) {
    this.filePath = filePath;
    this.watchEnabled = options.watch ?? true;
    this.loadKeys();

    if (this.watchEnabled) {
      this.startWatching();
    }
  }

  /**
   * Load keys from the file.
   */
  private loadKeys(): void {
    if (!existsSync(this.filePath)) {
      console.warn(`API keys file not found: ${this.filePath}`);
      this.keys.clear();
      return;
    }

    try {
      const content = readFileSync(this.filePath, "utf-8");
      const data: KeysFile = JSON.parse(content);

      this.keys.clear();
      for (const stored of data.keys || []) {
        const key = parseStoredKey(stored);
        this.keys.set(key.id, key);
      }

      console.warn(`Loaded ${this.keys.size} API keys from ${this.filePath}`);
    } catch (error) {
      console.error(`Failed to load API keys from ${this.filePath}:`, error);
    }
  }

  /**
   * Start watching the file for changes.
   */
  private startWatching(): void {
    watchFile(this.filePath, { interval: 5000 }, (curr, prev) => {
      if (curr.mtimeMs !== prev.mtimeMs && curr.mtimeMs !== this.lastModified) {
        this.lastModified = curr.mtimeMs;
        console.warn("API keys file changed, reloading...");
        this.loadKeys();
      }
    });
  }

  /**
   * Stop watching the file.
   */
  stopWatching(): void {
    unwatchFile(this.filePath);
  }

  /**
   * Create is not supported - keys must be provisioned via the Secret.
   */
  async create(
    _userId: string,
    _options: CreateApiKeyOptions
  ): Promise<NewApiKey> {
    throw new Error(
      "API key creation is not supported in file-based mode. " +
        "Keys must be provisioned via Kubernetes Secret. " +
        "See docs/api-keys.md for instructions."
    );
  }

  async listByUser(userId: string): Promise<ApiKeyInfo[]> {
    const userKeys: ApiKeyInfo[] = [];

    for (const key of this.keys.values()) {
      if (key.userId === userId) {
        userKeys.push(toApiKeyInfo(key));
      }
    }

    // Sort by creation date, newest first
    userKeys.sort((a, b) => b.createdAt.getTime() - a.createdAt.getTime());

    return userKeys;
  }

  async findByKey(key: string): Promise<ApiKey | null> {
    // Must start with the correct prefix
    if (!key.startsWith(API_KEY_PREFIX)) {
      return null;
    }

    // Check each stored key
    for (const storedKey of this.keys.values()) {
      // Skip expired keys
      if (storedKey.expiresAt && storedKey.expiresAt < new Date()) {
        continue;
      }

      // Compare with bcrypt
      const matches = await bcrypt.compare(key, storedKey.keyHash);
      if (matches) {
        return storedKey;
      }
    }

    return null;
  }

  /**
   * Delete is not supported - keys must be removed via the Secret.
   */
  async delete(_keyId: string, _userId: string): Promise<boolean> {
    throw new Error(
      "API key deletion is not supported in file-based mode. " +
        "Keys must be removed via Kubernetes Secret."
    );
  }

  /**
   * Update last used is a no-op in file mode (file is read-only).
   */
  async updateLastUsed(_keyId: string): Promise<void> {
    // No-op - we can't write to the mounted secret
    // Usage tracking would require a separate writable store
  }

  /**
   * Delete expired is a no-op in file mode.
   */
  async deleteExpired(): Promise<number> {
    // No-op - we can't modify the mounted secret
    return 0;
  }

  /**
   * Force reload keys from file.
   */
  reload(): void {
    this.loadKeys();
  }

  /**
   * Get the count of loaded keys.
   */
  get size(): number {
    return this.keys.size;
  }
}

// Singleton instance
let store: FileApiKeyStore | null = null;

/**
 * Get the singleton file store instance.
 */
export function getFileApiKeyStore(filePath: string): FileApiKeyStore {
  if (!store || store["filePath"] !== filePath) {
    store?.stopWatching();
    store = new FileApiKeyStore(filePath);
  }
  return store;
}
