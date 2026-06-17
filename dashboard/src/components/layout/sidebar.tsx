"use client";

import Image from "next/image";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Bot,
  FileText,
  Wrench,
  MessageSquare,
  Settings,
  Network,
  DollarSign,
  Cpu,
  Target,
  ShieldCheck,
  Brain,
  BookOpen,
  BarChart3,
  Zap,
  PanelLeftClose,
  PanelLeftOpen,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useShowEnterpriseNav } from "@/components/license/license-gate";
import { useEnterpriseConfig } from "@/hooks/core";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useSidebarStore } from "@/stores/sidebar-store";
import { useIsNarrow } from "@/hooks/use-is-narrow";
import { SidebarNavItem } from "./sidebar-nav-item";

interface NavItem {
  name: string;
  href: string;
  icon: LucideIcon;
  enterprise?: boolean;
}

const navigation: NavItem[] = [
  { name: "Overview", href: "/", icon: LayoutDashboard },
  { name: "Topology", href: "/topology", icon: Network },
  { name: "Agents", href: "/agents", icon: Bot },
  { name: "Functions", href: "/functions", icon: Zap },
  { name: "PromptPacks", href: "/promptpacks", icon: FileText },
  { name: "Skills", href: "/skills", icon: BookOpen },
  { name: "Tools", href: "/tools", icon: Wrench },
  { name: "Providers", href: "/providers", icon: Cpu },
  { name: "Sessions", href: "/sessions", icon: MessageSquare },
  { name: "Memories", href: "/memories", icon: Brain, enterprise: true },
  { name: "Memory analytics", href: "/memory-analytics", icon: BarChart3, enterprise: true },
  { name: "Quality", href: "/quality", icon: ShieldCheck },
  { name: "Costs", href: "/costs", icon: DollarSign },
  { name: "Arena", href: "/arena", icon: Target, enterprise: true },
];

const secondaryNavigation: NavItem[] = [
  { name: "Settings", href: "/settings", icon: Settings },
];

function isItemActive(pathname: string, href: string): boolean {
  if (href === "/") return pathname === href;
  return pathname === href || pathname.startsWith(href + "/");
}

export function Sidebar() {
  const pathname = usePathname();
  const { showEnterpriseNav } = useShowEnterpriseNav();
  const { enterpriseEnabled } = useEnterpriseConfig();
  const collapsedPref = useSidebarStore((s) => s.collapsed);
  const toggle = useSidebarStore((s) => s.toggle);
  const isNarrow = useIsNarrow();

  const collapsed = collapsedPref || isNarrow;

  const visibleNavigation = navigation.filter(
    (item) => !item.enterprise || showEnterpriseNav,
  );

  const renderItem = (item: NavItem) => (
    <SidebarNavItem
      key={item.name}
      name={item.name}
      href={item.href}
      icon={item.icon}
      isActive={isItemActive(pathname, item.href)}
      collapsed={collapsed}
      showEnterpriseBadge={Boolean(item.enterprise) && !enterpriseEnabled}
    />
  );

  return (
    <TooltipProvider>
      <div
        className={cn(
          "flex h-full flex-col border-r border-white/10 bg-[#0F172A] text-[#E2E8F0] transition-[width] duration-200",
          collapsed ? "w-16" : "w-64",
        )}
        data-testid="sidebar"
      >
        {/* Logo header + collapse toggle */}
        <div
          className={cn(
            "flex border-b border-white/10",
            collapsed
              ? "flex-col items-center gap-2 px-2 py-3"
              : "h-16 items-center gap-3 px-6",
          )}
        >
          <Image src="/logo-dark.svg" alt="Omnia" width={28} height={28} />
          {!collapsed && (
            <span className="flex-1 text-lg font-semibold text-white">Omnia</span>
          )}
          <button
            type="button"
            onClick={toggle}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            className="rounded-md p-1.5 text-[#E2E8F0]/70 transition-colors hover:bg-[#1E293B] hover:text-white"
          >
            {collapsed ? (
              <PanelLeftOpen className="h-4 w-4" />
            ) : (
              <PanelLeftClose className="h-4 w-4" />
            )}
          </button>
        </div>

        {/* Primary Navigation */}
        <nav className={cn("flex-1 space-y-1 py-4", collapsed ? "px-2" : "px-3")}>
          {visibleNavigation.map(renderItem)}
        </nav>

        {/* Secondary Navigation */}
        <div
          className={cn(
            "border-t border-white/10 py-4",
            collapsed ? "px-2" : "px-3",
          )}
        >
          {secondaryNavigation.map(renderItem)}
        </div>
      </div>
    </TooltipProvider>
  );
}
