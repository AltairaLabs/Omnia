"use client";

import { useState, useEffect } from "react";
import { useAuth } from "@/hooks/use-auth";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { LogOut, User, Shield } from "lucide-react";

/**
 * Generate Gravatar URL from email.
 * Uses SHA-256 hash (Gravatar supports both MD5 and SHA-256).
 */
async function getGravatarUrl(email: string, size = 80): Promise<string> {
  const normalizedEmail = email.trim().toLowerCase();
  const encoder = new TextEncoder();
  const data = encoder.encode(normalizedEmail);
  const hashBuffer = await crypto.subtle.digest("SHA-256", data);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hash = hashArray.map(b => b.toString(16).padStart(2, "0")).join("");
  return `https://www.gravatar.com/avatar/${hash}?s=${size}&d=identicon`;
}

/**
 * Hook to get Gravatar URL for an email.
 */
function useGravatar(email?: string): string | undefined {
  const [gravatarUrl, setGravatarUrl] = useState<string>();

  useEffect(() => {
    if (email) {
      getGravatarUrl(email).then(setGravatarUrl);
    }
  }, [email]);

  return gravatarUrl;
}

/**
 * User menu dropdown showing current user info and actions.
 */
export function UserMenu() {
  const { user, isAuthenticated, role, logout } = useAuth();
  const gravatarUrl = useGravatar(user.email);

  // Don't show menu for anonymous users
  if (!isAuthenticated) {
    return (
      <Badge variant="secondary" className="text-xs">
        Anonymous
      </Badge>
    );
  }

  const initials = getInitials(user.displayName || user.username);
  const roleColor = getRoleColor(role);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="relative h-8 w-8 rounded-full">
          <Avatar className="h-8 w-8">
            {gravatarUrl && <AvatarImage src={gravatarUrl} alt={user.displayName || user.username} />}
            <AvatarFallback className="text-xs">{initials}</AvatarFallback>
          </Avatar>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-56" align="end" forceMount>
        <DropdownMenuLabel className="font-normal">
          <div className="flex flex-col space-y-1">
            <p className="text-sm font-medium leading-none">
              {user.displayName || user.username}
            </p>
            {user.email && (
              <p className="text-xs leading-none text-muted-foreground">
                {user.email}
              </p>
            )}
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem disabled>
          <User className="mr-2 h-4 w-4" />
          <span>Profile</span>
        </DropdownMenuItem>
        <DropdownMenuItem disabled>
          <Shield className="mr-2 h-4 w-4" />
          <span className="flex-1">Role</span>
          <Badge variant="outline" className={`ml-2 text-xs ${roleColor}`}>
            {role}
          </Badge>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={logout}>
          <LogOut className="mr-2 h-4 w-4" />
          <span>Log out</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * Get initials from a name.
 */
function getInitials(name: string): string {
  const parts = name.split(/[\s._-]+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].substring(0, 2).toUpperCase();
  return (parts[0][0] + (parts.at(-1)?.[0] ?? "")).toUpperCase();
}

/**
 * Get badge color class for role.
 */
function getRoleColor(role: string): string {
  switch (role) {
    case "admin":
      return "text-red-600 border-red-600";
    case "editor":
      return "text-blue-600 border-blue-600";
    default:
      return "text-gray-600 border-gray-600";
  }
}
