"use client";

import { AlertTriangle, XCircle } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useLicense } from "@/hooks/use-license";

const WARNING_THRESHOLD_DAYS = 30;

/**
 * Calculate days until license expiry.
 * Returns the number of days between expiration date and current time.
 */
function getDaysUntilExpiry(expiresAt: string): number {
  const expirationTime = new Date(expiresAt).getTime();
  const currentTime = new Date().getTime();
  return Math.floor((expirationTime - currentTime) / (1000 * 60 * 60 * 24));
}

/**
 * Banner that displays license expiry warnings.
 * Shows warning when license expires within 30 days.
 * Shows error when license is expired.
 */
export function LicenseExpiryBanner() {
  const { license, isExpired, isEnterprise } = useLicense();

  // Don't show banner for open-core licenses (they don't expire meaningfully)
  if (!isEnterprise) {
    return null;
  }

  // Calculate days - this is acceptable as the component will re-render when needed
  const daysUntilExpiry = getDaysUntilExpiry(license.expiresAt);

  // Show error banner for expired licenses
  if (isExpired) {
    return (
      <Alert variant="destructive" className="rounded-none border-x-0 border-t-0">
        <XCircle className="h-4 w-4" />
        <AlertDescription>
          Your enterprise license has expired. Some features are now disabled.
          Please contact sales to renew your license.
        </AlertDescription>
      </Alert>
    );
  }

  // Show warning banner when expiring soon
  if (daysUntilExpiry <= WARNING_THRESHOLD_DAYS) {
    return (
      <Alert variant="default" className="rounded-none border-x-0 border-t-0 border-yellow-500 bg-yellow-50 dark:bg-yellow-950">
        <AlertTriangle className="h-4 w-4 text-yellow-600" />
        <AlertDescription className="text-yellow-800 dark:text-yellow-200">
          Your enterprise license expires in {daysUntilExpiry} day{daysUntilExpiry !== 1 ? "s" : ""}.
          Please contact sales to renew your license.
        </AlertDescription>
      </Alert>
    );
  }

  return null;
}
