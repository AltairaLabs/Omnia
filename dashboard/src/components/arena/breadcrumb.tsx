"use client";

import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

export interface BreadcrumbItem {
  label: string;
  href?: string;
}

interface ArenaBreadcrumbProps {
  items: BreadcrumbItem[];
}

export function ArenaBreadcrumb({ items }: Readonly<ArenaBreadcrumbProps>) {
  return (
    <nav aria-label="Breadcrumb" className="flex items-center text-sm text-muted-foreground">
      <Link
        href="/arena"
        className="hover:text-foreground transition-colors"
      >
        Arena
      </Link>
      {items.map((item, index) => (
        <span key={item.label} className="flex items-center">
          <ChevronRight className="h-4 w-4 mx-1" />
          {item.href ? (
            <Link
              href={item.href}
              className={cn(
                "hover:text-foreground transition-colors",
                index === items.length - 1 && "text-foreground font-medium"
              )}
            >
              {item.label}
            </Link>
          ) : (
            <span className="text-foreground font-medium">{item.label}</span>
          )}
        </span>
      ))}
    </nav>
  );
}
