import { NextResponse } from "next/server";
import type { NextFetchEvent, NextRequest } from "next/server";
import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
import { authMode } from "./lib/auth-mode";

// Only matched once page routes are reached (the /api/, /mcp, /spike bypass
// below runs first in both modes), so this just needs to exempt /login.
const isPublicRoute = createRouteMatcher(["/login(.*)"]);

// Gated behind the ternary so clerkMiddleware() — and any Clerk env
// validation it does — is never invoked in token mode, matching the
// dark-ship requirement that token mode has zero Clerk runtime behaviour.
const clerkHandler =
  authMode === "clerk"
    ? clerkMiddleware(async (auth, req) => {
        if (!isPublicRoute(req)) {
          await auth.protect({ unauthenticatedUrl: new URL("/login", req.url).toString() });
        }
      })
    : null;

function legacyMiddleware(req: NextRequest) {
  const hasToken = req.cookies.has("om_token");
  const isLogin = req.nextUrl.pathname.startsWith("/login");
  if (!hasToken && !isLogin) {
    return NextResponse.redirect(new URL("/login", req.url));
  }
  if (hasToken && isLogin) {
    return NextResponse.redirect(new URL("/", req.url));
  }
  return NextResponse.next();
}

export function middleware(req: NextRequest, event: NextFetchEvent) {
  if (
    req.nextUrl.pathname.startsWith("/api/") ||
    req.nextUrl.pathname.startsWith("/mcp") ||
    req.nextUrl.pathname.startsWith("/spike")
  ) {
    // /mcp is a bearer-authed proxy to the API (no cookie session); the API
    // enforces the token, so the cookie-gate redirect must not intercept it.
    // Same bypass in both modes — the API/MCP layer enforces its own auth.
    return NextResponse.next();
  }
  if (authMode === "clerk") {
    return clerkHandler!(req, event);
  }
  return legacyMiddleware(req);
}

export const config = {
  matcher: ["/((?!_next|api/auth|favicon.ico).*)"],
};
