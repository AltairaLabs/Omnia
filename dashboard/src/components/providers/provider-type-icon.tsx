"use client";

interface ProviderTypeIconProps {
  type?: string;
  size?: "sm" | "md" | "lg";
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
  mock: "bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-400",
};

const sizeClasses = {
  sm: "w-8 h-8 text-sm",
  md: "w-10 h-10 text-base",
  lg: "w-12 h-12 text-lg",
};

export function ProviderTypeIcon({ type, size = "sm" }: Readonly<ProviderTypeIconProps>) {
  const icon = type ? iconMap[type] || type[0]?.toUpperCase() : "?";
  const bgColor = type
    ? bgColorMap[type] || "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-400"
    : "bg-gray-100";

  return (
    <div
      className={`rounded-full flex items-center justify-center font-bold ${sizeClasses[size]} ${bgColor}`}
    >
      {icon}
    </div>
  );
}
