#!/usr/bin/env bash
#
# Register (or rotate credentials for) an Entra ID app for local Omnia
# dashboard OAuth development, configure group-based role mapping, and
# populate the K8s secret + workspace grants that the Helm chart expects.
#
# What it does:
#   1. Creates/reuses an Entra app registration, with the dashboard
#      callback URI, and rotates a client secret (--append, so prior
#      secrets keep working).
#   2. Enables the `groups` optional claim on the ID token so OIDC
#      sign-in carries the user's group memberships.
#   3. Creates/reuses an "Omnia Admins" security group and adds the
#      signed-in az user to it.
#   4. Writes charts/omnia/values-dev-entra.yaml pointing Helm at the
#      K8s Secret and wiring the group object ID into the admin role
#      mapping (gitignored; regenerate by rerunning).
#   5. With --create-secret: creates the dashboard-oauth Secret and
#      patches the dev-agents Workspace to grant the admin group
#      owner-level access.
#
# Usage:
#   scripts/setup-entra-dev.sh                       # Entra setup + values file only
#   scripts/setup-entra-dev.sh --create-secret       # also create Secret + patch Workspace
#   APP_NAME="Omnia Dev (alice)" scripts/setup-entra-dev.sh --create-secret
#
# Env overrides:
#   APP_NAME         display name for the app registration         (default: "Omnia Dashboard Dev")
#   REDIRECT_URI     redirect URI                                  (default: http://localhost:3000/api/auth/callback)
#   SECRET_YEARS     client secret lifetime in years               (default: 1, Azure max: 2)
#   NAMESPACE        K8s namespace for the Secret                  (default: omnia-system)
#   SECRET_NAME      K8s Secret name                               (default: dashboard-oauth)
#   GROUP_NAME       Entra security group for admins               (default: "Omnia Admins")
#   WORKSPACE_NAME   Workspace CRD to grant owner access to        (default: dev-agents)
#
# Requires: az (logged in via `az login`), jq, openssl;
#           kubectl only if --create-secret is passed.
#
# IMPORTANT: after rerunning, users must sign out of the dashboard and
# back in to refresh their ID token with the new groups claim.

set -euo pipefail

APP_NAME="${APP_NAME:-Omnia Dashboard Dev}"
REDIRECT_URI="${REDIRECT_URI:-http://localhost:3000/api/auth/callback}"
SECRET_YEARS="${SECRET_YEARS:-1}"
NAMESPACE="${NAMESPACE:-omnia-system}"
SECRET_NAME="${SECRET_NAME:-dashboard-oauth}"
GROUP_NAME="${GROUP_NAME:-Omnia Admins}"
WORKSPACE_NAME="${WORKSPACE_NAME:-dev-agents}"
CREATE_SECRET=false

for arg in "$@"; do
  case "$arg" in
    --create-secret) CREATE_SECRET=true ;;
    -h|--help)
      sed -n '2,38p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "unknown arg: $arg" >&2
      exit 2
      ;;
  esac
done

for cmd in az jq openssl; do
  command -v "$cmd" >/dev/null || { echo "required command not found: $cmd" >&2; exit 1; }
done
if $CREATE_SECRET; then
  command -v kubectl >/dev/null || { echo "kubectl required for --create-secret" >&2; exit 1; }
fi

az account show >/dev/null 2>&1 || { echo "run 'az login' first" >&2; exit 1; }

TENANT_ID=$(az account show --query tenantId -o tsv)

# ---------------------------------------------------------------------------
# App registration + redirect URI
# ---------------------------------------------------------------------------

APP_ID=$(az ad app list --display-name "$APP_NAME" --query '[0].appId' -o tsv)

if [[ -z "$APP_ID" ]]; then
  echo ">> creating app registration: $APP_NAME" >&2
  APP_ID=$(az ad app create \
    --display-name "$APP_NAME" \
    --sign-in-audience AzureADMyOrg \
    --web-redirect-uris "$REDIRECT_URI" \
    --query appId -o tsv)
