// Default to the hosted Openmind instance's Clerk so a plain build signs in
// out of the box. A publishable key is public by design (it ships in every
// client bundle), so committing it is safe. Self-hosters override it with
// NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY at build time. The Clerk *secret* key is a
// real secret and is never defaulted here — it stays env-only (CLERK_SECRET_KEY).
export const clerkPublishableKey =
  process.env.NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY ??
  "pk_live_Y2xlcmsub3Blbm1pbmQuZ2lsbGEuZnVuJA";
