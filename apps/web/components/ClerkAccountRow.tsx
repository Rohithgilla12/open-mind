"use client";

import { UserButton, useUser } from "@clerk/nextjs";
import { tokens } from "@openmind/ui";

/**
 * The sidebar account row in Clerk (multi-user) mode.
 *
 * `UserButton` supplies both the avatar and the account-management menu —
 * manage account, security, sign out — so we don't render our own avatar or
 * duplicate any of those actions. Only mounted when authMode is "clerk": the
 * root layout skips ClerkProvider in token mode, and these hooks throw without
 * it. Shell picks the variant, so this file never has to check.
 */
export function ClerkAccountRow({ usage }: { usage?: string }) {
  const { user, isLoaded } = useUser();

  const email = user?.primaryEmailAddress?.emailAddress ?? "";
  // Prefer a real name, fall back to the e-mail, and render neither before
  // Clerk has loaded — a flash of placeholder text is worse than empty space.
  const label = user?.fullName?.trim() || email;

  return (
    <div style={row}>
      <UserButton
        appearance={{
          elements: {
            userButtonAvatarBox: { width: 30, height: 30 },
            // The row itself is the click target's visual container; Clerk's
            // own focus ring would sit awkwardly inside it.
            userButtonTrigger: { borderRadius: "50%" },
          },
        }}
      />
      <div style={{ minWidth: 0, flex: 1 }}>
        <div title={label} style={nameStyle}>
          {isLoaded ? label : ""}
        </div>
        <div className="meta" style={captionStyle}>
          {usage ?? (isLoaded && email && label !== email ? email : "")}
        </div>
      </div>
    </div>
  );
}

const row = {
  marginTop: "auto",
  display: "flex",
  alignItems: "center",
  gap: 10,
  padding: 8,
  borderRadius: 11,
  border: `1px solid ${tokens.color.hairline}`,
  background: tokens.color.paper,
} as const;

const nameStyle = {
  fontFamily: tokens.font.sans,
  fontSize: 12.5,
  fontWeight: 600,
  lineHeight: 1.1,
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
  minHeight: 14,
} as const;

const captionStyle = {
  textTransform: "none",
  letterSpacing: ".02em",
  color: tokens.color.inkFaintAlt,
  marginTop: 3,
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
} as const;