else
  echo ">> reusing existing app: $APP_NAME (appId=$APP_ID)" >&2
  # Make sure the redirect URI is registered (idempotent append).
  UPDATED_URIS=$(az ad app show --id "$APP_ID" --query 'web.redirectUris' -o json \
    | jq --arg uri "$REDIRECT_URI" 'if index($uri) then . else . + [$uri] end')
  if [[ "$UPDATED_URIS" != "$(az ad app show --id "$APP_ID" --query 'web.redirectUris' -o json)" ]]; then
    # --web-redirect-uris takes a space-separated list; splat from jq.
    # shellcheck disable=SC2046
    az ad app update --id "$APP_ID" --web-redirect-uris $(jq -r '.[]' <<<"$UPDATED_URIS")
  fi
fi

APP_OID=$(az ad app show --id "$APP_ID" --query id -o tsv)

# ---------------------------------------------------------------------------
# Groups claim on the ID token
# ---------------------------------------------------------------------------
# Idempotent: only PATCH the application manifest if the groups claim is
# missing or groupMembershipClaims is not already set to include security
# groups. Uses the Graph API via `az rest` so we can round-trip the full
# manifest without az CLI's dot-path quoting quirks.

echo ">> ensuring groups optional claim on app manifest" >&2
CURRENT_MANIFEST=$(az rest --method GET \
  --url "https://graph.microsoft.com/v1.0/applications/$APP_OID" \
  --query '{optionalClaims: optionalClaims, groupMembershipClaims: groupMembershipClaims}' \
  -o json)

