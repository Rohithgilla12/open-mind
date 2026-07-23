import type { Metadata } from "next";
import { ClerkProvider } from "@clerk/nextjs";
import { tokens } from "@openmind/ui";
import { fontVariables } from "../lib/fonts";
import { authMode } from "../lib/auth-mode";
import { clerkPublishableKey } from "../lib/clerk";
import "./globals.css";

export const metadata: Metadata = {
  title: "Openmind",
  description: "Save anything, find it by fragments.",
};

// Conditional at module level is fine — NEXT_PUBLIC_AUTH_MODE is inlined at
// build time. Token mode never mounts ClerkProvider, so it never runs or
// validates Clerk env at runtime.
function Providers({ children }: { children: React.ReactNode }) {
  if (authMode === "clerk") {
    return <ClerkProvider publishableKey={clerkPublishableKey}>{children}</ClerkProvider>;
  }
  return children;
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body
        className={fontVariables}
        style={{
          backgroundColor: tokens.color.canvas,
          color: tokens.color.ink,
          fontFamily: tokens.font.sans,
          minHeight: "100vh",
        }}
      >
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
