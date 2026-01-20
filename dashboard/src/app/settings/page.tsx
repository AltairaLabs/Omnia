import { Metadata } from "next";
import { Settings } from "lucide-react";
import { ApiKeysSection, LicenseSection, ActivationSection } from "@/components/settings";
import { CredentialsSection } from "@/components/credentials";

export const metadata: Metadata = {
  title: "Settings | Omnia Dashboard",
  description: "Manage your dashboard settings, license, and API keys",
};

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
          <Settings className="h-5 w-5 text-primary" />
        </div>
        <div>
          <h1 className="text-2xl font-bold">Settings</h1>
          <p className="text-sm text-muted-foreground">
            Manage your dashboard settings, license, and API keys
          </p>
        </div>
      </div>

      <div className="grid gap-6">
        <LicenseSection />
        <ActivationSection />
        <CredentialsSection />
        <ApiKeysSection />
      </div>
    </div>
  );
}
