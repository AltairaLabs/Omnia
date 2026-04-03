"use client";

import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { useShowEnterpriseNav } from "@/components/license/license-gate";
import { useEnterpriseConfig } from "@/hooks/core";
import {
  LayoutDashboard,
  Bot,
  FileText,
  Wrench,
  MessageSquare,
  Settings,
  Network,
  DollarSign,
  Terminal,
  Cpu,
  Target,
  Sparkles,
  ShieldCheck,
  Brain,
} from "lucide-react";

interface NavItem {
  name: string;
  href: string;
  icon: typeof LayoutDashboard;
  enterprise?: boolean;
}

const navigation: NavItem[] = [
  { name: "Overview", href: "/", icon: LayoutDashboard },
  { name: "Topology", href: "/topology", icon: Network },
  { name: "Agents", href: "/agents", icon: Bot },
  { name: "Console", href: "/console", icon: Terminal },
  { name: "PromptPacks", href: "/promptpacks", icon: FileText },
  { name: "Tools", href: "/tools", icon: Wrench },
  { name: "Providers", href: "/providers", icon: Cpu },
  { name: "Sessions", href: "/sessions", icon: MessageSquare },
  { name: "Memories", href: "/memories", icon: Brain },
  { name: "Quality", href: "/quality", icon: ShieldCheck },
  { name: "Costs", href: "/costs", icon: DollarSign },
  { name: "Arena", href: "/arena", icon: Target, enterprise: true },
];

const secondaryNavigation = [
  { name: "Settings", href: "/settings", icon: Settings },
];

export function Sidebar() {
  const pathname = usePathname();
  const { showEnterpriseNav } = useShowEnterpriseNav();
  const { enterpriseEnabled } = useEnterpriseConfig();

  // Filter navigation items based on enterprise visibility
  const visibleNavigation = navigation.filter(
    (item) => !item.enterprise || showEnterpriseNav
  );

  return (
    <div className="flex h-full w-64 flex-col border-r border-white/10 bg-[#0F172A] text-[#E2E8F0]" data-testid="sidebar">
      {/* Logo — always show light-on-dark variant since sidebar is Deep Space blue */}
      <div className="flex h-16 items-center gap-3 border-b border-white/10 px-6">
        <Image
          src="/logo-dark.svg"
          alt="Omnia"
          width={28}
          height={28}
        />
        <span className="text-lg font-semibold text-white">Omnia</span>
      </div>

      {/* Primary Navigation */}
      <nav className="flex-1 space-y-1 px-3 py-4">
        {visibleNavigation.map((item) => {
          const isActive = item.href === "/"
            ? pathname === item.href
            : pathname === item.href || pathname.startsWith(item.href + "/");

          const showEnterpriseBadge = item.enterprise && !enterpriseEnabled;

          return (
            <Link
              key={item.name}
              href={item.href}
              className={cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                isActive
                  ? "bg-primary text-white"
                  : "text-[#E2E8F0]/70 hover:bg-[#1E293B] hover:text-white"
              )}
            >
              <item.icon className="h-4 w-4" />
              <span className="flex-1">{item.name}</span>
              {showEnterpriseBadge && (
                <Sparkles className="h-3 w-3 text-amber-500" />
              )}
            </Link>
          );
        })}
      </nav>

      {/* Secondary Navigation */}
      <div className="border-t border-white/10 px-3 py-4">
        {secondaryNavigation.map((item) => {
          const isActive = pathname === item.href;
          return (
            <Link
              key={item.name}
              href={item.href}
              className={cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                isActive
                  ? "bg-primary text-white"
                  : "text-[#E2E8F0]/70 hover:bg-[#1E293B] hover:text-white"
              )}
            >
              <item.icon className="h-4 w-4" />
              {item.name}
            </Link>
          );
        })}
      </div>
    </div>
  );
}
