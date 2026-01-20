"use client";

import { useState, useMemo, useCallback } from "react";
import { Upload, CheckCircle, XCircle, AlertTriangle, Shield, Building2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useLicense } from "@/hooks/use-license";
import type { LicenseFeatures } from "@/types/license";

const FEATURE_LABELS: Record<keyof LicenseFeatures, string> = {
  gitSource: "Git Sources",
  ociSource: "OCI Sources",
  s3Source: "S3 Sources",
  loadTesting: "Load Testing",
  dataGeneration: "Data Generation",
  scheduling: "Job Scheduling",
  distributedWorkers: "Distributed Workers",
};

interface LicenseStatusProps {
  isExpired: boolean;
  daysUntilExpiry: number;
}

function LicenseStatus({ isExpired, daysUntilExpiry }: LicenseStatusProps) {
  if (isExpired) {
    return (
      <Badge variant="destructive" className="gap-1">
        <XCircle className="h-3 w-3" />
        Expired
      </Badge>
    );
  }

  if (daysUntilExpiry <= 30) {
    return (
      <Badge variant="outline" className="gap-1 border-yellow-500 text-yellow-600">
        <AlertTriangle className="h-3 w-3" />
        Expires in {daysUntilExpiry} day{daysUntilExpiry === 1 ? "" : "s"}
      </Badge>
    );
  }

  return (
    <Badge variant="outline" className="gap-1 border-green-500 text-green-600">
      <CheckCircle className="h-3 w-3" />
      Active
    </Badge>
  );
}

function FeatureList({ features }: { features: LicenseFeatures }) {
  const featureEntries = Object.entries(features) as [keyof LicenseFeatures, boolean][];

  return (
    <div className="grid grid-cols-2 gap-2">
      {featureEntries.map(([key, enabled]) => (
        <div key={key} className="flex items-center gap-2">
          {enabled ? (
            <CheckCircle className="h-4 w-4 text-green-500" />
          ) : (
            <XCircle className="h-4 w-4 text-muted-foreground" />
          )}
          <span className={enabled ? "" : "text-muted-foreground"}>
            {FEATURE_LABELS[key]}
          </span>
        </div>
      ))}
    </div>
  );
}

function UploadLicenseDialog({ onUpload }: { onUpload: (file: File) => void }) {
  const [isOpen, setIsOpen] = useState(false);
  const [dragActive, setDragActive] = useState(false);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragActive(false);
      const file = e.dataTransfer.files[0];
      if (file) {
        onUpload(file);
        setIsOpen(false);
      }
    },
    [onUpload]
  );

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (file) {
        onUpload(file);
        setIsOpen(false);
      }
    },
    [onUpload]
  );

  return (
    <Dialog open={isOpen} onOpenChange={setIsOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          <Upload className="h-4 w-4 mr-2" />
          Upload License
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Upload License</DialogTitle>
          <DialogDescription>
            Upload your enterprise license file to unlock additional features.
          </DialogDescription>
        </DialogHeader>
        <div
          role="region"
          aria-label="Drop zone for license file upload"
          className={`border-2 border-dashed rounded-lg p-8 text-center transition-colors ${
            dragActive ? "border-primary bg-primary/5" : "border-muted-foreground/25"
          }`}
          onDragOver={(e) => {
            e.preventDefault();
            setDragActive(true);
          }}
          onDragLeave={() => setDragActive(false)}
          onDrop={handleDrop}
        >
          <Upload className="h-8 w-8 mx-auto mb-4 text-muted-foreground" />
          <p className="text-sm text-muted-foreground mb-2">
            Drag and drop your license file here, or
          </p>
          <label>
            <input
              type="file"
              className="hidden"
              accept=".pem,.license"
              onChange={handleFileSelect}
            />
            <Button variant="secondary" size="sm" asChild>
              <span>Browse Files</span>
            </Button>
          </label>
        </div>
      </DialogContent>
    </Dialog>
  );
}

/**
 * License management section for settings page.
 */
export function LicenseSection() {
  const { license, isExpired, isEnterprise, refresh } = useLicense();

  const daysUntilExpiry = useMemo(() => {
    const now = Date.now();
    return Math.floor(
      (new Date(license.expiresAt).getTime() - now) / (1000 * 60 * 60 * 24)
    );
  }, [license.expiresAt]);

  const handleUpload = useCallback(
    async (file: File) => {
      const formData = new FormData();
      formData.append("license", file);

      try {
        const response = await fetch("/api/license", {
          method: "POST",
          body: formData,
        });

        if (!response.ok) {
          throw new Error("Failed to upload license");
        }

        refresh();
      } catch {
        // Error handling will be added in a future iteration
      }
    },
    [refresh]
  );

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {isEnterprise ? (
              <Building2 className="h-5 w-5 text-primary" />
            ) : (
              <Shield className="h-5 w-5 text-muted-foreground" />
            )}
            <CardTitle>License</CardTitle>
          </div>
          <LicenseStatus
            isExpired={isExpired}
            daysUntilExpiry={daysUntilExpiry}
          />
        </div>
        <CardDescription>
          {isEnterprise
            ? `Enterprise license for ${license.customer}`
            : "Open Core - Free for evaluation and development"}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div>
          <h4 className="text-sm font-medium mb-3">Enabled Features</h4>
          <FeatureList features={license.features} />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div>
            <h4 className="text-sm font-medium mb-1">Max Scenarios</h4>
            <p className="text-sm text-muted-foreground">
              {license.limits.maxScenarios === 0
                ? "Unlimited"
                : license.limits.maxScenarios}
            </p>
          </div>
          <div>
            <h4 className="text-sm font-medium mb-1">Max Worker Replicas</h4>
            <p className="text-sm text-muted-foreground">
              {license.limits.maxWorkerReplicas === 0
                ? "Unlimited"
                : license.limits.maxWorkerReplicas}
            </p>
          </div>
        </div>

        {isEnterprise && (
          <div className="grid grid-cols-2 gap-4">
            <div>
              <h4 className="text-sm font-medium mb-1">Issued</h4>
              <p className="text-sm text-muted-foreground">
                {new Date(license.issuedAt).toLocaleDateString()}
              </p>
            </div>
            <div>
              <h4 className="text-sm font-medium mb-1">Expires</h4>
              <p className="text-sm text-muted-foreground">
                {new Date(license.expiresAt).toLocaleDateString()}
              </p>
            </div>
          </div>
        )}

        <div className="pt-2">
          <UploadLicenseDialog onUpload={handleUpload} />
        </div>
      </CardContent>
    </Card>
  );
}
