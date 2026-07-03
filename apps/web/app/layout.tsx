import type { Metadata } from "next";
import { tokens } from "@openmind/ui";
import "./globals.css";

export const metadata: Metadata = {
  title: "Openmind",
  description: "Save anything, find it by fragments.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body
        style={{
          backgroundColor: tokens.color.paper,
          color: tokens.color.ink,
          fontFamily: tokens.font.sans,
          minHeight: "100vh",
        }}
      >
        {children}
      </body>
    </html>
  );
}
