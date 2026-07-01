import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { Providers } from "@/components/providers";
import { AuthWrapper } from "@/components/auth-wrapper";
import { AppShell } from "@/components/layout";
import { buildBrandMetadata } from "@/lib/branding/metadata";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-jetbrains-mono",
  subsets: ["latin"],
});

export function generateMetadata(): Metadata {
  return buildBrandMetadata(process.env);
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body
        className={`${inter.variable} ${jetbrainsMono.variable} font-sans antialiased`}
      >
        <Providers>
          <AuthWrapper>
            <AppShell>{children}</AppShell>
          </AuthWrapper>
        </Providers>
      </body>
    </html>
  );
}
