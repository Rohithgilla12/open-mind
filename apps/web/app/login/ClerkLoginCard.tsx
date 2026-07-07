"use client";

import { SignIn } from "@clerk/nextjs";
import { tokens } from "@openmind/ui";

export function ClerkLoginCard() {
  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        backgroundColor: tokens.color.paper,
      }}
    >
      <SignIn routing="hash" />
    </div>
  );
}
