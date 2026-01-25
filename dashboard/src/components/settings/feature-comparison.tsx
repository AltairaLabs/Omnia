"use client";

import React from "react";
import { CheckCircle, XCircle, Sparkles } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

interface FeatureItem {
  name: string;
  openCore: boolean | string;
  enterprise: boolean | string;
}

interface FeatureCategory {
  category: string;
  features: FeatureItem[];
}

const FEATURE_COMPARISON: FeatureCategory[] = [
  {
    category: "Source Types",
    features: [
      { name: "ConfigMap Sources", openCore: true, enterprise: true },
      { name: "Git Repository Sources", openCore: true, enterprise: true },
      { name: "OCI Registry Sources", openCore: false, enterprise: true },
      { name: "S3 Bucket Sources", openCore: false, enterprise: true },
    ],
  },
  {
    category: "Job Types",
    features: [
      { name: "Evaluation Jobs", openCore: true, enterprise: true },
      { name: "Load Testing Jobs", openCore: false, enterprise: true },
      { name: "Data Generation Jobs", openCore: false, enterprise: true },
    ],
  },
  {
    category: "Execution Features",
    features: [
      { name: "Manual Job Execution", openCore: true, enterprise: true },
      { name: "Scheduled Jobs (Cron)", openCore: false, enterprise: true },
      { name: "Distributed Workers", openCore: false, enterprise: true },
    ],
  },
  {
    category: "Limits",
    features: [
      { name: "Max Scenarios per Job", openCore: "10", enterprise: "Unlimited" },
      { name: "Max Worker Replicas", openCore: "1", enterprise: "Unlimited" },
    ],
  },
];

function FeatureValue({ value }: { value: boolean | string }) {
  if (typeof value === "string") {
    return <span className="text-sm">{value}</span>;
  }

  return value ? (
    <CheckCircle className="h-5 w-5 text-green-500" />
  ) : (
    <XCircle className="h-5 w-5 text-muted-foreground" />
  );
}

/**
 * Feature comparison table showing Open Core vs Enterprise features.
 */
export function FeatureComparison() {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-2">
          <Sparkles className="h-5 w-5 text-primary" />
          <CardTitle>Feature Comparison</CardTitle>
        </div>
        <CardDescription>
          Compare Open Core and Enterprise features
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[50%]">Feature</TableHead>
              <TableHead className="text-center">Open Core</TableHead>
              <TableHead className="text-center">Enterprise</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {FEATURE_COMPARISON.map((section) => (
              <React.Fragment key={section.category}>
                <TableRow className="bg-muted/50">
                  <TableCell colSpan={3} className="font-semibold">
                    {section.category}
                  </TableCell>
                </TableRow>
                {section.features.map((feature) => (
                  <TableRow key={feature.name}>
                    <TableCell className="pl-6">{feature.name}</TableCell>
                    <TableCell className="text-center">
                      <div className="flex justify-center">
                        <FeatureValue value={feature.openCore} />
                      </div>
                    </TableCell>
                    <TableCell className="text-center">
                      <div className="flex justify-center">
                        <FeatureValue value={feature.enterprise} />
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </React.Fragment>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
