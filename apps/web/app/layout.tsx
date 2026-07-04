import type { Metadata } from "next";
import { tokens } from "@openmind/ui";
import { fontVariables } from "../lib/fonts";
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
        className={fontVariables}
        style={{
          backgroundColor: tokens.color.canvas,
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
