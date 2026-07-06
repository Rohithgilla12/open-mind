import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function middleware(req: NextRequest) {
  if (
    req.nextUrl.pathname.startsWith("/api/") ||
    req.nextUrl.pathname.startsWith("/mcp") ||
    req.nextUrl.pathname.startsWith("/spike")
  ) {
    // /mcp is a bearer-authed proxy to the API (no cookie session); the API
    // enforces the token, so the cookie-gate redirect must not intercept it.
    return NextResponse.next();
  }
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

export const config = {
  matcher: ["/((?!_next|api/auth|favicon.ico).*)"],
};
