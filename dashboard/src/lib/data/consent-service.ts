/**
 * Consent API service for managing user privacy consent grants.
 *
 * Calls the workspace-scoped consent proxy routes:
 *   GET  /api/workspaces/{name}/privacy/consent?userId=X
 *   PUT  /api/workspaces/{name}/privacy/consent?userId=X
 */

import type { ConsentResponse, ConsentRequest } from "./types";

const CONSENT_API_BASE = "/api/workspaces";

export class ConsentService {
  readonly name = "ConsentService";

  async getConsent(workspace: string, userId: string): Promise<ConsentResponse> {
    const params = new URLSearchParams({ userId });
    const response = await fetch(
      `${CONSENT_API_BASE}/${encodeURIComponent(workspace)}/privacy/consent?${params.toString()}`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return { grants: [], defaults: [], denied: [] };
      }
      throw new Error(`Failed to fetch consent: ${response.statusText}`);
    }
    return response.json();
  }

  async updateConsent(
    workspace: string,
    userId: string,
    request: ConsentRequest
  ): Promise<ConsentResponse> {
    const params = new URLSearchParams({ userId });
    const response = await fetch(
      `${CONSENT_API_BASE}/${encodeURIComponent(workspace)}/privacy/consent?${params.toString()}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
      }
    );
    if (!response.ok) {
      throw new Error(`Failed to update consent: ${response.statusText}`);
    }
    return response.json();
  }
}
