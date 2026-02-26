import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * Generate a unique ID using timestamp, counter, and random suffix.
 * Used for console messages and other ephemeral identifiers.
 */
let idCounter = 0;
export function generateId(): string {
  idCounter += 1;
  return `${Date.now()}-${idCounter}-${crypto.randomUUID().slice(0, 8)}`;
}
