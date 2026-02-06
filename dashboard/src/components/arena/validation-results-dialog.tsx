"use client";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import {
  CheckCircle2,
  XCircle,
  AlertTriangle,
  FileText,
  Info,
} from "lucide-react";
import { cn } from "@/lib/utils";

export interface ValidationDiagnostic {
  range: {
    start: { line: number; character: number };
    end: { line: number; character: number };
  };
  severity: number; // 1=Error, 2=Warning, 3=Info, 4=Hint
  source: string;
  message: string;
}

export interface ValidationSummary {
  totalFiles: number;
  validFiles: number;
  invalidFiles: number;
  errorCount: number;
  warningCount: number;
}

export interface ValidationResults {
  valid: boolean;
  diagnostics: Record<string, ValidationDiagnostic[]>;
  warnings?: string[];
  summary?: ValidationSummary;
}

interface ValidationResultsDialogProps {
  readonly open: boolean;
  readonly onOpenChange: (open: boolean) => void;
  readonly results: ValidationResults | null;
  readonly onFileClick?: (path: string, line: number) => void;
}

function getSeverityIcon(severity: number) {
  switch (severity) {
    case 1:
      return <XCircle className="h-4 w-4 text-destructive" />;
    case 2:
      return <AlertTriangle className="h-4 w-4 text-amber-500" />;
    case 3:
      return <Info className="h-4 w-4 text-blue-500" />;
    default:
      return <Info className="h-4 w-4 text-muted-foreground" />;
  }
}

function getSeverityLabel(severity: number) {
  switch (severity) {
    case 1:
      return "error";
    case 2:
      return "warning";
    case 3:
      return "info";
    default:
      return "hint";
  }
}

export function ValidationResultsDialog({
  open,
  onOpenChange,
  results,
  onFileClick,
}: ValidationResultsDialogProps) {
  if (!results) return null;

  const filesWithIssues = Object.entries(results.diagnostics).filter(
    ([, diags]) => diags.length > 0
  );
  const validFiles = Object.entries(results.diagnostics).filter(
    ([, diags]) => diags.length === 0
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[80vh] flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {results.valid ? (
              <>
                <CheckCircle2 className="h-5 w-5 text-green-500" />
                Validation Passed
              </>
            ) : (
              <>
                <XCircle className="h-5 w-5 text-destructive" />
                Validation Failed
              </>
            )}
          </DialogTitle>
          <DialogDescription>
            {results.summary ? (
              <span className="flex items-center gap-4 mt-2">
                <span>{results.summary.totalFiles} files checked</span>
                <Badge variant="outline" className="text-green-600">
                  {results.summary.validFiles} valid
                </Badge>
                {results.summary.invalidFiles > 0 && (
                  <Badge variant="destructive">
                    {results.summary.invalidFiles} invalid
                  </Badge>
                )}
                {results.summary.errorCount > 0 && (
                  <span className="text-destructive">
                    {results.summary.errorCount} errors
                  </span>
                )}
                {results.summary.warningCount > 0 && (
                  <span className="text-amber-500">
                    {results.summary.warningCount} warnings
                  </span>
                )}
              </span>
            ) : (
              "Review the validation results below"
            )}
          </DialogDescription>
        </DialogHeader>

        <ScrollArea className="flex-1 mt-4 -mx-6 px-6">
          {filesWithIssues.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              <CheckCircle2 className="h-12 w-12 mx-auto mb-4 text-green-500" />
              <p>All files are valid!</p>
              <p className="text-sm mt-1">
                {validFiles.length} file{validFiles.length === 1 ? "" : "s"} checked
              </p>
            </div>
          ) : (
            <Accordion type="multiple" className="w-full">
              {filesWithIssues.map(([path, diagnostics]) => {
                const errorCount = diagnostics.filter((d) => d.severity === 1).length;
                const warningCount = diagnostics.filter((d) => d.severity === 2).length;

                return (
                  <AccordionItem key={path} value={path}>
                    <AccordionTrigger className="hover:no-underline">
                      <div className="flex items-center gap-2 text-left">
                        <FileText className="h-4 w-4 text-muted-foreground flex-shrink-0" />
                        <span className="font-mono text-sm truncate">{path}</span>
                        <div className="flex items-center gap-1 ml-auto mr-2">
                          {errorCount > 0 && (
                            <Badge variant="destructive" className="text-xs">
                              {errorCount}
                            </Badge>
                          )}
                          {warningCount > 0 && (
                            <Badge
                              variant="outline"
                              className="text-xs text-amber-500 border-amber-500"
                            >
                              {warningCount}
                            </Badge>
                          )}
                        </div>
                      </div>
                    </AccordionTrigger>
                    <AccordionContent>
                      <div className="space-y-2 pl-6">
                        {diagnostics.map((diag) => (
                          <button
                            key={`${diag.range.start.line}-${diag.range.start.character}-${diag.message.slice(0, 20)}`}
                            type="button"
                            className={cn(
                              "flex items-start gap-2 w-full text-left p-2 rounded-md",
                              "hover:bg-muted/50 transition-colors",
                              onFileClick && "cursor-pointer"
                            )}
                            onClick={() =>
                              onFileClick?.(path, diag.range.start.line)
                            }
                            disabled={!onFileClick}
                          >
                            {getSeverityIcon(diag.severity)}
                            <div className="flex-1 min-w-0">
                              <p className="text-sm">{diag.message}</p>
                              <p className="text-xs text-muted-foreground mt-0.5">
                                Line {diag.range.start.line + 1}
                                {diag.source && ` (${diag.source})`}
                              </p>
                            </div>
                            <Badge
                              variant="outline"
                              className={cn(
                                "text-xs flex-shrink-0",
                                diag.severity === 1 && "text-destructive border-destructive",
                                diag.severity === 2 && "text-amber-500 border-amber-500"
                              )}
                            >
                              {getSeverityLabel(diag.severity)}
                            </Badge>
                          </button>
                        ))}
                      </div>
                    </AccordionContent>
                  </AccordionItem>
                );
              })}
            </Accordion>
          )}

          {/* Show valid files collapsed */}
          {validFiles.length > 0 && filesWithIssues.length > 0 && (
            <div className="mt-4 pt-4 border-t">
              <p className="text-sm text-muted-foreground mb-2 flex items-center gap-2">
                <CheckCircle2 className="h-4 w-4 text-green-500" />
                {validFiles.length} file{validFiles.length === 1 ? "" : "s"} passed
                validation
              </p>
              <div className="text-xs text-muted-foreground space-y-1 pl-6">
                {validFiles.slice(0, 5).map(([path]) => (
                  <div key={path} className="font-mono truncate">
                    {path}
                  </div>
                ))}
                {validFiles.length > 5 && (
                  <div className="text-muted-foreground">
                    ...and {validFiles.length - 5} more
                  </div>
                )}
              </div>
            </div>
          )}
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
}
