"use client";

import Link from "next/link";
import { AlertCircle, ChevronDown } from "lucide-react";
import { useState } from "react";
import {
  classifyAgentError,
  summariseAgentError,
  type AgentErrorKind,
} from "@/lib/agent-errors/classify";

interface AgentErrorBannerProps {
  /**
   * Raw error string from the agent runtime. Pass it through unchanged
   * — the classifier handles "is this auth?" / "is this a rate limit?"
   * / "is this network?" detection internally.
   */
  error: string;
  /**
   * Workspace name for the "Check provider" link target. When omitted
   * the banner still renders but the action button doesn't link to a
   * specific provider page.
   */
  workspace?: string;
}

/**
 * Issue #1037 part 2. Pre-this, the agent console rendered every
 * runtime error as a single 400-line stack trace. Operators couldn't
 * tell "your API key is invalid" apart from "the network blipped"
 * apart from "the LLM provider is down" — we lost an hour during the
 * #1035 audit chasing a quota-exhaustion red herring when Gemini was
 * actually returning 429 for an INVALID_API_KEY.
 *
 * The banner classifies the error and surfaces an actionable
 * headline + optional link to the provider page. The raw text stays
 * available in a "Show details" disclosure so the existing diagnostic
 * path isn't worse.
 *
 * When the classifier returns "unknown" we render the plain banner
 * the dashboard had before — no regression for unknown errors.
 */
export function AgentErrorBanner({ error, workspace }: AgentErrorBannerProps) {
  const [showDetails, setShowDetails] = useState(false);
  const info = classifyAgentError(error);

  // Unknown class → preserve the legacy plain-text banner. Avoids
  // over-fitting: if the classifier doesn't recognise the error, the
  // user still sees the raw text exactly like before.
  if (info.kind === "unknown") {
    return (
      <div className="px-4 py-2 bg-red-500/10 border-b border-red-500/20 text-red-600 dark:text-red-400 text-sm">
        {error}
      </div>
    );
  }

  const headline = summariseAgentError(info);
  const action = renderAction(info.kind, info.provider, workspace);

  return (
    <div className="px-4 py-2 bg-red-500/10 border-b border-red-500/20 text-red-600 dark:text-red-400 text-sm">
      <div className="flex items-start gap-2">
        <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
        <div className="flex-1 min-w-0">
          <div className="flex items-baseline gap-3">
            <span className="font-medium">{headline}</span>
            {action}
          </div>
          <button
            type="button"
            className="mt-1 inline-flex items-center gap-1 text-xs underline-offset-2 hover:underline opacity-80"
            onClick={() => setShowDetails((v) => !v)}
            aria-expanded={showDetails}
          >
            <ChevronDown
              className={`h-3 w-3 transition-transform ${showDetails ? "rotate-180" : ""}`}
            />
            {showDetails ? "Hide details" : "Show details"}
          </button>
          {showDetails && (
            <pre className="mt-2 p-2 rounded bg-red-500/5 text-xs overflow-x-auto whitespace-pre-wrap break-words">
              {info.raw}
            </pre>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Renders an action affordance for each error class. invalid_credential
 * deep-links to the provider page when both workspace and provider
 * name are known; rate_limited and provider_unavailable get neutral
 * guidance because there's nothing to click.
 */
function renderAction(
  kind: AgentErrorKind,
  provider?: string,
  workspace?: string,
): React.ReactNode {
  if (kind === "invalid_credential" && provider && workspace) {
    return (
      <Link
        href={`/providers/${provider}-provider?namespace=${workspace}`}
        className="text-xs underline underline-offset-2 hover:opacity-80"
      >
        Check provider →
      </Link>
    );
  }
  if (kind === "invalid_credential") {
    // Provider unknown — give the operator the next-best landing page.
    return (
      <Link
        href="/providers"
        className="text-xs underline underline-offset-2 hover:opacity-80"
      >
        Check providers →
      </Link>
    );
  }
  if (kind === "rate_limited") {
    return (
      <span className="text-xs opacity-80">
        Wait a moment, then retry. If this persists, check the provider&apos;s quota dashboard.
      </span>
    );
  }
  if (kind === "provider_unavailable") {
    return (
      <span className="text-xs opacity-80">
        Network or upstream issue — retry shortly. If it persists, check the provider&apos;s status page.
      </span>
    );
  }
  return null;
}
