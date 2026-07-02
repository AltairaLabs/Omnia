"use client";

interface ProviderTypeIconProps {
  type?: string;
  size?: "xs" | "sm" | "md" | "lg";
}

const iconMap: Record<string, string> = {
  anthropic: "A",
  claude: "C",
  openai: "O",
  gemini: "G",
  ollama: "L",
  bedrock: "B",
  mock: "M",
};

const bgColorMap: Record<string, string> = {
  anthropic: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
  claude: "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
  openai: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
  gemini: "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
  ollama: "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
  bedrock: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
  mock: "bg-muted text-muted-foreground",
};

const sizeClasses = {
  xs: "w-6 h-6 text-xs",
  sm: "w-8 h-8 text-sm",
  md: "w-10 h-10 text-base",
  lg: "w-12 h-12 text-lg",
};

export function ProviderTypeIcon({ type, size = "sm" }: Readonly<ProviderTypeIconProps>) {
  const icon = type ? iconMap[type] || type[0]?.toUpperCase() : "?";
  const bgColor = type
    ? bgColorMap[type] || "bg-muted text-muted-foreground"
    : "bg-muted";

  return (
    <div
      className={`rounded-full flex items-center justify-center font-bold ${sizeClasses[size]} ${bgColor}`}
    >
      {icon}
    </div>
  );
}