PATCHED_MANIFEST=$(jq '
  .optionalClaims = (.optionalClaims // {idToken: [], accessToken: [], saml2Token: []})
  | .optionalClaims.idToken = (.optionalClaims.idToken // [])
  | if (.optionalClaims.idToken | map(.name) | index("groups"))
    then .
    else .optionalClaims.idToken += [{"name":"groups","essential":false,"additionalProperties":[]}]
    end
  | if (.groupMembershipClaims == "SecurityGroup" or .groupMembershipClaims == "All")
    then .
    else .groupMembershipClaims = "SecurityGroup"
    end
' <<<"$CURRENT_MANIFEST")

if [[ "$PATCHED_MANIFEST" != "$CURRENT_MANIFEST" ]]; then
  az rest --method PATCH \
    --url "https://graph.microsoft.com/v1.0/applications/$APP_OID" \
    --headers "Content-Type=application/json" \
    --body "$PATCHED_MANIFEST" >/dev/null
  echo ">> groups claim + groupMembershipClaims=SecurityGroup applied" >&2
else
  echo ">> groups claim already configured" >&2
fi

# ---------------------------------------------------------------------------
# Client secret
# ---------------------------------------------------------------------------

echo ">> resetting client secret (append mode, ${SECRET_YEARS}y validity)" >&2
CLIENT_SECRET=$(az ad app credential reset \
  --id "$APP_ID" \
  --years "$SECRET_YEARS" \
  --append \
  --display-name "omnia-dev-$(date +%Y%m%d)" \
  --query password -o tsv)

SESSION_SECRET=$(openssl rand -base64 32)

# ---------------------------------------------------------------------------
# Admin group + membership
# ---------------------------------------------------------------------------

GROUP_ID=$(az ad group list --display-name "$GROUP_NAME" --query '[0].id' -o tsv)
if [[ -z "$GROUP_ID" ]]; then
  MAIL_NICK=$(echo "$GROUP_NAME" | tr '[:upper:] ' '[:lower:]-')
  echo ">> creating security group: $GROUP_NAME (mailNickname=$MAIL_NICK)" >&2
  GROUP_ID=$(az ad group create \
    --display-name "$GROUP_NAME" \
    --mail-nickname "$MAIL_NICK" \
    --query id -o tsv)
else
  echo ">> reusing existing group: $GROUP_NAME (id=$GROUP_ID)" >&2
fi

USER_OID=$(az ad signed-in-user show --query id -o tsv)
IS_MEMBER=$(az ad group member check --group "$GROUP_ID" --member-id "$USER_OID" --query value -o tsv 2>/dev/null || echo false)
if [[ "$IS_MEMBER" != "true" ]]; then
  echo ">> adding signed-in user ($USER_OID) to $GROUP_NAME" >&2
  az ad group member add --group "$GROUP_ID" --member-id "$USER_OID" >/dev/null
else
  echo ">> signed-in user already in $GROUP_NAME" >&2
fi

# ---------------------------------------------------------------------------
# K8s Secret + Workspace roleBinding (opt-in)
# ---------------------------------------------------------------------------

if $CREATE_SECRET; then
  echo ">> creating/updating Secret $NAMESPACE/$SECRET_NAME" >&2
  kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  kubectl create secret generic "$SECRET_NAME" \
    --namespace "$NAMESPACE" \
    --from-literal=OMNIA_OAUTH_CLIENT_ID="$APP_ID" \
    --from-literal=OMNIA_OAUTH_CLIENT_SECRET="$CLIENT_SECRET" \
    --from-literal=OMNIA_SESSION_SECRET="$SESSION_SECRET" \
    --dry-run=client -o yaml \
    | kubectl apply -f - >/dev/null

  # Patch workspace roleBindings — read-modify-write to avoid clobbering
  # any existing bindings. No-op if the binding for our group is already
  # present.
  if kubectl get workspace "$WORKSPACE_NAME" >/dev/null 2>&1; then
    WS_JSON=$(kubectl get workspace "$WORKSPACE_NAME" -o json)
    NEEDS_PATCH=$(jq --arg gid "$GROUP_ID" '
      [.spec.roleBindings // []
        | .[]
        | select(.role == "owner" and ((.groups // []) | index($gid)))
      ] | length == 0
    ' <<<"$WS_JSON")
    if [[ "$NEEDS_PATCH" == "true" ]]; then
      echo ">> patching workspace $WORKSPACE_NAME: owner roleBinding for $GROUP_NAME" >&2
      NEW_BINDINGS=$(jq --arg gid "$GROUP_ID" '
        (.spec.roleBindings // []) + [{"role":"owner","groups":[$gid]}]
      ' <<<"$WS_JSON")
      kubectl patch workspace "$WORKSPACE_NAME" --type=merge -p "$(jq -n --argjson rb "$NEW_BINDINGS" '{spec:{roleBindings:$rb}}')" >/dev/null
    else
      echo ">> workspace $WORKSPACE_NAME already has roleBinding for $GROUP_NAME" >&2
    fi
  else
    echo ">> workspace $WORKSPACE_NAME not found — skipping roleBinding patch" >&2
  fi
fi

# ---------------------------------------------------------------------------
# values-dev-entra.yaml
# ---------------------------------------------------------------------------

VALUES_FILE="$(cd "$(dirname "$0")/.." && pwd)/charts/omnia/values-dev-entra.yaml"
cat > "$VALUES_FILE" <<EOF
# Generated by scripts/setup-entra-dev.sh — do not commit.
# Regenerate by rerunning the script.
dashboard:
  auth:
    mode: oauth
    existingSessionSecret: $SECRET_NAME
    roleMapping:
      # Members of the "$GROUP_NAME" security group get global admin role
      # (OMNIA_AUTH_ROLE_ADMIN_GROUPS). Workspace-level access is granted
      # separately via spec.roleBindings on the Workspace CRD.
      adminGroups:
        - $GROUP_ID
  oauth:
    provider: azure
    azureTenantId: "$TENANT_ID"
    existingSecret: $SECRET_NAME
EOF
echo ">> wrote $VALUES_FILE" >&2

cat <<EOF

# ─── Entra app ready ────────────────────────────────────────────────────────
# Tenant:      $TENANT_ID
# AppId:       $APP_ID
# Redirect:    $REDIRECT_URI
# Admin group: $GROUP_NAME ($GROUP_ID)
# K8s secret:  $(if $CREATE_SECRET; then echo "created → $NAMESPACE/$SECRET_NAME"; else echo "NOT created (pass --create-secret to create)"; fi)
# Workspace:   $(if $CREATE_SECRET; then echo "$WORKSPACE_NAME patched with owner roleBinding for $GROUP_NAME"; else echo "NOT patched (pass --create-secret to patch)"; fi)
#
# Next steps:
#
#   1. Start (or restart) Tilt with Entra enabled:
#        ENABLE_ENTRA=true tilt up
#
#   2. Sign out of the dashboard (if already signed in) and log back in.
#      The ID token needs to be reissued to pick up the new groups claim.
#      You may see an Entra consent prompt the first time.
#
# ────────────────────────────────────────────────────────────────────────────
EOF
