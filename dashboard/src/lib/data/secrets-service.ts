/**
 * Frontend service for managing provider credentials (secrets).
 *
 * Calls the dashboard API endpoints for secret management.
 * Security: API never returns secret values, only metadata.
 */

/**
 * Reference to a provider that uses a secret.
 */
export interface ProviderRef {
  namespace: string;
  name: string;
  type: string;
}

/**
 * Secret summary - metadata only, no values.
 */
export interface SecretSummary {
  namespace: string;
  name: string;
  keys: string[];
  annotations?: Record<string, string>;
  referencedBy: ProviderRef[];
  createdAt: string;
  modifiedAt: string;
}

/**
 * Request to create or update a secret.
 */
export interface SecretWriteRequest {
  namespace: string;
  name: string;
  data: Record<string, string>;
  providerType?: string;
}

/**
 * Service for managing provider credentials (K8s secrets).
 */
export class SecretsService {
  private readonly baseUrl = "/api/secrets";

  /**
   * List all credential secrets, optionally filtered by namespace.
   */
  async listSecrets(namespace?: string): Promise<SecretSummary[]> {
    const url = namespace
      ? `${this.baseUrl}?namespace=${encodeURIComponent(namespace)}`
      : this.baseUrl;

    const response = await fetch(url);

    if (!response.ok) {
      const error = await this.parseError(response);
      throw new Error(error);
    }

    const data = await response.json();
    return data.secrets as SecretSummary[];
  }

  /**
   * Get a single secret's metadata.
   */
  async getSecret(namespace: string, name: string): Promise<SecretSummary | null> {
    const url = `${this.baseUrl}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`;
    const response = await fetch(url);

    if (response.status === 404) {
      return null;
    }

    if (!response.ok) {
      const error = await this.parseError(response);
      throw new Error(error);
    }

    const data = await response.json();
    return data.secret as SecretSummary;
  }

  /**
   * Create or update a secret.
   */
  async createOrUpdateSecret(request: SecretWriteRequest): Promise<SecretSummary> {
    const response = await fetch(this.baseUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      const error = await this.parseError(response);
      throw new Error(error);
    }

    const data = await response.json();
    return data.secret as SecretSummary;
  }

  /**
   * Delete a secret.
   */
  async deleteSecret(namespace: string, name: string): Promise<boolean> {
    const url = `${this.baseUrl}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`;
    const response = await fetch(url, {
      method: "DELETE",
    });

    if (response.status === 404) {
      return false;
    }

    if (!response.ok) {
      const error = await this.parseError(response);
      throw new Error(error);
    }

    return true;
  }

  private async parseError(response: Response): Promise<string> {
    try {
      const data = await response.json();
      return data.error || `Request failed with status ${response.status}`;
    } catch {
      return `Request failed with status ${response.status}`;
    }
  }
}

// Singleton instance
let secretsService: SecretsService | null = null;

export function getSecretsService(): SecretsService {
  if (!secretsService) {
    secretsService = new SecretsService();
  }
  return secretsService;
}
