/**
 * Type declaration for monocart-reporter module.
 * This package doesn't provide its own TypeScript types.
 */
declare module "monocart-reporter" {
  export interface MCROptions {
    name?: string;
    outputFile?: string;
    reports?: (string | [string, Record<string, unknown>])[];
    entryFilter?: (entry: { url?: string }) => boolean;
    sourceFilter?: (sourcePath: string) => boolean;
    [key: string]: unknown;
  }

  export interface CoverageEntry {
    url: string;
    source?: string;
    functions: Array<{
      functionName: string;
      ranges: Array<{ startOffset: number; endOffset: number; count: number }>;
      isBlockCoverage: boolean;
    }>;
  }

  export function addCoverageReport(
    coverageData: CoverageEntry[],
    testInfo: unknown,
  ): Promise<void>;

  const MCR: {
    (options?: MCROptions): unknown;
    addCoverageReport: typeof addCoverageReport;
  };

  export default MCR;
}
