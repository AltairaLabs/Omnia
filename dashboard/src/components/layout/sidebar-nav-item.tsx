"use client";

import Link from "next/link";
import { Sparkles, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipTrigger, TooltipContent } from "@/components/ui/tooltip";

export interface SidebarNavItemProps {
  name: string;
  href: string;
  icon: LucideIcon;
  isActive: boolean;
  collapsed: boolean;
  showEnterpriseBadge: boolean;
}

export function SidebarNavItem({
  name,
  href,
  icon: Icon,
  isActive,
  collapsed,
  showEnterpriseBadge,
}: Readonly<SidebarNavItemProps>) {
  const link = (
    <Link
      href={href}
      aria-label={name}
      className={cn(
        "relative flex items-center rounded-md py-2 text-sm font-medium transition-colors",
        collapsed ? "justify-center px-2" : "gap-3 px-3",
        isActive
          ? "bg-primary text-primary-foreground"
          : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
      )}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {!collapsed && <span className="flex-1">{name}</span>}
      {!collapsed && showEnterpriseBadge && (
        <Sparkles className="h-3 w-3 text-warning" />
      )}
      {collapsed && showEnterpriseBadge && (
        <span className="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-warning" />
      )}
    </Link>
  );

  if (!collapsed) return link;

  const tooltipLabel = showEnterpriseBadge ? `${name} (Enterprise)` : name;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{link}</TooltipTrigger>
      <TooltipContent side="right">{tooltipLabel}</TooltipContent>
    </Tooltip>
  );
}
