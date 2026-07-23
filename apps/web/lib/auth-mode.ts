// Build-time public env: NEXT_PUBLIC_* is inlined at build time, so this is
// safe to branch on at module scope in both server and client code (layout,
// middleware, apiFetch). Defaults to "clerk" to match the hosted instance;
// self-hosters opt back into the legacy om_token flow with
// NEXT_PUBLIC_AUTH_MODE=token.
export const authMode: "clerk" | "token" =
  process.env.NEXT_PUBLIC_AUTH_MODE === "token" ? "token" : "clerk";
