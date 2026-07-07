// Build-time public env: NEXT_PUBLIC_* is inlined at build time, so this is
// safe to branch on at module scope in both server and client code (layout,
// middleware, apiFetch). Anything other than "clerk" keeps today's om_token
// behaviour byte-identical.
export const authMode: "clerk" | "token" =
  process.env.NEXT_PUBLIC_AUTH_MODE === "clerk" ? "clerk" : "token";
