/** DeployIntent contract versions this dashboard/operator understands. Mirror of
 *  Go `deploy.APIVersionV1` (internal/api/deploy/types.go). The adapter sends the
 *  highest version it shares with this list. */
export const SUPPORTED_DEPLOY_INTENT_VERSIONS: readonly string[] = [
  "deploy.omnia.altairalabs.ai/v1",
];
