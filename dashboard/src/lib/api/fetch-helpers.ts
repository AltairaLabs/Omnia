/**
 * Shared fetch helper utilities for API hooks.
 *
 * Extracts common patterns used across multiple React Query hooks
 * to reduce code duplication.
 */

/**
 * Validate a mutation response and throw with the server error message on failure.
 */
export async function handleMutationResponse(response: Response, fallbackMsg: string): Promise<Response> {
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(errorText || fallbackMsg);
  }
  return response;
}
