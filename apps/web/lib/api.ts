import "server-only";
import { cookies } from "next/headers";
import { auth } from "@clerk/nextjs/server";
import { authMode } from "./auth-mode";

const API_URL = process.env.API_URL ?? "http://localhost:8080";

async function sessionToken(): Promise<string | undefined> {
  if (authMode !== "clerk") {
    return (await cookies()).get("om_token")?.value;
  }
  // A server-component render can happen outside Clerk's request context
  // (or with no signed-in user); either way we must not throw — just send
  // no auth and let the API 401.
  try {
    const { getToken } = await auth();
    return (await getToken()) ?? undefined;
  } catch {
    return undefined;
  }
}

export async function apiFetch(path: string, init?: RequestInit, req?: Request): Promise<Response> {
  let token = await sessionToken();
  const header = req?.headers.get("authorization");
  if (header?.startsWith("Bearer ")) token = header.slice(7);
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  // Only force JSON for string bodies. FormData/streams (multipart uploads)
  // must keep their own content-type (incl. the multipart boundary).
  if (typeof init?.body === "string") headers.set("content-type", "application/json");
  return fetch(`${API_URL}${path}`, { ...init, headers, cache: "no-store" });
}
