import type { Metadata } from "next";

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
      <body>{children}</body>
    </html>
  );
}
