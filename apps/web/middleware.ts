import { NextResponse } from "next/server";
import type { NextFetchEvent, NextRequest } from "next/server";
import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";
import { authMode } from "./lib/auth-mode";
import { clerkPublishableKey } from "./lib/clerk";

const isPublicRoute = createRouteMatcher(["/login(.*)", "/privacy", "/terms", "/welcome"]);

// Proxy/protocol paths enforce their own auth at the API layer (bearer keys,
// MCP session). They must still pass THROUGH clerkMiddleware in clerk mode —
// otherwise auth() throws inside the proxy routes and every client fetch
// 401s — but they are never protect()ed (no redirect for bearer clients).
function isSelfAuthedPath(pathname: string): boolean {
  return (
    pathname.startsWith("/api/") ||
    pathname.startsWith("/mcp") ||
    pathname.startsWith("/spike")
  );
}

// Gated behind the ternary so clerkMiddleware() — and any Clerk env
// validation it does — is never invoked in token mode, matching the
// dark-ship requirement that token mode has zero Clerk runtime behaviour.
const clerkHandler =
  authMode === "clerk"
    ? clerkMiddleware(async (auth, req) => {
        if (isPublicRoute(req) || isSelfAuthedPath(req.nextUrl.pathname)) return;
        // Anonymous visitors to the homepage see the public landing (same URL)
        // rather than a login redirect — the homepage must explain the product.
        if (req.nextUrl.pathname === "/") {
          const { userId } = await auth();
          if (!userId) {
            return NextResponse.rewrite(new URL("/welcome", req.url));
          }
          return;
        }
        await auth.protect({ unauthenticatedUrl: new URL("/login", req.url).toString() });
      }, { publishableKey: clerkPublishableKey })
    : null;

function legacyMiddleware(req: NextRequest) {
  if (
    req.nextUrl.pathname.startsWith("/privacy") ||
    req.nextUrl.pathname.startsWith("/terms") ||
    req.nextUrl.pathname.startsWith("/welcome")
  ) {
    return NextResponse.next();
  }
  const hasToken = req.cookies.has("om_token");
  const isLogin = req.nextUrl.pathname.startsWith("/login");
  if (!hasToken && req.nextUrl.pathname === "/") {
    return NextResponse.rewrite(new URL("/welcome", req.url));
  }
  if (!hasToken && !isLogin) {
    return NextResponse.redirect(new URL("/login", req.url));
  }
  if (hasToken && isLogin) {
    return NextResponse.redirect(new URL("/", req.url));
  }
  return NextResponse.next();
}

export function middleware(req: NextRequest, event: NextFetchEvent) {
  if (authMode === "clerk") {
    // Everything flows through clerkMiddleware so auth() works in proxy
    // routes; self-authed paths are exempted from protect() inside it.
    return clerkHandler!(req, event);
  }
  if (isSelfAuthedPath(req.nextUrl.pathname)) {
    // /mcp is a bearer-authed proxy to the API (no cookie session); the API
    // enforces the token, so the cookie-gate redirect must not intercept it.
    return NextResponse.next();
  }
  return legacyMiddleware(req);
}

export const config = {
  // Static files under public/ must bypass the cookie gate, or anonymous
  // visitors to the public landing page get a /login redirect for every image
  // it references. App routes never carry a file extension, so excluding
  // extensioned paths is safe.
  matcher: [
    "/((?!_next|api/auth|favicon\\.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|avif|ico|txt|xml|webmanifest)$).*)",
  ],
};
