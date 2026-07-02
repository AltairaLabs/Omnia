"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";
import { Button } from "@/components/ui/button";
import { RefreshCw, Moon, Sun, Terminal } from "lucide-react";
import { useTheme } from "next-themes";
import { useQueryClient, useIsFetching } from "@tanstack/react-query";
import { UserMenu } from "./user-menu";
import { WorkspaceSwitcher } from "@/components/workspace-switcher";
import { BrandPresetSwitcher } from "@/components/branding/brand-preset-switcher";
import { cn } from "@/lib/utils";

// Hydration-safe mount flag: false on the server + initial client render, true
// after. Avoids a setState-in-effect while keeping SSR and hydration in sync.
const NO_SUBSCRIBE = () => () => {};

interface HeaderProps {
  title: React.ReactNode;
  description?: React.ReactNode;
  children?: React.ReactNode;
}

export function Header({ title, description, children }: Readonly<HeaderProps>) {
  const { theme, setTheme } = useTheme();
  const queryClient = useQueryClient();
  // `useIsFetching()` is 0 during SSR but >0 once client queries start, so gate
  // the fetch-dependent UI on mount to avoid a hydration mismatch on the button.
  const mounted = useSyncExternalStore(NO_SUBSCRIBE, () => true, () => false);
  const fetching = useIsFetching() > 0;
  const isFetching = mounted && fetching;
  const pathname = usePathname();
  const consoleActive = pathname === "/console" || pathname.startsWith("/console/");

  return (
    <header className="flex h-16 items-center justify-between border-b border-border bg-card px-6">
      <div>
        <h1 className="text-xl font-semibold">{title}</h1>
        {description && (
          <div className="text-sm text-muted-foreground">{description}</div>
        )}
      </div>
      <div className="flex items-center gap-3">
        <WorkspaceSwitcher />
        <Button
          asChild
          variant="ghost"
          size="icon"
          aria-label="Open Console"
          data-testid="console-link"
          data-active={consoleActive}
          className={cn(consoleActive && "bg-accent text-accent-foreground")}
        >
          <Link href="/console">
            <Terminal className="h-4 w-4" />
          </Link>
        </Button>
        {children}
        <Button
          variant="ghost"
          size="icon"
          aria-label="Refresh data"
          onClick={() => queryClient.invalidateQueries()}
          disabled={isFetching}
        >
          <RefreshCw className={cn("h-4 w-4", isFetching && "animate-spin")} />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
          data-testid="theme-toggle"
        >
          <Sun className="h-4 w-4 rotate-0 scale-100 transition-all dark:-rotate-90 dark:scale-0" />
          <Moon className="absolute h-4 w-4 rotate-90 scale-0 transition-all dark:rotate-0 dark:scale-100" />
          <span className="sr-only">Toggle theme</span>
        </Button>
        <BrandPresetSwitcher />
        <UserMenu />
      </div>
    </header>
  );
}
