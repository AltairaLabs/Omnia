"use client";

/**
 * Data service React provider.
 *
 * Provides the appropriate DataService implementation based on runtime config.
 * Wraps the app to give all components access to data via useDataService().
 */

import { createContext, useContext, useMemo } from "react";
import type { DataService } from "./types";
import { MockDataService } from "./mock-service";
import { LiveDataService } from "./live-service";
import { useRuntimeConfig } from "@/hooks/use-runtime-config";

const DataServiceContext = createContext<DataService | null>(null);

/**
 * Create a DataService instance based on demo mode flag.
 */
export function createDataService(isDemoMode: boolean): DataService {
  if (isDemoMode) {
    return new MockDataService();
  }
  return new LiveDataService();
}

interface DataServiceProviderProps {
  children: React.ReactNode;
  /** Optional service instance to use (primarily for testing) */
  initialService?: DataService;
}

/**
 * Provider component that supplies the DataService to the component tree.
 * Automatically selects MockDataService or OperatorApiService based on config.
 */
export function DataServiceProvider({ children, initialService }: DataServiceProviderProps) {
  const { config } = useRuntimeConfig();

  const service = useMemo(() => {
    // Use provided service if given (for testing), otherwise create based on config
    if (initialService) {
      return initialService;
    }
    return createDataService(config.demoMode);
  }, [config.demoMode, initialService]);

  return (
    <DataServiceContext.Provider value={service}>
      {children}
    </DataServiceContext.Provider>
  );
}

/**
 * Hook to access the DataService from any component.
 *
 * @example
 * const service = useDataService();
 * const agents = await service.getAgents();
 */
export function useDataService(): DataService {
  const context = useContext(DataServiceContext);
  if (!context) {
    throw new Error("useDataService must be used within a DataServiceProvider");
  }
  return context;
}
