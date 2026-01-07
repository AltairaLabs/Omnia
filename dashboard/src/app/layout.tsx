import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";
import { Providers } from "@/components/providers";
import { AuthWrapper } from "@/components/auth-wrapper";
import { Sidebar, ReadOnlyBanner, DemoModeBanner } from "@/components/layout";

const inter = Inter({
  variable: "--font-inter",
  subsets: ["latin"],
});

const jetbrainsMono = JetBrains_Mono({
  variable: "--font-jetbrains-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "Omnia Dashboard",
  description: "AI Agent Operations Platform - Monitor and manage your Kubernetes-native AI agents",
  icons: {
    icon: "/favicon.svg",
  },
};

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
            <div className="flex h-screen">
              <Sidebar />
              <div className="flex-1 flex flex-col overflow-hidden">
                <DemoModeBanner />
                <ReadOnlyBanner />
                <main className="flex-1 overflow-auto bg-background">
                  {children}
                </main>
              </div>
            </div>
          </AuthWrapper>
        </Providers>
      </body>
    </html>
  );
}
